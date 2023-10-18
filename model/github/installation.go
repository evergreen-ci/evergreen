package github

import (
	"context"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

const GitHubAppCollection = "github_hooks"

type GitHubAppInstallation struct {
	Owner string `bson:"owner"`
	Repo  string `bson:"repo"`

	// InstallationID is the GitHub app's installation ID for the owner/repo.
	InstallationID int64 `bson:"installation_id"`
}

//nolint:megacheck,unused
var (
	ownerKey          = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Owner")
	repoKey           = bsonutil.MustHaveTag(GitHubAppInstallation{}, "Repo")
	installationIDKey = bsonutil.MustHaveTag(GitHubAppInstallation{}, "InstallationID")
)

func byOwnerRepo(owner, repo string) bson.M {
	q := bson.M{
		ownerKey: owner,
		repoKey:  repo,
	}
	return q
}

func validateOwnerRepo(owner, repo string) error {
	if len(owner) == 0 || len(repo) == 0 {
		return errors.New("Owner and repository must not be empty strings")
	}
	return nil
}

// Upsert updates the installation information in the database.
func (h *GitHubAppInstallation) Upsert(ctx context.Context) error {
	if err := validateOwnerRepo(h.Owner, h.Repo); err != nil {
		return err
	}

	_, err := db.Upsert(GitHubAppCollection, byOwnerRepo(h.Owner, h.Repo), h)
	// _, err := GetEnvironment().DB().Collection(GitHubAppCollection).UpdateOne(
	// 	ctx,
	// 	byOwnerRepo(h.Owner, h.Repo),
	// 	bson.M{
	// 		"$set": h,
	// 	},
	// 	&options.UpdateOptions{
	// 		Upsert: utility.TruePtr(),
	// 	},
	// )
	return err
}

// GetInstallationID returns the installation ID for GitHub app if it's installed for the given owner/repo.
func GetInstallationID(ctx context.Context, owner, repo string) (int64, error) {
	if err := validateOwnerRepo(owner, repo); err != nil {
		return 0, err
	}

	installation := &GitHubAppInstallation{}
	err := db.FindOneQ(GitHubAppCollection, db.Query(byOwnerRepo(owner, repo)), installation)
	if adb.ResultsNotFound(err) {
		return 0, nil
	}
	// res := GetEnvironment().DB().Collection(GitHubAppCollection).FindOne(ctx, byOwnerRepo(owner, repo))
	// if err := res.Err(); err != nil {
	// 	if err == mongo.ErrNoDocuments {
	// 		return 0, nil
	// 	}
	// 	return 0, errors.Wrapf(err, "finding cached installation ID for '%s/%s", owner, repo)
	// }
	// if err := res.Decode(&installation); err != nil {
	// 	return 0, errors.Wrapf(err, "decoding installation ID for '%s/%s", owner, repo)
	// }

	return installation.InstallationID, nil
}
