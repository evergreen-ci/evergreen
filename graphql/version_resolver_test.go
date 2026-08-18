package graphql

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUpstreamProjectTaskRequiresTaskView(t *testing.T) {
	setupPermissions(t)
	require.NoError(t, db.ClearCollections(task.Collection, model.VersionCollection))

	ctx := getContext(t)
	usr, err := user.GetOrCreateUser(t.Context(), testUser, "User Name", "testuser@mongodb.com", "access_token", "refresh_token", []string{})
	require.NoError(t, err)
	ctx = gimlet.AttachUser(ctx, usr)

	upstreamTask := &task.Task{
		Id:      "upstream_task",
		Project: "unauthorized_project",
	}
	require.NoError(t, upstreamTask.Insert(t.Context()))
	downstreamVersion := &model.Version{
		Id:          "downstream_version",
		Requester:   evergreen.TriggerRequester,
		TriggerID:   upstreamTask.Id,
		TriggerType: model.ProjectTriggerLevelTask,
	}
	require.NoError(t, downstreamVersion.Insert(t.Context()))

	config := New("/graphql")
	res, err := config.Resolvers.Version().UpstreamProject(ctx, &restModel.APIVersion{
		Id:        utility.ToStringPtr(downstreamVersion.Id),
		Requester: utility.ToStringPtr(evergreen.TriggerRequester),
	})
	assert.Nil(t, res)
	require.EqualError(t, err, "input: user 'test_user' does not have permission to 'view tasks' for the project 'unauthorized_project'")
}
