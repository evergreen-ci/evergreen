package util

import (
	"fmt"

	"github.com/pkg/errors"
)

type KeyValuePair struct {
	Key   string `bson:"key" json:"key" yaml:"key"`
	Value any    `bson:"value" json:"value" yaml:"value"`
}

type KeyValuePairSlice []KeyValuePair

func (in KeyValuePairSlice) Map() (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range in {
		if _, exists := out[pair.Key]; exists {
			return nil, fmt.Errorf("key '%s' is duplicated", pair.Key)
		}
		switch v := pair.Value.(type) {
		case string:
			out[pair.Key] = v
		default:
			return nil, fmt.Errorf("unrecognized type %T in key '%s'", v, pair.Key)
		}

	}
	return out, nil
}

func (in KeyValuePairSlice) NestedMap() (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for _, pair := range in {
		if _, exists := out[pair.Key]; exists {
			return nil, fmt.Errorf("key '%s' is duplicated", pair.Key)
		}
		switch v := pair.Value.(type) {
		case KeyValuePairSlice:
			outMap, err := v.Map()
			if err != nil {
				return nil, errors.Wrapf(err, "parsing key '%s'", pair.Key)
			}
			out[pair.Key] = outMap

		default:
			return nil, fmt.Errorf("unrecognized type %T in key '%s'", v, pair.Key)
		}

	}
	return out, nil
}
