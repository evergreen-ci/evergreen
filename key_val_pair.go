package evergreen

import (
	"fmt"

	"github.com/pkg/errors"
)

type KeyValuePair struct {
	Key   string      `bson:"key" json:"key" yaml:"key"`
	Value interface{} `bson:"value" json:"value" yaml:"value"`
}

type KeyValuePairSlice []KeyValuePair

func (in KeyValuePairSlice) KvSliceToMap() (map[string]string, error) {
	out := map[string]string{}
	for _, pair := range in {
		if _, exists := out[pair.Key]; exists {
			return nil, fmt.Errorf("key '%s' is duplicated", pair.Key)
		}
		switch v := pair.Value.(type) {
		case string:
			out[pair.Key] = v
		default:
			return nil, fmt.Errorf("unrecognized type in key '%s'", pair.Key)
		}

	}
	return out, nil
}

func (in KeyValuePairSlice) KvSliceToMapNested() (map[string]map[string]string, error) {
	out := map[string]map[string]string{}
	for _, pair := range in {
		if _, exists := out[pair.Key]; exists {
			return nil, fmt.Errorf("key '%s' is duplicated", pair.Key)
		}
		switch v := pair.Value.(type) {
		case KeyValuePairSlice:
			outMap, err := v.KvSliceToMap()
			if err != nil {
				return nil, errors.Wrapf(err, "error parsing key '%s'", pair.Key)
			}
			for k, v := range outMap {
				out[pair.Key] = map[string]string{}
				out[pair.Key][k] = v
			}
		default:
			return nil, fmt.Errorf("unrecognized type in key '%s'", pair.Key)
		}

	}
	return out, nil
}

func MapToKvSlice(in map[string]string) []KeyValuePair {
	out := []KeyValuePair{}
	for k, v := range in {
		out = append(out, KeyValuePair{Key: k, Value: v})
	}
	return out
}

func MapToKvSliceNested(in map[string]map[string]string) []KeyValuePair {
	out := []KeyValuePair{}
	for k1, v1 := range in {
		tempKvSlice := []KeyValuePair{}
		for k2, v2 := range v1 {
			tempKvSlice = append(tempKvSlice, KeyValuePair{Key: k2, Value: v2})
		}
		out = append(out, KeyValuePair{Key: k1, Value: tempKvSlice})
	}
	return out
}
