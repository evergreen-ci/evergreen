package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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

	// MaxPendingGeneratedTasks is the maximum number of tasks that can be created
	// by generated task at once.
	MaxPendingGeneratedTasks int `bson:"max_pending_generated_tasks" json:"max_pending_generated_tasks" yaml:"max_pending_generated_tasks"`
}

var (
	maxTasksPerVersionKey    = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxTasksPerVersion")
	maxIncludesPerVersionKey = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxIncludesPerVersion")
	maxPendingGeneratedTasks = bsonutil.MustHaveTag(TaskLimitsConfig{}, "MaxPendingGeneratedTasks")
)

func (c *TaskLimitsConfig) SectionId() string { return "task_limits" }

func (c *TaskLimitsConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = TaskLimitsConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *TaskLimitsConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			maxTasksPerVersionKey:    c.MaxTasksPerVersion,
			maxIncludesPerVersionKey: c.MaxIncludesPerVersion,
			maxPendingGeneratedTasks: c.MaxPendingGeneratedTasks,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *TaskLimitsConfig) ValidateAndDefault() error {
	return nil
}
