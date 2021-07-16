package mock

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/pkg/errors"

	"github.com/aws/aws-sdk-go/aws/arn"
	"github.com/aws/aws-sdk-go/service/ecs"
	"github.com/evergreen-ci/utility"
)

// ECSTaskDefinition represents a mock ECS task definition in the global ECS service.
type ECSTaskDefinition struct {
	ARN           *string
	Family        *string
	Revision      *int64
	ContainerDefs []ECSContainerDefinition
	MemoryMB      *string
	CPU           *string
	TaskRole      *string
	Tags          map[string]string
	Status        *string
	Registered    *time.Time
}

func newECSTaskDefinition(def *ecs.RegisterTaskDefinitionInput, rev int) ECSTaskDefinition {
	id := arn.ARN{
		Partition: "aws",
		Service:   "ecs",
		Resource:  fmt.Sprintf("task-definition:%s/%s", utility.FromStringPtr(def.Family), strconv.Itoa(rev)),
	}

	taskDef := ECSTaskDefinition{
		ARN:        utility.ToStringPtr(id.String()),
		Family:     def.Family,
		Revision:   utility.ToInt64Ptr(int64(rev)),
		CPU:        def.Cpu,
		MemoryMB:   def.Memory,
		TaskRole:   def.TaskRoleArn,
		Status:     utility.ToStringPtr(ecs.TaskDefinitionStatusActive),
		Registered: utility.ToTimePtr(time.Now()),
	}

	taskDef.Tags = newTags(def.Tags)

	for _, containerDef := range def.ContainerDefinitions {
		if containerDef == nil {
			continue
		}
		taskDef.ContainerDefs = append(taskDef.ContainerDefs, newECSContainerDefinition(containerDef))
	}

	return taskDef
}

func (d *ECSTaskDefinition) export() *ecs.TaskDefinition {
	var containerDefs []*ecs.ContainerDefinition
	for _, def := range d.ContainerDefs {
		containerDefs = append(containerDefs, def.export())
	}

	return &ecs.TaskDefinition{
		TaskDefinitionArn:    d.ARN,
		Family:               d.Family,
		Revision:             d.Revision,
		Cpu:                  d.CPU,
		Memory:               d.MemoryMB,
		TaskRoleArn:          d.TaskRole,
		Status:               d.Status,
		ContainerDefinitions: containerDefs,
		RegisteredAt:         d.Registered,
	}
}

// ECSContainerDefinition represents a mock ECS container definition in a mock
// ECS task definition.
type ECSContainerDefinition struct {
	Name     *string
	Image    *string
	Command  []string
	MemoryMB *int64
	CPU      *int64
	EnvVars  map[string]string
	Secrets  map[string]string
}

func newECSContainerDefinition(def *ecs.ContainerDefinition) ECSContainerDefinition {
	return ECSContainerDefinition{
		Name:     def.Name,
		Image:    def.Image,
		Command:  utility.FromStringPtrSlice(def.Command),
		MemoryMB: def.Memory,
		CPU:      def.Cpu,
		EnvVars:  newEnvVars(def.Environment),
		Secrets:  newSecrets(def.Secrets),
	}
}

func (d *ECSContainerDefinition) export() *ecs.ContainerDefinition {
	return &ecs.ContainerDefinition{
		Name:        d.Name,
		Image:       d.Image,
		Command:     utility.ToStringPtrSlice(d.Command),
		Memory:      d.MemoryMB,
		Cpu:         d.CPU,
		Environment: exportEnvVars(d.EnvVars),
		Secrets:     exportSecrets(d.Secrets),
	}
}

// ECSCluster represents a mock ECS cluster running tasks in the global ECS
// service.
type ECSCluster map[string]ECSTask

// ECSTask represents a mock running ECS task within a cluster.
type ECSTask struct {
	ARN         *string
	TaskDef     ECSTaskDefinition
	Cluster     *string
	Containers  []ECSContainer
	ExecEnabled *bool
	Status      *string
	GoalStatus  *string
	Created     *time.Time
	StopCode    *string
	StopReason  *string
	Stopped     *time.Time
	Tags        map[string]string
}

