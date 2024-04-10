package model

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	githubAppAuthIdKey      = bsonutil.MustHaveTag(GithubAppAuth{}, "Id")
	githubAppAuthAppId      = bsonutil.MustHaveTag(GithubAppAuth{}, "AppId")
	githubAppAuthPrivateKey = bsonutil.MustHaveTag(GithubAppAuth{}, "PrivateKey")
)

const (
	GitHubAppAuthCollection = "github_app_auth"
)

// GithubAppAuth hold the appId and privateKey for the github app associated with the project.
// It will not be stored along with the project settings, instead it is fetched only when needed
type GithubAppAuth struct {
	// Should match the identifier of the project it refers to
	Id string `bson:"_id" json:"_id"`

	AppId      int64  `bson:"app_id" json:"app_id"`
	PrivateKey []byte `bson:"private_key" json:"private_key"`
}

// FindOneGithubAppAuth finds the github app auth for the given project id
func FindOneGithubAppAuth(projectId string) (*GithubAppAuth, error) {
	githubAppAuth := &GithubAppAuth{}
	q := db.Query(bson.M{githubAppAuthIdKey: projectId})
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
	var app *GithubAppAuth
	var err error
	if app, err = FindOneGithubAppAuth(projectId); err != nil {
		return false, err
	}

	return app != nil, nil
}

// Upsert inserts or updates the app auth for the given project id in the database
func (githubAppAuth *GithubAppAuth) Upsert() error {
	_, err := db.Upsert(
		GitHubAppAuthCollection,
		bson.M{
			githubAppAuthIdKey: githubAppAuth.Id,
		},
		bson.M{
			"$set": bson.M{
				githubAppAuthAppId:      githubAppAuth.AppId,
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
