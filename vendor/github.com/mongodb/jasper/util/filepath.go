package util

import (
	"path/filepath"
	"strings"
)

// ConsistentFilepath returns a filepath that always uses forward slashes ('/')
// rather than platform-dependent file path separators.
func ConsistentFilepath(parts ...string) string {
	return strings.Replace(filepath.Join(parts...), "\\", "/", -1)
}
