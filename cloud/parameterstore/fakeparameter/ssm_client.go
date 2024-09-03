package fakeparameter

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"github.com/evergreen-ci/utility"
)

// FakeSSMClient implements the parameterstore.SSMClient interface backed by the
// fake parameters in the DB. This should only be used in testing.
type FakeSSMClient struct{}

// NewFakeSSMClient returns a fake SSM client implementation backed by the DB.
// This should only be used in testing.
func NewFakeSSMClient() *FakeSSMClient {
	return &FakeSSMClient{}
}

// PutParameter inserts a parameter into the fake parameter store.
func (c *FakeSSMClient) PutParameter(ctx context.Context, input *ssm.PutParameterInput) (*ssm.PutParameterOutput, error) {
	name := utility.FromStringPtr(input.Name)
	value := utility.FromStringPtr(input.Value)
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(name == "", "name is required")
	catcher.NewWhen(value == "", "value is required")
	if catcher.HasErrors() {
		return nil, errors.Wrap(catcher.Resolve(), "invalid input")
	}

	p := FakeParameter{
		Name:        name,
		Value:       value,
		LastUpdated: time.Now(),
	}
	if aws.ToBool(input.Overwrite) {
		if err := p.Upsert(ctx); err != nil {
			return nil, errors.Wrap(err, "upserting fake parameter")
		}
	} else {
		if err := p.Insert(ctx); err != nil {
			return nil, errors.Wrap(err, "inserting fake parameter")
		}
	}

	return &ssm.PutParameterOutput{}, nil
}

// DeleteParametersSimple is the same as DeleteParameters but only returns the
// deleted parameter names.
func (c *FakeSSMClient) DeleteParametersSimple(ctx context.Context, input *ssm.DeleteParametersInput) ([]string, error) {
	output, err := c.DeleteParameters(ctx, input)
	if err != nil {
		return nil, err
	}

	var names []string
	for _, o := range output {
		names = append(names, o.DeletedParameters...)
	}
	return names, nil
}

// DeleteParameters deletes the specified parameters from the fake parameter
// store.
func (c *FakeSSMClient) DeleteParameters(ctx context.Context, input *ssm.DeleteParametersInput) ([]*ssm.DeleteParametersOutput, error) {
	names := input.Names
	if len(names) == 0 {
		return nil, nil
	}

	var output ssm.DeleteParametersOutput
	for _, name := range names {
		deleted, err := DeleteOneID(ctx, name)
		if err != nil {
			return nil, errors.Wrapf(err, "deleting fake parameter '%s'", name)
		}
		if !deleted {
			// AWS Parameter Store does not return an error if a parameter does
			// not exist. Instead it returns it as an invalid parameter in the
			// result.
			output.InvalidParameters = append(output.InvalidParameters, name)
			continue
		}

		output.DeletedParameters = append(output.DeletedParameters, name)
	}

	return []*ssm.DeleteParametersOutput{&output}, nil
}

// GetParametersSimple is the same as GetParameters but only returns the
// parameters.
func (c *FakeSSMClient) GetParametersSimple(ctx context.Context, input *ssm.GetParametersInput) ([]ssmTypes.Parameter, error) {
	outputs, err := c.GetParameters(ctx, input)
	if err != nil {
		return nil, err
	}

	var params []ssmTypes.Parameter
	for _, o := range outputs {
		params = append(params, o.Parameters...)
	}
	return params, nil
}

// GetParameters retrieves the specified parameters from the fake parameter
// store.
func (c *FakeSSMClient) GetParameters(ctx context.Context, input *ssm.GetParametersInput) ([]*ssm.GetParametersOutput, error) {
	names := input.Names
	if len(names) == 0 {
		return nil, nil
	}

	var output ssm.GetParametersOutput
	for _, name := range names {
		p, err := FindOneID(ctx, name)
		if err != nil {
			return nil, errors.Wrapf(err, "finding parameter '%s'", name)
		}
		if p == nil {
			// AWS Parameter Store does not return an error if a parameter does
			// not exist. Instead it returns it as an invalid parameter in the
			// result.
			output.InvalidParameters = append(output.InvalidParameters, name)
			continue
		}

		output.Parameters = append(output.Parameters, ssmTypes.Parameter{
			Name:             utility.ToStringPtr(p.Name),
			Value:            utility.ToStringPtr(p.Value),
			LastModifiedDate: utility.ToTimePtr(p.LastUpdated),
		})
	}

	return []*ssm.GetParametersOutput{&output}, nil
}
