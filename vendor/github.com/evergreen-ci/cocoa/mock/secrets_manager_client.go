package mock

import (
	"context"
	"errors"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/evergreen-ci/utility"
)

// StoredSecret is a representation of a secret kept in the global secret
// storage cache.
type StoredSecret struct {
	Value       string
	BinaryValue []byte
	Created     time.Time
}

// GlobalSecretCache is a global secret storage cache that provides a simplified
// in-memory implementation of a secrets storage service. This can be used
// indirectly with the SecretsManagerClient to access and modify secrets, or
// used directly.
var GlobalSecretCache map[string]StoredSecret

func init() {
	GlobalSecretCache = map[string]StoredSecret{}
}

// SecretsManagerClient provides a mock implementation of a
// cocoa.SecretsManagerClient. This makes it possible to introspect on inputs
// to the client and control the client's output. It provides some default
// implementations where possible.
type SecretsManagerClient struct {
	CreateSecretInput  *secretsmanager.CreateSecretInput
	CreateSecretOutput *secretsmanager.CreateSecretOutput

	GetSecretValueInput  *secretsmanager.GetSecretValueInput
	GetSecretValueOutput *secretsmanager.GetSecretValueOutput

	UpdateSecretInput  *secretsmanager.UpdateSecretInput
	UpdateSecretOutput *secretsmanager.UpdateSecretOutput

	DeleteSecretInput  *secretsmanager.DeleteSecretInput
	DeleteSecretOutput *secretsmanager.DeleteSecretOutput
}

// CreateSecret saves the input options and returns a new mock secret. The mock
// output can be customized. By default, it will create and save a cached mock
// secret based on the input in the global secret cache.
func (c *SecretsManagerClient) CreateSecret(ctx context.Context, in *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
	c.CreateSecretInput = in

	if c.CreateSecretOutput != nil {
		return c.CreateSecretOutput, nil
	}

	if in.Name == nil {
		return nil, errors.New("missing secret name")
	}
	if in.SecretBinary != nil && in.SecretString != nil {
		return nil, errors.New("cannot specify both secret binary and secret string")
	}
	if in.SecretBinary == nil && in.SecretString == nil {
		return nil, errors.New("must specify either secret binary or secret string")
	}

	GlobalSecretCache[*in.Name] = StoredSecret{
		BinaryValue: in.SecretBinary,
		Created:     time.Now(),
		Value:       utility.FromStringPtr(in.SecretString),
	}

	return &secretsmanager.CreateSecretOutput{
		ARN:  in.Name,
		Name: in.Name,
	}, nil
}

// GetSecretValue saves the input options and returns an existing mock secret's
// value. The mock output can be customized. By default, it will return a cached
// mock secret if it exists in the global secret cache.
func (c *SecretsManagerClient) GetSecretValue(ctx context.Context, in *secretsmanager.GetSecretValueInput) (*secretsmanager.GetSecretValueOutput, error) {
	c.GetSecretValueInput = in

	if c.GetSecretValueOutput != nil {
		return c.GetSecretValueOutput, nil
	}

	if in.SecretId == nil {
		return nil, errors.New("missing secret ID")
	}

	s, ok := GlobalSecretCache[*in.SecretId]
	if !ok {
		return nil, errors.New("secret not found")
	}

	return &secretsmanager.GetSecretValueOutput{
		Name:         in.SecretId,
		ARN:          in.SecretId,
		SecretString: aws.String(s.Value),
		SecretBinary: s.BinaryValue,
		CreatedDate:  aws.Time(s.Created),
	}, nil
}

// UpdateSecretValue saves the input options and returns an updated mock secret
// value. The mock output can be customized. By default, it will update a cached
// mock secret if it exists in the global secret cache.
func (c *SecretsManagerClient) UpdateSecretValue(ctx context.Context, in *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
	c.UpdateSecretInput = in

	if c.UpdateSecretOutput != nil {
		return c.UpdateSecretOutput, nil
	}

	if in.SecretId == nil {
		return nil, errors.New("missing secret ID")
	}
	if in.SecretBinary != nil && in.SecretString != nil {
		return nil, errors.New("cannot specify both secret binary and secret string")
	}
	if in.SecretBinary == nil && in.SecretString == nil {
		return nil, errors.New("must specify either secret binary or secret string")
	}

	s, ok := GlobalSecretCache[*in.SecretId]
	if !ok {
		return nil, errors.New("secret not found")
	}

	if in.SecretBinary != nil {
		s.BinaryValue = in.SecretBinary
	}
	if in.SecretString != nil {
		s.Value = *in.SecretString
	}

	GlobalSecretCache[*in.SecretId] = s

	return &secretsmanager.UpdateSecretOutput{
		ARN:  in.SecretId,
		Name: in.SecretId,
	}, nil
}

// DeleteSecret saves the input options and deletes an existing mock secret. The
// mock output can be customized. By default, it will delete a cached mock
// secret if it exists.
func (c *SecretsManagerClient) DeleteSecret(ctx context.Context, in *secretsmanager.DeleteSecretInput) (*secretsmanager.DeleteSecretOutput, error) {
	c.DeleteSecretInput = in

	if c.DeleteSecretOutput != nil {
		return c.DeleteSecretOutput, nil
	}

	if in.SecretId == nil {
		return nil, errors.New("missing secret ID")
	}

	if !utility.FromBoolPtr(in.ForceDeleteWithoutRecovery) {
		if _, ok := GlobalSecretCache[*in.SecretId]; !ok {
			return nil, errors.New("secret not found")
		}
	}

	delete(GlobalSecretCache, *in.SecretId)

	return &secretsmanager.DeleteSecretOutput{
		ARN:          in.SecretId,
		Name:         in.SecretId,
		DeletionDate: aws.Time(time.Now()),
	}, nil
}

// Close closes the mock client. The mock output can be customized. By default,
// it is a no-op that returns no error.
func (c *SecretsManagerClient) Close(ctx context.Context) error {
	return nil
}
