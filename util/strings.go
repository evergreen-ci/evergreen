package util

import (
	"regexp"
	"strings"
	"unicode"
)

var cleanFileRegex = regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)

// Truncate returns a string of at most the given length.
func Truncate(input string, outputLength int) string {
	if len(input) <= outputLength {
		return input
	}
	return input[:outputLength]
}

func CleanForPath(name string) string {
	return cleanFileRegex.ReplaceAllLiteralString(name, "_")
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

// see https://confluence.atlassian.com/jiracoreserver073/search-syntax-for-text-fields-861257223.html
func EscapeJQLReservedChars(in string) string {
	in = strings.Replace(in, `\`, `\\`, -1)
	in = strings.Replace(in, `+`, `\\+`, -1)
	in = strings.Replace(in, `-`, `\\-`, -1)
	in = strings.Replace(in, `&`, `\\&`, -1)
	in = strings.Replace(in, `|`, `\\|`, -1)
	in = strings.Replace(in, `!`, `\\!`, -1)
	in = strings.Replace(in, `(`, `\\(`, -1)
	in = strings.Replace(in, `)`, `\\)`, -1)
	in = strings.Replace(in, `{`, `\\{`, -1)
	in = strings.Replace(in, `}`, `\\}`, -1)
	in = strings.Replace(in, `[`, `\\[`, -1)
	in = strings.Replace(in, `]`, `\\]`, -1)
	in = strings.Replace(in, `^`, `\\^`, -1)
	in = strings.Replace(in, `~`, `\\~`, -1)
	in = strings.Replace(in, `*`, `\\*`, -1)
	in = strings.Replace(in, `?`, `\\?`, -1)
	in = strings.Replace(in, `:`, `\\:`, -1)
	return in
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

func CoalesceString(in ...string) string {
	for _, str := range in {
		if str != "" {
			return str
		}
	}
	return ""
}

func CoalesceStrings(inArray []string, inStrs ...string) string {
	return CoalesceString(CoalesceString(inArray...), CoalesceString(inStrs...))
}

// StringContainsSliceRegex determines if a string matches a regex in a slice
func StringContainsSliceRegex(slice []string, item string) bool {
	for _, sliceItem := range slice {
		matched, err := regexp.MatchString(sliceItem, item)
		if err == nil && matched {
			return true
		}
	}
	return false
}
