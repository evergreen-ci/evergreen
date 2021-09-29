package testutil

import (
	"os"

	"github.com/evergreen-ci/utility"
)

// runtimeNamespace is a random string generated during testing runtime that
// acts as a namespace for this particular runtime's tests. It is used to
// namespace AWS resources (e.g. secrets, task definitions). This avoids an
// issue where the tests can be running concurrently on different machines and
// may interfere with each other due to the way AWS resources are cleaned up at
// the end of tests. For example, if one machine is running the ECS tests and at
// the same time, another machine is cleaning up the resources for the same ECS
// tests, they should not affect one another.
var runtimeNamespace = utility.RandomString()

// AWSRegion returns the AWS region from the environment variable.
func AWSRegion() string {
	return os.Getenv("AWS_REGION")
}

// AWSRole returns the AWS IAM role from the environment variable.
func AWSRole() string {
	return os.Getenv("AWS_ROLE")
}
