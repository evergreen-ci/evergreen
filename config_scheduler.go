package evergreen

import (
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SchedulerConfig holds relevant settings for the scheduler process.
type SchedulerConfig struct {
	TaskFinder                    string  `bson:"task_finder" json:"task_finder" yaml:"task_finder"`
	HostAllocator                 string  `bson:"host_allocator" json:"host_allocator" yaml:"host_allocator"`
	FreeHostFraction              float64 `bson:"free_host_fraction" json:"free_host_fraction" yaml:"free_host_fraction"`
	CacheDurationSeconds          int     `bson:"cache_duration_seconds" json:"cache_duration_seconds" yaml:"cache_duration_seconds"`
	Planner                       string  `bson:"planner" json:"planner" mapstructure:"planner"`
	TargetTimeSeconds             int     `bson:"target_time_seconds" json:"target_time_seconds" mapstructure:"target_time_seconds"`
	AcceptableHostIdleTimeSeconds int     `bson:"acceptable_host_idle_time_seconds" json:"acceptable_host_idle_time_seconds" mapstructure:"acceptable_host_idle_time_seconds"`
	GroupVersions                 bool    `bson:"group_versions" json:"group_versions" mapstructure:"group_versions"`
	PatchFactor                   int64   `bson:"patch_zipper_factor" json:"patch_factor" mapstructure:"patch_zipper"`
	PatchTimeInQueueFactor        int64   `bson:"patch_time_in_queue_factor" json:"patch_time_in_queue_factor" mapstructure:"patch_time_in_queue_factor"`
	MainlineTimeInQueueFactor     int64   `bson:"mainline_time_in_queue_factor" json:"mainline_time_in_queue_factor" mapstructure:"mainline_time_in_queue_factor"`
	ExpectedRuntimeFactor         int64   `bson:"expected_runtime_factor" json:"expected_runtime_factor" mapstructure:"expected_runtime_factor"`
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
			"free_host_fraction":                c.FreeHostFraction,
			"cache_duration_seconds":            c.CacheDurationSeconds,
			"planner":                           c.Planner,
			"target_time_seconds":               c.TargetTimeSeconds,
			"acceptable_host_idle_time_seconds": c.AcceptableHostIdleTimeSeconds,
			"group_versions":                    c.GroupVersions,
			"patch_zipper_factor":               c.PatchFactor,
			"patch_time_in_queue_factor":        c.PatchTimeInQueueFactor,
			"mainline_time_in_queue_factor":     c.MainlineTimeInQueueFactor,
			"expected_runtime_factor":           c.ExpectedRuntimeFactor,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *SchedulerConfig) ValidateAndDefault() error {
	if c.TaskFinder == "" {
		// default to legacy
		c.TaskFinder = FinderVersionLegacy
	}

	if !util.StringSliceContains(ValidTaskFinderVersions, c.TaskFinder) {
		return errors.Errorf("supported task finders are %s; %s is not supported",
			ValidTaskFinderVersions, c.TaskFinder)
	}

	if c.HostAllocator == "" {
		// default to "utilization"
		c.HostAllocator = HostAllocatorUtilization
	}

	if !util.StringSliceContains(ValidHostAllocators, c.HostAllocator) {
		return errors.Errorf("supported host allocators are %s; %s is not supported",
			ValidHostAllocators, c.HostAllocator)
	}

	if c.FreeHostFraction < 0 || c.FreeHostFraction > 1 {
		return errors.New("free host fraction must be between 0 and 1")
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

	if !util.StringSliceContains(ValidTaskPlannerVersions, c.Planner) {
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

	if c.MainlineTimeInQueueFactor < 0 || c.MainlineTimeInQueueFactor > 100 {
		return errors.New("mainline time in queue factor must be between 0 and 100")
	}

	if c.ExpectedRuntimeFactor < 0 || c.ExpectedRuntimeFactor > 100 {
		return errors.New("expected runtime factor must be between 0 and 100")
	}

	return nil
}
