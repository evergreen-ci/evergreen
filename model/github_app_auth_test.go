package model

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen/cloud/parameterstore/fakeparameter"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TODO (DEVPROD-11883): move all these tests into the githubapp package once
// all project GitHub apps are using Parameter Store and the rollout is stable.
// The tests are only here temporarily to avoid a dependency cycle between the
// githubapp and model packages due to the project ref feature flag.

func TestUpsertGitHubAppAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(githubapp.GitHubAppAuthCollection, ProjectRefCollection))

	const projectID = "mongodb"
	pRef := ProjectRef{
		Id:                    projectID,
		ParameterStoreEnabled: true,
	}
	require.NoError(t, pRef.Insert())

	key := []byte("private_key")
	appAuth := &githubapp.GithubAppAuth{
		Id:         projectID,
		AppID:      1234,
		PrivateKey: key,
	}
	require.NoError(t, GitHubAppAuthUpsert(appAuth))

	dbAppAuth, err := GitHubAppAuthFindOne(projectID)
	require.NoError(t, err)
	require.NotZero(t, dbAppAuth)
	checkParameterMatchesPrivateKey(ctx, t, dbAppAuth)
	paramName := appAuth.PrivateKeyParameter

	appAuth.PrivateKey = []byte("new_private_key")
	require.NoError(t, GitHubAppAuthUpsert(appAuth))

	dbAppAuth, err = GitHubAppAuthFindOne(projectID)
	require.NoError(t, err)
	require.NotZero(t, dbAppAuth)
	checkParameterMatchesPrivateKey(ctx, t, dbAppAuth)
	assert.Equal(t, paramName, dbAppAuth.PrivateKeyParameter, "private key parameter name should remain the same on following upsert")
}

func TestRemoveGitHubAppAuth(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(ProjectRefCollection, githubapp.GitHubAppAuthCollection, fakeparameter.Collection))

	const projectID = "mongodb"
	pRef := ProjectRef{
		Id:                    projectID,
		ParameterStoreEnabled: true,
	}
	require.NoError(t, pRef.Insert())

	key := []byte("private_key")
	appAuth := &githubapp.GithubAppAuth{
		Id:         projectID,
		AppID:      1234,
		PrivateKey: key,
	}
	require.NoError(t, GitHubAppAuthUpsert(appAuth))

	dbAppAuth, err := GitHubAppAuthFindOne(projectID)
	require.NoError(t, err)
	require.NotZero(t, dbAppAuth)
	checkParameterMatchesPrivateKey(ctx, t, appAuth)
	paramName := appAuth.PrivateKeyParameter

	require.NoError(t, GitHubAppAuthRemove(dbAppAuth))

	dbAppAuth, err = GitHubAppAuthFindOne(projectID)
	assert.NoError(t, err)
	assert.Zero(t, dbAppAuth)

	fakeParams, err := fakeparameter.FindByIDs(ctx, paramName)
	assert.NoError(t, err)
	assert.Empty(t, fakeParams, "private key parameter should be deleted")
}

func checkParameterMatchesPrivateKey(ctx context.Context, t *testing.T, appAuth *githubapp.GithubAppAuth) {
	fakeParams, err := fakeparameter.FindByIDs(ctx, appAuth.PrivateKeyParameter)
	assert.NoError(t, err)
	require.Len(t, fakeParams, 1)
	assert.Equal(t, string(appAuth.PrivateKey), fakeParams[0].Value)
}
