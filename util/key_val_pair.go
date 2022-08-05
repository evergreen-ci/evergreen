package util

import (
	"fmt"

	"github.com/pkg/errors"
)

type KeyValuePair struct {
	Key   string      `bson:"key" json:"key" yaml:"key"`
	Value interface{} `bson:"value" json:"value" yaml:"value"`
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

func MakeKeyValuePair(in map[string]string) KeyValuePairSlice {
	out := KeyValuePairSlice{}
	for k, v := range in {
		out = append(out, KeyValuePair{Key: k, Value: v})
	}
	return out
}

func MakeNestedKeyValuePair(in map[string]map[string]string) KeyValuePairSlice {
	out := KeyValuePairSlice{}
	for k1, v1 := range in {
		tempKvSlice := KeyValuePairSlice{}
		for k2, v2 := range v1 {
			tempKvSlice = append(tempKvSlice, KeyValuePair{Key: k2, Value: v2})
		}
		out = append(out, KeyValuePair{Key: k1, Value: tempKvSlice})
	}
	return out
}
