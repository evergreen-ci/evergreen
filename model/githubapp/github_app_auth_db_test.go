package githubapp

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpsertGitHubAppAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(GitHubAppAuthCollection))

	const projectID = "mongodb"

	key := []byte("private_key")
	appAuth := &GithubAppAuth{
		Id:         projectID,
		AppID:      1234,
		PrivateKey: key,
	}
	require.NoError(t, UpsertGitHubAppAuth(appAuth))

	dbAppAuth, err := FindOneGitHubAppAuth(projectID)
	require.NoError(t, err)
	require.NotZero(t, dbAppAuth)
	checkParameterMatchesPrivateKey(ctx, t, dbAppAuth)
	paramName := appAuth.PrivateKeyParameter

	appAuth.PrivateKey = []byte("new_private_key")
	require.NoError(t, UpsertGitHubAppAuth(appAuth))

	dbAppAuth, err = FindOneGitHubAppAuth(projectID)
	require.NoError(t, err)
	require.NotZero(t, dbAppAuth)
	checkParameterMatchesPrivateKey(ctx, t, dbAppAuth)
	assert.Equal(t, paramName, dbAppAuth.PrivateKeyParameter, "private key parameter name should remain the same on following upsert")
}

func TestRemoveGitHubAppAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(GitHubAppAuthCollection, fakeparameter.Collection))

	const projectID = "mongodb"
	key := []byte("private_key")
	appAuth := &GithubAppAuth{
		Id:         projectID,
		AppID:      1234,
		PrivateKey: key,
	}
	require.NoError(t, UpsertGitHubAppAuth(appAuth))

	dbAppAuth, err := FindOneGitHubAppAuth(projectID)
	require.NoError(t, err)
	require.NotZero(t, dbAppAuth)
	checkParameterMatchesPrivateKey(ctx, t, appAuth)
	paramName := appAuth.PrivateKeyParameter

	require.NoError(t, RemoveGitHubAppAuth(dbAppAuth))

	dbAppAuth, err = FindOneGitHubAppAuth(projectID)
	assert.NoError(t, err)
	assert.Zero(t, dbAppAuth)

	fakeParams, err := fakeparameter.FindByIDs(ctx, paramName)
	assert.NoError(t, err)
	assert.Empty(t, fakeParams, "private key parameter should be deleted")
}

func checkParameterMatchesPrivateKey(ctx context.Context, t *testing.T, appAuth *GithubAppAuth) {
	fakeParams, err := fakeparameter.FindByIDs(ctx, appAuth.PrivateKeyParameter)
	assert.NoError(t, err)
	require.Len(t, fakeParams, 1)
	assert.Equal(t, string(appAuth.PrivateKey), fakeParams[0].Value)
}
