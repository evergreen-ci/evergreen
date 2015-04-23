package angier

import (
	"fmt"
	"reflect"
)

const (
	ANGIER_TAG_PREFIX = "angier"
)

// Takes in two pointers to structs.  For any fields with the same name and type
// in the two structs, the values of the exported fields in the src struct are
// copied over to the corresponding field in the dest struct.
func TransferByFieldNames(src interface{}, dest interface{}) error {

	// first, check that both are pointers
	if !bothPointers(src, dest) {
		return fmt.Errorf("both the src and dest must be pointers")
	}

	// get the values pointed to
	srcVal := reflect.Indirect(reflect.ValueOf(src))
	destVal := reflect.Indirect(reflect.ValueOf(dest))

	// validate that both are structs
	if !bothStructs(srcVal, destVal) {
		return fmt.Errorf("both the src and dest must be pointers to structs")
	}

	// iterate over the fields of the src struct
	fieldsInSrc := srcVal.NumField()
	for i := 0; i < fieldsInSrc; i++ {
		fieldInSrc := srcVal.Type().Field(i)
		fieldName := fieldInSrc.Name

		// check to see if the dest struct has a field of the same value
		fieldInDest, exists := destVal.Type().FieldByName(fieldName)
		if !exists || fieldInDest.Type.Kind() != fieldInSrc.Type.Kind() {
			continue
		}

		// convert to reflect.Value
		srcFieldVal := srcVal.FieldByName(fieldName)
		destFieldVal := destVal.FieldByName(fieldName)

		// make sure the field is exported
		if !destFieldVal.CanSet() {
			continue
		}

		// set the field
		destFieldVal.Set(srcFieldVal)

	}

	return nil

}

// Taking in two pointers to structs, transfers field values from the source
// to the destination, using struct tags to determine the matching fields
func TransferByTags(src interface{}, dest interface{}) error {

	// first, check that both are pointers
	if !bothPointers(src, dest) {
		return fmt.Errorf("both the src and dest must be pointers")
	}

	// get the values pointed to
	srcVal := reflect.Indirect(reflect.ValueOf(src))
	destVal := reflect.Indirect(reflect.ValueOf(dest))

	// validate that both are structs
	if !bothStructs(srcVal, destVal) {
		return fmt.Errorf("both the src and dest must be pointers to structs")
	}

	// iterate over the fields of the src struct
	fieldsInSrc := srcVal.NumField()
	fieldsInDest := destVal.NumField()
	for i := 0; i < fieldsInSrc; i++ {
		fieldInSrc := srcVal.Type().Field(i)
		srcFieldTag := fieldInSrc.Tag.Get(ANGIER_TAG_PREFIX)

		// doesn't have a tag, skip it
		if srcFieldTag == "" {
			continue
		}

		// iterate over the dest struct, finding the field with the matching
		// tag
		for j := 0; j < fieldsInDest; j++ {

			fieldInDest := destVal.Type().Field(j)

			// tag doesn't match
			if fieldInDest.Tag.Get(ANGIER_TAG_PREFIX) != srcFieldTag {
				continue
			}

			srcFieldVal := srcVal.FieldByName(fieldInSrc.Name)
			destFieldVal := destVal.FieldByName(fieldInDest.Name)

			// type doesn't match
			if fieldInDest.Type.Kind() != fieldInSrc.Type.Kind() {
				continue
			}

			// make sure the field is exported
			if !destFieldVal.CanSet() {
				continue
			}

			// set the field
			destFieldVal.Set(srcFieldVal)

		}

	}

	return nil

}

// helper functions

func bothPointers(src interface{}, dest interface{}) bool {
	return reflect.ValueOf(src).Type().Kind() == reflect.Ptr &&
		reflect.ValueOf(dest).Type().Kind() == reflect.Ptr
}

func bothStructs(srcVal reflect.Value, destVal reflect.Value) bool {
	return srcVal.Type().Kind() == reflect.Struct &&
		destVal.Type().Kind() == reflect.Struct
}
