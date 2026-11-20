package data

import (
	"encoding/json"
	"testing"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTaskQuarantineMetadata(t *testing.T) {
	t.Run("TaskWithJiraAssignmentTagPopulatesAllFields", func(t *testing.T) {
		tsk := &task.Task{Id: "task/1", Execution: 2, Tags: []string{"release_critical", "assigned_to_jira_team_devprod_evergreen"}}

		metadata := NewTaskQuarantineMetadata(tsk, "me", "https://spruce.example.com")

		assert.Equal(t, "me", metadata.InitiatedBy)
		assert.Equal(t, "https://spruce.example.com/task/task%2F1/2", metadata.TaskInitiatedFrom)
		assert.Equal(t, "assigned_to_jira_team_devprod_evergreen", metadata.JiraAssignTo)
	})
	t.Run("TaskWithoutJiraAssignmentTagLeavesAssigneeEmpty", func(t *testing.T) {
		tsk := &task.Task{Id: "task", Tags: []string{"release_critical"}}

		metadata := NewTaskQuarantineMetadata(tsk, "me", "https://spruce.example.com")

		assert.Empty(t, metadata.JiraAssignTo)
	})
}

func TestQuarantineMetadataToClientOmitsEmptyFields(t *testing.T) {
	raw, err := json.Marshal(QuarantineMetadata{InitiatedBy: "me"}.toClient())
	require.NoError(t, err)

	assert.JSONEq(t, `{"initiated_by": "me"}`, string(raw))
}
