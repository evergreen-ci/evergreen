package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindOneGithubAppAuth(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.Clear(GitHubAppAuthCollection),
		"Error clearing collection")

	key := []byte("I'm private!")
	githubAppAuth := GithubAppAuth{
		Id:         "mongodb",
		AppId:      1234,
		PrivateKey: key,
	}
	change, err := githubAppAuth.Upsert()
	assert.NotNil(change)
	assert.NoError(err)
	assert.NotNil(change.UpsertedId)
	assert.Equal(1, change.Updated, "%+v", change)

	githubAppAuthFromDB, err := FindOneGithubAppAuth("mongodb")
	assert.NoError(err)

	assert.Equal("mongodb", githubAppAuthFromDB.Id)
	assert.Equal(int64(1234), githubAppAuthFromDB.AppId)
	assert.Equal(key, githubAppAuthFromDB.PrivateKey)
}

func TestRemove(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.Clear(GitHubAppAuthCollection),
		"Error clearing collection")

	key := []byte("I'm private")
	githubAppAuth := GithubAppAuth{
		Id:         "mongodb",
		AppId:      1234,
		PrivateKey: key,
	}
	_, err := githubAppAuth.Upsert()
	assert.NoError(err)

	githubAppAuthFromDB, err := FindOneGithubAppAuth("mongodb")
	assert.NoError(err)
	assert.NotNil(githubAppAuthFromDB)

	err = Remove(githubAppAuthFromDB.Id)

	assert.NoError(err)
	githubAppAuthFromDB, err = FindOneGithubAppAuth("mongodb")
	assert.NoError(err)
	assert.Nil(githubAppAuthFromDB)
}
