package util

import (
	"reflect"
)

func RecursivelySetUndefinedFields(structToSet, structToDefaultFrom reflect.Value) {
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
		}
	}
}
