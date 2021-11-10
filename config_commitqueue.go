package evergreen

import (
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

type CommitQueueConfig struct {
	MergeTaskDistro string `yaml:"merge_task_distro" bson:"merge_task_distro" json:"merge_task_distro"`
	CommitterName   string `yaml:"committer_name" bson:"committer_name" json:"committer_name"`
	CommitterEmail  string `yaml:"committer_email" bson:"committer_email" json:"committer_email"`
	BatchSize       int    `yaml:"batch_size" bson:"batch_size" json:"batch_size"`
}

var (
	mergeTaskDistroKey      = bsonutil.MustHaveTag(CommitQueueConfig{}, "MergeTaskDistro")
	committerNameKey        = bsonutil.MustHaveTag(CommitQueueConfig{}, "CommitterName")
	committerEmailKey       = bsonutil.MustHaveTag(CommitQueueConfig{}, "CommitterEmail")
	commitQueueBatchSizeKey = bsonutil.MustHaveTag(CommitQueueConfig{}, "BatchSize")
)

func (c *CommitQueueConfig) SectionId() string { return "commit_queue" }

func (c *CommitQueueConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()

	coll := env.DB().Collection(ConfigCollection)
	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = CommitQueueConfig{}
			return nil
		}

		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *CommitQueueConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			mergeTaskDistroKey:      c.MergeTaskDistro,
			committerNameKey:        c.CommitterName,
			committerEmailKey:       c.CommitterEmail,
			commitQueueBatchSizeKey: c.BatchSize,
		},
	}, options.Update().SetUpsert(true))
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *CommitQueueConfig) ValidateAndDefault() error {
	return nil
}
