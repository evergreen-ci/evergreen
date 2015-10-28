package util

import (
	"fmt"
	"reflect"
)

// SliceContains returns true if elt is in slice, panics if slice is not of Kind reflect.Slice
func SliceContains(slice, elt interface{}) bool {
	if slice == nil {
		return false
	}
	v := reflect.ValueOf(slice)
	if v.Kind() != reflect.Slice {
		panic(fmt.Sprintf("Cannot call SliceContains on a non-slice %#v of kind %#v", slice, v.Kind().String()))
	}
	for i := 0; i < v.Len(); i++ {
		if reflect.DeepEqual(v.Index(i).Interface(), elt) {
			return true
		}
	}
	return false
}

// StringSliceIntersection returns the intersecting elements of slices a and b.
func StringSliceIntersection(a, b []string) []string {
	inA := map[string]bool{}
	out := []string{}
	for _, elem := range a {
		inA[elem] = true
	}
	for _, elem := range b {
		if inA[elem] {
			out = append(out, elem)
		}
	}
	return out
}
