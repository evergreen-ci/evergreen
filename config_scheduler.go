package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// SchedulerConfig holds relevant settings for the scheduler process.
type SchedulerConfig struct {
	TaskFinder    string `bson:"task_finder" json:"task_finder" yaml:"task_finder"`
	HostAllocator string `bson:"host_allocator" json:"host_allocator" yaml:"host_allocator"`
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
			"task_finder":    c.TaskFinder,
			"host_allocator": c.HostAllocator,
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

	if !util.StringSliceContains(finders, c.TaskFinder) {
		return errors.Errorf("supported finders are %s; %s is not supported",
			finders, c.TaskFinder)
	}

	allocators := []string{"duration", "deficit"}
	if c.HostAllocator == "" {
		c.HostAllocator = allocators[0]
		return nil
	}

	if !util.StringSliceContains(allocators, c.HostAllocator) {
		return errors.Errorf("supported allocators are %s; %s is not suported",
			allocators, c.HostAllocator)
	}

	return nil
}
