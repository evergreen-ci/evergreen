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

func StringSliceContains(slice []string, item string) bool {
	if slice == nil {
		return false
	}

	for idx := range slice {
		if slice[idx] == item {
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

// UniqueStrings takes a slice of strings and returns a new slice with duplicates removed.
// Order is preserved.
func UniqueStrings(slice []string) []string {
	seen := map[string]bool{}
	out := []string{}
	for _, s := range slice {
		if seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, s)
	}
	return out
}
