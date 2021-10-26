package mock

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// defaultTestTimeout is the default test timeout for mock tests.
const defaultTestTimeout = time.Second

func TestECSClient(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSClient)(nil), &ECSClient{})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &ECSClient{}
	defer func() {
		cleanupECSAndSecretsManagerCache()

		assert.NoError(t, c.Close(ctx))
	}()

	for tName, tCase := range testcase.ECSClientTaskDefinitionTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			cleanupECSAndSecretsManagerCache()

			tCase(tctx, t, c)
		})
	}

	registerIn := &ecs.RegisterTaskDefinitionInput{
		ContainerDefinitions: []*ecs.ContainerDefinition{
			{
				Command: []*string{aws.String("echo"), aws.String("foo")},
				Image:   aws.String("busybox"),
				Name:    aws.String("print_foo"),
			},
		},
		Cpu:    aws.String("128"),
		Memory: aws.String("4"),
		Family: aws.String(testutil.NewTaskDefinitionFamily(t)),
	}

	registerOut, err := c.RegisterTaskDefinition(ctx, registerIn)
	require.NoError(t, err)
	require.NotZero(t, registerOut)
	require.NotZero(t, registerOut.TaskDefinition)

	for tName, tCase := range testcase.ECSClientRegisteredTaskDefinitionTests(*registerIn, *registerOut) {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			cleanupECSAndSecretsManagerCache()

			tCase(tctx, t, c)
		})
	}
}
