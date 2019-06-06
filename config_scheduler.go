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
	PlannerVersion                string  `bson:"planner_version" json:"planner_version" mapstructure:"planner_version"`
	TargetTimeSeconds             int     `bson:"target_time_seconds" json:"target_time_seconds" mapstructure:"target_time_seconds"`
	AcceptableHostIdleTimeSeconds int     `bson:"acceptable_host_idle_time_seconds" json:"acceptable_host_idle_time_seconds" mapstructure:"acceptable_host_idle_time_seconds"`
	GroupVersions                 bool    `bson:"group_versions" json:"group_versions" mapstructure:"group_versions"`
	PatchZipperFactor             int     `bson:"patch_zipper_factor" json:"patch_zipper_factor" mapstructure:"patch_zipper_factor"`
	Interleave                    bool    `bson:"interleave" json:"interleave" mapstructure:"interleave"`
	MainlineFirst                 bool    `bson:"mainline_first" json:"mainline_first" mapstructure:"mainline_first"`
	PatchFirst                    bool    `bson:"patch_first" json:"patch_first" mapstructure:"patch_first"`
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
			"planner_version":                   c.PlannerVersion,
			"target_time_seconds":               c.TargetTimeSeconds,
			"acceptable_host_idle_time_seconds": c.AcceptableHostIdleTimeSeconds,
			"group_versions":                    c.GroupVersions,
			"patch_zipper_factor":               c.PatchZipperFactor,
			"interleave":                        c.Interleave,
			"mainline_first":                    c.MainlineFirst,
			"patch_first":                       c.PatchFirst,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *SchedulerConfig) ValidateAndDefault() error {
	if c.TaskFinder == "" {
		// default to legacy
		c.TaskFinder = FinderVersionLegacy
		return nil
	}

	if c.CacheDurationSeconds == 0 || c.CacheDurationSeconds > 600 {
		c.CacheDurationSeconds = 20
	}

	if !util.StringSliceContains(ValidFinderVersions, c.TaskFinder) {
		return errors.Errorf("supported finders are %s; %s is not supported",
			ValidFinderVersions, c.TaskFinder)
	}

	if c.HostAllocator == "" {
		c.HostAllocator = HostAllocatorDuration
		return nil
	}

	if !util.StringSliceContains(ValidHostAllocators, c.HostAllocator) {
		return errors.Errorf("supported allocators are %s; %s is not supported",
			ValidHostAllocators, c.HostAllocator)
	}

	if c.FreeHostFraction < 0 || c.FreeHostFraction > 1 {
		return errors.New("free host fraction must be between 0 and 1")
	}

	if !util.StringSliceContains(ValidPlannerVersions, c.PlannerVersion) {
		return errors.Errorf("supported planner versions are %s; %s is not supported",
			ValidPlannerVersions, c.HostAllocator)
	}

	if c.MainlineFirst && c.PatchFirst {
		return errors.New("mainline first and patch first cannot both be set to true")
	}

	taskOrders := []bool{c.Interleave, c.MainlineFirst, c.PatchFirst}
	nTrue := 0
	for i := 0; i < len(taskOrders); i++ {
		if taskOrders[i] {
			nTrue++
		}
	}
	if nTrue == 0 {
		return errors.Errorf("invalid task ordering - one of the following fields must be true: SchedulerConfig.Interleave [%t], SchedulerConfig.MainlineFirst [%t] or SchedulerConfig.PatchFirst [%t]",
			c.Interleave,
			c.MainlineFirst,
			c.PatchFirst,
		)
	}
	if nTrue > 1 {
		return errors.Errorf("invalid task ordering - only one of the following fields can be true: SchedulerConfig.Interleave [%t], SchedulerConfig.MainlineFirst [%t] or SchedulerConfig.SchedulerConfig [%t]",
			c.Interleave,
			c.MainlineFirst,
			c.PatchFirst,
		)
	}

	if c.PatchZipperFactor < 0 || c.PatchZipperFactor > 100 {
		return errors.Errorf("patch zipper factor must be an integer between 0 and 100, inclusive - a value of %d is invalid", c.PatchZipperFactor)
	}

	return nil
}
