package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

var taskFinderKey = "task_finder"

// SchedulerConfig holds relevant settings for the scheduler process.
type SchedulerConfig struct {
	TaskFinder string `bson:"task_finder" json:"task_finder" yaml:"task_finder"`
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
			taskFinderKey: c.TaskFinder,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *SchedulerConfig) ValidateAndDefault() error {
	finders := []string{"legacy", "alternate", "parallel", "pipeline"}

	if c.TaskFinder == "" {
		// default to alternate
		c.TaskFinder = finders[0]
		return nil
	}

	if !sliceContains(finders, c.TaskFinder) {
		return errors.Errorf("supported finders are %s; %s is not supported",
			finders, c.TaskFinder)

	}
	return nil
}
