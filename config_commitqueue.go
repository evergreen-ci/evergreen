package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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

func (c *CommitQueueConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *CommitQueueConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			mergeTaskDistroKey:      c.MergeTaskDistro,
			committerNameKey:        c.CommitterName,
			committerEmailKey:       c.CommitterEmail,
			commitQueueBatchSizeKey: c.BatchSize,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *CommitQueueConfig) ValidateAndDefault() error {
	return nil
}
