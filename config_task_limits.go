package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// TaskLimitsConfig holds relevant settings for Evergreen task limitations.
// These are usually protections against inputs that can cause issues like
// service instability.
type TaskLimitsConfig struct {
	// MaxTasksPerVersion is the maximum number of tasks that a single version
	// can have.
	MaxTasksPerVersion int `bson:"max_tasks_per_version" json:"max_tasks_per_version" yaml:"max_tasks_per_version"`

	// MaxIncludesPerVersion is the maximum number of includes that a single
	// version can have.
	MaxIncludesPerVersion int `bson:"max_includes_per_version" json:"max_includes_per_version" yaml:"max_includes_per_version"`

	// MaxHourlyPatchTasks is the maximum number of patch tasks a single user can
	// schedule per hour.
	MaxHourlyPatchTasks int `bson:"max_hourly_patch_tasks" json:"max_hourly_patch_tasks" yaml:"max_hourly_patch_tasks"`

	// MaxPendingGeneratedTasks is the maximum number of tasks that can be created
	// by all generated task at once.
	MaxPendingGeneratedTasks int `bson:"max_pending_generated_tasks" json:"max_pending_generated_tasks" yaml:"max_pending_generated_tasks"`

	// MaxGenerateTaskJSONSize is the maximum size of a JSON file in MB that can be specified in the GenerateTasks command.
	MaxGenerateTaskJSONSize int `bson:"max_generate_task_json_size" json:"max_generate_task_json_size" yaml:"max_generate_task_json_size"`

	// MaxConcurrentLargeParserProjectTasks is the maximum number of tasks with >16MB parser projects that can be running at once.
	MaxConcurrentLargeParserProjectTasks int `bson:"max_concurrent_large_parser_project_tasks" json:"max_concurrent_large_parser_project_tasks" yaml:"max_concurrent_large_parser_project_tasks"`

	// MaxDegradedModeParserProjectSize is the maximum parser project size during CPU degraded mode.
	MaxDegradedModeParserProjectSize int `bson:"max_degraded_mode_parser_project_size" json:"max_degraded_mode_parser_project_size" yaml:"max_degraded_mode_parser_project_size"`

	// MaxParserProjectSize is the maximum allowed parser project size.
	MaxParserProjectSize int `bson:"max_parser_project_size" json:"max_parser_project_size" yaml:"max_parser_project_size"`

	// MaxExecTimeoutSecs is the maximum number of seconds a task can run and set their timeout to.
	MaxExecTimeoutSecs int `bson:"max_exec_timeout_secs" json:"max_exec_timeout_secs" yaml:"max_exec_timeout_secs"`

	// MaxTaskExecution is the maximum task (zero based) execution number
	MaxTaskExecution int `bson:"max_task_execution" json:"max_task_execution" yaml:"max_task_execution"`
}

var (
	maxTasksPerVersionKey                = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxTasksPerVersion")
	maxIncludesPerVersionKey             = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxIncludesPerVersion")
	maxHourlyPatchTasksKey               = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxHourlyPatchTasks")
	maxPendingGeneratedTasks             = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxPendingGeneratedTasks")
	maxGenerateTaskJSONSize              = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxGenerateTaskJSONSize")
	maxConcurrentLargeParserProjectTasks = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxConcurrentLargeParserProjectTasks")
	maxDegradedModeParserProjectSize     = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxDegradedModeParserProjectSize")
	maxParserProjectSize                 = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxParserProjectSize")
	MaxExecTimeoutSecs                   = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxExecTimeoutSecs")
	maxTaskExecutionKey                  = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxTaskExecution")
)

func (c *TaskLimitsConfig) SectionId() string { return "task_limits" }

func (c *TaskLimitsConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *TaskLimitsConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			maxTasksPerVersionKey:                c.MaxTasksPerVersion,
			maxIncludesPerVersionKey:             c.MaxIncludesPerVersion,
			maxPendingGeneratedTasks:             c.MaxPendingGeneratedTasks,
			maxHourlyPatchTasksKey:               c.MaxHourlyPatchTasks,
			maxGenerateTaskJSONSize:              c.MaxGenerateTaskJSONSize,
			maxConcurrentLargeParserProjectTasks: c.MaxConcurrentLargeParserProjectTasks,
			maxDegradedModeParserProjectSize:     c.MaxDegradedModeParserProjectSize,
			maxParserProjectSize:                 c.MaxParserProjectSize,
			MaxExecTimeoutSecs:                   c.MaxExecTimeoutSecs,
			maxTaskExecutionKey:                  c.MaxTaskExecution,
		},
	}), "updating config section '%s'", c.SectionId())
}

func (c *TaskLimitsConfig) ValidateAndDefault() error {
	return nil
}
