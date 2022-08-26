package apimodels

import (
	"time"
)

// APIBuild represents part of a build from the REST API for use in the smoke test.
type APIBuild struct {
	Tasks []string `json:"tasks"`
}

// APITask represents part of a task from the REST API for use in the smoke test.
type APITask struct {
	Id                      string            `json:"task_id"`
	ProjectId               string            `json:"project_id"`
	ProjectIdentifier       string            `json:"project_identifier"`
	CreateTime              time.Time         `json:"create_time"`
	DispatchTime            time.Time         `json:"dispatch_time"`
	ScheduledTime           time.Time         `json:"scheduled_time"`
	ContainerAllocatedTime  time.Time         `json:"container_allocated_time"`
	StartTime               time.Time         `json:"start_time"`
	FinishTime              time.Time         `json:"finish_time"`
	IngestTime              time.Time         `json:"ingest_time"`
	ActivatedTime           time.Time         `json:"activated_time"`
	Version                 string            `json:"version_id"`
	Revision                string            `json:"revision"`
	Priority                int64             `json:"priority"`
	Activated               bool              `json:"activated"`
	ActivatedBy             string            `json:"activated_by"`
	BuildId                 string            `json:"build_id"`
	DistroId                string            `json:"distro_id"`
	Container               string            `json:"container"`
	BuildVariant            string            `json:"build_variant"`
	BuildVariantDisplayName string            `json:"build_variant_display_name"`
	DisplayName             string            `json:"display_name"`
	HostId                  string            `json:"host_id"`
	Execution               int               `json:"execution"`
	Order                   int               `json:"order"`
	Status                  string            `json:"status"`
	DisplayStatus           string            `json:"display_status"`
	PreviousExecutions      []APITask         `json:"previous_executions,omitempty"`
	GenerateTask            bool              `json:"generate_task"`
	GeneratedBy             string            `json:"generated_by"`
	DisplayOnly             bool              `json:"display_only"`
	ParentTaskId            string            `json:"parent_task_id"`
	ExecutionTasks          []string          `json:"execution_tasks,omitempty"`
	Tags                    []string          `json:"tags,omitempty"`
	Mainline                bool              `json:"mainline"`
	TaskGroup               string            `json:"task_group,omitempty"`
	TaskGroupMaxHosts       int               `json:"task_group_max_hosts,omitempty"`
	Blocked                 bool              `json:"blocked"`
	Requester               string            `json:"requester"`
	Aborted                 bool              `json:"aborted"`
	CanSync                 bool              `json:"can_sync,omitempty"`
	AMI                     string            `json:"ami"`
	MustHaveResults         bool              `json:"must_have_test_results"`
	Logs                    map[string]string `json:"logs"`
}
