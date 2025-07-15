package graphql

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/99designs/gqlgen/graphql"
	"github.com/mongodb/grip"
)

// MarshalStringMap handles marshaling StringMap
func MarshalStringMap(val map[string]string) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		err := json.NewEncoder(w).Encode(val)
		if err != nil {
			_, err = w.Write([]byte(fmt.Sprintf("Error marshaling StringMap %v: %v", val, err.Error())))
			if err != nil {
				grip.Error(err)
			}
		}
	})
}

// UnmarshalStringMap handles unmarshaling StringMap
func UnmarshalStringMap(v any) (map[string]string, error) {
	stringMap := make(map[string]string)
	stringInterface, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%T is not a StringMap", v)
	}
	for key, value := range stringInterface {
		_, ok := value.(string)
		if !ok {
			return nil, fmt.Errorf("%v is not a StringMap. Value %v for key %v should be type string but got %T", v, value, key, value)
		}
		strValue := fmt.Sprintf("%v", value)
		stringMap[key] = strValue
	}
	return stringMap, nil
}

// MarshalBooleanMap handles marshaling BooleanMap
func MarshalBooleanMap(val map[string]bool) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		err := json.NewEncoder(w).Encode(val)
		if err != nil {
			_, err = w.Write([]byte(fmt.Sprintf("Error marshaling BooleanMap %v: %v", val, err.Error())))
			if err != nil {
				grip.Error(err)
			}
		}
	})
}

// UnmarshalBooleanMap handles unmarshaling BooleanMap
func UnmarshalBooleanMap(v any) (map[string]bool, error) {
	booleanMap := make(map[string]bool)
	booleanInterface, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%T is not a BooleanMap", v)
	}
	for key, value := range booleanInterface {
		boolValue, ok := value.(bool)
		if !ok {
			return nil, fmt.Errorf("%v is not a BooleanMap. Value %v for key %v should be type bool but got %T", v, value, key, value)
		}
		booleanMap[key] = boolValue
	}
	return booleanMap, nil
}
