package util

import (
	"strings"
	"unicode"
)

// RemoveSuffix returns s with 'suffix' removed from end of string if present
func RemoveSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}

// Truncate returns a string of at most the given length.
func Truncate(input string, outputLength int) string {
	if len(input) <= outputLength {
		return input
	}
	return input[:outputLength]
}

// CleanName returns a name with spaces and dashes replaced with safe underscores
func CleanName(name string) string {
	name = strings.Replace(name, "-", "_", -1)
	name = strings.Replace(name, " ", "_", -1)
	return name
}

// IndexWhiteSpace returns the first index of white space in the given string.
// Returns -1 if no white space exists.
func IndexWhiteSpace(s string) int {
	for i, r := range s {
		if unicode.IsSpace(r) {
			return i
		}
	}
	return -1
}
