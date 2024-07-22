package parameterstore

import (
	"context"
	"fmt"
	"time"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo"
)

var parameterCache = make(map[string]parameter)

type parameter struct {
	id         string    `bson:"_id"`
	lastUpdate time.Time `bson:"last_update"`
	value      string    `bson:"value"`
}

type parameterStore struct {
	ssm  *ssmClient
	opts ParameterStoreOptions
}

type ParameterStoreOptions struct {
	Database   *mongo.Database
	Prefix     string
	SSMBackend bool
}

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

func (p *parameterStore) SetParameter(ctx context.Context, name, value string) error {
	fullName := fmt.Sprintf("%s/%s", p.opts.Prefix, name)

	if !p.opts.SSMBackend {
		return p.setLocalValue(ctx, fullName, value)
	}

	if err := p.ssm.putParameter(ctx, fullName, value); err != nil {
		return errors.Wrap(err, "putting parameter in Parameter Store")
	}

	now := time.Now()
	if err := p.SetLastUpdate(ctx, fullName, now); err != nil {
		return errors.Wrap(err, "setting last updated")
	}
	parameterCache[fullName] = parameter{value: value, lastUpdate: now}
	return nil
}

func (p *parameterStore) GetParameter(ctx context.Context, name string) (string, error) {
	paramMap, err := p.GetParameters(ctx, []string{name})
	return paramMap[name], err
}

func (p *parameterStore) GetParameters(ctx context.Context, names []string) (map[string]string, error) {
	fullNames := make([]string, 0, len(names))
	for _, name := range names {
		fullNames = append(fullNames, fmt.Sprintf("%s/%s", p.opts.Prefix, name))
	}

	params, err := p.find(ctx, fullNames)
	if err != nil {
		return nil, errors.Wrap(err, "getting parameters")
	}
	paramMap := make(map[string]string, len(fullNames))
	n := 0
	for _, name := range fullNames {
		for _, param := range params {
			if param.id != name {
				continue
			}
			// Values set in the database override values set in Parameter Store.
			if param.value != "" {
				paramMap[name] = param.value
				continue
			}
			if cachedParam, ok := parameterCache[name]; ok && cachedParam.lastUpdate.After(param.lastUpdate) {
				paramMap[name] = cachedParam.value
				continue
			}
			fullNames[n] = name
			n++
		}
	}
	fullNames = fullNames[:n]

	if !p.opts.SSMBackend {
		return paramMap, nil
	}

	data, err := p.ssm.getParameters(ctx, fullNames)
	if err != nil {
		return nil, errors.Wrap(err, "fetching values for parameters")
	}
	catcher := grip.NewBasicCatcher()
	for _, ssmParam := range data {
		paramMap[ssmParam.id] = ssmParam.value
		parameterCache[ssmParam.id] = parameter{value: ssmParam.value, lastUpdate: time.Now()}
		for _, dbParam := range params {
			if dbParam.id != ssmParam.id {
				continue
			}
			if !dbParam.lastUpdate.Equal(ssmParam.lastUpdate) {
				catcher.Wrapf(p.SetLastUpdate(ctx, ssmParam.id, ssmParam.lastUpdate), "setting last update for parameter '%s'", ssmParam.id)
			}
		}
	}
	return paramMap, catcher.Resolve()
}
