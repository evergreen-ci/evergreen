package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	ghAuthIdKey         = bsonutil.MustHaveTag(githubapp.GithubAppAuth{}, "Id")
	ghAuthAppIdKey      = bsonutil.MustHaveTag(githubapp.GithubAppAuth{}, "AppID")
	ghAuthPrivateKeyKey = bsonutil.MustHaveTag(githubapp.GithubAppAuth{}, "PrivateKey")
)

const (
	GitHubAppAuthCollection = "github_app_auth"
)

// FindOneGithubAppAuth finds the github app auth for the given project id
func FindOneGithubAppAuth(projectId string) (*githubapp.GithubAppAuth, error) {
	githubAppAuth := &githubapp.GithubAppAuth{}
	err := db.FindOneQ(GitHubAppAuthCollection, byGithubAppAuthID(projectId), githubAppAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return githubAppAuth, err
}

// byGithubAppAuthID returns a query that finds a github app auth by the given identifier
// corresponding to the project id
func byGithubAppAuthID(projectId string) db.Q {
	return db.Query(bson.M{ghAuthIdKey: projectId})
}

// GetGitHubAppID returns the app id for the given project id
func GetGitHubAppID(projectId string) (*int64, error) {
	githubAppAuth := &githubapp.GithubAppAuth{}

	q := byGithubAppAuthID(projectId).WithFields(ghAuthAppIdKey)
	err := db.FindOneQ(GitHubAppAuthCollection, q, githubAppAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}

	return &githubAppAuth.AppID, err
}

// UpsertGithubAppAuth inserts or updates the app auth for the given project id in the database
func UpsertGithubAppAuth(githubAppAuth *githubapp.GithubAppAuth) error {
	_, err := db.Upsert(
		GitHubAppAuthCollection,
		bson.M{
			ghAuthIdKey: githubAppAuth.Id,
		},
		bson.M{
			"$set": bson.M{
				ghAuthAppIdKey:      githubAppAuth.AppID,
				ghAuthPrivateKeyKey: githubAppAuth.PrivateKey,
			},
		},
	)
	return err
}

// RemoveGithubAppAuth deletes the app auth for the given project id from the database
func RemoveGithubAppAuth(id string) error {
	return db.Remove(
		GitHubAppAuthCollection,
		bson.M{ghAuthIdKey: id},
	)
}
