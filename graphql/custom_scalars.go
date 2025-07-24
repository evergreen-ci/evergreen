package graphql

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/99designs/gqlgen/graphql"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/mongodb/grip"
)

type JIRANotificationsProjectMap map[string]restModel.APIJIRANotificationsProject

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

// MarshalJIRANotificationsProjectMap handles marshaling JIRANotificationsProjectMap
func MarshalJIRANotificationsProjectMap(val JIRANotificationsProjectMap) graphql.Marshaler {
	return graphql.WriterFunc(func(w io.Writer) {
		err := json.NewEncoder(w).Encode(val)
		if err != nil {
			_, err = w.Write([]byte(fmt.Sprintf("Error marshaling JIRANotificationsProjectMap %v: %v", val, err.Error())))
			if err != nil {
				grip.Error(err)
			}
		}
	})
}

// UnmarshalJIRANotificationsProjectMap handles unmarshaling JIRANotificationsProjectMap
func UnmarshalJIRANotificationsProjectMap(v any) (JIRANotificationsProjectMap, error) {
	projectMap := make(JIRANotificationsProjectMap)
	projectInterface, ok := v.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("%T is not a JIRANotificationsProjectMap", v)
	}

	for projectName, value := range projectInterface {
		valueMap, ok := value.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("value for project %s should be an object but got %T", projectName, value)
		}

		project := restModel.APIJIRANotificationsProject{}

		if fieldsVal, exists := valueMap["fields"]; exists && fieldsVal != nil {
			if fieldsMap, ok := fieldsVal.(map[string]any); ok {
				fields := make(map[string]string)
				for fieldKey, fieldValue := range fieldsMap {
					if fieldStr, ok := fieldValue.(string); ok {
						fields[fieldKey] = fieldStr
					} else {
						return nil, fmt.Errorf("field value for %s.%s should be string but got %T", projectName, fieldKey, fieldValue)
					}
				}
				project.Fields = fields
			}
		}

		if componentsVal, exists := valueMap["components"]; exists && componentsVal != nil {
			if componentsSlice, ok := componentsVal.([]any); ok {
				components := make([]string, len(componentsSlice))
				for i, comp := range componentsSlice {
					if compStr, ok := comp.(string); ok {
						components[i] = compStr
					} else {
						return nil, fmt.Errorf("component value for %s[%d] should be string but got %T", projectName, i, comp)
					}
				}
				project.Components = components
			}
		}

		if labelsVal, exists := valueMap["labels"]; exists && labelsVal != nil {
			if labelsSlice, ok := labelsVal.([]any); ok {
				labels := make([]string, len(labelsSlice))
				for i, label := range labelsSlice {
					if labelStr, ok := label.(string); ok {
						labels[i] = labelStr
					} else {
						return nil, fmt.Errorf("label value for %s[%d] should be string but got %T", projectName, i, label)
					}
				}
				project.Labels = labels
			}
		}

		projectMap[projectName] = project
	}

	return projectMap, nil
}
