package util

import "strings"

// StringSliceContains determines if a string is in a slice
func StringSliceContains(slice []string, item string) bool {
	if len(slice) == 0 {
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

// SplitCommas returns the slice of strings after splitting each string by
// commas.
func SplitCommas(originals []string) []string {
	splitted := []string{}
	for _, original := range originals {
		splitted = append(splitted, strings.Split(original, ",")...)
	}
	return splitted
}