func newECSTask(in *ecs.RunTaskInput, taskDef ECSTaskDefinition) ECSTask {
	id := arn.ARN{
		Partition: "aws",
		Service:   "ecs",
		Resource:  fmt.Sprintf("task:%s/%s", utility.FromStringPtr(taskDef.Family), strconv.Itoa(int(utility.FromInt64Ptr(taskDef.Revision)))),
	}

	t := ECSTask{
		ARN:         utility.ToStringPtr(id.String()),
		Cluster:     in.Cluster,
		ExecEnabled: in.EnableExecuteCommand,
		Status:      utility.ToStringPtr(ecs.DesiredStatusRunning),
		GoalStatus:  utility.ToStringPtr(ecs.DesiredStatusRunning),
		Created:     utility.ToTimePtr(time.Now()),
		TaskDef:     taskDef,
		Tags:        newTags(in.Tags),
	}

	for _, containerDef := range taskDef.ContainerDefs {
		t.Containers = append(t.Containers, newECSContainer(containerDef, t))
	}

	return t
}

func (t *ECSTask) export() *ecs.Task {
	exported := ecs.Task{
		TaskArn:              t.ARN,
		ClusterArn:           t.Cluster,
		EnableExecuteCommand: t.ExecEnabled,
		Tags:                 exportTags(t.Tags),
		TaskDefinitionArn:    t.TaskDef.ARN,
		Cpu:                  t.TaskDef.CPU,
		Memory:               t.TaskDef.MemoryMB,
		LastStatus:           t.Status,
		DesiredStatus:        t.GoalStatus,
		CreatedAt:            t.Created,
		StopCode:             t.StopCode,
		StoppedReason:        t.StopReason,
		StoppedAt:            t.Stopped,
	}

	for _, container := range t.Containers {
		exported.Containers = append(exported.Containers, container.export())
	}

	return &exported
}

// ECSContainer represents a mock running ECS container within a task.
type ECSContainer struct {
	ARN      *string
	TaskARN  *string
	Name     *string
	Image    *string
	CPU      *int64
	MemoryMB *int64
}

func newECSContainer(def ECSContainerDefinition, task ECSTask) ECSContainer {
	name := utility.FromStringPtr(def.Name)
	if name == "" {
		name = utility.RandomString()
	}
	id := arn.ARN{
		Partition: "aws",
		Service:   "ecs",
		Resource:  fmt.Sprintf("container-definition:%s-%s/%s", utility.FromStringPtr(task.TaskDef.Family), name, strconv.Itoa(int(utility.FromInt64Ptr(task.TaskDef.Revision)))),
	}

	return ECSContainer{
		ARN:      utility.ToStringPtr(id.String()),
		TaskARN:  task.ARN,
		Name:     def.Name,
		Image:    def.Image,
		CPU:      def.CPU,
		MemoryMB: def.MemoryMB,
	}
}

func (c *ECSContainer) export() *ecs.Container {
	exported := &ecs.Container{
		ContainerArn: c.ARN,
		TaskArn:      c.TaskARN,
		Name:         c.Name,
		Image:        c.Image,
	}

	if c.CPU != nil {
		exported.Cpu = utility.ToStringPtr(strconv.Itoa(int(utility.FromInt64Ptr(c.CPU))))
	}
	if c.MemoryMB != nil {
		exported.Memory = utility.ToStringPtr(strconv.Itoa(int(utility.FromInt64Ptr(c.MemoryMB))))
	}

	return exported
}

func newTags(tags []*ecs.Tag) map[string]string {
	converted := map[string]string{}
	for _, t := range tags {
		if t == nil {
			continue
		}
		converted[utility.FromStringPtr(t.Key)] = utility.FromStringPtr(t.Value)
	}
	return converted
}

func exportTags(tags map[string]string) []*ecs.Tag {
	var exported []*ecs.Tag
	for k, v := range tags {
		exported = append(exported, &ecs.Tag{
			Key:   utility.ToStringPtr(k),
			Value: utility.ToStringPtr(v),
		})
	}
	return exported
}

func newEnvVars(envVars []*ecs.KeyValuePair) map[string]string {
	converted := map[string]string{}
	for _, envVar := range envVars {
		if envVar == nil {
			continue
		}
		converted[utility.FromStringPtr(envVar.Name)] = utility.FromStringPtr(envVar.Value)
	}
	return converted
}

func exportEnvVars(envVars map[string]string) []*ecs.KeyValuePair {
	var exported []*ecs.KeyValuePair
	for k, v := range envVars {
		exported = append(exported, &ecs.KeyValuePair{
			Name:  utility.ToStringPtr(k),
			Value: utility.ToStringPtr(v),
		})
	}
	return exported
}

