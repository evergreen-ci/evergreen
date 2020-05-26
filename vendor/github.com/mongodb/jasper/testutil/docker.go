package testutil

import (
	"os"
	"runtime"
	"strconv"
	"strings"
	"testing"
)

// DefaultDockerImage is the default image for testing Docker.
const DefaultDockerImage = "ubuntu"

// IsDockerCase returns whether or not the test case is a Docker test case.
func IsDockerCase(testName string) bool {
	return strings.Contains(strings.ToLower(testName), "docker")
}

// SkipDockerIfUnsupported skips the test if the platform is not Linux or the
// SKIP_DOCKER_TESTS environment variable is explicitly set.
func SkipDockerIfUnsupported(t *testing.T) {
	skip, _ := strconv.ParseBool(os.Getenv("SKIP_DOCKER_TESTS"))
	if runtime.GOOS != "linux" || skip {
		t.Skip("skipping Docker tests on non-Linux platforms since testing CI infrastructure only supports Docker on Linux currently")
	}
}
