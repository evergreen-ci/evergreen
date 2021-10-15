package evergreen

import (
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SchedulerConfig holds relevant settings for the scheduler process.
type SchedulerConfig struct {
	TaskFinder                    string  `bson:"task_finder" json:"task_finder" yaml:"task_finder"`
	HostAllocator                 string  `bson:"host_allocator" json:"host_allocator" yaml:"host_allocator"`
	HostAllocatorRoundingRule     string  `bson:"host_allocator_rounding_rule" json:"host_allocator_rounding_rule" mapstructure:"host_allocator_rounding_rule"`
	HostAllocatorFeedbackRule     string  `bson:"host_allocator_feedback_rule" json:"host_allocator_feedback_rule" mapstructure:"host_allocator_feedback_rule"`
	HostsOverallocatedRule        string  `bson:"hosts_overallocated_rule" json:"hosts_overallocated_rule" mapstructure:"hosts_overallocated_rule"`
	FutureHostFraction            float64 `bson:"free_host_fraction" json:"free_host_fraction" yaml:"free_host_fraction"`
	CacheDurationSeconds          int     `bson:"cache_duration_seconds" json:"cache_duration_seconds" yaml:"cache_duration_seconds"`
	Planner                       string  `bson:"planner" json:"planner" mapstructure:"planner"`
	TargetTimeSeconds             int     `bson:"target_time_seconds" json:"target_time_seconds" mapstructure:"target_time_seconds"`
	AcceptableHostIdleTimeSeconds int     `bson:"acceptable_host_idle_time_seconds" json:"acceptable_host_idle_time_seconds" mapstructure:"acceptable_host_idle_time_seconds"`
	GroupVersions                 bool    `bson:"group_versions" json:"group_versions" mapstructure:"group_versions"`
	PatchFactor                   int64   `bson:"patch_zipper_factor" json:"patch_factor" mapstructure:"patch_zipper"`
	PatchTimeInQueueFactor        int64   `bson:"patch_time_in_queue_factor" json:"patch_time_in_queue_factor" mapstructure:"patch_time_in_queue_factor"`
	CommitQueueFactor             int64   `bson:"commit_queue_factor" json:"commit_queue_factor" mapstructure:"commit_queue_factor"`
	MainlineTimeInQueueFactor     int64   `bson:"mainline_time_in_queue_factor" json:"mainline_time_in_queue_factor" mapstructure:"mainline_time_in_queue_factor"`
	ExpectedRuntimeFactor         int64   `bson:"expected_runtime_factor" json:"expected_runtime_factor" mapstructure:"expected_runtime_factor"`
	GenerateTaskFactor            int64   `bson:"generate_task_factor" json:"generate_task_factor" mapstructure:"generate_task_factor"`
	StepbackTaskFactor            int64   `bson:"stepback_task_factor" json:"stepback_task_factor" mapstructure:"stepback_task_factor"`
}

func (c *SchedulerConfig) SectionId() string { return "scheduler" }

func (c *SchedulerConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = SchedulerConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	// Clear the struct because Decode will not set fields that are omitempty to
	// the zero value if they're zero in the database.
	*c = SchedulerConfig{}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *SchedulerConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"task_finder":                       c.TaskFinder,
			"host_allocator":                    c.HostAllocator,
			"host_allocator_rounding_rule":      c.HostAllocatorRoundingRule,
			"host_allocator_feedback_rule":      c.HostAllocatorFeedbackRule,
			"hosts_overallocated_rule":          c.HostsOverallocatedRule,
			"free_host_fraction":                c.FutureHostFraction,
			"cache_duration_seconds":            c.CacheDurationSeconds,
			"planner":                           c.Planner,
			"target_time_seconds":               c.TargetTimeSeconds,
			"acceptable_host_idle_time_seconds": c.AcceptableHostIdleTimeSeconds,
			"group_versions":                    c.GroupVersions,
			"patch_zipper_factor":               c.PatchFactor,
			"patch_time_in_queue_factor":        c.PatchTimeInQueueFactor,
			"commit_queue_factor":               c.CommitQueueFactor,
			"mainline_time_in_queue_factor":     c.MainlineTimeInQueueFactor,
			"expected_runtime_factor":           c.ExpectedRuntimeFactor,
			"generate_task_factor":              c.GenerateTaskFactor,
			"stepback_task_factor":              c.StepbackTaskFactor,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *SchedulerConfig) ValidateAndDefault() error {
	if c.TaskFinder == "" {
		// default to legacy
		c.TaskFinder = FinderVersionLegacy
	}

	if !utility.StringSliceContains(ValidTaskFinderVersions, c.TaskFinder) {
		return errors.Errorf("supported task finders are %s; %s is not supported",
			ValidTaskFinderVersions, c.TaskFinder)
	}

	if c.HostAllocator == "" {
		// default to "utilization"
		c.HostAllocator = HostAllocatorUtilization
	}

	if !utility.StringSliceContains(ValidHostAllocators, c.HostAllocator) {
		return errors.Errorf("supported host allocators are %s; %s is not supported",
			ValidHostAllocators, c.HostAllocator)
	}

	if c.HostAllocatorRoundingRule == "" {
		c.HostAllocatorRoundingRule = HostAllocatorRoundDown
	}

	if !utility.StringSliceContains(ValidDefaultHostAllocatorRoundingRules, c.HostAllocatorRoundingRule) {
		return errors.Errorf("supported host allocator rounding rules are %s; %s is not supported",
			ValidDefaultHostAllocatorRoundingRules, c.HostAllocatorRoundingRule)
	}

	if c.HostAllocatorFeedbackRule == "" {
		c.HostAllocatorFeedbackRule = HostAllocatorNoFeedback
	}

	if !utility.StringSliceContains(ValidDefaultHostAllocatorFeedbackRules, c.HostAllocatorFeedbackRule) {
		return errors.Errorf("supported host allocator feedback rules are %s; %s is not supported",
			ValidDefaultHostAllocatorFeedbackRules, c.HostAllocatorFeedbackRule)
	}
	if c.HostsOverallocatedRule == "" {
		c.HostsOverallocatedRule = HostsOverallocatedIgnore
	}

	if !utility.StringSliceContains(ValidDefaultHostsOverallocatedRules, c.HostsOverallocatedRule) {
		return errors.Errorf("supported hosts overallocation handling rules are %s; %s is not supported",
			ValidDefaultHostsOverallocatedRules, c.HostsOverallocatedRule)
	}

	if c.FutureHostFraction < 0 || c.FutureHostFraction > 1 {
		// traditional default
		c.FutureHostFraction = .4
	}

	if c.CacheDurationSeconds > 600 {
		return errors.New("cache duration seconds cannot be greater that 600 seconds (10 minutes)")
	}

	if c.CacheDurationSeconds <= 0 {
		c.CacheDurationSeconds = 20
	}

	if c.Planner == "" {
		// default to 'legacy'
		c.Planner = PlannerVersionLegacy
	}

	if !utility.StringSliceContains(ValidTaskPlannerVersions, c.Planner) {
		return errors.Errorf("supported planners are %s; %s is not supported",
			ValidTaskPlannerVersions, c.Planner)
	}

	if c.TargetTimeSeconds < 0 {
		return errors.New("target time seconds cannot be a negative value")
	}

	if c.AcceptableHostIdleTimeSeconds < 0 {
		return errors.New("acceptable host idle time seconds cannot be a negative value")
	}

	if c.PatchFactor < 0 || c.PatchFactor > 100 {
		return errors.New("patch factor must be between 0 and 100")
	}

	if c.PatchTimeInQueueFactor < 0 || c.PatchTimeInQueueFactor > 100 {
		return errors.New("patch time in queue factor must be between 0 and 100")
	}

	if c.CommitQueueFactor < 0 || c.CommitQueueFactor > 100 {
		return errors.New("commit queue factor must be between 0 and 100")
	}

	if c.MainlineTimeInQueueFactor < 0 || c.MainlineTimeInQueueFactor > 100 {
		return errors.New("mainline time in queue factor must be between 0 and 100")
	}

	if c.ExpectedRuntimeFactor < 0 || c.ExpectedRuntimeFactor > 100 {
		return errors.New("expected runtime factor must be between 0 and 100")
	}
	if c.GenerateTaskFactor < 0 || c.GenerateTaskFactor > 100 {
		return errors.New("generate task factor must be between 0 and 100")
	}

	if c.StepbackTaskFactor < 0 || c.StepbackTaskFactor > 100 {
		return errors.New("stepback task factor must be between 0 and 100")
	}

	return nil
}
