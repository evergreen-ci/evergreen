package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// RepoTrackerConfig holds settings for polling project repositories.
type RepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int `bson:"revs_to_fetch" json:"revs_to_fetch" yaml:"numnewreporevisionstofetch"`
	MaxRepoRevisionsToSearch   int `bson:"max_revs_to_search" json:"max_revs_to_search" yaml:"maxreporevisionstosearch"`
	MaxConcurrentRequests      int `bson:"max_con_requests" json:"max_con_requests" yaml:"maxconcurrentrequests"`
}

func (c *RepoTrackerConfig) SectionId() string { return "repotracker" }

func (c *RepoTrackerConfig) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = RepoTrackerConfig{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		return errors.Wrap(err, "problem decoding result")
	}

	return nil
}

func (c *RepoTrackerConfig) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"revs_to_fetch":      c.NumNewRepoRevisionsToFetch,
			"max_revs_to_search": c.MaxRepoRevisionsToSearch,
			"max_con_requests":   c.MaxConcurrentRequests,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *RepoTrackerConfig) ValidateAndDefault() error { return nil }
