package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// GitHubCheckRunConfig configures settings for the GitHub Check Run API.
type GitHubCheckRunConfig struct {
	// CheckRunLimit is the number of check runs that Evergreen is willing to support for each patch created by GitHub PRs.
	CheckRunLimit int `bson:"check_run_limit" json:"check_run_limit" yaml:"check_run_limit"`
}

// SectionId returns the ID of this config section.
func (c *GitHubCheckRunConfig) SectionId() string { return "github_check_run" }

// Get populates the config from the database.
func (c *GitHubCheckRunConfig) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = GitHubCheckRunConfig{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}

	return nil
}

// Set sets the document in the database to match the in-memory config struct.
func (c *GitHubCheckRunConfig) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			checkRunLimitKey: c.CheckRunLimit,
		},
	}, options.Update().SetUpsert(true))
	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

// ValidateAndDefault validates the check run configuration.
func (c *GitHubCheckRunConfig) ValidateAndDefault() error {
	if c.CheckRunLimit < 0 {
		return errors.New("check run limit must be greater than or equal to 0")
	}
	return nil
}
