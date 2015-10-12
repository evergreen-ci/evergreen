package util

import (
	"strings"
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
