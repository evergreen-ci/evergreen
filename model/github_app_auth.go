package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	githubAppAuthIdKey      = bsonutil.MustHaveTag(evergreen.GithubAppAuth{}, "Id")
	githubAppAuthAppIdKey   = bsonutil.MustHaveTag(evergreen.GithubAppAuth{}, "AppID")
	githubAppAuthPrivateKey = bsonutil.MustHaveTag(evergreen.GithubAppAuth{}, "PrivateKey")
)

const (
	GitHubAppAuthCollection = "github_app_auth"
)

// FindOneGithubAppAuth finds the github app auth for the given project id
func FindOneGithubAppAuth(projectId string) (*evergreen.GithubAppAuth, error) {
	githubAppAuth := &evergreen.GithubAppAuth{}
	err := db.FindOneQ(GitHubAppAuthCollection, byAppAuthID(projectId), githubAppAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return githubAppAuth, err
}

// byAppAuthID returns a query that finds a github app auth by the given identifier
// corresponding to the project id
func byAppAuthID(projectId string) db.Q {
	return db.Query(bson.M{githubAppAuthAppIdKey: projectId})
}

// GetGitHubAppID returns the app id for the given project id
func GetGitHubAppID(projectId string) (*int64, error) {
	githubAppAuth := &evergreen.GithubAppAuth{}

	q := byAppAuthID(projectId).WithFields(githubAppAuthIdKey)
	err := db.FindOneQ(GitHubAppAuthCollection, q, githubAppAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return &githubAppAuth.AppID, err
}

// UpsertGithubAppAuth inserts or updates the app auth for the given project id in the database
func UpsertGithubAppAuth(githubAppAuth *evergreen.GithubAppAuth) error {
	_, err := db.Upsert(
		GitHubAppAuthCollection,
		bson.M{
			githubAppAuthIdKey: githubAppAuth.Id,
		},
		bson.M{
			"$set": bson.M{
				githubAppAuthAppIdKey:   githubAppAuth.AppID,
				githubAppAuthPrivateKey: githubAppAuth.PrivateKey,
			},
		},
	)
	return err
}

// RemoveGithubAppAuth deletes the app auth for the given project id from the database
func RemoveGithubAppAuth(id string) error {
	return db.Remove(
		GitHubAppAuthCollection,
		bson.M{githubAppAuthIdKey: id},
	)
}
