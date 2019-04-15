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
	TaskFinder           string  `bson:"task_finder" json:"task_finder" yaml:"task_finder"`
	HostAllocator        string  `bson:"host_allocator" json:"host_allocator" yaml:"host_allocator"`
	FreeHostFraction     float64 `bson:"free_host_fraction" json:"free_host_fraction" yaml:"free_host_fraction"`
	CacheDurationSeconds int     `bson:"cache_duration_seconds" json:"cache_duration_seconds" yaml:"cache_duration_seconds"`
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
			"task_finder":        c.TaskFinder,
			"host_allocator":     c.HostAllocator,
			"free_host_fraction": c.FreeHostFraction,
		},
	}, options.Update().SetUpsert(true))

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
