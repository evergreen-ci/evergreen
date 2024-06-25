package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindOneGithubAppAuth(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.Clear(GitHubAppAuthCollection),
		"Error clearing collection")

	key := []byte("I'm private!")
	githubAppAuth := evergreen.GithubAppAuth{
		Id:         "mongodb",
		AppID:      1234,
		PrivateKey: key,
	}
	err := UpsertGithubAppAuth(&githubAppAuth)
	require.NoError(t, err)

	githubAppAuthFromDB, err := FindOneGithubAppAuth("mongodb")
	require.NoError(t, err)

	assert.Equal("mongodb", githubAppAuthFromDB.Id)
	assert.Equal(int64(1234), githubAppAuthFromDB.AppID)
	assert.Equal(key, githubAppAuthFromDB.PrivateKey)
}

func TestGetGitHubAppID(t *testing.T) {
	assert := assert.New(t)

	require.NoError(t, db.Clear(GitHubAppAuthCollection),
		"Error clearing collection")

	key := []byte("I'm private!")
	githubAppAuth := evergreen.GithubAppAuth{
		Id:         "mongodb",
		AppID:      1234,
		PrivateKey: key,
	}
	err := UpsertGithubAppAuth(&githubAppAuth)
	require.NoError(t, err)

	appIDFromDB, err := GetGitHubAppID("mongodb")
	require.NoError(t, err)
	assert.Equal("mongodb", appIDFromDB)
}

func TestRemoveGithubAppAuth(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.Clear(GitHubAppAuthCollection),
		"Error clearing collection")

	key := []byte("I'm private")
	githubAppAuth := evergreen.GithubAppAuth{
		Id:         "mongodb",
		AppID:      1234,
		PrivateKey: key,
	}
	err := UpsertGithubAppAuth(&githubAppAuth)
	require.NoError(t, err)

	hasGithubAppAuth, err := HasGithubAppAuth("mongodb")
	require.NoError(t, err)
	assert.True(hasGithubAppAuth)

	err = RemoveGithubAppAuth(githubAppAuth.Id)
	require.NoError(t, err)

	hasGithubAppAuth, err = HasGithubAppAuth("mongodb")
	require.NoError(t, err)
	assert.False(hasGithubAppAuth)

}
