package util

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGetEC2InstanceID(t *testing.T) {
	testutil.SkipEC2TestOnNonEC2Instance(t)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	instanceID, err := GetEC2InstanceID(ctx)
	require.NoError(t, err)
	assert.NotZero(t, instanceID)
	assert.True(t, cloud.IsEC2InstanceID(instanceID))
}
