package parameterstore

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

var parameterCache = make(map[string]cachedParameter)

type cachedParameter struct {
	value       string
	lastRefresh time.Time
}

type parameter struct {
	// ID is the internal name of a parameter, comprised of the parameter store prefix
	// prepended to the name.
	ID string `bson:"_id"`
	// LastUpdate is the time of the last update to the parameter in its backing data store.
	LastUpdate time.Time `bson:"last_update"`
	// Value is the content of the parameter. It is only set in the database when SSM is disabled.
	Value string `bson:"value"`
}

type parameterStore struct {
	ssm  ssmClient
	opts ParameterStoreOptions
}

// ParameterStoreOptions configures ParameterStore.
type ParameterStoreOptions struct {
	// Database is a connection to a mongo database.
	Database *mongo.Database
	// Prefix is an internal string prepended to parameter names in storage.
	Prefix string
	// SSMBackend determines if parameters are backed by the AWS SSM service or stored in the database.
	SSMBackend bool
}

// NewParameterStore returns a configured parameter store.
func NewParameterStore(ctx context.Context, opts ParameterStoreOptions) (*parameterStore, error) {
	p := &parameterStore{opts: opts}
	if opts.SSMBackend {
		ssm, err := newSSMClient(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "making SSM client")
		}
		p.ssm = ssm
	}
	return p, nil
}

// SetParameter sets a parameter in the backing parameter store.
func (p *parameterStore) SetParameter(ctx context.Context, name, value string) error {
	fullName := p.prefixedName(name)

	if !p.opts.SSMBackend {
		return errors.Wrap(p.setLocalValue(ctx, fullName, value), "setting value locally")
	}

	if err := p.ssm.putParameter(ctx, fullName, value); err != nil {
		return errors.Wrap(err, "putting parameter in Parameter Store")
	}

	now := time.Now()
	if err := p.SetLastUpdate(ctx, fullName, now); err != nil {
		return errors.Wrap(err, "setting last updated")
	}
	parameterCache[fullName] = cachedParameter{value: value, lastRefresh: now}
	return nil
}

// GetParameter gets a parameter from the backing parameter store.
func (p *parameterStore) GetParameter(ctx context.Context, name string) (string, error) {
	paramMap, err := p.GetParameters(ctx, []string{name})
	return paramMap[name], errors.Wrapf(err, "getting parameter '%s'", name)
}

// GetParameters gets parameters for the given names from the backing parameter store. Parameters
// are returned in a map of name to parameter value.
func (p *parameterStore) GetParameters(ctx context.Context, names []string) (map[string]string, error) {
	fullNames := make([]string, 0, len(names))
	for _, name := range names {
		fullNames = append(fullNames, p.prefixedName(name))
	}

	params, err := p.find(ctx, fullNames)
	if err != nil {
		return nil, errors.Wrap(err, "getting parameters")
	}
	paramMap := make(map[string]string, len(fullNames))
	n := 0
	for _, name := range fullNames {
		var found bool
		var param parameter
		for _, param = range params {
			if param.ID == name {
				found = true
				break
			}
		}
		if found {
			// Values set in the database override values set in Parameter Store.
			if param.Value != "" {
				paramMap[p.basename(name)] = param.Value
				continue
			}
			if cachedParam, ok := parameterCache[name]; ok && !cachedParam.lastRefresh.Before(param.LastUpdate) {
				paramMap[p.basename(name)] = cachedParam.value
				continue
			}
		}
		fullNames[n] = name
		n++
	}
	fullNames = fullNames[:n]

	if !p.opts.SSMBackend || len(fullNames) == 0 {
		return paramMap, nil
	}

	data, err := p.ssm.getParameters(ctx, fullNames)
	if err != nil {
		return nil, errors.Wrap(err, "fetching values for parameters")
	}
	catcher := grip.NewBasicCatcher()
	for _, ssmParam := range data {
		paramMap[p.basename(ssmParam.ID)] = ssmParam.Value
		parameterCache[ssmParam.ID] = cachedParameter{value: ssmParam.Value, lastRefresh: time.Now()}
		catcher.Wrapf(p.SetLastUpdate(ctx, ssmParam.ID, ssmParam.LastUpdate), "setting last update for parameter '%s'", ssmParam.ID)
	}
	return paramMap, catcher.Resolve()
}

func (p *parameterStore) prefixedName(name string) string {
	return fmt.Sprintf("%s/%s", p.opts.Prefix, name)
}

func (p *parameterStore) basename(fullName string) string {
	return strings.TrimPrefix(fullName, p.opts.Prefix+"/")
}
