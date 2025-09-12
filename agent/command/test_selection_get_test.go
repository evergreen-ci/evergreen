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
	"github.com/stretchr/testify/suite"
)

type TestSelectionGetSuite struct {
	cancel func()
	conf   *internal.TaskConfig
	comm   *client.Mock
	logger client.LoggerProducer
	ctx    context.Context

	cmd *testSelectionGet
	suite.Suite
}

func TestTestSelectionGetSuite(t *testing.T) {
	suite.Run(t, new(TestSelectionGetSuite))
}

func (s *TestSelectionGetSuite) SetupSuite() {
	s.comm = client.NewMock("http://localhost.com")
	s.comm.SelectTestsResponse = []string{"test1", "test3"}
	s.conf = &internal.TaskConfig{
		Task: task.Task{
			Id:           "task_id",
			Secret:       "task_secret",
			Project:      "project_id",
			Version:      "version",
			BuildVariant: "build_variant",
			DisplayName:  "display_name",
		},
		ProjectRef: model.ProjectRef{
			Id: "project_id",
		},
	}
	logger, err := s.comm.GetLoggerProducer(s.ctx, &s.conf.Task, nil)
	s.NoError(err)
	s.logger = logger

	tmpDir := s.T().TempDir()
	s.conf.WorkDir = tmpDir
	testFile, err := os.Create(filepath.Join(tmpDir, "test.json"))
	s.NoError(err)
	defer testFile.Close()
}

func (s *TestSelectionGetSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.conf.Task.TestSelectionEnabled = true
	s.conf.ProjectRef = model.ProjectRef{
		TestSelection: model.TestSelectionSettings{
			Allowed: utility.TruePtr(),
		},
	}
	s.cmd = &testSelectionGet{}
}

func (s *TestSelectionGetSuite) TearDownTest() {
	s.cancel()
}

func (s *TestSelectionGetSuite) TestParseFailsWithMissingOutputFile() {
	params := map[string]any{
		"project":       "test_project",
		"requester":     "patch_request",
		"build_variant": "test_variant",
		"task_id":       "task_123",
		"task_name":     "test_task",
	}
	s.Error(s.cmd.ParseParams(params))
}

func (s *TestSelectionGetSuite) TestParseSucceedsWithValidParams() {
	params := map[string]any{
		"output_file": "test.json",
	}
	s.NoError(s.cmd.ParseParams(params))
	s.Equal("test.json", s.cmd.OutputFile)
	s.Empty(s.cmd.Tests)

	params["tests"] = []string{"test1", "test2"}
	s.NoError(s.cmd.ParseParams(params))
	s.Equal("test.json", s.cmd.OutputFile)
	s.Equal([]string{"test1", "test2"}, s.cmd.Tests)
}

func (s *TestSelectionGetSuite) TestSkipsWhenTestSelectionNotAllowed() {
	s.cmd.OutputFile = "test.json"

	// Test selection not allowed in project settings
	s.conf.ProjectRef.TestSelection.Allowed = utility.FalsePtr()
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))

	output := TestSelectionOutput{}
	err := utility.ReadJSONFile(s.cmd.OutputFile, &output)
	s.NoError(err)
	s.Empty(output.Tests)

	// Test selection allowed in project settings but not enabled for task
	s.conf.ProjectRef.TestSelection.Allowed = utility.TruePtr()
	s.conf.Task.TestSelectionEnabled = false
	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))

	output = TestSelectionOutput{}
	err = utility.ReadJSONFile(s.cmd.OutputFile, &output)
	s.NoError(err)
	s.Empty(output.Tests)
}

func (s *TestSelectionGetSuite) TestCallsAPIWhenEnabled() {
	s.cmd.OutputFile = "test.json"
	s.cmd.Tests = []string{"test1", "test2"}

	s.NoError(s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf))

	// Should return the expected tests from the mock API.
	data, err := os.ReadFile(s.cmd.OutputFile)
	s.NoError(err)
	var output TestSelectionOutput
	s.NoError(json.Unmarshal(data, &output))
	s.Len(output.Tests, 2)
	s.Equal("test1", output.Tests[0]["name"])
	s.Equal("test3", output.Tests[1]["name"])

	// Verify the API was called with correct parameters from TaskConfig
	s.True(s.comm.SelectTestsCalled)
	s.Equal(s.conf.Task.Project, s.comm.SelectTestsRequest.Project)
	s.Equal(s.conf.Task.Requester, s.comm.SelectTestsRequest.Requester)
	s.Equal(s.conf.Task.BuildVariant, s.comm.SelectTestsRequest.BuildVariant)
	s.Equal(s.conf.Task.Id, s.comm.SelectTestsRequest.TaskID)
	s.Equal(s.conf.Task.DisplayName, s.comm.SelectTestsRequest.TaskName)
	s.Equal([]string{"test1", "test2"}, s.comm.SelectTestsRequest.Tests)
}

func (s *TestSelectionGetSuite) TestHandlesAPIError() {
	s.cmd.OutputFile = "test.json"

	// Mock API error
	s.comm.SelectTestsError = errors.New("test error")

	err := s.cmd.Execute(s.ctx, s.comm, s.logger, s.conf)
	s.Error(err)
	s.Contains(err.Error(), "calling test selection API")
}
