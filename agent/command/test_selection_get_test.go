package command

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func setupTestEnv(t *testing.T) (context.Context, context.CancelFunc, *internal.TaskConfig, *client.Mock, client.LoggerProducer) {
	comm := client.NewMock("http://localhost.com")
	comm.SelectTestsResponse = []string{"test1", "test3"}
	conf := &internal.TaskConfig{
		Task: task.Task{
			Id:           "task_id",
			Secret:       "task_secret",
			Project:      "project_id",
			Version:      "version",
			BuildVariant: "build_variant",
			DisplayName:  "display_name",
		},
		ProjectRef: model.ProjectRef{Id: "project_id"},
	}
	tmpDir := t.TempDir()
	conf.WorkDir = tmpDir
	f, err := os.Create(filepath.Join(tmpDir, "test.json"))
	require.NoError(t, err)
	_ = f.Close()

	ctx, cancel := context.WithCancel(t.Context())
	logger, err := comm.GetLoggerProducer(ctx, &conf.Task, nil)
	require.NoError(t, err)

	conf.Task.TestSelectionEnabled = true
	conf.ProjectRef.TestSelection.Allowed = utility.TruePtr()
	return ctx, cancel, conf, comm, logger
}

func TestTestSelectionGetParseFailsWithMissingOutputFile(t *testing.T) {
	cmd := &testSelectionGet{}
	params := map[string]any{}
	require.Error(t, cmd.ParseParams(params))
}

func TestParseSucceedsWithValidParams(t *testing.T) {
	cmd := &testSelectionGet{}
	params := map[string]any{
		"output_file": "test.json",
	}
	require.NoError(t, cmd.ParseParams(params))
	assert.Equal(t, "test.json", cmd.OutputFile)
	assert.Empty(t, cmd.Tests)

	params["tests"] = []string{"test1", "test2"}
	require.NoError(t, cmd.ParseParams(params))
	assert.Equal(t, "test.json", cmd.OutputFile)
	assert.Equal(t, []string{"test1", "test2"}, cmd.Tests)
}

func TestSkipsWhenTestSelectionNotAllowed(t *testing.T) {
	ctx, cancel, conf, comm, logger := setupTestEnv(t)
	defer cancel()
	cmd := &testSelectionGet{OutputFile: "test.json"}

	// Test selection not allowed in project settings
	conf.ProjectRef.TestSelection.Allowed = utility.FalsePtr()
	require.NoError(t, cmd.Execute(ctx, comm, logger, conf))

	var output TestSelectionOutput
	require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
	assert.Empty(t, output.Tests)

	// Test selection allowed in project settings but not enabled for task
	conf.ProjectRef.TestSelection.Allowed = utility.TruePtr()
	conf.Task.TestSelectionEnabled = false
	require.NoError(t, cmd.Execute(ctx, comm, logger, conf))

	output = TestSelectionOutput{}
	require.NoError(t, utility.ReadJSONFile(cmd.OutputFile, &output))
	assert.Empty(t, output.Tests)
}

func TestCallsAPIWhenEnabled(t *testing.T) {
	ctx, cancel, conf, comm, logger := setupTestEnv(t)
	defer cancel()
	cmd := &testSelectionGet{OutputFile: "test.json", Tests: []string{"test1", "test3"}}

	require.NoError(t, cmd.Execute(ctx, comm, logger, conf))

	// Should return the expected tests from the mock API.
	data, err := os.ReadFile(cmd.OutputFile)
	require.NoError(t, err)
	var output TestSelectionOutput
	require.NoError(t, json.Unmarshal(data, &output))
	require.Len(t, output.Tests, 2)
	assert.Equal(t, "test1", output.Tests[0].Name)
	assert.Equal(t, "test3", output.Tests[1].Name)

	// Verify the API was called with correct parameters from TaskConfig
	assert.True(t, comm.SelectTestsCalled)
	assert.Equal(t, conf.Task.Project, comm.SelectTestsRequest.Project)
	assert.Equal(t, conf.Task.Requester, comm.SelectTestsRequest.Requester)
	assert.Equal(t, conf.Task.BuildVariant, comm.SelectTestsRequest.BuildVariant)
	assert.Equal(t, conf.Task.Id, comm.SelectTestsRequest.TaskID)
	assert.Equal(t, conf.Task.DisplayName, comm.SelectTestsRequest.TaskName)
	assert.Equal(t, []string{"test1", "test3"}, comm.SelectTestsRequest.Tests)
}

func TestHandlesAPIError(t *testing.T) {
	ctx, cancel, conf, comm, logger := setupTestEnv(t)
	defer cancel()
	cmd := &testSelectionGet{OutputFile: "test.json"}

	// Mock API error
	comm.SelectTestsError = errors.New("test error")

	err := cmd.Execute(ctx, comm, logger, conf)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "test error")
	assert.Contains(t, err.Error(), "calling test selection API")
}
