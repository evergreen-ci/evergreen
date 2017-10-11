// Package bsonutil provides a number of simple common utilities for
// interacting bson-tagged structs in go.
//
// This code originates in the Evergreen project and is a centerpiece
// of that project's document mapping strategy.
package bsonutil

import (
	"fmt"
	"reflect"
	"strings"

	"github.com/pkg/errors"
)

// Tag returns the value of the "bson" tag for the given struct field name of
// the "data" struct, stripping any tag modifiers such as "omitempty".
// Returns the empty string if there is no tag, and an error if the field
// does not exist in the struct.
//
// If data is a slice of structs, this check applies to the type of
// struct in the slice.
func Tag(data interface{}, fieldName string) (string, error) {
	dataType := reflect.TypeOf(data)
	if dataType.Kind() == reflect.Slice {
		dataType = dataType.Elem()
	}

	if dataType.Kind() != reflect.Struct {
		return "", errors.Errorf("must pass in a struct data type [%T]", data)
	}

	field, found := dataType.FieldByName(fieldName)
	if !found {
		return "", errors.Errorf("struct of type '%T' does not have a field %s",
			data, fieldName)
	}
	tag := field.Tag.Get("bson")

	// NOTE: this stops us from being able to use commas in the bson field names
	// of our models
	if index := strings.Index(tag, ","); index != -1 {
		tag = tag[:index]
	}
	return tag, nil
}

// MustHaveTag gets the "bson" struct tag for a field, panicking if
// either the field does not exist or has no "bson" tag.
func MustHaveTag(data interface{}, fieldName string) string {
	tagValue, err := Tag(data, fieldName)
	if err != nil {
		panic(fmt.Sprintf("error getting bson tag: %s", err.Error()))
	}
	if tagValue == "" {
		panic(fmt.Sprintf("field %v cannot have an empty bson tag", fieldName))
	}
	return tagValue
}
