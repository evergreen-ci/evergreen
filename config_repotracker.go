package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RepoTrackerConfig holds settings for polling project repositories.
type RepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int `bson:"revs_to_fetch" json:"revs_to_fetch" yaml:"revs_to_fetch"`
	MaxRepoRevisionsToSearch   int `bson:"max_revs_to_search" json:"max_revs_to_search" yaml:"max_revs_to_search"`
	MaxConcurrentRequests      int `bson:"max_con_requests" json:"max_con_requests" yaml:"max_concurrent_requests"`
}

func (c *RepoTrackerConfig) SectionId() string { return "repotracker" }

func (c *RepoTrackerConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = RepoTrackerConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

func (c *RepoTrackerConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"revs_to_fetch":      c.NumNewRepoRevisionsToFetch,
			"max_revs_to_search": c.MaxRepoRevisionsToSearch,
			"max_con_requests":   c.MaxConcurrentRequests,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *RepoTrackerConfig) ValidateAndDefault() error { return nil }
