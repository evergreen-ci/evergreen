package model

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	IdKey         = bsonutil.MustHaveTag(evergreen.GithubAppAuth{}, "Id")
	AppIdKey      = bsonutil.MustHaveTag(evergreen.GithubAppAuth{}, "AppID")
	PrivateKeyKey = bsonutil.MustHaveTag(evergreen.GithubAppAuth{}, "PrivateKey")
)

const (
	GitHubAppAuthCollection = "github_app_auth"
)

// FindOneGithubAppAuth finds the github app auth for the given project id
func FindOneGithubAppAuth(projectId string) (*evergreen.GithubAppAuth, error) {
	githubAppAuth := &evergreen.GithubAppAuth{}
	q := db.Query(bson.M{IdKey: projectId})
	err := db.FindOneQ(GitHubAppAuthCollection, q, githubAppAuth)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return githubAppAuth, nil
}

// HasGithubAppAuth checks if the github app auth for the given project id exists
func HasGithubAppAuth(projectId string) (bool, error) {
	var app *evergreen.GithubAppAuth
	var err error
	if app, err = FindOneGithubAppAuth(projectId); err != nil {
		return false, err
	}

	return app != nil, nil
}

// UpsertGithubAppAuth inserts or updates the app auth for the given project id in the database
func UpsertGithubAppAuth(githubAppAuth *evergreen.GithubAppAuth) error {
	_, err := db.Upsert(
		GitHubAppAuthCollection,
		bson.M{
			IdKey: githubAppAuth.Id,
		},
		bson.M{
			"$set": bson.M{
				AppIdKey:      githubAppAuth.AppID,
				PrivateKeyKey: githubAppAuth.PrivateKey,
			},
		},
	)
	return err
}

// RemoveGithubAppAuth deletes the app auth for the given project id from the database
func RemoveGithubAppAuth(id string) error {
	return db.Remove(
		GitHubAppAuthCollection,
		bson.M{IdKey: id},
	)
}
