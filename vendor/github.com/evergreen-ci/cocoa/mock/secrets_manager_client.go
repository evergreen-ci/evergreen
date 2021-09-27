package mock

import (
	"context"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/awserr"
	"github.com/aws/aws-sdk-go/service/secretsmanager"
	"github.com/evergreen-ci/utility"
)

// StoredSecret is a representation of a secret kept in the global secret
// storage cache.
type StoredSecret struct {
	ARN         string
	Value       string
	BinaryValue []byte
	IsDeleted   bool
	Created     time.Time
	Updated     time.Time
	Accessed    time.Time
	Deleted     time.Time
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
	CreateSecretError  error

	GetSecretValueInput  *secretsmanager.GetSecretValueInput
	GetSecretValueOutput *secretsmanager.GetSecretValueOutput
	GetSecretValueError  error

	DescribeSecretInput  *secretsmanager.DescribeSecretInput
	DescribeSecretOutput *secretsmanager.DescribeSecretOutput
	DescribeSecretError  error

	ListSecretsInput  *secretsmanager.ListSecretsInput
	ListSecretsOutput *secretsmanager.ListSecretsOutput
	ListSecretsError  error

	UpdateSecretInput  *secretsmanager.UpdateSecretInput
	UpdateSecretOutput *secretsmanager.UpdateSecretOutput
	UpdateSecretError  error

	DeleteSecretInput  *secretsmanager.DeleteSecretInput
	DeleteSecretOutput *secretsmanager.DeleteSecretOutput
	DeleteSecretError  error
}

// CreateSecret saves the input options and returns a new mock secret. The mock
// output can be customized. By default, it will create and save a cached mock
// secret based on the input in the global secret cache.
func (c *SecretsManagerClient) CreateSecret(ctx context.Context, in *secretsmanager.CreateSecretInput) (*secretsmanager.CreateSecretOutput, error) {
	c.CreateSecretInput = in

	if c.CreateSecretOutput != nil || c.CreateSecretError != nil {
		return c.CreateSecretOutput, c.CreateSecretError
	}

	if in.Name == nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "missing secret name", nil)
	}
	if in.SecretBinary != nil && in.SecretString != nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "cannot specify both secret binary and secret string", nil)
	}
	if in.SecretBinary == nil && in.SecretString == nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "must specify either secret binary or secret string", nil)
	}

	name := utility.FromStringPtr(in.Name)
	if _, ok := GlobalSecretCache[name]; ok {
		return nil, awserr.New(secretsmanager.ErrCodeResourceExistsException, "secret already exists", nil)
	}

	ts := time.Now()
	GlobalSecretCache[name] = StoredSecret{
		ARN:         utility.FromStringPtr(in.Name),
		Value:       utility.FromStringPtr(in.SecretString),
		BinaryValue: in.SecretBinary,
		Created:     ts,
		Accessed:    ts,
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

	if c.GetSecretValueOutput != nil || c.GetSecretValueError != nil {
		return c.GetSecretValueOutput, c.GetSecretValueError
	}

	if in.SecretId == nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "missing secret ID", nil)
	}

	s, ok := GlobalSecretCache[*in.SecretId]
	if !ok {
		return nil, awserr.New(secretsmanager.ErrCodeResourceNotFoundException, "secret not found", nil)
	}

	if s.IsDeleted {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidRequestException, "secret is deleted", nil)
	}

	s.Accessed = time.Now()
	GlobalSecretCache[*in.SecretId] = s

	return &secretsmanager.GetSecretValueOutput{
		Name:         in.SecretId,
		ARN:          in.SecretId,
		SecretString: aws.String(s.Value),
		SecretBinary: s.BinaryValue,
		CreatedDate:  aws.Time(s.Created),
	}, nil
}

// DescribeSecret saves the input options and returns an existing mock secret's
// metadata information. The mock output can be customized. By default, it will
// return information about the cached mock secret if it exists in the global
// secret cache.
func (c *SecretsManagerClient) DescribeSecret(ctx context.Context, in *secretsmanager.DescribeSecretInput) (*secretsmanager.DescribeSecretOutput, error) {
	c.DescribeSecretInput = in

	if c.DescribeSecretOutput != nil || c.DescribeSecretError != nil {
		return c.DescribeSecretOutput, c.DescribeSecretError
	}

	if in.SecretId == nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "missing secret ID", nil)
	}

	s, ok := GlobalSecretCache[utility.FromStringPtr(in.SecretId)]
	if !ok {
		return nil, awserr.New(secretsmanager.ErrCodeResourceNotFoundException, "secret not found", nil)
	}

	return &secretsmanager.DescribeSecretOutput{
		ARN:              in.SecretId,
		Name:             in.SecretId,
		CreatedDate:      utility.ToTimePtr(s.Created),
		LastAccessedDate: utility.ToTimePtr(s.Accessed),
		LastChangedDate:  utility.ToTimePtr(s.Updated),
		DeletedDate:      utility.ToTimePtr(s.Deleted),
	}, nil
}

