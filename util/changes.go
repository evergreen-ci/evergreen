package util

import (
	"reflect"
)

// RecursivelySetUndefinedFields sets all fields that are not set in structToSet to the value of the corresponding field in structToDefaultFrom.
// It takes two reflect.Values, the first is the struct to set the fields on, and the second is the struct to get the default values from.
func RecursivelySetUndefinedFields(structToSet, structToDefaultFrom reflect.Value) {
	if structToSet.Kind() == reflect.Ptr {
		structToSet = structToSet.Elem()
	}
	if structToDefaultFrom.Kind() == reflect.Ptr {
		structToDefaultFrom = structToDefaultFrom.Elem()
	}
	// Iterate through each field of the struct.
	for i := 0; i < structToSet.NumField(); i++ {
		branchField := structToSet.Field(i)

		// If the field isn't set, use the default field.
		// Note for pointers and maps, we consider the field undefined if the item is nil or empty length,
		// and we don't check for subfields. This allows us to group some settings together as defined or undefined.
		if IsFieldUndefined(branchField) {
			reflectedField := structToDefaultFrom.Field(i)
			branchField.Set(reflectedField)

			// If the field is a struct and isn't undefined, then we check each subfield recursively.
		} else if branchField.Kind() == reflect.Struct {
			RecursivelySetUndefinedFields(branchField, structToDefaultFrom.Field(i))
		} else if IsFieldPtr(branchField) {
			// If the field is a struct and isn't undefined, then we check each subfield recursively.
			if branchField.Elem().Kind() == reflect.Struct {
				RecursivelySetUndefinedFields(branchField.Elem(), structToDefaultFrom.Field(i).Elem())
			}
		}
	}
}
