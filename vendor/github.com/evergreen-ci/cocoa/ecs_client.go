package cocoa

import (
	"context"

	"github.com/aws/aws-sdk-go/service/ecs"
)

// ECSClient provides a common interface to interact with a client backed by
// AWS ECS. Implementations must handle retrying and backoff.
type ECSClient interface {
	// RegisterTaskDefinition registers the definition for a new task with ECS.
	RegisterTaskDefinition(context.Context, *ecs.RegisterTaskDefinitionInput) (*ecs.RegisterTaskDefinitionOutput, error)
	// DescribeTaskDefinitions gets information about the configuration and
	// status of a task definition.
	DescribeTaskDefinition(ctx context.Context, in *ecs.DescribeTaskDefinitionInput) (*ecs.DescribeTaskDefinitionOutput, error)
	// ListTaskDefinitions lists all ECS task definitions matching the input.
	ListTaskDefinitions(ctx context.Context, in *ecs.ListTaskDefinitionsInput) (*ecs.ListTaskDefinitionsOutput, error)
	// DeregisterTaskDefinition deregisters an existing ECS task definition.
	DeregisterTaskDefinition(ctx context.Context, in *ecs.DeregisterTaskDefinitionInput) (*ecs.DeregisterTaskDefinitionOutput, error)
	// RunTask runs a registered task.
	RunTask(ctx context.Context, in *ecs.RunTaskInput) (*ecs.RunTaskOutput, error)
	// DescribeTasks gets information about the configuration and status of
	// tasks.
	DescribeTasks(ctx context.Context, in *ecs.DescribeTasksInput) (*ecs.DescribeTasksOutput, error)
	// ListTasks lists all ECS tasks matching the input.
	ListTasks(ctx context.Context, in *ecs.ListTasksInput) (*ecs.ListTasksOutput, error)
	// StopTask stops a running task.
	StopTask(ctx context.Context, in *ecs.StopTaskInput) (*ecs.StopTaskOutput, error)
	// Close closes the client and cleans up its resources. Implementations
	// should ensure that this is idempotent.
	Close(ctx context.Context) error
}
