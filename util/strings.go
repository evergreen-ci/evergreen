package util

import (
	"strings"
	"time"
)

// RemoveSuffix returns s with 'suffix' removed from end of string if present
func RemoveSuffix(s, suffix string) string {
	if strings.HasSuffix(s, suffix) {
		return s[:len(s)-len(suffix)]
	}
	return s
}

func Truncate(input string, outputLength int) string {
	if len(input) <= outputLength {
		return input
	}
	return input[:outputLength]
}

func DateAsString(when time.Time, layout string) string {
	return when.Format(layout)
}
