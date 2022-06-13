package generated

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
func UnmarshalStringMap(v interface{}) (map[string]string, error) {
	stringMap := make(map[string]string)
	stringInterface, ok := v.(map[string]interface{})
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
