package graphql

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/99designs/gqlgen/graphql"
)

// MarshalStringMap handles marshaling StringMap
func MarshalStringMap(val map[string]string) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		err := json.NewEncoder(w).Encode(val)
		if err != nil {
			panic(err)
		}
	})
}

// UnmarshalStringMap handles unmarshaling StringMap
func UnmarshalStringMap(v interface{}) (map[string]string, error) {
	stringMap := make(map[string]string)
	stringInterface, ok := v.(map[string]interface{})
	if !ok {
		return stringMap, fmt.Errorf("%T is not a StringMap", v)
	}
	for key, value := range stringInterface {
		_, ok := value.(string)
		if !ok {
			return stringMap, fmt.Errorf("%v is not a StringMap. Value %v for key %v should be type string but got %T", v, value, key, value)
		}
		strValue := fmt.Sprintf("%v", value)
		stringMap[key] = strValue
	}
	return stringMap, nil
}
