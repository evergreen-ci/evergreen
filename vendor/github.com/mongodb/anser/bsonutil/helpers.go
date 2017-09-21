package bsonutil

import "strings"

// GetDottedKeyName assembles a key name, for use in queries, of a
// nested field name.
func GetDottedKeyName(fieldNames ...string) string {
	return strings.Join(fieldNames, ".")
}
