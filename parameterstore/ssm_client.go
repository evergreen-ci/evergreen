package parameterstore

import (
	"context"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/aws/smithy-go"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const region = "us-east-1"

var (
	cachedClient *ssm.Client

	retryOptions = utility.RetryOptions{
		MaxAttempts: 9,
		MinDelay:    time.Second,
	}
)

type ssmClient struct {
	client *ssm.Client
}

func newSSMClient(ctx context.Context) (*ssmClient, error) {
	if cachedClient != nil {
		return &ssmClient{client: cachedClient}, nil
	}

	config, err := config.LoadDefaultConfig(ctx,
		config.WithRegion(region),
	)
	if err != nil {
		return nil, errors.Wrap(err, "loading AWS config")
	}

	return &ssmClient{client: ssm.NewFromConfig(config)}, nil
}

func (c *ssmClient) getParameters(ctx context.Context, parameters []string) ([]parameter, error) {
	res, err := c.callGetParameters(ctx, &ssm.GetParametersInput{
		Names:          parameters,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting parameters")
	}

	var params []parameter
	for _, param := range res.Parameters {
		if param.Name == nil || param.Value == nil || param.LastModifiedDate == nil {
			continue
		}
		params = append(params, parameter{id: *param.Name, value: *param.Value, lastUpdate: *param.LastModifiedDate})
	}
	return params, nil
}

func (c *ssmClient) putParameter(ctx context.Context, parameterName, value string) error {
	_, err := c.callPutParameter(ctx, &ssm.PutParameterInput{
		Name:  aws.String(parameterName),
		Value: aws.String(value),
		Type:  types.ParameterTypeSecureString,
	})
	return errors.Wrapf(err, "setting parameter for '%s'", parameterName)
}

func (c *ssmClient) callGetParameters(ctx context.Context, input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
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

func (c *ssmClient) callPutParameter(ctx context.Context, input *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
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

func makeAWSLogMessage(name, client string, args interface{}) message.Fields {
	msg := message.Fields{
		"message":  "AWS API call",
		"api_name": name,
		"client":   client,
	}

	argMap := make(map[string]interface{})
	if err := mapstructure.Decode(args, &argMap); err == nil {
		msg["args"] = argMap
	} else {
		msg["args"] = fmt.Sprintf("%+v", args)
	}

	return msg
}
