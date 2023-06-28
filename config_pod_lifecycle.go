package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var podLifecycleConfigKey = bsonutil.MustHaveTag(Settings{}, "PodLifecycle") //nolint:unused

// PodLifecycleConfig holds logging settings for the pod init process.
type PodLifecycleConfig struct {
	S3BaseURL                   string `bson:"s3_base_url" json:"s3_base_url" yaml:"s3_base_url"`
	MaxParallelPodRequests      int    `bson:"max_parallel_pod_requests" json:"max_parallel_pod_requests" yaml:"max_parallel_pod_requests"`
	MaxPodDefinitionCleanupRate int    `bson:"max_pod_definition_cleanup_rate" json:"max_pod_definition_cleanup_rate" yam:"max_pod_definition_cleanup_rate"`
	MaxSecretCleanupRate        int    `bson:"max_secret_cleanup_rate" json:"max_secret_cleanup_rate" yaml:"max_secret_cleanup_rate"`
}

func (c *PodLifecycleConfig) SectionId() string { return "pod_lifecycle" }

func (c *PodLifecycleConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = PodLifecycleConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *PodLifecycleConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": c,
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *PodLifecycleConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	if c.MaxParallelPodRequests == 0 {
		// TODO: (EVG-16217) Determine empirically if this is indeed reasonable
		c.MaxParallelPodRequests = 2000
	}
	catcher.NewWhen(c.MaxParallelPodRequests < 0, "max parallel pod requests cannot be negative")
	return catcher.Resolve()
}
