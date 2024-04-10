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
	err := githubAppAuth.Upsert()

	require.NoError(t, err)
	githubAppAuthFromDB, err := FindOneGithubAppAuth("mongodb")
	require.NoError(t, err)

	assert.Equal("mongodb", githubAppAuthFromDB.Id)
	assert.Equal(int64(1234), githubAppAuthFromDB.AppId)
	assert.Equal(key, githubAppAuthFromDB.PrivateKey)
}

func TestRemoveGithubAppAuth(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.Clear(GitHubAppAuthCollection),
		"Error clearing collection")

	key := []byte("I'm private")
	githubAppAuth := GithubAppAuth{
		Id:         "mongodb",
		AppId:      1234,
		PrivateKey: key,
	}
	err := githubAppAuth.Upsert()
	require.NoError(t, err)

	githubAppAuthFromDB, err := FindOneGithubAppAuth("mongodb")
	require.NoError(t, err)
	assert.NotNil(githubAppAuthFromDB)

	err = RemoveGithubAppAuth(githubAppAuthFromDB.Id)

	require.NoError(t, err)
	githubAppAuthFromDB, err = FindOneGithubAppAuth("mongodb")
	require.NoError(t, err)
	assert.Nil(githubAppAuthFromDB)
}
