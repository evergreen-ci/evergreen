package evergreen

import (
	"context"

	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// GitHubRecoveryAppConfig holds the credentials for the GitHub app that
// Evergreen uses to recover from GitHub outages, separately from its main
// GitHub app.
type GitHubRecoveryAppConfig struct {
	AppID      int64  `bson:"app_id" json:"app_id" yaml:"app_id"`
	PrivateKey string `bson:"private_key" json:"private_key" yaml:"private_key" secret:"true"`
}

var (
	githubRecoveryAppAppIDKey      = bsonutil.MustHaveTag(GitHubRecoveryAppConfig{}, "AppID")
	githubRecoveryAppPrivateKeyKey = bsonutil.MustHaveTag(GitHubRecoveryAppConfig{}, "PrivateKey")
)

func (*GitHubRecoveryAppConfig) SectionId() string { return "github_recovery_app" }

func (c *GitHubRecoveryAppConfig) Get(ctx context.Context) error {
	return getConfigSection(ctx, c)
}

func (c *GitHubRecoveryAppConfig) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			githubRecoveryAppAppIDKey:      c.AppID,
			githubRecoveryAppPrivateKeyKey: c.PrivateKey,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *GitHubRecoveryAppConfig) ValidateAndDefault() error {
	catcher := grip.NewSimpleCatcher()
	catcher.NewWhen(c.AppID < 0, "GitHub recovery app ID cannot be negative")
	catcher.NewWhen(c.AppID != 0 && c.PrivateKey == "", "GitHub recovery app private key must be set if the app ID is set")
	return catcher.Resolve()
}
