package parameterstore

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/github.com/aws/aws-sdk-go-v2/otelaws"
)

var (
	retryOptions = utility.RetryOptions{
		MaxAttempts: 9,
		MinDelay:    time.Second,
	}
)

type ssmClient interface {
	PutParameter(context.Context, *ssm.PutParameterInput) (*ssm.PutParameterOutput, error)
	GetParameters(context.Context, *ssm.GetParametersInput) (*ssm.GetParametersOutput, error)
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
func (c *ssmClientImpl) GetParameters(ctx context.Context, input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
	// kim: TODO: use NextToken instead of manually batching
	// batches := make([][]string, 0, (len(parameters)+maxParametersPerRequest-1)/maxParametersPerRequest)
	// for maxParametersPerRequest < len(parameters) {
	//     parameters, batches = parameters[maxParametersPerRequest:], append(batches, parameters[0:maxParametersPerRequest:maxParametersPerRequest])
	// }
	// batches = append(batches, parameters)

	// kim: TODO: implement pagination
	return c.getParameters(ctx, input)

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

func (c *ssmClientImpl) getParameters(ctx context.Context, input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
	var output *ssm.GetParametersOutput
	var err error

	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("GetParameters", fmt.Sprintf("%T", c), input)
			output, err = c.client.GetParameters(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, retryOptions)
	if err != nil {
		return nil, err
	}
	return output, nil
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
	var output *ssm.PutParameterOutput
	var err error

	err = utility.Retry(
		ctx,
		func() (bool, error) {
			msg := makeAWSLogMessage("PutParameter", fmt.Sprintf("%T", c), input)
			output, err = c.client.PutParameter(ctx, input)
			if err != nil {
				var apiErr smithy.APIError
				if errors.As(err, &apiErr) {
					grip.Debug(message.WrapError(apiErr, msg))
				}
				return true, err
			}
			grip.Info(msg)
			return false, nil
		}, retryOptions)
	if err != nil {
		return nil, err
	}
	return output, nil
}

// makeAWSLogMessage logs a structured message for an SSM API call. Callers must
// take extra precaution to ensure that this does not log any sensitive values
// (e.g. the parameter value in plaintext).
func makeAWSLogMessage(name, client string, args interface{}) message.Fields {
	msg := message.Fields{
		"message":  "AWS API call",
		"api_name": name,
		"client":   client,
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
