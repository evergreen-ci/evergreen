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

const (
	region = "us-east-1"

	// maxParametersPerRequest is an AWS limit on how many parameters can be included in a single request.
	// https://docs.aws.amazon.com/systems-manager/latest/APIReference/API_GetParameters.html
	maxParametersPerRequest = 10
)

var (
	cachedClient *ssm.Client

	retryOptions = utility.RetryOptions{
		MaxAttempts: 9,
		MinDelay:    time.Second,
	}
)

type ssmClient interface {
	getParameters(context.Context, []string) ([]parameter, error)
	putParameter(context.Context, string, string) error
}

type ssmClientImpl struct {
	client *ssm.Client
}

func newSSMClient(ctx context.Context) (ssmClient, error) {
	if cachedClient == nil {
		config, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(region),
		)
		if err != nil {
			return nil, errors.Wrap(err, "loading AWS config")
		}
		cachedClient = ssm.NewFromConfig(config)
	}

	return &ssmClientImpl{client: cachedClient}, nil
}

func (c *ssmClientImpl) getParameters(ctx context.Context, parameters []string) ([]parameter, error) {
	batches := make([][]string, 0, (len(parameters)+maxParametersPerRequest-1)/maxParametersPerRequest)
	for maxParametersPerRequest < len(parameters) {
		parameters, batches = parameters[maxParametersPerRequest:], append(batches, parameters[0:maxParametersPerRequest:maxParametersPerRequest])
	}
	batches = append(batches, parameters)

	var params []parameter
	for _, batch := range batches {
		res, err := c.callGetParameters(ctx, &ssm.GetParametersInput{
			Names:          batch,
			WithDecryption: aws.Bool(true),
		})
		if err != nil {
			return nil, errors.Wrap(err, "getting parameters")
		}

		for _, param := range res.Parameters {
			if param.Name == nil || param.Value == nil || param.LastModifiedDate == nil {
				continue
			}
			params = append(params, parameter{ID: *param.Name, Value: *param.Value, LastUpdate: *param.LastModifiedDate})
		}
	}
	return params, nil
}

func (c *ssmClientImpl) putParameter(ctx context.Context, parameterName, value string) error {
	_, err := c.callPutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(parameterName),
		Value:     aws.String(value),
		Type:      types.ParameterTypeSecureString,
		Overwrite: aws.Bool(true),
	})
	return errors.Wrapf(err, "setting parameter for '%s'", parameterName)
}

func (c *ssmClientImpl) callGetParameters(ctx context.Context, input *ssm.GetParametersInput) (*ssm.GetParametersOutput, error) {
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

func (c *ssmClientImpl) callPutParameter(ctx context.Context, input *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
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

type ssmClientMock struct {
	parameters map[string]parameter
	getError   bool
	putError   bool
}

func (s *ssmClientMock) getParameters(_ context.Context, names []string) ([]parameter, error) {
	if s.getError {
		return nil, errors.New("getParameters error")
	}

	var result []parameter
	for _, name := range names {
		if _, ok := s.parameters[name]; ok {
			result = append(result, s.parameters[name])
		}
	}
	return result, nil
}

func (s *ssmClientMock) putParameter(_ context.Context, name, value string) error {
	if s.putError {
		return errors.New("putParameter error")
	}

	if s.parameters == nil {
		s.parameters = make(map[string]parameter)
	}

	s.parameters[name] = parameter{ID: name, Value: value, LastUpdate: time.Now()}
	return nil
}
