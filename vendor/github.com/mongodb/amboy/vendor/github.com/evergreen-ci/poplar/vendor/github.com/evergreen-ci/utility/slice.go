package utility

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

// StringSliceSymmetricDifference returns only elements not in common between 2 slices
// (ie. inverse of the intersection)
func StringSliceSymmetricDifference(a, b []string) ([]string, []string) {
	mapA := map[string]bool{}
	mapAcopy := map[string]bool{}
	for _, elem := range a {
		mapA[elem] = true
		mapAcopy[elem] = true
	}
	inB := []string{}
	for _, elem := range b {
		if mapAcopy[elem] { // need to delete from the copy in case B has duplicates of the same value in A
			delete(mapA, elem)
		} else {
			inB = append(inB, elem)
		}
	}
	inA := []string{}
	for elem := range mapA {
		inA = append(inA, elem)
	}
	return inA, inB
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

// GetSetDifference returns the elements in A that are not in B
func GetSetDifference(a, b []string) []string {
	setB := make(map[string]struct{})
	setDifference := make(map[string]struct{})

	for _, e := range b {
		setB[e] = struct{}{}
	}
	for _, e := range a {
		if _, ok := setB[e]; !ok {
			setDifference[e] = struct{}{}
		}
	}

	d := make([]string, 0, len(setDifference))
	for k := range setDifference {
		d = append(d, k)
	}

	return d
}
