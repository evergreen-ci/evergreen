package mock

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/cocoa"
	"github.com/evergreen-ci/cocoa/internal/testcase"
	"github.com/evergreen-ci/cocoa/internal/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestECSClient(t *testing.T) {
	assert.Implements(t, (*cocoa.ECSClient)(nil), &ECSClient{})

	GlobalECSService.Clusters[testutil.ECSClusterName()] = ECSCluster{}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	c := &ECSClient{}
	defer func() {
		assert.NoError(t, c.Close(ctx))
	}()

	for tName, tCase := range testcase.ECSClientTaskDefinitionTests() {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, time.Second)
			defer tcancel()

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
		Family: aws.String(testutil.NewTaskDefinitionFamily(t.Name())),
	}

	registerOut, err := c.RegisterTaskDefinition(ctx, registerIn)
	require.NoError(t, err)
	require.NotZero(t, registerOut)
	require.NotZero(t, registerOut.TaskDefinition)

	defer func() {
		taskDefs := cleanupTaskDefinitions(ctx, t, c)
		grip.InfoWhen(len(taskDefs) > 0, message.Fields{
			"message":          "cleaned up leftover task definitions",
			"task_definitions": taskDefs,
			"test":             t.Name(),
		})
		tasks := cleanupTasks(ctx, t, c, taskDefs)
		grip.InfoWhen(len(tasks) > 0, message.Fields{
			"message": "cleaned up leftover running tasks",
			"tasks":   tasks,
			"test":    t.Name(),
		})
		require.NoError(t, c.Close(ctx))
	}()

	for tName, tCase := range testcase.ECSClientRegisteredTaskDefinitionTests(*registerIn, *registerOut) {
		t.Run(tName, func(t *testing.T) {
			tctx, tcancel := context.WithTimeout(ctx, time.Second)
			defer tcancel()

			tCase(tctx, t, c)
		})
	}
}

// cleanupTaskDefinitions cleans up all existing task definitions for testing
// teardown.
func cleanupTaskDefinitions(ctx context.Context, t *testing.T, c cocoa.ECSClient) []string {
	out, err := c.ListTaskDefinitions(ctx, &ecs.ListTaskDefinitionsInput{
		Status: aws.String(ecs.TaskDefinitionStatusActive),
	})
	require.NoError(t, err)
	require.NotZero(t, out)

	var taskDefs []string

	for _, arn := range out.TaskDefinitionArns {
		if arn == nil {
			continue
		}
		taskDefArn := *arn
		// Ignore task definitions that were not generated with testutil.NewTaskDefinitionFamily.
		name := strings.Join([]string{testutil.TaskDefinitionPrefix(), "cocoa", t.Name()}, "-")
		if !strings.Contains(taskDefArn, name) {
			continue
		}
		taskDefs = append(taskDefs, *arn)
		_, err := c.DeregisterTaskDefinition(ctx, &ecs.DeregisterTaskDefinitionInput{
			TaskDefinition: arn,
		})
		assert.NoError(t, err)
	}

	return taskDefs
}

// cleanupTasks cleans up all tasks for testing teardown. It only cleans up
// tasks if they are running and are created from one of the given task
// definitions.
func cleanupTasks(ctx context.Context, t *testing.T, c cocoa.ECSClient, taskDefs []string) []string {
	out, err := c.ListTasks(ctx, &ecs.ListTasksInput{
		Cluster: aws.String(testutil.ECSClusterName()),
	})
	require.NoError(t, err)
	require.NotZero(t, out)

	if len(out.TaskArns) == 0 {
		return nil
	}

	describeOut, err := c.DescribeTasks(ctx, &ecs.DescribeTasksInput{
		Cluster: aws.String(testutil.ECSClusterName()),
		Tasks:   out.TaskArns,
	})
	require.NoError(t, err)

	var tasks []string
	for _, task := range describeOut.Tasks {
		if task == nil {
			continue
		}

		if task.TaskDefinitionArn == nil {
			grip.Notice(message.Fields{
				"message": "cannot check task for test teardown because it does not have a task definition ARN",
				"context": "cocoa test teardown",
				"task":    *task,
				"test":    t.Name(),
			})
			continue
		}
		// Ignore tasks started by task definitions outside of the cocoa testing
		// environment.
		if taskDef := *task.TaskDefinitionArn; !utility.StringSliceContains(taskDefs, taskDef) {
			continue
		}

		if task.LastStatus == nil {
			grip.Notice(message.Fields{
				"message": "cannot check task for test teardown because it does not have a status",
				"context": "cocoa test teardown",
				"task":    *task,
				"test":    t.Name(),
			})
			continue
		}

		status := *task.LastStatus
		switch status {
		case "PROVISIONING", "PENDING", "ACTIVATING":
			grip.Notice(message.Fields{
				"message": "cannot stop the task because it is in an unstoppable state",
				"context": "cocoa test teardown",
				"status":  status,
				"task":    *task,
				"test":    t.Name(),
			})
			continue
		case "RUNNING":
			if task.DesiredStatus != nil && *task.DesiredStatus == "STOPPED" {
				continue
			}
			if task.TaskArn == nil {
				grip.Notice(message.Fields{
					"message": "cannot stop the task because it is missing a task ARN",
					"context": "cocoa test teardown",
					"status":  status,
					"task":    *task,
					"test":    t.Name(),
				})
				continue
			}
			_, err := c.StopTask(ctx, &ecs.StopTaskInput{
				Cluster: aws.String(testutil.ECSClusterName()),
				Reason:  aws.String(fmt.Sprintf("cocoa test teardown for test '%s'", t.Name())),
				Task:    task.TaskArn,
			})
			if assert.NoError(t, err) {
				tasks = append(tasks, *task.TaskArn)
			}
		case "DEACTIVATING", "STOPPING", "DEPROVISIONING", "STOPPED":
			continue
		default:
			assert.Fail(t, "unrecognized task status '%s'", status)
		}

	}

	return tasks
}
