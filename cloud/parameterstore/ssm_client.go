package parameterstore

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
)

// SSMClient is an interface to interact with AWS Systems Manager (SSM)
// Parameter Store.
type SSMClient interface {
	// PutParameter puts a parameter into Parameter Store.
	PutParameter(context.Context, *ssm.PutParameterInput) (*ssm.PutParameterOutput, error)
	// DeleteParametersSimple is the same as DeleteParameters but only returns
	// the names of the deleted parameters rather than the full output.
	DeleteParametersSimple(context.Context, *ssm.DeleteParametersInput) ([]string, error)
	// DeleteParameters deletes the specified parameters. Parameter Store limits
	// how many parameters can be deleted per call, so implementations are
	// expected to handle batching transparently. The returned output slice
	// contains the output of each batched request.
	DeleteParameters(context.Context, *ssm.DeleteParametersInput) ([]*ssm.DeleteParametersOutput, error)
	// GetParametersSimple is the same as GetParameters but only returns the
	// parameter information rather than the full output.
	GetParametersSimple(context.Context, *ssm.GetParametersInput) ([]ssmTypes.Parameter, error)
	// GetParameters retrieves the specified parameters. Parameter Store
	// limits how many parameters can be retrieved per call, so implementations
	// must handle batching transparently. The returned output slice contains
	// the output of each batched request.
	GetParameters(context.Context, *ssm.GetParametersInput) ([]*ssm.GetParametersOutput, error)
}

type ssmClientImpl struct {
	client *ssm.Client
}

// configCache is a mapping of configuration options to the cached AWS
// configuration created by it. Currently the only supported option is the AWS
// region.
var configCache map[string]*aws.Config = make(map[string]*aws.Config)

func newSSMClient(ctx context.Context, region string) (*ssmClientImpl, error) {
	if region == "" {
		region = "us-east-1"
	}

	if configCache[region] == nil {
		cfg, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
		if err != nil {
			return nil, errors.Wrap(err, "loading AWS config")
		}
		otelaws.AppendMiddlewares(&cfg.APIOptions)
		configCache[region] = &cfg
	}

	c := ssm.NewFromConfig(*configCache[region])

	return &ssmClientImpl{client: c}, nil
}

func (c *ssmClientImpl) PutParameter(ctx context.Context, input *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
	return retrySSMClientOp(ctx, c.client, input, func(ctx context.Context, client *ssm.Client, input *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
		return client.PutParameter(ctx, input)
	}, "PutParameter")
}

func (c *ssmClientImpl) DeleteParametersSimple(ctx context.Context, input *ssm.DeleteParametersInput) ([]string, error) {
	output, err := c.DeleteParameters(ctx, input)
	if err != nil {
		return nil, err
	}

	var deletedParams []string
	for _, o := range output {
		deletedParams = append(deletedParams, o.DeletedParameters...)
	}
	return deletedParams, nil
}

func (c *ssmClientImpl) DeleteParameters(ctx context.Context, input *ssm.DeleteParametersInput) ([]*ssm.DeleteParametersOutput, error) {
	return retryBatchSSMClientOp(ctx, input.Names, c.client, input, func(paramNames []string, input *ssm.DeleteParametersInput) *ssm.DeleteParametersInput {
		batchInput := *input
		batchInput.Names = paramNames
		return &batchInput
	}, func(ctx context.Context, client *ssm.Client, input *ssm.DeleteParametersInput) (*ssm.DeleteParametersOutput, error) {
		return client.DeleteParameters(ctx, input)
	}, "DeleteParameters")
}

func (c *ssmClientImpl) GetParametersSimple(ctx context.Context, input *ssm.GetParametersInput) ([]ssmTypes.Parameter, error) {
	allOutputs, err := c.GetParameters(ctx, input)
	if err != nil {
		return nil, err
	}

	var params []ssmTypes.Parameter
	for _, output := range allOutputs {
		params = append(params, output.Parameters...)
	}
	return params, nil
}

func (c *ssmClientImpl) GetParameters(ctx context.Context, input *ssm.GetParametersInput) ([]*ssm.GetParametersOutput, error) {
	return retryBatchSSMClientOp(ctx, input.Names, c.client, input, func(paramNames []string, input *ssm.GetParametersInput) *ssm.GetParametersInput {
		batchInput := *input
		batchInput.Names = paramNames
		return &batchInput
	}, func(ctx context.Context, client *ssm.Client, input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
		return client.GetParameters(ctx, input)
	}, "GetParameters")
}

// retryBatchSSMClientOp runs an SSM operation with retries and handles batching for
// SSM API methods that have a limit on the number of parameters that can be
// queried at once.
func retryBatchSSMClientOp[Input interface{}, Output interface{}](ctx context.Context, paramNames []string, client *ssm.Client, input Input, makeBatchInput func(paramNames []string, input Input) Input, op func(ctx context.Context, client *ssm.Client, input Input) (Output, error), opName string) ([]Output, error) {
	// This limitation comes from the SSM docs.
	const maxParamsPerRequest = 10
	batches := utility.MakeSliceBatches(paramNames, maxParamsPerRequest)

	var outputs []Output
	for _, batch := range batches {
		batchInput := makeBatchInput(batch, input)
		output, err := retrySSMClientOp(ctx, client, batchInput, op, opName)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	return outputs, nil
}

// retrySSMClientOp runs a single SSM operation with retries.
func retrySSMClientOp[Input interface{}, Output interface{}](ctx context.Context, client *ssm.Client, input Input, clientOp func(ctx context.Context, client *ssm.Client, input Input) (Output, error), opName string) (Output, error) {
	var output Output
	var err error

	err = utility.Retry(ctx, func() (bool, error) {
		msg := makeAWSLogMessage(opName, fmt.Sprintf("%T", client), input)
		output, err = clientOp(ctx, client, input)
		if err != nil {
			grip.Debug(message.WrapError(err, msg))
			return true, err
		}
		grip.Info(msg)
		return false, nil
	}, ssmDefaultRetryOptions())
	if err != nil {
		return output, err
	}
	return output, nil
}

const defaultSSMMaxRetries = 9
const defaultSSMMinDelay = time.Second

func ssmDefaultRetryOptions() utility.RetryOptions {
	return utility.RetryOptions{
		MaxAttempts: defaultSSMMaxRetries,
		MinDelay:    defaultSSMMinDelay,
	}
}

// makeAWSLogMessage logs a structured message for an SSM API call. If used to
// trace API call inputs/outputs, callers must take extra precaution to ensure
// that this does not log any sensitive values (e.g. the parameter value in
// plaintext).
func makeAWSLogMessage(name, client string, args interface{}) message.Fields {
	msg := message.Fields{
		"message":  "AWS API call",
		"api_name": name,
		"client":   client,
	}

	argMap := make(map[string]interface{})
	if err := mapstructure.Decode(args, &argMap); err == nil {
		// Avoid logging the plaintext value of the parameter for PutParameter's
		// input. This is not a comprehensive solution to redacting sensitive
		// values, and only works specifically for PutParameter.
		if _, ok := argMap["Value"]; ok {
			argMap["Value"] = "{REDACTED}"
		}
		msg["args"] = argMap
	}

	return msg
}
