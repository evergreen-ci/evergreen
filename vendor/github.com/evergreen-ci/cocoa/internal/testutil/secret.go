package testutil

import (
	"fmt"
	"os"
	"path"

	"github.com/evergreen-ci/utility"
)

const projectName = "cocoa"

// NewSecretName creates a new test secret name with a common prefix, the given
// name, and a random string.
func NewSecretName(name string) string {
	return fmt.Sprint(path.Join(SecretPrefix(), projectName, name, utility.RandomString()))
}

// SecretPrefix returns the prefix name for secrets from the environment
// variable.
func SecretPrefix() string {
	return os.Getenv("AWS_SECRET_PREFIX")
}
