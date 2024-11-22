package util

import "github.com/evergreen-ci/utility"

// GetSHA256Hash returns the SHA256 hash of the given string.
func GetSHA256Hash(s string) string {
	hasher := utility.NewSHA256Hash()
	hasher.Add(s)
	return hasher.Sum()
}
