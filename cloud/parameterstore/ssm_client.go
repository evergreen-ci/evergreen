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

var ()

type ssmClient interface {
	PutParameter(context.Context, *ssm.PutParameterInput) (*ssm.PutParameterOutput, error)
	DeleteParametersSimple(context.Context, *ssm.DeleteParametersInput) ([]string, error)
	DeleteParameters(context.Context, *ssm.DeleteParametersInput) ([]*ssm.DeleteParametersOutput, error)
	GetParametersSimple(context.Context, *ssm.GetParametersInput) ([]ssmTypes.Parameter, error)
	GetParameters(context.Context, *ssm.GetParametersInput) ([]*ssm.GetParametersOutput, error)
}

type ssmClientImpl struct {
	client *ssm.Client
}

// configCache is a mapping of configuration options to the cached AWS
// configuration created by it. Currently the only supported option is the AWS
// region.
var configCache map[string]*aws.Config = make(map[string]*aws.Config)

func newSSMClient(ctx context.Context, region string) (ssmClient, error) {
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

// kim: TODO: test manually
func (c *ssmClientImpl) PutParameter(ctx context.Context, input *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
	// Name:      aws.String(parameterName),
	// Value:     aws.String(value),
	// Type:      types.ParameterTypeSecureString,
	// Overwrite: aws.Bool(true),
	return c.putParameter(ctx, input)
}

func (c *ssmClientImpl) putParameter(ctx context.Context, input *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
	return retrySSMOp(ctx, c.client, input, func(ctx context.Context, client *ssm.Client, input *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
		return client.PutParameter(ctx, input)
	}, "PutParameter")
	// var output *ssm.PutParameterOutput
	// var err error
	//
	// err = utility.Retry(
	//     ctx,
	//     func() (bool, error) {
	//         msg := makeAWSLogMessage("PutParameter", fmt.Sprintf("%T", c), input)
	//         output, err = c.client.PutParameter(ctx, input)
	//         if err != nil {
	//             var apiErr smithy.APIError
	//             if errors.As(err, &apiErr) {
	//                 grip.Debug(message.WrapError(apiErr, msg))
	//             }
	//             return true, err
	//         }
	//         grip.Info(msg)
	//         return false, nil
	//     }, ssmDefaultRetryOptions())
	// if err != nil {
	//     return nil, err
	// }
	// return output, nil
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
	const maxParamsPerRequest = 10
	return retryBatchSSMOp(ctx, input.Names, maxParamsPerRequest, c.client, input, func(names []string, input *ssm.DeleteParametersInput) *ssm.DeleteParametersInput {
		batchInput := *input
		batchInput.Names = names
		return &batchInput
	}, func(ctx context.Context, client *ssm.Client, input *ssm.DeleteParametersInput) (*ssm.DeleteParametersOutput, error) {
		return client.DeleteParameters(ctx, input)
	}, "DeleteParameters")
	// kim: TODO: needs pagination, wait for generic method
	// return retrySSMOp(ctx, c.client, input, func(ctx context.Context, client *ssm.Client, input *ssm.DeleteParametersInput) (*ssm.DeleteParametersOutput, error) {
	//     return client.DeleteParameters(ctx, input)
	// }, "DeleteParameters")
	// var output *ssm.DeleteParametersOutput
	// var err error
	//
	// err = utility.Retry(
	//     ctx,
	//     func() (bool, error) {
	//         msg := makeAWSLogMessage("PutParameter", fmt.Sprintf("%T", c), input)
	//         output, err = c.client.DeleteParameters(ctx, input)
	//         if err != nil {
	//             var apiErr smithy.APIError
	//             if errors.As(err, &apiErr) {
	//                 grip.Debug(message.WrapError(apiErr, msg))
	//             }
	//             return true, err
	//         }
	//         grip.Info(msg)
	//         return false, nil
	//     }, ssmDefaultRetryOptions())
	// if err != nil {
	//     return nil, err
	// }
	//
	// return output, nil
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

// kim: TODO: test manually
// kim: TODO: write simple helper to wrap this and just return the
// ssm.Parameters.
func (c *ssmClientImpl) GetParameters(ctx context.Context, input *ssm.GetParametersInput) ([]*ssm.GetParametersOutput, error) {
	const maxParamsPerRequest = 10
	return retryBatchSSMOp(ctx, input.Names, maxParamsPerRequest, c.client, input, func(names []string, input *ssm.GetParametersInput) *ssm.GetParametersInput {
		batchInput := *input
		batchInput.Names = names
		return &batchInput
	}, func(ctx context.Context, client *ssm.Client, input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
		return client.GetParameters(ctx, input)
	}, "GetParameters")
	// paramNames := input.Names
	// const maxParametersPerRequest = 10
	// // Batching parameters to avoid exceeding the maximum number of parameters
	// // in a single request.
	// // kim: TODO: check if pagination works.
	// // kim: TODO: convert pagination to generic function so that other functions
	// // can also paginate.
	// // kim: TODO: break batching into its own PR.
	// var batches [][]string
	// for maxParametersPerRequest < len(paramNames) {
	//     batches = append(batches, paramNames[0:maxParametersPerRequest:maxParametersPerRequest])
	//     paramNames = paramNames[maxParametersPerRequest:]
	// }
	// batches = append(batches, paramNames)
	//
	// allOutputs := make([]ssm.GetParametersOutput, 0, len(batches))
	// for _, batch := range batches {
	//     batchInput := *input
	//     batchInput.Names = batch
	//     output, err := c.getParameters(ctx, &batchInput)
	//     if err != nil {
	//         return nil, err
	//     }
	//     allOutputs = append(allOutputs, *output)
	// }
	//
	// return allOutputs, nil

	// var params []parameter
	// for _, batch := range batches {
	//     res, err := c.callGetParameters(ctx, &ssm.GetParametersInput{
	//         Names:          batch,
	//         WithDecryption: aws.Bool(true),
	//     })
	//     if err != nil {
	//         return nil, errors.Wrap(err, "getting parameters")
	//     }
	//
	//     for _, param := range res.Parameters {
	//         if param.Name == nil || param.Value == nil || param.LastModifiedDate == nil {
	//             continue
	//         }
	//         params = append(params, parameter{ID: *param.Name, Value: *param.Value, LastUpdate: *param.LastModifiedDate})
	//     }
	// }
	// return params, nil
}

// func (c *ssmClientImpl) getParameters(ctx context.Context, input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
//     return retrySSMOp(ctx, c.client, input, func(ctx context.Context, client *ssm.Client, input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
//         return client.GetParameters(ctx, input)
//     }, "GetParameters")
//     // var output *ssm.GetParametersOutput
//     // var err error
//     // err = utility.Retry(
//     //     ctx,
//     //     func() (bool, error) {
//     //         msg := makeAWSLogMessage("GetParameters", fmt.Sprintf("%T", c), input)
//     //         output, err = c.client.GetParameters(ctx, input)
//     //         if err != nil {
//     //             var apiErr smithy.APIError
//     //             if errors.As(err, &apiErr) {
//     //                 grip.Debug(message.WrapError(apiErr, msg))
//     //             }
//     //             return true, err
//     //         }
//     //         grip.Info(msg)
//     //         return false, nil
//     //     }, ssmDefaultRetryOptions())
//     // if err != nil {
//     //     return nil, err
//     // }
//     // return output, nil
// }

// retryBatchSSMOp runs an SSM operation with retries and handles batching for
// SSM API methods that have a limit on the number of parameters that can be
// queried at once.
func retryBatchSSMOp[Input interface{}, Output interface{}](ctx context.Context, paramNames []string, maxParamsPerRequest int, client *ssm.Client, input Input, makeBatchInput func(names []string, input Input) Input, op func(ctx context.Context, client *ssm.Client, input Input) (Output, error), opName string) ([]Output, error) {
	batches := makeBatches(paramNames, maxParamsPerRequest)

	var outputs []Output
	for _, batch := range batches {
		batchInput := makeBatchInput(batch, input)
		output, err := retrySSMOp(ctx, client, batchInput, op, opName)
		if err != nil {
			return nil, err
		}
		outputs = append(outputs, output)
	}
	return outputs, nil
}

// makeBatches partitions paramNames into batches of at most maxParamsPerBatch.
func makeBatches(paramNames []string, maxParamsPerBatch int) [][]string {
	// kim: TODO: double-check/test that this batching works as intended.
	var batches [][]string
	for maxParamsPerBatch < len(paramNames) {
		batches = append(batches, paramNames[0:maxParamsPerBatch:maxParamsPerBatch])
		paramNames = paramNames[maxParamsPerBatch:]
	}
	batches = append(batches, paramNames)
	return batches
}

// retrySSMOp runs a single SSM operation with retries.
func retrySSMOp[Input interface{}, Output interface{}](ctx context.Context, client *ssm.Client, input Input, op func(ctx context.Context, client *ssm.Client, input Input) (Output, error), opName string) (Output, error) {
	var output Output
	var err error

	err = utility.Retry(ctx, func() (bool, error) {
		msg := makeAWSLogMessage(opName, fmt.Sprintf("%T", client), input)
		output, err = op(ctx, client, input)
		if err != nil {
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

func ssmDefaultRetryOptions() utility.RetryOptions {
	return utility.RetryOptions{
		MaxAttempts: 9,
		MinDelay:    time.Second,
	}
}

// makeAWSLogMessage logs a structured message for an SSM API call. Callers must
// take extra precaution to ensure that this does not log any sensitive values
// (e.g. the parameter value in plaintext).
func makeAWSLogMessage(name, client string, args interface{}) message.Fields {
	msg := message.Fields{
		"message":  "AWS API call",
		"api_name": name,
		"client":   fmt.Sprintf("%T", client),
	}

	argMap := make(map[string]interface{})
	if err := mapstructure.Decode(args, &argMap); err == nil {
		// Avoid logging the plaintext value of the parameter for PutParameter's
		// input.
		// kim: TODO: double-check if this really redacts the value for both get
		// and put.
		if _, ok := argMap["Value"]; ok {
			argMap["Value"] = "{REDACTED}"
		}
		msg["args"] = argMap
	}

	return msg
}