func newSecrets(secrets []*ecs.Secret) map[string]string {
	converted := map[string]string{}
	for _, secret := range secrets {
		if secret == nil {
			continue
		}
		converted[utility.FromStringPtr(secret.Name)] = utility.FromStringPtr(secret.ValueFrom)
	}
	return converted
}

func exportSecrets(secrets map[string]string) []*ecs.Secret {
	var exported []*ecs.Secret
	for k, v := range secrets {
		exported = append(exported, &ecs.Secret{
			Name:      utility.ToStringPtr(k),
			ValueFrom: utility.ToStringPtr(v),
		})
	}
	return exported
}

// ECSService is a global implementation of ECS that provides a simplified
// in-memory implementation of the service that only stores metadata and does
// not orchestrate real containers or container instances. This can be used
// indirectly with the ECSClient to access or modify ECS resources, or used
// directly.
type ECSService struct {
	Clusters map[string]ECSCluster
	TaskDefs map[string][]ECSTaskDefinition
}

// GlobalECSService represents the global fake ECS service state.
var GlobalECSService ECSService

func init() {
	GlobalECSService = ECSService{
		Clusters: map[string]ECSCluster{},
		TaskDefs: map[string][]ECSTaskDefinition{},
	}
}

// ECSClient provides a mock implementation of a cocoa.ECSClient. This makes
// it possible to introspect on inputs to the client and control the client's
// output. It provides some default implementations where possible.
type ECSClient struct {
	RegisterTaskDefinitionInput  *ecs.RegisterTaskDefinitionInput
	RegisterTaskDefinitionOutput *ecs.RegisterTaskDefinitionOutput

	DeregisterTaskDefinitionInput  *ecs.DeregisterTaskDefinitionInput
	DeregisterTaskDefinitionOutput *ecs.DeregisterTaskDefinitionOutput

	ListTaskDefinitionsInput  *ecs.ListTaskDefinitionsInput
	ListTaskDefinitionsOutput *ecs.ListTaskDefinitionsOutput

	RunTaskInput  *ecs.RunTaskInput
	RunTaskOutput *ecs.RunTaskOutput

	DescribeTasksInput  *ecs.DescribeTasksInput
	DescribeTasksOutput *ecs.DescribeTasksOutput

	ListTasksInput  *ecs.ListTasksInput
	ListTasksOutput *ecs.ListTasksOutput

	StopTaskInput  *ecs.StopTaskInput
	StopTaskOutput *ecs.StopTaskOutput

	CloseError error
}

// RegisterTaskDefinition saves the input and returns a new mock task
// definition. The mock output can be customized. By default, it will create a
// cached task definition based on the input.
func (c *ECSClient) RegisterTaskDefinition(ctx context.Context, in *ecs.RegisterTaskDefinitionInput) (*ecs.RegisterTaskDefinitionOutput, error) {
	c.RegisterTaskDefinitionInput = in

	if c.RegisterTaskDefinitionOutput != nil {
		return c.RegisterTaskDefinitionOutput, nil
	}

	if in.Family == nil {
		return nil, errors.New("missing family")
	}

	revisions := GlobalECSService.TaskDefs[utility.FromStringPtr(in.Family)]
	rev := len(revisions) + 1

	taskDef := newECSTaskDefinition(in, rev)

	GlobalECSService.TaskDefs[utility.FromStringPtr(in.Family)] = append(revisions, taskDef)

	return &ecs.RegisterTaskDefinitionOutput{
		TaskDefinition: taskDef.export(),
		Tags:           in.Tags,
	}, nil
}

