package githubapp

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/utility"
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
	githubAppAuth := GithubAppAuth{
		Id:         "mongodb",
		AppID:      1234,
		PrivateKey: key,
	}
	err := UpsertGithubAppAuth(&githubAppAuth)
	require.NoError(t, err)

	appIDFromDB, err := GetGitHubAppID("mongodb")
	require.NoError(t, err)
	assert.Equal(int64(1234), utility.FromInt64Ptr(appIDFromDB))
}

func TestRemoveGithubAppAuth(t *testing.T) {
	assert := assert.New(t)
	require.NoError(t, db.Clear(GitHubAppAuthCollection),
		"Error clearing collection")

	key := []byte("I'm private")
	githubAppAuth := GithubAppAuth{
		Id:         "mongodb",
		AppID:      1234,
		PrivateKey: key,
	}
	err := UpsertGithubAppAuth(&githubAppAuth)
	require.NoError(t, err)

	githubAppAuthFromDB, err := FindOneGithubAppAuth("mongodb")
	require.NoError(t, err)
	assert.NotNil(githubAppAuthFromDB)

	err = RemoveGithubAppAuth(githubAppAuth.Id)
	require.NoError(t, err)

	githubAppAuthFromDB, err = FindOneGithubAppAuth("mongodb")
	require.NoError(t, err)
	assert.Nil(githubAppAuthFromDB)
}
