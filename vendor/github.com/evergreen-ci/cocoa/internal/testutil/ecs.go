package testutil

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
)

// NewTaskDefinitionFamily makes a new test family for a task definition with a
// common prefix, the given name, and a random string.
func NewTaskDefinitionFamily(t *testing.T) string {
	return strings.Join([]string{taskDefinitionFamily(t), utility.RandomString()}, "-")
}

func taskDefinitionFamily(t *testing.T) string {
	return strings.Join([]string{strings.TrimSuffix(TaskDefinitionPrefix(), "-"), projectName, runtimeNamespace, strings.ReplaceAll(t.Name(), "/", "-")}, "-")
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

// ECSTaskRole returns the ECS task role from the environment variable.
func ECSTaskRole() string {
	return os.Getenv("AWS_ECS_TASK_ROLE")
}

// ECSExecutionRole returns the ECS execution role from the environment
// variable.
func ECSExecutionRole() string {
	return os.Getenv("AWS_ECS_EXECUTION_ROLE")
}

// CleanupTaskDefinitions cleans up all existing task definitions used in a
// test.
func CleanupTaskDefinitions(ctx context.Context, t *testing.T, c cocoa.ECSClient) {
	for token := cleanupTaskDefinitionsWithToken(ctx, t, c, nil); token != nil; token = cleanupTaskDefinitionsWithToken(ctx, t, c, token) {
	}
}

// cleanupTaskDefinitionsWithToken cleans up active task definitions used in
// Cocoa tests based on the results from the pagination token.
func cleanupTaskDefinitionsWithToken(ctx context.Context, t *testing.T, c cocoa.ECSClient, token *string) (nextToken *string) {
	out, err := c.ListTaskDefinitions(ctx, &ecs.ListTaskDefinitionsInput{
		Status:    aws.String(ecs.TaskDefinitionStatusActive),
		NextToken: token,
	})
	if !assert.NoError(t, err) {
		return nil
	}
	if !assert.NotZero(t, out) {
		return nil
	}

	for _, arn := range out.TaskDefinitionArns {
		if arn == nil {
			continue
		}

		taskDefARN := *arn

		// Ignore task definitions that were not generated within this test.
		name := taskDefinitionFamily(t)
		if !strings.Contains(taskDefARN, name) {
			continue
		}

		_, err := c.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{
			TaskDefinition: arn,
		})
		if assert.NoError(t, err) {
			grip.Info(message.Fields{
				"message": "cleaned up leftover task definition",
				"arn":     taskDefARN,
				"test":    t.Name(),
			})
		}
	}

	return out.NextToken
}

// CleanupTasks cleans up all tasks used in the Cocoa cluster.
func CleanupTasks(ctx context.Context, t *testing.T, c cocoa.ECSClient) {
	for token := cleanupTasksWithToken(ctx, t, c, nil); token != nil; token = cleanupTasksWithToken(ctx, t, c, token) {
	}
}

// cleanupTasksWithToken cleans up running tasks used in the Cocoa cluster based
// on the results from the pagination token.
func cleanupTasksWithToken(ctx context.Context, t *testing.T, c cocoa.ECSClient, token *string) (nextToken *string) {
	out, err := c.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster: aws.String(ECSClusterName()),
	})
	if !assert.NoError(t, err) {
		return nil
	}
	if !assert.NotZero(t, out) {
		return nil
	}
	if len(out.TaskArns) == 0 {
		return nil
	}

	describeOut, err := c.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(ECSClusterName()),
		Tasks:   out.TaskArns,
	})
	if !assert.NoError(t, err) {
		return nil
	}
	if !assert.NotZero(t, out) {
		return nil
	}

	for _, task := range describeOut.Tasks {
		if task == nil {
			continue
		}
		if task.TaskArn == nil {
			continue
		}
		if task.TaskDefinitionArn == nil {
			continue
		}

		// Ignore tasks created from task definitions not generated within this
		// test.
		taskDef := taskDefinitionFamily(t)
		if !strings.Contains(*task.TaskDefinitionArn, taskDef) {
			continue
		}

		arn := *task.TaskArn

		_, err := c.StopTask(ctx, &ecs.StopTaskInput{
			Cluster: aws.String(ECSClusterName()),
			Reason:  aws.String(fmt.Sprintf("cocoa test teardown for test '%s'", t.Name())),
			Task:    task.TaskArn,
		})
		if assert.NoError(t, err) {
			grip.Info(message.Fields{
				"message": "cleaned up leftover task",
				"arn":     arn,
				"test":    t.Name(),
			})
		}
	}

	return out.NextToken
}
