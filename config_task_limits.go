package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
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
	// schedule per hour. This can be overridden for individual projects or
	// repos (see HourlyPatchTaskOverrides).
	MaxHourlyPatchTasks int `bson:"max_hourly_patch_tasks" json:"max_hourly_patch_tasks" yaml:"max_hourly_patch_tasks"`

	// MaxPendingGeneratedTasks is the maximum number of tasks that can be created
	// by all generated task at once.
	MaxPendingGeneratedTasks int `bson:"max_pending_generated_tasks" json:"max_pending_generated_tasks" yaml:"max_pending_generated_tasks"`

	// MaxGenerateTaskJSONSize is the maximum size of a JSON file in MB that can be specified in the GenerateTasks command.
	MaxGenerateTaskJSONSize int `bson:"max_generate_task_json_size" json:"max_generate_task_json_size" yaml:"max_generate_task_json_size"`

	// MaxConcurrentLargeParserProjectTasks is the maximum number of tasks with parser projects stored in S3 that can be running at once.
	MaxConcurrentLargeParserProjectTasks int `bson:"max_concurrent_large_parser_project_tasks" json:"max_concurrent_large_parser_project_tasks" yaml:"max_concurrent_large_parser_project_tasks"`

	// MaxDegradedModeConcurrentLargeParserProjectTasks is the maximum number of tasks with parser projects stored in S3 that can be running at once during CPU degraded mode.
	MaxDegradedModeConcurrentLargeParserProjectTasks int `bson:"max_degraded_mode_concurrent_large_parser_project_tasks" json:"max_degraded_mode_concurrent_large_parser_project_tasks" yaml:"max_degraded_mode_concurrent_large_parser_project_tasks"`

	// MaxDegradedModeParserProjectSize is the maximum parser project size in MB during CPU degraded mode.
	MaxDegradedModeParserProjectSize int `bson:"max_degraded_mode_parser_project_size" json:"max_degraded_mode_parser_project_size" yaml:"max_degraded_mode_parser_project_size"`

	// MaxParserProjectSize is the maximum allowed size in MB for parser projects that are stored in S3.
	MaxParserProjectSize int `bson:"max_parser_project_size" json:"max_parser_project_size" yaml:"max_parser_project_size"`

	// MaxExecTimeoutSecs is the maximum number of seconds a task can run and set their timeout to.
	MaxExecTimeoutSecs int `bson:"max_exec_timeout_secs" json:"max_exec_timeout_secs" yaml:"max_exec_timeout_secs"`

	// MaxTaskExecution is the maximum task (zero based) execution number.
	MaxTaskExecution int `bson:"max_task_execution" json:"max_task_execution" yaml:"max_task_execution"`

	// MaxDailyAutomaticRestarts is the maximum number of times a project can automatically restart a task within a 24-hour period.
	MaxDailyAutomaticRestarts int `bson:"max_daily_automatic_restarts" json:"max_daily_automatic_restarts" yaml:"max_daily_automatic_restarts"`

	// MaxScheduledTasksPerDistro is the cap for the number of max tasks materialized into a distro's queue doc per pass.
	MaxScheduledTasksPerDistro int `bson:"max_scheduled_tasks_per_distro" json:"max_scheduled_tasks_per_distro" yaml:"max_scheduled_tasks_per_distro"`

	// HourlyPatchTaskOverrides sets a separate hourly patch task
	// scheduling limit for individual branch projects or repos. If a project or
	// repo has an override, users scheduling patch tasks in that project or
	// repo will have their usage count against the override's limit instead of
	// MaxHourlyPatchTasks.
	HourlyPatchTaskOverrides []HourlyPatchTaskOverride `bson:"hourly_patch_task_overrides" json:"hourly_patch_task_overrides" yaml:"hourly_patch_task_overrides"`
}

// HourlyPatchTaskOverride is a per-project or per-repo override to the default
// hourly per-user patch task scheduling limit.
type HourlyPatchTaskOverride struct {
	// ProjectOrRepoID is the ID of the branch project or repo that the override
	// applies to. If it's a repo, all branch projects tracking the repo that
	// have no override of their own share the repo's limit.
	ProjectOrRepoID string `bson:"project_or_repo_id" json:"project_or_repo_id" yaml:"project_or_repo_id"`
	// MaxHourlyPatchTasks is the maximum number of patch tasks a single user
	// can schedule per hour in the project or repo.
	MaxHourlyPatchTasks int `bson:"max_hourly_patch_tasks" json:"max_hourly_patch_tasks" yaml:"max_hourly_patch_tasks"`
}

