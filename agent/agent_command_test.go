package agent

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/mongodb/jasper"
	"github.com/stretchr/testify/suite"
)

type CommandSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tmpDirName       string
}

func TestCommandSuite(t *testing.T) {
	suite.Run(t, new(CommandSuite))
}

func (s *CommandSuite) SetupTest() {
	s.a = &Agent{
		opts: Options{
			HostID:     "host",
			HostSecret: "secret",
			StatusPort: 2286,
			LogPrefix:  evergreen.LocalLoggingOverride,
		},
		comm: client.NewMock("url"),
	}
	s.mockCommunicator = s.a.comm.(*client.Mock)

	var err error
	s.tmpDirName, err = ioutil.TempDir("", "agent-command-suite-")
	s.Require().NoError(err)
	s.a.jasper, err = jasper.NewLocalManager(false)
	s.Require().NoError(err)
}

func (s *CommandSuite) TearDownTest() {
	s.Require().NoError(os.RemoveAll(s.tmpDirName))
}

func (s *CommandSuite) TestShellExec() {
	f, err := ioutil.TempFile(s.tmpDirName, "shell-exec-")
	s.Require().NoError(err)
	defer os.Remove(f.Name())

	tmpFile := f.Name()
	s.mockCommunicator.ShellExecFilename = tmpFile
	s.Require().NoError(f.Close())

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "shellexec"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		runGroupSetup: true,
	}
	s.NoError(s.a.resetLogging(ctx, tc))
	defer s.a.removeTaskDirectory(tc)
	_, err = s.a.runTask(ctx, cancel, tc)
	s.NoError(err)

	messages := s.mockCommunicator.GetMockMessages()
	s.Len(messages, 1)
	foundSuccessLogMessage := false
	foundShellLogMessage := false
	for _, msg := range messages[taskID] {
		if msg.Message == "Task completed - SUCCESS." {
			foundSuccessLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Finished 'shell.exec'") {
			foundShellLogMessage = true
		}
	}
	s.True(foundSuccessLogMessage)
	s.True(foundShellLogMessage)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal("success", detail.Status)
	s.Equal("test", detail.Type)
	s.Contains(detail.Description, "shell.exec")
	s.False(detail.TimedOut)

	data, err := ioutil.ReadFile(tmpFile)
	s.Require().NoError(err)
	s.Equal("shell.exec test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

func (s *CommandSuite) TestS3Copy() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "s3copy"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
	}
	s.NoError(s.a.resetLogging(ctx, tc))
	defer s.a.removeTaskDirectory(tc)
	_, err := s.a.runTask(ctx, cancel, tc)
	s.NoError(err)

	messages := s.mockCommunicator.GetMockMessages()
	s.Len(messages, 1)
	foundSuccessLogMessage := false
	foundS3CopyLogMessage := false
	for _, msg := range messages[taskID] {
		if msg.Message == "Task completed - SUCCESS." {
			foundSuccessLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Finished 's3Copy.copy'") {
			foundS3CopyLogMessage = true
		}
	}
	s.True(foundSuccessLogMessage)
	s.True(foundS3CopyLogMessage)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal("success", detail.Status)
	s.False(detail.TimedOut)

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}
