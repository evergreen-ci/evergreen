package evergreen

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	"github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/pkg/errors"
)

var (
	awsConfig     *aws.Config
	parameterPath = os.Getenv("SSM_PARAMETER_PATH")
)

func getClient(ctx context.Context) (*ssm.Client, error) {
	if awsConfig == nil {
		config, err := config.LoadDefaultConfig(ctx,
			config.WithRegion(DefaultEC2Region),
		)
		if err != nil {
			return nil, errors.Wrap(err, "loading AWS config")
		}

		awsConfig = &config
	}

	return ssm.NewFromConfig(*awsConfig), nil
}

func getAllParameters(ctx context.Context) (map[string]string, error) {
	if parameterPath == "" {
		return nil, errors.New("parameter path is not set")
	}

	client, err := getClient(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "getting SSM client")
	}

	res, err := client.GetParametersByPath(ctx, &ssm.GetParametersByPathInput{
		Path:           aws.String(parameterPath),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, errors.Wrap(err, "getting parameters")
	}

	params := make(map[string]string)
	for _, param := range res.Parameters {
		if param.Name == nil && param.Value == nil {
			continue
		}
		sectionID, _ := strings.CutPrefix(*param.Name, parameterPath)
		params[sectionID] = *param.Value
	}
	return params, nil
}

func decodeParameter(ctx context.Context, target ConfigSection) error {
	if parameterPath == "" {
		return errors.New("parameter path is not set")
	}

	client, err := getClient(ctx)
	if err != nil {
		return errors.Wrap(err, "getting SSM client")
	}

	res, err := client.GetParameter(ctx, &ssm.GetParameterInput{
		Name:           aws.String(fmt.Sprintf("%s%s", parameterPath, target.SectionId())),
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return errors.Wrapf(err, "getting parameter '%s'", target.SectionId())
	}
	if res.Parameter.Value == nil {
		return errors.Errorf("parameter value is nil for '%s'", target.SectionId())
	}

	return errors.Wrapf(json.Unmarshal([]byte(*res.Parameter.Value), target), "unmarshalling parameter '%s'", target.SectionId())
}

func setParameter(ctx context.Context, input ConfigSection) error {
	if parameterPath == "" {
		return errors.New("parameter path is not set")
	}

	client, err := getClient(ctx)
	if err != nil {
		return errors.Wrap(err, "getting config")
	}

	data, err := json.Marshal(input)
	if err != nil {
		return errors.Wrap(err, "marshalling input as json")
	}
	_, err = client.PutParameter(ctx, &ssm.PutParameterInput{
		Name:  aws.String(parameterPath + input.SectionId()),
		Value: aws.String(string(data)),
		Type:  types.ParameterTypeSecureString,
	})
	return errors.Wrapf(err, "setting parameter for '%s'", input.SectionId())
}
