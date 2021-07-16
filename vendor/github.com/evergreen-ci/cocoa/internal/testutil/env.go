package testutil

import (
	"fmt"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// CheckAWSEnvVars checks that the required environment variables are defined
// for testing against any AWS API.
func CheckAWSEnvVars(t *testing.T) {
	CheckEnvVars(t,
		"AWS_ACCESS_KEY",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_ROLE",
		"AWS_REGION",
	)
}

// CheckAWSEnvVarsForECS checks that the required environment variables are
// defined for testing against AWS ECS.
func CheckAWSEnvVarsForECS(t *testing.T) {
	CheckEnvVars(t,
		"AWS_ACCESS_KEY",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_ROLE",
		"AWS_REGION",
		"AWS_ECS_CLUSTER",
		"AWS_ECS_TASK_DEFINITION_PREFIX",
		"AWS_ECS_TASK_ROLE",
		"AWS_ECS_EXECUTION_ROLE",
	)
}

// CheckAWSEnvVarsForSecretsManager checks that the required environment
// variables are defined for testing against AWS Secrets Manager.
func CheckAWSEnvVarsForSecretsManager(t *testing.T) {
	CheckEnvVars(t,
		"AWS_ACCESS_KEY",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_SECRET_PREFIX",
		"AWS_ROLE",
		"AWS_REGION",
	)
}

// CheckAWSEnvVarsForECSAndSecretsManager checks that the required environment
// variables are defined for testing against both AWS ECS and Secrets Manager.
func CheckAWSEnvVarsForECSAndSecretsManager(t *testing.T) {
	CheckEnvVars(t,
		"AWS_ACCESS_KEY",
		"AWS_SECRET_ACCESS_KEY",
		"AWS_ROLE",
		"AWS_REGION",
		"AWS_ECS_CLUSTER",
		"AWS_SECRET_PREFIX",
		"AWS_ECS_TASK_DEFINITION_PREFIX",
		"AWS_ECS_TASK_ROLE",
		"AWS_ECS_EXECUTION_ROLE",
	)
}

// CheckEnvVars checks that the required environment variables are set.
func CheckEnvVars(t *testing.T, envVars ...string) {
	var missing []string

	for _, envVar := range envVars {
		if os.Getenv(envVar) == "" {
			missing = append(missing, envVar)
		}
	}

	if len(missing) > 0 {
		require.FailNow(t, fmt.Sprintf("missing required AWS environment variables: %s", missing))
	}
}
