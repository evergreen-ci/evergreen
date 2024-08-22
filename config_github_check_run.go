package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
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
	return getConfigSection(ctx, c)
}

// Set sets the document in the database to match the in-memory config struct.
func (c *GitHubCheckRunConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			checkRunLimitKey: c.CheckRunLimit,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

// ValidateAndDefault validates the check run configuration.
func (c *GitHubCheckRunConfig) ValidateAndDefault() error {
	if c.CheckRunLimit < 0 {
		return errors.New("check run limit must be greater than or equal to 0")
	}
	return nil
}
