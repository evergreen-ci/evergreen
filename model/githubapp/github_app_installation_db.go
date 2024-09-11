package githubapp

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// GitHubAppCollection contains information about Evergreen's GitHub app
// installations for internal use. This does not contain project-owned GitHub
// app credentials.
const GitHubAppCollection = "github_hooks"

//nolint:megacheck,unused
var (
	ownerKey = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Owner")
	repoKey  = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Repo")
	appIDKey = bsonutil.MustHaveTag(GitHubAppInstallation{}, "AppID")
)

func byAppOwnerRepo(appId int64, owner, repo string) bson.M {
	q := bson.M{
		ownerKey: owner,
		repoKey:  repo,
		appIDKey: appId,
	}
	return q
}

func validateOwnerRepo(appId int64, owner, repo string) error {
	if appId == 0 {
		return errors.New("App ID must not be 0")
	}

	if len(owner) == 0 || len(repo) == 0 {
		return errors.New("Owner and repository must not be empty strings")
	}
	return nil
}

// getInstallationID returns the cached installation ID for GitHub app from the database.
func getInstallationIDFromCache(ctx context.Context, app int64, owner, repo string) (int64, error) {
	if err := validateOwnerRepo(app, owner, repo); err != nil {
		return 0, err
	}

	installation := &GitHubAppInstallation{}
	res := evergreen.GetEnvironment().DB().Collection(GitHubAppCollection).FindOne(ctx, byAppOwnerRepo(app, owner, repo))
	if err := res.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return 0, nil
		}
		return 0, errors.Wrapf(err, "finding cached installation ID for '%s/%s", owner, repo)
	}
	if err := res.Decode(&installation); err != nil {
		return 0, errors.Wrapf(err, "decoding installation ID for '%s/%s", owner, repo)
	}

	return installation.InstallationID, nil
}
