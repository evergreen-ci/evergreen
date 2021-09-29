package ecs

import (
	"context"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/awsutil"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const defaultTestTimeout = time.Minute

func TestECSClient(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSClient)(nil), &BasicECSClient{})

	testutil.CheckAWSEnvVarsForECS(t)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	hc := utility.GetHTTPClient()
	defer utility.PutHTTPClient(hc)

	c, err := NewBasicECSClient(awsutil.ClientOptions{
		Creds:  credentials.NewEnvCredentials(),
		Region: aws.String(testutil.AWSRegion()),
		Role:   aws.String(testutil.AWSRole()),
		RetryOpts: &utility.RetryOptions{
			MaxAttempts: 5,
		},
		HTTPClient: hc,
	})
	require.NoError(t, err)
	require.NotNil(t, c)

	defer func() {
		testutil.CleanupTaskDefinitions(ctx, t, c)
		testutil.CleanupTasks(ctx, t, c)

		assert.NoError(t, c.Close(ctx))
	}()

	for tName, tCase := range testcase.ECSClientTaskDefinitionTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			defer c.Close(tctx)

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
	defer func() {
		_, err := c.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{
			TaskDefinition: registerOut.TaskDefinition.TaskDefinitionArn,
		})
		assert.NoError(t, err)
	}()

	for tName, tCase := range testcase.ECSClientRegisteredTaskDefinitionTests(*registerIn, *registerOut) {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, defaultTestTimeout)
			defer tcancel()

			tCase(tctx, t, c)
		})
	}
}
