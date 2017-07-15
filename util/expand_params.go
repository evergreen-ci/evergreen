package util

import (
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

const (
	pluginTagPrefix      = "plugin"
	pluginExpandAllowed  = "expand"
	pluginExpandStartTag = "${"
	pluginExpandEndTag   = "}"
)

// Taking in the input and expansions map, apply the expansions to any
// appropriate fields in the input.  The input must be a pointer to a struct
// so that the underlying struct can be modified.
func ExpandValues(input interface{}, expansions *Expansions) error {

	// make sure the input is a pointer to a map or struct
	if reflect.ValueOf(input).Type().Kind() != reflect.Ptr {
		return errors.New("input to expand must be a pointer")
	}
	inputVal := reflect.Indirect(reflect.ValueOf(input))

	// expand map or struct
	switch inputVal.Type().Kind() {
	case reflect.Struct:
		if err := expandStruct(inputVal, expansions); err != nil {
			return errors.Wrap(err, "error expanding struct")
		}
	case reflect.Map:
		if err := expandMap(inputVal, expansions); err != nil {
			return errors.Wrap(err, "error expanding map")
		}
	default:
		return errors.New("input to expand must be a pointer to a struct or map")
	}

	return nil
}

// Helper function to expand a map. Returns expanded version with
// both keys and values expanded
func expandMap(inputMap reflect.Value, expansions *Expansions) error {

	if inputMap.Type().Key().Kind() != reflect.String {
		return errors.New("input map to expand must have keys of string type")
	}

	// iterate through keys and value, expanding them
	for _, key := range inputMap.MapKeys() {
		expandedKeyString, err := expansions.ExpandString(key.String())
		if err != nil {
			return errors.Wrapf(err, "could not expand key %v", key.String())
		}

		// expand and set new value
		val := inputMap.MapIndex(key)
		expandedVal := reflect.Value{}
		switch val.Type().Kind() {
		case reflect.String:
			expandedValString, err := expansions.ExpandString(val.String())
			if err != nil {
				return errors.Wrapf(err, "could not expand value %v", val.String())
			}
			expandedVal = reflect.ValueOf(expandedValString)
		case reflect.Map:
			if err := expandMap(val, expansions); err != nil {
				return errors.Wrapf(err, "could not expand value %v: %v", val.String())
			}
			expandedVal = val
		default:
			return errors.Errorf(
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
func expandStruct(inputVal reflect.Value, expansions *Expansions) error {

	// find any values with an expandable tag
	numFields := inputVal.NumField()
	for i := 0; i < numFields; i++ {
		field := inputVal.Type().Field(i)
		fieldTag := field.Tag.Get(pluginTagPrefix)

		// no tag, skip
		if fieldTag == "" {
			continue
		}

		// split the tag into its parts
		tagParts := strings.Split(fieldTag, ",")

		// see if the field is expandable
		if !SliceContains(tagParts, pluginExpandAllowed) {
			continue
		}

		// if the field is a struct, descend recursively
		if field.Type.Kind() == reflect.Struct {
			err := expandStruct(inputVal.FieldByName(field.Name), expansions)
			if err != nil {
				return errors.Wrapf(err, "error expanding struct in field %v",
					field.Name)
			}
			continue
		}

		// if the field is a map, descend recursively
		if field.Type.Kind() == reflect.Map {
			inputMap := inputVal.FieldByName(field.Name)
			err := expandMap(inputMap, expansions)
			if err != nil {
				return errors.Wrapf(err, "error expanding map in field %v",
					field.Name)
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
					return errors.Wrapf(err, "error expanding struct in field %v",
						field.Name)
				}
			}
			continue
		}

		// make sure the field is a string
		if field.Type.Kind() != reflect.String {
			return errors.Errorf("cannot expand non-string field '%v' "+
				"which is of type %v", field.Name, field.Type.Kind())
		}

		// it's expandable - apply the expansions
		fieldOfElem := inputVal.FieldByName(field.Name)
		err := expandString(fieldOfElem, expansions)
		if err != nil {
			return errors.Wrapf(err, "error applying expansions to field %v with value %v",
				field.Name, fieldOfElem.String())
		}
	}

	return nil
}

func expandString(inputVal reflect.Value, expansions *Expansions) error {
	expanded, err := expansions.ExpandString(inputVal.String())
	if err != nil {
		return errors.WithStack(err)
	}
	inputVal.SetString(expanded)
	return nil
}

// IsExpandable returns true if the passed in string contains an
// expandable parameter
func IsExpandable(param string) bool {
	startIndex := strings.Index(param, pluginExpandStartTag)
	endIndex := strings.Index(param, pluginExpandEndTag)
	return startIndex >= 0 && endIndex >= 0 && endIndex > startIndex
}
