package util

import (
	"regexp"
	"strings"
	"unicode"
)

var cleanFileRegex = regexp.MustCompile(`[^a-zA-Z0-9_\-\.]`)

func CleanForPath(name string) string {
	return cleanFileRegex.ReplaceAllLiteralString(name, "_")
}

// CleanName returns a name with spaces and dashes replaced with safe underscores
func CleanName(name string) string {
	name = strings.Replace(name, "-", "_", -1)
	name = strings.Replace(name, " ", "_", -1)
	name = strings.Replace(name, "/", "_", -1)
	return name
}

// HasAllowedImageAsPrefix returns true if the given string has one of the allowed image prefixes
func HasAllowedImageAsPrefix(str string, imageList []string) bool {
	for _, imagePrefix := range imageList {
		if strings.HasPrefix(str, imagePrefix) {
			return true
		}
	}
	return false
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
	// kim: NOTE: this escapes the characters, and the escaped characters are
	// then put into a URL query parameter. It's possible that maybe this causes
	// issues if the URL does not query escape the JQL query.
	in = strings.Replace(in, `\`, `\\`, -1)
	in = strings.Replace(in, `+`, `\\+`, -1)
	// kim: NOTE: test names that are not producing suggestions have dashes in
	// them. A task name search that succeeded only has underscores, which is
	// not a special character.
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