// ListSecrets saves the input options and returns all matching mock secrets'
// metadata information. The mock output can be customized. By default, it will
// return any matching cached mock secrets in the global secret cache.
func (c *SecretsManagerClient) ListSecrets(ctx context.Context, in *secretsmanager.ListSecretsInput) (*secretsmanager.ListSecretsOutput, error) {
	c.ListSecretsInput = in

	if c.ListSecretsOutput != nil || c.ListSecretsError != nil {
		return c.ListSecretsOutput, c.ListSecretsError
	}

	matched := map[string]StoredSecret{}
	for _, filter := range in.Filters {
		if filter == nil {
			continue
		}

		if filter.Key != nil {
			name := utility.FromStringPtr(filter.Key)
			s, ok := GlobalSecretCache[name]
			if !ok {
				continue
			}

			matched[name] = s
		}

		for _, v := range filter.Values {
			if v == nil {
				continue
			}

			for _, name := range c.namesMatchingValue(utility.FromStringPtr(v)) {
				matched[name] = GlobalSecretCache[name]
			}
		}
	}

	var converted []*secretsmanager.SecretListEntry
	for name, s := range matched {
		converted = append(converted, &secretsmanager.SecretListEntry{
			ARN:              utility.ToStringPtr(s.ARN),
			Name:             utility.ToStringPtr(name),
			CreatedDate:      utility.ToTimePtr(s.Created),
			LastAccessedDate: utility.ToTimePtr(s.Accessed),
			LastChangedDate:  utility.ToTimePtr(s.Updated),
			DeletedDate:      utility.ToTimePtr(s.Deleted),
		})
	}

	return &secretsmanager.ListSecretsOutput{
		SecretList: converted,
	}, nil
}

// namesMatchingValue returns the names of all secrets that match the given
// value. If the value begins with a "!", the match is negated.
func (c *SecretsManagerClient) namesMatchingValue(val string) []string {
	var names []string
	for name, s := range GlobalSecretCache {
		if strings.HasPrefix(val, "!") && s.Value != val[1:] {
			names = append(names, name)
		}
		if !strings.HasPrefix(val, "!") && s.Value == val {
			names = append(names, name)
		}
	}
	return names
}

// UpdateSecretValue saves the input options and returns an updated mock secret
// value. The mock output can be customized. By default, it will update a cached
// mock secret if it exists in the global secret cache.
func (c *SecretsManagerClient) UpdateSecretValue(ctx context.Context, in *secretsmanager.UpdateSecretInput) (*secretsmanager.UpdateSecretOutput, error) {
	c.UpdateSecretInput = in

	if c.UpdateSecretOutput != nil || c.UpdateSecretError != nil {
		return c.UpdateSecretOutput, c.UpdateSecretError
	}

	if in.SecretId == nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "missing secret ID", nil)
	}
	if in.SecretBinary != nil && in.SecretString != nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "cannot specify both secret binary and secret string", nil)
	}
	if in.SecretBinary == nil && in.SecretString == nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "must specify either secret binary or secret string", nil)
	}

	s, ok := GlobalSecretCache[*in.SecretId]
	if !ok {
		return nil, awserr.New(secretsmanager.ErrCodeResourceNotFoundException, "secret not found", nil)
	}

	if in.SecretBinary != nil {
		s.BinaryValue = in.SecretBinary
	}
	if in.SecretString != nil {
		s.Value = *in.SecretString
	}

	ts := time.Now()
	s.Accessed = ts
	s.Updated = ts

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

	if c.DeleteSecretOutput != nil || c.DeleteSecretError != nil {
		return c.DeleteSecretOutput, c.DeleteSecretError
	}

	if in.SecretId == nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "missing secret ID", nil)
	}

	if utility.FromBoolPtr(in.ForceDeleteWithoutRecovery) && in.RecoveryWindowInDays != nil {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "cannot force delete without recovery and also schedule a recovery window", nil)
	}

	window := int(utility.FromInt64Ptr(in.RecoveryWindowInDays))
	if in.RecoveryWindowInDays != nil && (window < 7 || window > 30) {
		return nil, awserr.New(secretsmanager.ErrCodeInvalidParameterException, "recovery window must be between 7 and 30 days", nil)
	}
	if window == 0 {
		window = 30
	}

	s, ok := GlobalSecretCache[*in.SecretId]
	if !utility.FromBoolPtr(in.ForceDeleteWithoutRecovery) && !ok {
		return nil, awserr.New(secretsmanager.ErrCodeResourceNotFoundException, "secret not found", nil)
	}

	ts := time.Now()
	s.Accessed = ts
	s.Updated = ts
	if !utility.FromBoolPtr(in.ForceDeleteWithoutRecovery) {
		s.Deleted = ts.AddDate(0, 0, window)
	}
	s.IsDeleted = true
	GlobalSecretCache[*in.SecretId] = s

	return &secretsmanager.DeleteSecretOutput{
		ARN:          in.SecretId,
		Name:         in.SecretId,
		DeletionDate: aws.Time(s.Deleted),
	}, nil
}

// Close closes the mock client. The mock output can be customized. By default,
// it is a no-op that returns no error.
func (c *SecretsManagerClient) Close(ctx context.Context) error {
	return nil
}
