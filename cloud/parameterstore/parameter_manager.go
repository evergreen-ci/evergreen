package parameterstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
	// cache holds the in-memory cache of parameters. If parameter caching is
	// enabled, the cache will reduce the number of reads from Parameter Store
	// by only fetching directly from Parameter Store if the value is missing
	// from the cache or is stale.
	cache     *parameterCache
	ssmClient SSMClient
	db        *mongo.Database
}

// NewParameterManager creates a new ParameterManager instance.
func NewParameterManager(pathPrefix string, cachingEnabled bool, ssmClient SSMClient, db *mongo.Database) *ParameterManager {
	if pathPrefix != "" {
		// Ensure the prefix has a leading slash to make it an absolute path in
		// the hierarchy and a trailing slash to separate it from the remaining
		// name.
		pathPrefix = fmt.Sprintf("/%s/", strings.TrimPrefix(strings.TrimSuffix(pathPrefix, "/"), "/"))
	}
	pm := ParameterManager{
		pathPrefix: pathPrefix,
		ssmClient:  ssmClient,
		db:         db,
	}
	if cachingEnabled {
		pm.cache = newParameterCache()
	}
	return &pm
}

// Put adds or updates a parameter. This returns the created parameter.
func (pm *ParameterManager) Put(ctx context.Context, name, value string) (*Parameter, error) {
	if name == "" {
		return nil, errors.New("cannot put a parameter with an empty name")
	}

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

	// Regardless of whether caching is enabled or not, still record that the
	// parameter was changed in case caching gets enabled or a different
	// ParameterManager instance has caching enabled.
	if err := BumpParameterRecord(ctx, pm.db, fullName, time.Now()); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "could not bump parameter update timestamp, possibly because it is being concurrently updated",
			"name":    fullName,
		}))
	}

	return &Parameter{
		Name:     fullName,
		Basename: getBasename(fullName),
		Value:    value,
	}, nil
}

// Get retrieves the parameters given by the provided name(s). If some
// parameters cannot be found, they will not be returned. Use GetStrict to both
// get the parameters and validate that all the requested parameters were found.
// kim: TODO: test externally (and verify cache contents during testing):
// - Getting nonexistent parameter (get nonexistent)
// - Getting partially nonexistent parameters (get existing and nonexistent)
// - Getting partially cached parameters (get existing + uncached, existing +
// cached, and nonexistent)
// - Getting parameter that was cached but later deleted (put, get, del, get)
// - Getting parameter that was cached, deleted, and then later recreated (put,
// get, del, put, get)
// - Getting parameter that was cached multiple times (put, get, put, get)
func (pm *ParameterManager) Get(ctx context.Context, names ...string) ([]Parameter, error) {
	if len(names) == 0 {
		return nil, nil
	}

	fullNames := make([]string, 0, len(names))
	for _, name := range names {
		fullNames = append(fullNames, pm.getPrefixedName(name))
	}

	fullNamesToFind := fullNames
	params := make([]Parameter, 0, len(fullNamesToFind))
	if pm.isCachingEnabled() {
		paramRecords, err := FindByIDs(ctx, pm.db, fullNames...)
		if err != nil {
			return nil, errors.Wrapf(err, "finding parameter records for %d parameters", len(names))
		}
		cachedParams, namesNotFound := pm.cache.get(paramRecords...)
		for _, cachedParam := range cachedParams {
			params = append(params, cachedParam.export())
		}
		fullNamesToFind = namesNotFound
	}

	if len(fullNamesToFind) == 0 {
		// Cache found all the parameters.
		return params, nil
	}

	// It's important to set the time of retrieval for the cache before actually
	// retrieving the value from Parameter Store. This is to be on the
	// conservative side and ensure the cache doesn't return outdated values. If
	// a parameter is updated in between getting the parameter from Parameter
	// Store and caching it, the cache has just updated to an already-outdated
	// value, so it should evict the outdated value on the next read, thus
	// ensuring eventual consistency.
	lastRetrieved := utility.BSONTime(time.Now())
	ssmParams, err := pm.ssmClient.GetParametersSimple(ctx, &ssm.GetParametersInput{
		Names:          fullNamesToFind,
		WithDecryption: aws.Bool(true),
	})
	if err != nil {
		return nil, errors.Wrapf(err, "getting %d parameters", len(fullNames))
	}

	cachedParams := make([]cachedParameter, 0, len(ssmParams))
	for _, p := range ssmParams {
		name := aws.ToString(p.Name)
		value := aws.ToString(p.Value)
		params = append(params, Parameter{
			Name:     name,
			Basename: getBasename(name),
			Value:    value,
		})
		cachedParams = append(cachedParams, newCachedParameter(name, value, lastRetrieved))
	}

	if pm.isCachingEnabled() {
		pm.cache.put(cachedParams...)
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

	params, err := pm.Get(ctx, fullNames...)
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
		return errors.Wrapf(err, "deleting %d parameters", len(fullNames))
	}

	for _, fullName := range fullNames {
		// Regardless of whether caching is enabled or not, still record that
		// the parameter was changed in case caching gets enabled or a different
		// ParameterManager instance has caching enabled.
		if err := BumpParameterRecord(ctx, pm.db, fullName, time.Now()); err != nil {
			grip.Warning(message.WrapError(err, message.Fields{
				"message": "could not bump parameter record last updated timestamp, possibly because it is being concurrently updated",
				"name":    fullName,
			}))
		}
	}

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
func getBasename(name string) string {
	idx := strings.LastIndex(name, "/")
	if idx == -1 {
		return name
	}
	return name[idx+1:]
}

// isCachingEnabled returns whether parameter caching is enabled.
func (pm *ParameterManager) isCachingEnabled() bool {
	return pm.cache != nil
}
