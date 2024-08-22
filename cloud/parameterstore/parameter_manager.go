package parameterstore

import (
	"context"
	"fmt"
	"strings"

	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/ssm"
	ssmTypes "github.com/aws/aws-sdk-go-v2/service/ssm/types"
	"go.mongodb.org/mongo-driver/mongo"
)

// Parameter represents a parameter kept in Parameter Store.
type Parameter struct {
	// Name is the full name of the parameter, including its path. This is a
	// unique identifier.
	Name string `bson:"-" json:"-" yaml:"-"`
	// Basename is the parameter's name without any hierarchical paths. For
	// example, if the full name including the path is
	// /evergreen/prefix/my-parameter, the basename is my-parameter.
	Basename string `bson:"-" json:"-" yaml:"-"`
	// Value is the parameter's plaintext value.
	Value string `bson:"-" json:"-" yaml:"-"`
}

// ParameterManager is an intermediate abstraction layer for interacting with
// parameters in AWS Systems Manager Parameter Store. It supports caching to
// optimize parameter retrieval.
type ParameterManager struct {
	prefix         string
	cachingEnabled bool
	ssmClient      SSMClient
	db             *mongo.Database
}

// NewParameterManager creates a new ParameterManager instance.
func NewParameterManager(prefix string, cachingEnabled bool, ssmClient SSMClient, db *mongo.Database) *ParameterManager {
	prefix = fmt.Sprintf("/%s/", strings.TrimPrefix(strings.TrimSuffix(prefix, "/"), "/"))
	return &ParameterManager{
		prefix:         prefix,
		cachingEnabled: cachingEnabled,
		ssmClient:      ssmClient,
		db:             db,
	}
}

// Put adds or updates a parameter. This returns the created parameter.
// kim: TODO: test in fakeparameters package once fake PS client is available
// (to avoid cyclical dependency).
func (pm *ParameterManager) Put(ctx context.Context, name, value string) (*Parameter, error) {
	fullName := pm.getPrefixedName(name)
	if _, err := pm.ssmClient.PutParameter(ctx, &ssm.PutParameterInput{
		Name:      aws.String(fullName),
		Value:     aws.String(value),
		Overwrite: aws.Bool(true),
		Type:      ssmTypes.ParameterTypeSecureString,
		Tier:      ssmTypes.ParameterTierIntelligentTiering,
	}); err != nil {
		return nil, errors.Wrapf(err, "putting parameter '%s'", name)
	}

	// TODO (DEVPROD-9403): update cache as needed.

	return &Parameter{
		Name:     fullName,
		Basename: pm.getBasename(fullName),
		Value:    value,
	}, nil
}

// Get retrieves the parameters given by the provided name(s). If some
// parameters cannot be found, they will not be returned. Use GetStrict to both
// get the parameters and validate that all the requested parameters were found.
// kim: TODO: test in fakeparameters package once fake PS client is available
// (to avoid cyclical dependency).
func (pm *ParameterManager) Get(ctx context.Context, names ...string) ([]Parameter, error) {
	if len(names) == 0 {
		return nil, nil
	}

	// TODO (DEVPROD-9403): use caching if possible before retrieving value from
	// Parameter Store.

	fullNames := make([]string, 0, len(names))
	for _, name := range names {
		fullNames = append(fullNames, pm.getPrefixedName(name))
	}

	ssmParams, err := pm.ssmClient.GetParametersSimple(ctx, &ssm.GetParametersInput{
		Names:          fullNames,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "getting %d parameters", len(names))
	}

	params := make([]Parameter, 0, len(ssmParams))
	for _, p := range ssmParams {
		name := aws.ToString(p.Name)
		params = append(params, Parameter{
			Name:     name,
			Basename: pm.getBasename(name),
			Value:    aws.ToString(p.Value),
		})
	}

	return params, nil
}

// GetStrict is the same as Get but verifies that all the requested parameter
// names were found before returning the result.
// kim: TODO: test in fakeparameters package once fake PS client is available
// (to avoid cyclical dependency).
func (pm *ParameterManager) GetStrict(ctx context.Context, names ...string) ([]Parameter, error) {
	fullNames := make([]string, 0, len(names))
	for _, name := range names {
		fullNames = append(fullNames, pm.getPrefixedName(name))
	}

	params, err := pm.Get(ctx, names...)
	if err != nil {
		return nil, err
	}

	if len(params) != len(fullNames) {
		foundNames := make(map[string]struct{}, len(params))
		for _, p := range params {
			foundNames[p.Name] = struct{}{}
		}

		var missingNames []string
		for _, name := range fullNames {
			if _, ok := foundNames[name]; !ok {
				missingNames = append(missingNames, name)
			}
		}

		if len(missingNames) > 0 {
			return nil, errors.Errorf("parameter(s) not found: %s", missingNames)
		}
	}

	return params, nil
}

// Delete deletes the parameters given by the provided name(s).
// kim: TODO: test in fakeparameters package once fake PS client is available
// (to avoid cyclical dependency).
func (pm *ParameterManager) Delete(ctx context.Context, names ...string) error {
	if len(names) == 0 {
		return nil
	}

	fullNames := make([]string, 0, len(names))
	for _, name := range names {
		fullNames = append(fullNames, pm.getPrefixedName(name))
	}

	_, err := pm.ssmClient.DeleteParameters(ctx, &ssm.DeleteParametersInput{
		Names: fullNames,
	})
	if err != nil {
		return errors.Wrapf(err, "deleting %d parameters", len(names))
	}

	// TODO (DEVPROD-9403): update cache as needed.

	return nil
}

// getPrefixedName returns the parameter name with the common parameter prefix
// to ensure it is a full path rather than a basename.
func (pm *ParameterManager) getPrefixedName(basename string) string {
	if pm.prefix == "" {
		return basename
	}
	if strings.HasPrefix(basename, pm.prefix) {
		return basename
	}
	prefix := strings.TrimSuffix(strings.TrimPrefix(pm.prefix, "/"), "/")
	return fmt.Sprintf("/%s/%s", prefix, strings.TrimPrefix(basename, "/"))
}

// getBasename returns the parameter basename without any intermediate paths.
func (pm *ParameterManager) getBasename(name string) string {
	idx := strings.LastIndex(name, "/")
	if idx == -1 {
		return name
	}
	return name[idx+1:]
}