// DeregisterTaskDefinition saves the input and deletes an existing mock task
// definition. The mock output can be customized. By default, it will delete a
// cached task definition if it exists.
func (c *ECSClient) DeregisterTaskDefinition(ctx context.Context, in *ecs.DeregisterTaskDefinitionInput) (*ecs.DeregisterTaskDefinitionOutput, error) {
	c.DeregisterTaskDefinitionInput = in

	if c.DeregisterTaskDefinitionOutput != nil {
		return c.DeregisterTaskDefinitionOutput, nil
	}

	if in.TaskDefinition == nil {
		return nil, errors.New("missing task definition")
	}

	id := utility.FromStringPtr(in.TaskDefinition)

	if arn.IsARN(id) {
		family, revNum, found := taskDefIndexFromARN(id)
		if !found {
			return nil, errors.New("task definition not found")
		}
		taskDef := GlobalECSService.TaskDefs[family][revNum-1]
		taskDef.Status = utility.ToStringPtr(ecs.TaskDefinitionStatusInactive)
		GlobalECSService.TaskDefs[family][revNum-1] = taskDef

		return &ecs.DeregisterTaskDefinitionOutput{
			TaskDefinition: taskDef.export(),
		}, nil
	}

	family, revNum, err := parseFamilyAndRevision(id)
	if err != nil {
		return nil, errors.Wrap(err, "invalid task definition")
	}

	revisions, ok := GlobalECSService.TaskDefs[family]
	if !ok {
		return nil, errors.New("family not found")
	}
	if len(revisions) < revNum {
		return nil, errors.New("revision not found")
	}

	taskDef := revisions[revNum-1]
	taskDef.Status = utility.ToStringPtr(ecs.TaskDefinitionStatusInactive)
	GlobalECSService.TaskDefs[family][revNum-1] = taskDef

	return &ecs.DeregisterTaskDefinitionOutput{
		TaskDefinition: taskDef.export(),
	}, nil
}

func parseFamilyAndRevision(taskDef string) (family string, revNum int, err error) {
	partition := strings.LastIndex(taskDef, ":")
	if partition == -1 {
		return "", -1, errors.New("task definition is not in family:revision format")
	}

	family = taskDef[:partition]

	revNum, err = strconv.Atoi(taskDef[partition+1:])
	if err != nil {
		return "", -1, errors.Wrap(err, "parsing revision")
	}

	return family, revNum, nil
}

func taskDefIndexFromARN(arn string) (family string, revNum int, found bool) {
	for family, revisions := range GlobalECSService.TaskDefs {
		for revIdx, def := range revisions {
			if utility.FromStringPtr(def.ARN) == arn {
				return family, revIdx + 1, true
			}
		}
	}
	return "", -1, false
}

// ListTaskDefinitions saves the input and lists all matching task definitions.
// The mock output can be customized. By default, it will list all cached task
// definitions that match the input filters.
func (c *ECSClient) ListTaskDefinitions(ctx context.Context, in *ecs.ListTaskDefinitionsInput) (*ecs.ListTaskDefinitionsOutput, error) {
	c.ListTaskDefinitionsInput = in

	if c.ListTaskDefinitionsOutput != nil {
		return c.ListTaskDefinitionsOutput, nil
	}

	var arns []*string
	for _, revisions := range GlobalECSService.TaskDefs {
		for _, def := range revisions {
			if in.FamilyPrefix != nil && utility.FromStringPtr(def.Family) != *in.FamilyPrefix {
				continue
			}
			if in.Status != nil && utility.FromStringPtr(def.Status) != *in.Status {
				continue
			}

			arns = append(arns, def.ARN)
		}
	}

	return &ecs.ListTaskDefinitionsOutput{
		TaskDefinitionArns: arns,
	}, nil
}

// RunTask saves the input options and returns the mock result of running a task
// definition. The mock output can be customized. By default, it will create
// mock output based on the input.
func (c *ECSClient) RunTask(ctx context.Context, in *ecs.RunTaskInput) (*ecs.RunTaskOutput, error) {
	c.RunTaskInput = in

	if c.RunTaskOutput != nil {
		return c.RunTaskOutput, nil
	}

	if in.TaskDefinition == nil {
		return nil, errors.New("missing task definition")
	}

	clusterName := c.getOrDefaultCluster(in.Cluster)
	cluster, ok := GlobalECSService.Clusters[clusterName]
	if !ok {
		return nil, errors.New("cluster not found")
	}

	taskDefID := utility.FromStringPtr(in.TaskDefinition)

	if arn.IsARN(taskDefID) {
		family, revNum, found := taskDefIndexFromARN(taskDefID)
		if !found {
			return nil, errors.New("task definition not found")
		}
		taskDef := GlobalECSService.TaskDefs[family][revNum-1]
		task := newECSTask(in, taskDef)

		cluster[utility.FromStringPtr(task.ARN)] = task

		return &ecs.RunTaskOutput{
			Tasks: []*ecs.Task{task.export()},
		}, nil
	}

	var taskDef ECSTaskDefinition
	family, revNum, err := parseFamilyAndRevision(taskDefID)
	if err == nil {
		revisions, ok := GlobalECSService.TaskDefs[family]
		if !ok {
			return nil, errors.New("task definition family not found")
		}
		if len(revisions) < revNum {
			return nil, errors.New("task definition revision not found")
		}

		taskDef = revisions[revNum-1]
	} else {
		// Use the latest revision if none is specified.
		family = taskDefID
		revisions, ok := GlobalECSService.TaskDefs[family]
		if !ok {
			return nil, errors.New("task definition family not found")
		}

		taskDef = revisions[len(revisions)-1]
	}

	task := newECSTask(in, taskDef)

	cluster[utility.FromStringPtr(task.ARN)] = task

	return &ecs.RunTaskOutput{
		Tasks: []*ecs.Task{task.export()},
	}, nil
}

