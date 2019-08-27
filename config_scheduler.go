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
	TaskOrdering                  string  `bson:"task_ordering" json:"task_ordering" yaml:"task_ordering"`
	TargetTimeSeconds             int     `bson:"target_time_seconds" json:"target_time_seconds" mapstructure:"target_time_seconds"`
	AcceptableHostIdleTimeSeconds int     `bson:"acceptable_host_idle_time_seconds" json:"acceptable_host_idle_time_seconds" mapstructure:"acceptable_host_idle_time_seconds"`
	GroupVersions                 bool    `bson:"group_versions" json:"group_versions" mapstructure:"group_versions"`
	PatchFactor                   int64   `bson:"patch_zipper_factor" json:"patch_factor" mapstructure:"patch_factor"`
	TimeInQueueFactor             int64   `bson:"time_in_queue_factor" json:"time_in_queue_factor" mapstructure:"time_in_queue_factor"`
	ExpectedRuntimeFactor         int64   `bson:"expected_runtime_factor" json:"expected_runtime_factor" mapstructure:"expected_runtime_factor"`
}

func (c *SchedulerConfig) SectionId() string { return "scheduler" }

func (c *SchedulerConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = SchedulerConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
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
			"task_ordering":                     c.TaskOrdering,
			"target_time_seconds":               c.TargetTimeSeconds,
			"acceptable_host_idle_time_seconds": c.AcceptableHostIdleTimeSeconds,
			"group_versions":                    c.GroupVersions,
			"patch_zipper_factor":               c.PatchFactor,
			"time_in_queue_factor":              c.TimeInQueueFactor,
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

	if c.CacheDurationSeconds == 0 || c.CacheDurationSeconds > 600 {
		c.CacheDurationSeconds = 20
	}

	if !util.StringSliceContains(ValidFinderVersions, c.TaskFinder) {
		return errors.Errorf("supported finders are %s; %s is not supported",
			ValidFinderVersions, c.TaskFinder)
	}

	if c.HostAllocator == "" {
		// default to duration
		c.HostAllocator = HostAllocatorDuration
	}

	if !util.StringSliceContains(ValidHostAllocators, c.HostAllocator) {
		return errors.Errorf("supported allocators are %s; %s is not supported",
			ValidHostAllocators, c.HostAllocator)
	}

	if c.FreeHostFraction < 0 || c.FreeHostFraction > 1 {
		return errors.New("free host fraction must be between 0 and 1")
	}

	if c.Planner == "" {
		// default to 'legacy'
		c.Planner = PlannerVersionLegacy
	}

	if c.TaskOrdering == "" {
		// default to 'interleave'
		c.TaskOrdering = TaskOrderingInterleave
	}

	return nil
}
