package graphql

import (
	"context"
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	testutil.Setup()
}

const projectId = "evergreen"

func getContext(t *testing.T) context.Context {
	require.NoError(t, db.Clear(user.Collection),
		"unable to clear user collection")
	dbUser := &user.DBUser{
		Id: testUser,
		Settings: user.UserSettings{
			SlackUsername: "testuser",
			SlackMemberId: "testuser",
		},
	}
	require.NoError(t, dbUser.Insert(t.Context()))
	const email = "testuser@mongodb.com"
	const accessToken = "access_token"
	const refreshToken = "refresh_token"
	ctx := context.Background()
	usr, err := user.GetOrCreateUser(testUser, "Mohamed Khelif", email, accessToken, refreshToken, []string{})
	require.NoError(t, err)
	assert.NotNil(t, usr)

	ctx = gimlet.AttachUser(ctx, usr)
	assert.NotNil(t, ctx)
	return ctx
}

func populateMainlineCommits(t *testing.T) {
	require.NoError(t, db.ClearCollections(model.VersionCollection, task.Collection))
	for i := 0; i < 12; i++ {
		versionId := fmt.Sprintf("v%d", i)
		// Every other version is activated
		isActivated := i%2 == 0
		requester := evergreen.RepotrackerVersionRequester
		if i%3 == 0 {
			requester = evergreen.GitTagRequester
		}
		v := &model.Version{
			Id:                  versionId,
			Requester:           requester,
			Activated:           utility.ToBoolPtr(isActivated),
			Branch:              projectId,
			RevisionOrderNumber: 12 - i,
			Identifier:          projectId,
		}
		require.NoError(t, v.Insert(t.Context()))
		if isActivated {
			// Every third version should have a task with a failure. This emulates filtering by task status
			hasFailure := i%3 == 0
			for j := 0; j < 10; j++ {
				aTask := &task.Task{
					Id:            fmt.Sprintf("t%d_%s", j, versionId),
					Version:       versionId,
					DisplayName:   fmt.Sprintf("%s_%d", "lint", j),
					DisplayTaskId: utility.ToStringPtr(""),
				}
				if hasFailure {
					aTask.Status = evergreen.TaskFailed
				} else {
					aTask.Status = evergreen.TaskSucceeded
				}
				require.NoError(t, aTask.Insert(t.Context()))
			}
		}
	}
}

func TestMainlineCommits(t *testing.T) {
	setupPermissions(t)
	populateMainlineCommits(t)
	config := New("/graphql")
	assert.NotNil(t, config)
	ctx := getContext(t)

	ref := model.ProjectRef{
		Id:         projectId,
		Identifier: "evergreen",
	}
	require.NoError(t, ref.Insert(t.Context()))

	// Should return all mainline commits while folding up inactive ones when there are no filters
	mainlineCommitOptions := MainlineCommitsOptions{
		ProjectIdentifier: "evergreen",
		SkipOrderNumber:   nil,
		Limit:             utility.ToIntPtr(2),
		ShouldCollapse:    utility.FalsePtr(),
	}
	buildVariantOptions := BuildVariantOptions{}
	res, err := config.Resolvers.Query().MainlineCommits(ctx, mainlineCommitOptions, &buildVariantOptions)
	require.NoError(t, err)
	assert.NotNil(t, res)

	require.Equal(t, 10, utility.FromIntPtr(res.NextPageOrderNumber))
	assert.Nil(t, res.PrevPageOrderNumber)
	require.Len(t, res.Versions, 3)

	buildVariantOptions = BuildVariantOptions{
		Statuses: []string{evergreen.TaskFailed},
	}

	mainlineCommitOptions.ShouldCollapse = utility.TruePtr()
	// Should return all mainline commits while folding up inactive/unmatching ones when there are filters and shouldCollapse is true
	res, err = config.Resolvers.Query().MainlineCommits(ctx, mainlineCommitOptions, &buildVariantOptions)
	require.NoError(t, err)
	assert.NotNil(t, res)

	require.Equal(t, 6, utility.FromIntPtr(res.NextPageOrderNumber))
	assert.Nil(t, res.PrevPageOrderNumber)
	require.Len(t, res.Versions, 3)

	assert.Nil(t, res.Versions[0].RolledUpVersions)
	assert.NotNil(t, res.Versions[0].Version)

	assert.NotNil(t, res.Versions[1].RolledUpVersions)
	require.Len(t, res.Versions[1].RolledUpVersions, 5)

	assert.NotNil(t, res.Versions[2].Version)

	lastCommit := res.Versions[len(res.Versions)-1].Version
	assert.NotNil(t, lastCommit)
	require.Equal(t, utility.FromIntPtr(res.NextPageOrderNumber), lastCommit.Order)

	mainlineCommitOptions.ShouldCollapse = utility.FalsePtr()
	// Should return all mainline commits without folding up unmatching ones when there are filters and shouldCollapse is false
	res, err = config.Resolvers.Query().MainlineCommits(ctx, mainlineCommitOptions, &buildVariantOptions)
	require.NoError(t, err)
	assert.NotNil(t, res)

	require.Equal(t, 10, utility.FromIntPtr(res.NextPageOrderNumber))
	assert.Nil(t, res.PrevPageOrderNumber)
	require.Len(t, res.Versions, 3)

	assert.Nil(t, res.Versions[0].RolledUpVersions)
	assert.NotNil(t, res.Versions[0].Version)

	assert.NotNil(t, res.Versions[1].RolledUpVersions)
	require.Len(t, res.Versions[1].RolledUpVersions, 1)

	assert.NotNil(t, res.Versions[2].Version)

	lastCommit = res.Versions[len(res.Versions)-1].Version
	assert.NotNil(t, lastCommit)
	require.Equal(t, utility.FromIntPtr(res.NextPageOrderNumber), lastCommit.Order)

	// Should only return mainline commits that match the passed in requester
	mainlineCommitOptions.Requesters = []string{evergreen.RepotrackerVersionRequester}
	res, err = config.Resolvers.Query().MainlineCommits(ctx, mainlineCommitOptions, &buildVariantOptions)
	require.NoError(t, err)
	assert.NotNil(t, res)

	require.Equal(t, 8, utility.FromIntPtr(res.NextPageOrderNumber))
	assert.Nil(t, res.PrevPageOrderNumber)
	require.Len(t, res.Versions, 3)

	for _, v := range res.Versions {
		if v.Version != nil {
			assert.Equal(t, evergreen.RepotrackerVersionRequester, utility.FromStringPtr(v.Version.Requester))
		}
	}
}

func TestImages(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	res, err := config.Resolvers.Query().Images(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, res)
}

func TestImage(t *testing.T) {
	config := New("/graphql")
	ctx := getContext(t)
	testConfig := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, testConfig)
	require.NoError(t, testConfig.RuntimeEnvironments.Set(ctx))
	res, err := config.Resolvers.Query().Image(ctx, "ubuntu2204")
	require.NoError(t, err)
	assert.NotEmpty(t, res)
}
