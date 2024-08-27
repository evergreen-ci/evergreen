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
	// unique identifier. For example, the full name could be
	// /evergreen/path/to/my-parameter.
	Name string `bson:"-" json:"-" yaml:"-"`
	// Basename is the parameter's name without any hierarchical paths. For
	// example, if the full name including the path is
	// /evergreen/path/to/my-parameter, the basename is my-parameter.
	Basename string `bson:"-" json:"-" yaml:"-"`
	// Value is the parameter's plaintext value. This value should never be
	// persisted in the DB.
	Value string `bson:"-" json:"-" yaml:"-"`
}

// ParameterManager is an intermediate abstraction layer for interacting with
// parameters in AWS Systems Manager Parameter Store. It supports caching to
// optimize parameter retrieval.
type ParameterManager struct {
	// pathPrefix is the prefix path in the Parameter Store hierarchy. If set,
	// all parameters should be stored under this prefix.
	pathPrefix string
	// cachingEnabled indicates whether parameter caching is enabled. If
	// enabled, the cache will reduce the number of reads from Parameter Store
	// by only fetching directly from Parameter Store if the value is missing
	// from the cache or is stale.
	cachingEnabled bool
	ssmClient      SSMClient
	db             *mongo.Database
}

// NewParameterManager creates a new ParameterManager instance.
func NewParameterManager(pathPrefix string, cachingEnabled bool, ssmClient SSMClient, db *mongo.Database) *ParameterManager {
	if pathPrefix != "" {
		// Ensure the prefix has a leading slash to make it an absolute path in
		// the hierarchy and a trailing slash to separate it from the remaining
		// name.
		pathPrefix = fmt.Sprintf("/%s/", strings.TrimPrefix(strings.TrimSuffix(pathPrefix, "/"), "/"))
	}
	return &ParameterManager{
		pathPrefix:     pathPrefix,
		cachingEnabled: cachingEnabled,
		ssmClient:      ssmClient,
		db:             db,
	}
}

// Put adds or updates a parameter. This returns the created parameter.
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
func (pm *ParameterManager) GetStrict(ctx context.Context, names ...string) ([]Parameter, error) {
	if len(names) == 0 {
		return nil, nil
	}

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
	if pm.pathPrefix == "" {
		return basename
	}
	if strings.HasPrefix(basename, pm.pathPrefix) {
		return basename
	}
	pathPrefix := strings.TrimSuffix(strings.TrimPrefix(pm.pathPrefix, "/"), "/")
	return fmt.Sprintf("/%s/%s", pathPrefix, strings.TrimPrefix(basename, "/"))
}

// getBasename returns the parameter basename without any intermediate paths.
func (pm *ParameterManager) getBasename(name string) string {
	idx := strings.LastIndex(name, "/")
	if idx == -1 {
		return name
	}
	return name[idx+1:]
}