// HourlyPatchTaskLimitForProject returns the hourly per-user patch task limit
// for the given project, along with the ID of the project or repo whose
// override supplied it. An empty ID means the default limit applies and
// usage is tracked against the user's general counter. A limit of 0 means no
// limit is enforced. A branch project-level override takes precedence over a
// repo-level one.
func (c *TaskLimitsConfig) HourlyPatchTaskLimitForProject(projectID, repoRefID string) (hourlyLimit int, projectOrRepoID string) {
	if projectID != "" {
		for _, o := range c.HourlyPatchTaskOverrides {
			if o.ProjectOrRepoID == projectID {
				return o.MaxHourlyPatchTasks, o.ProjectOrRepoID
			}
		}
	}

	if repoRefID != "" {
		for _, o := range c.HourlyPatchTaskOverrides {
			if o.ProjectOrRepoID == repoRefID {
				return o.MaxHourlyPatchTasks, o.ProjectOrRepoID
			}
		}
	}
	return c.MaxHourlyPatchTasks, ""
}

var (
	maxTasksPerVersionKey                            = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxTasksPerVersion")
	maxIncludesPerVersionKey                         = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxIncludesPerVersion")
	maxHourlyPatchTasksKey                           = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxHourlyPatchTasks")
	maxPendingGeneratedTasks                         = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxPendingGeneratedTasks")
	maxGenerateTaskJSONSize                          = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxGenerateTaskJSONSize")
	maxConcurrentLargeParserProjectTasks             = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxConcurrentLargeParserProjectTasks")
	maxDegradedModeParserProjectSize                 = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxDegradedModeParserProjectSize")
	maxParserProjectSize                             = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxParserProjectSize")
	MaxExecTimeoutSecs                               = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxExecTimeoutSecs")
	maxDegradedModeConcurrentLargeParserProjectTasks = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxDegradedModeConcurrentLargeParserProjectTasks")
	maxTaskExecutionKey                              = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxTaskExecution")
	maxDailyAutomaticRestartsKey                     = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxDailyAutomaticRestarts")
	maxScheduledTasksPerDistroKey                    = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxScheduledTasksPerDistro")
	hourlyPatchTaskOverridesKey                      = bsonutil.MustHaveTag(TaskLimitsConfig{}, "HourlyPatchTaskOverrides")
)

func (c *TaskLimitsConfig) SectionId() string { return "task_limits" }

func (c *TaskLimitsConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *TaskLimitsConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			maxTasksPerVersionKey:                            c.MaxTasksPerVersion,
			maxIncludesPerVersionKey:                         c.MaxIncludesPerVersion,
			maxPendingGeneratedTasks:                         c.MaxPendingGeneratedTasks,
			maxHourlyPatchTasksKey:                           c.MaxHourlyPatchTasks,
			maxGenerateTaskJSONSize:                          c.MaxGenerateTaskJSONSize,
			maxConcurrentLargeParserProjectTasks:             c.MaxConcurrentLargeParserProjectTasks,
			maxDegradedModeParserProjectSize:                 c.MaxDegradedModeParserProjectSize,
			maxParserProjectSize:                             c.MaxParserProjectSize,
			MaxExecTimeoutSecs:                               c.MaxExecTimeoutSecs,
			maxDegradedModeConcurrentLargeParserProjectTasks: c.MaxDegradedModeConcurrentLargeParserProjectTasks,
			maxTaskExecutionKey:                              c.MaxTaskExecution,
			maxDailyAutomaticRestartsKey:                     c.MaxDailyAutomaticRestarts,
			maxScheduledTasksPerDistroKey:                    c.MaxScheduledTasksPerDistro,
			hourlyPatchTaskOverridesKey:                      c.HourlyPatchTaskOverrides,
		},
	}), "updating config section '%s'", c.SectionId())
}

func (c *TaskLimitsConfig) ValidateAndDefault() error {
	catcher := grip.NewBasicCatcher()
	projectOrRepoIDs := make(map[string]bool, len(c.HourlyPatchTaskOverrides))
	for idx, o := range c.HourlyPatchTaskOverrides {
		catcher.ErrorfWhen(o.MaxHourlyPatchTasks <= 0, "hourly patch task limit override for project/repo '%s' at index %d must be positive", o.ProjectOrRepoID, idx)

		if o.ProjectOrRepoID == "" {
			catcher.Errorf("hourly patch task limit override at index %d must set a project/repo ID", idx)
			continue
		}

		if projectOrRepoIDs[o.ProjectOrRepoID] {
			catcher.Errorf("duplicate hourly patch task limit override for project/repo '%s'", o.ProjectOrRepoID)
			continue
		}
		projectOrRepoIDs[o.ProjectOrRepoID] = true

	}
	return catcher.Resolve()
}
