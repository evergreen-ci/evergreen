package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// RepoTrackerConfig holds settings for polling project repositories.
type RepoTrackerConfig struct {
	NumNewRepoRevisionsToFetch int `bson:"revs_to_fetch" json:"revs_to_fetch" yaml:"revs_to_fetch"`
	MaxRepoRevisionsToSearch   int `bson:"max_revs_to_search" json:"max_revs_to_search" yaml:"max_revs_to_search"`
	MaxConcurrentRequests      int `bson:"max_con_requests" json:"max_con_requests" yaml:"max_concurrent_requests"`
}

func (c *RepoTrackerConfig) SectionId() string { return "repotracker" }

func (c *RepoTrackerConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *RepoTrackerConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			"revs_to_fetch":      c.NumNewRepoRevisionsToFetch,
			"max_revs_to_search": c.MaxRepoRevisionsToSearch,
			"max_con_requests":   c.MaxConcurrentRequests,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *RepoTrackerConfig) ValidateAndDefault() error { return nil }
