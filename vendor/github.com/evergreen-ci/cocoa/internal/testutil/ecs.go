package testutil

import (
	"os"
	"strings"

	"github.com/evergreen-ci/utility"
)

// NewTaskDefinitionFamily makes a new test family for a task definition with a
// common prefix, the given name, and a random string.
func NewTaskDefinitionFamily(name string) string {
	return strings.Join([]string{TaskDefinitionPrefix(), "cocoa", strings.ReplaceAll(name, "/", "-"), utility.RandomString()}, "-")
}

// TaskDefinitionPrefix returns the prefix name for task definitions from the
// environment variable.
func TaskDefinitionPrefix() string {
	return os.Getenv("AWS_ECS_TASK_DEFINITION_PREFIX")
}

// ECSClusterName returns the ECS cluster name from the environment variable.
func ECSClusterName() string {
	return os.Getenv("AWS_ECS_CLUSTER")
}

// AWSRegion returns the AWS region from the environment variable.
func AWSRegion() string {
	return os.Getenv("AWS_REGION")
}

// AWSRole returns the AWS IAM role from the environment variable.
func AWSRole() string {
	return os.Getenv("AWS_ROLE")
}

// TaskRole returns the AWS task role from the environment variable.
func TaskRole() string {
	return os.Getenv("AWS_ECS_TASK_ROLE")
}

// ExecutionRole returns the AWS execution role from the environment variable.
func ExecutionRole() string {
	return os.Getenv("AWS_ECS_EXECUTION_ROLE")
}
