package plugin

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/util"
)

const (
	PluginTagPrefix      = "plugin"
	PluginExpandAllowed  = "expand"
	PluginExpandStartTag = "${"
	PluginExpandEndTag   = "}"
)

// Taking in the input and expansions map, apply the expansions to any
// appropriate fields in the input.  The input must be a pointer to a struct
// so that the underlying struct can be modified.
func ExpandValues(input interface{}, expansions *command.Expansions) error {

	// make sure the input is a pointer to a map or struct
	if reflect.ValueOf(input).Type().Kind() != reflect.Ptr {
		return fmt.Errorf("input to expand must be a pointer")
	}
	inputVal := reflect.Indirect(reflect.ValueOf(input))

	// expand map or struct
	switch inputVal.Type().Kind() {
	case reflect.Struct:
		if err := expandStruct(inputVal, expansions); err != nil {
			return fmt.Errorf("error expanding struct: %v", err)
		}
	case reflect.Map:
		if err := expandMap(inputVal, expansions); err != nil {
			return fmt.Errorf("error expanding map: %v", err)
		}
	default:
		return fmt.Errorf("input to expand must be a pointer to a struct or map")
	}

	return nil
}

// Helper function to expand a map. Returns expanded version with
// both keys and values expanded
func expandMap(inputMap reflect.Value, expansions *command.Expansions) error {

	if inputMap.Type().Key().Kind() != reflect.String {
		return fmt.Errorf("input map to expand must have keys of string type")
	}

	// iterate through keys and value, expanding them
	for _, key := range inputMap.MapKeys() {
		expandedKeyString, err := expansions.ExpandString(key.String())
		if err != nil {
			return fmt.Errorf("could not expand key %v: %v", key.String(), err)
		}

		// expand and set new value
		val := inputMap.MapIndex(key)
		expandedVal := reflect.Value{}
		switch val.Type().Kind() {
		case reflect.String:
			expandedValString, err := expansions.ExpandString(val.String())
			if err != nil {
				return fmt.Errorf("could not expand value %v: %v", val.String(), err)
			}
			expandedVal = reflect.ValueOf(expandedValString)
		case reflect.Map:
			if err := expandMap(val, expansions); err != nil {
				return fmt.Errorf("could not expand value %v: %v", val.String(), err)
			}
			expandedVal = val
		default:
			return fmt.Errorf(
				"could not expand value %v: must be string, map, or struct",
				val.String())
		}

		// unset unexpanded key then set expanded key
		inputMap.SetMapIndex(key, reflect.Value{})
		inputMap.SetMapIndex(reflect.ValueOf(expandedKeyString), expandedVal)
	}

	return nil
}

// Helper function to expand a single struct.  Returns the expanded version
// of the struct.
func expandStruct(inputVal reflect.Value, expansions *command.Expansions) error {

	// find any values with an expandable tag
	numFields := inputVal.NumField()
	for i := 0; i < numFields; i++ {
		field := inputVal.Type().Field(i)
		fieldTag := field.Tag.Get(PluginTagPrefix)

		// no tag, skip
		if fieldTag == "" {
			continue
		}

		// split the tag into its parts
		tagParts := strings.Split(fieldTag, ",")

		// see if the field is expandable
		if !util.SliceContains(tagParts, PluginExpandAllowed) {
			continue
		}

		// if the field is a struct, descend recursively
		if field.Type.Kind() == reflect.Struct {
			err := expandStruct(inputVal.FieldByName(field.Name), expansions)
			if err != nil {
				return fmt.Errorf("error expanding struct in field %v: %v",
					field.Name, err)
			}
			continue
		}

		// if the field is a map, descend recursively
		if field.Type.Kind() == reflect.Map {
			inputMap := inputVal.FieldByName(field.Name)
			err := expandMap(inputMap, expansions)
			if err != nil {
				return fmt.Errorf("error expanding map in field %v: %v",
					field.Name, err)
			}
			continue
		}

		// if the field is a slice, descend recursively
		if field.Type.Kind() == reflect.Slice {
			slice := inputVal.FieldByName(field.Name)
			for i := 0; i < slice.Len(); i++ {
				var err error
				sliceKind := slice.Index(i).Kind()
				if sliceKind == reflect.String {
					err = expandString(slice.Index(i), expansions)
				} else if sliceKind == reflect.Interface || sliceKind == reflect.Ptr {
					err = expandStruct(slice.Index(i).Elem(), expansions)
				} else {
					//don't take elem if it's not an array of interface/pointers
					err = expandStruct(slice.Index(i), expansions)
				}
				if err != nil {
					return fmt.Errorf("error expanding struct in field %v: %v",
						field.Name, err)
				}
			}
			continue
		}

		// make sure the field is a string
		if field.Type.Kind() != reflect.String {
			return fmt.Errorf("cannot expand non-string field '%v' "+
				"which is of type %v", field.Name, field.Type.Kind())
		}

		// it's expandable - apply the expansions
		fieldOfElem := inputVal.FieldByName(field.Name)
		err := expandString(fieldOfElem, expansions)
		if err != nil {
			return fmt.Errorf("error applying expansions to field %v with"+
				" value %v: %v", field.Name, fieldOfElem.String(), err)
		}
	}

	return nil
}

func expandString(inputVal reflect.Value, expansions *command.Expansions) error {
	expanded, err := expansions.ExpandString(inputVal.String())
	if err != nil {
		return err
	}
	inputVal.SetString(expanded)
	return nil
}

// IsExpandable returns true if the passed in string contains an
// expandable parameter
func IsExpandable(param string) bool {
	startIndex := strings.Index(param, PluginExpandStartTag)
	endIndex := strings.Index(param, PluginExpandEndTag)
	return startIndex >= 0 && endIndex >= 0 && endIndex > startIndex
}
