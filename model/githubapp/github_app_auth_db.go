package githubapp

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

const (
	// GitHubAppAuthCollection is the name of the collection that contains
	// GitHub app auth credentials.
	GitHubAppAuthCollection = "github_app_auth"
)

var (
	GhAuthIdKey                  = bsonutil.MustHaveTag(GithubAppAuth{}, "Id")
	GhAuthAppIdKey               = bsonutil.MustHaveTag(GithubAppAuth{}, "AppID")
	GhAuthPrivateKeyKey          = bsonutil.MustHaveTag(GithubAppAuth{}, "PrivateKey")
	GhAuthPrivateKeyParameterKey = bsonutil.MustHaveTag(GithubAppAuth{}, "PrivateKeyParameter")
)

// FindOneGithubAppAuth finds the github app auth for the given project or repo id
func FindOneGithubAppAuth(projectOrRepoId string) (*GithubAppAuth, error) {
	githubAppAuth := &GithubAppAuth{}
	err := db.FindOneQ(GitHubAppAuthCollection, byGithubAppAuthID(projectOrRepoId), githubAppAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return githubAppAuth, err
}

// byGithubAppAuthID returns a query that finds a github app auth by the given identifier
// corresponding to the project id
func byGithubAppAuthID(projectId string) db.Q {
	return db.Query(bson.M{GhAuthIdKey: projectId})
}

// GetGitHubAppID returns the app id for the given project id
func GetGitHubAppID(projectId string) (*int64, error) {
	githubAppAuth := &GithubAppAuth{}

	q := byGithubAppAuthID(projectId).WithFields(GhAuthAppIdKey)
	err := db.FindOneQ(GitHubAppAuthCollection, q, githubAppAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return &githubAppAuth.AppID, err
}

// UpsertGithubAppAuth inserts or updates the app auth for the given project id in the database
func UpsertGithubAppAuth(githubAppAuth *GithubAppAuth) error {
	_, err := db.Upsert(
		GitHubAppAuthCollection,
		bson.M{
			GhAuthIdKey: githubAppAuth.Id,
		},
		bson.M{
			"$set": bson.M{
				GhAuthAppIdKey:               githubAppAuth.AppID,
				GhAuthPrivateKeyKey:          githubAppAuth.PrivateKey,
				GhAuthPrivateKeyParameterKey: githubAppAuth.PrivateKeyParameter,
			},
		},
	)
	return err
}

// RemoveGithubAppAuth deletes the app auth for the given project id from the database
func RemoveGithubAppAuth(id string) error {
	return db.Remove(
		GitHubAppAuthCollection,
		bson.M{GhAuthIdKey: id},
	)
}
