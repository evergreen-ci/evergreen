package util

import (
	"context"
	"os"
	"strconv"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEC2InstanceID(t *testing.T) {
	skipEC2TestOnNonEC2Instance(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	instanceID, err := GetEC2InstanceID(ctx)
	require.NoError(t, err)
	assert.NotZero(t, instanceID)
	assert.True(t, cloud.IsEC2InstanceID(instanceID))
}

// skipEC2TestOnNonEC2Instance skips a test that can only be run on an EC2
// instance if the environment is not an EC2 instance.
func skipEC2TestOnNonEC2Instance(t *testing.T) {
	if runEC2Test, _ := strconv.ParseBool(os.Getenv("RUN_EC2_SPECIFIC_TESTS")); !runEC2Test {
		t.Skip("RUN_EC2_SPECIFIC_TESTS is unset, skipping test that can only run on an EC2 instance")
	}
}
