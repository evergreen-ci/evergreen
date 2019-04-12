package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2/bson"
)

// RepoTrackerConfig holds settings for polling project repositories.
type RepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int `bson:"revs_to_fetch" json:"revs_to_fetch" yaml:"numnewreporevisionstofetch"`
	MaxRepoRevisionsToSearch   int `bson:"max_revs_to_search" json:"max_revs_to_search" yaml:"maxreporevisionstosearch"`
	MaxConcurrentRequests      int `bson:"max_con_requests" json:"max_con_requests" yaml:"maxconcurrentrequests"`
}

func (c *RepoTrackerConfig) SectionId() string { return "repotracker" }

func (c *RepoTrackerConfig) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = RepoTrackerConfig{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *RepoTrackerConfig) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			"revs_to_fetch":      c.NumNewRepoRevisionsToFetch,
			"max_revs_to_search": c.MaxRepoRevisionsToSearch,
			"max_con_requests":   c.MaxConcurrentRequests,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *RepoTrackerConfig) ValidateAndDefault() error { return nil }