func (c *ECSClient) getOrDefaultCluster(name *string) string {
	if name == nil {
		return "default"
	}
	return *name
}

// DescribeTasks saves the input and returns information about the existing
// tasks. The mock output can be customized. By default, it will describe all
// cached tasks that match.
func (c *ECSClient) DescribeTasks(ctx context.Context, in *ecs.DescribeTasksInput) (*ecs.DescribeTasksOutput, error) {
	c.DescribeTasksInput = in

	if c.DescribeTasksOutput != nil {
		return c.DescribeTasksOutput, nil
	}

	cluster, ok := GlobalECSService.Clusters[c.getOrDefaultCluster(in.Cluster)]
	if !ok {
		return nil, errors.New("cluster not found")
	}

	ids := utility.FromStringPtrSlice(in.Tasks)

	var tasks []*ecs.Task
	var failures []*ecs.Failure
	for _, id := range ids {
		task, ok := cluster[id]
		if !ok {
			failures = append(failures, &ecs.Failure{
				Arn:    utility.ToStringPtr(id),
				Reason: utility.ToStringPtr("task not found"),
			})
			continue
		}

		tasks = append(tasks, task.export())
	}

	return &ecs.DescribeTasksOutput{
		Tasks:    tasks,
		Failures: failures,
	}, nil
}

// ListTasks saves the input and lists all matching tasks. The mock output can
// be customized. By default, it will list all cached task definitions that
// match the input filters.
func (c *ECSClient) ListTasks(ctx context.Context, in *ecs.ListTasksInput) (*ecs.ListTasksOutput, error) {
	c.ListTasksInput = in

	if c.ListTasksOutput != nil {
		return c.ListTasksOutput, nil
	}

	cluster, ok := GlobalECSService.Clusters[c.getOrDefaultCluster(in.Cluster)]
	if !ok {
		return nil, errors.New("cluster not found")
	}

	var arns []string
	for arn, task := range cluster {
		if in.DesiredStatus != nil && utility.FromStringPtr(task.GoalStatus) != *in.DesiredStatus {
			continue
		}

		if in.Family != nil && utility.FromStringPtr(task.TaskDef.Family) != *in.Family {
			continue
		}

		arns = append(arns, arn)
	}

	return &ecs.ListTasksOutput{
		TaskArns: utility.ToStringPtrSlice(arns),
	}, nil
}

// StopTask saves the input and stops a mock task. The mock output can be
// customized. By default, it will mark a cached task as stopped if it exists
// and is running.
func (c *ECSClient) StopTask(ctx context.Context, in *ecs.StopTaskInput) (*ecs.StopTaskOutput, error) {
	c.StopTaskInput = in

	if c.StopTaskOutput != nil {
		return c.StopTaskOutput, nil
	}

	cluster, ok := GlobalECSService.Clusters[c.getOrDefaultCluster(in.Cluster)]
	if !ok {
		return nil, errors.New("cluster not found")
	}

	task, ok := cluster[utility.FromStringPtr(in.Task)]
	if !ok {
		return nil, errors.New("task not found")
	}

	task.Status = utility.ToStringPtr(ecs.DesiredStatusStopped)
	task.GoalStatus = utility.ToStringPtr(ecs.DesiredStatusStopped)
	task.StopCode = utility.ToStringPtr(ecs.TaskStopCodeUserInitiated)
	task.StopReason = in.Reason
	task.Stopped = utility.ToTimePtr(time.Now())

	cluster[utility.FromStringPtr(in.Task)] = task

	return &ecs.StopTaskOutput{
		Task: task.export(),
	}, nil
}

// Close closes the mock client. The mock output can be customized. By default,
// it is a no-op that returns no error.
func (c *ECSClient) Close(ctx context.Context) error {
	if c.CloseError != nil {
		return c.CloseError
	}

	return nil
}
