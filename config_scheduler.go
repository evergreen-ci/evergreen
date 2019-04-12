package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// SchedulerConfig holds relevant settings for the scheduler process.
type SchedulerConfig struct {
	TaskFinder           string  `bson:"task_finder" json:"task_finder" yaml:"task_finder"`
	HostAllocator        string  `bson:"host_allocator" json:"host_allocator" yaml:"host_allocator"`
	FreeHostFraction     float64 `bson:"free_host_fraction" json:"free_host_fraction" yaml:"free_host_fraction"`
	CacheDurationSeconds int     `bson:"cache_duration_seconds" json:"cache_duration_seconds" yaml:"cache_duration_seconds"`
}

func (c *SchedulerConfig) SectionId() string { return "scheduler" }

func (c *SchedulerConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = SchedulerConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *SchedulerConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"task_finder":        c.TaskFinder,
			"host_allocator":     c.HostAllocator,
			"free_host_fraction": c.FreeHostFraction,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *SchedulerConfig) ValidateAndDefault() error {
	finders := []string{"legacy", "alternate", "parallel", "pipeline"}

	if c.TaskFinder == "" {
		// default to legacy
		c.TaskFinder = finders[0]
		return nil
	}

	if c.CacheDurationSeconds == 0 || c.CacheDurationSeconds > 600 {
		c.CacheDurationSeconds = 20
	}

	if !util.StringSliceContains(finders, c.TaskFinder) {
		return errors.Errorf("supported finders are %s; %s is not supported",
			finders, c.TaskFinder)
	}

	allocators := []string{"duration", "deficit", "utilization"}
	if c.HostAllocator == "" {
		c.HostAllocator = allocators[0]
		return nil
	}

	if !util.StringSliceContains(allocators, c.HostAllocator) {
		return errors.Errorf("supported allocators are %s; %s is not suported",
			allocators, c.HostAllocator)
	}

	if c.FreeHostFraction < 0 || c.FreeHostFraction > 1 {
		return errors.New("free host fraction must be between 0 and 1")
	}

	return nil
}
