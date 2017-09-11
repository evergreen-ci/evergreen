package proto

import (
	"io/ioutil"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/stretchr/testify/suite"
	"golang.org/x/net/context"
)

type CommandTestSuite struct {
	suite.Suite
	a                Agent
	mockCommunicator *client.Mock
}

func TestCommandTestSuite(t *testing.T) {
	suite.Run(t, new(CommandTestSuite))
}

func (s *CommandTestSuite) SetupTest() {
	s.a = Agent{
		opts: Options{
			HostID:     "host",
			HostSecret: "secret",
			StatusPort: 2286,
			LogPrefix:  "prefix",
		},
		comm: client.NewMock("url"),
	}
	s.mockCommunicator = s.a.comm.(*client.Mock)
}

func (s *CommandTestSuite) TestShellExec() {
	f, err := ioutil.TempFile("/tmp", "shell-exec-")
	if err != nil {
		panic(err)
	}
	tmpFile := f.Name()
	s.mockCommunicator.ShellExecFilename = tmpFile
	if err = f.Close(); err != nil {
		panic(err)
	}

	ctx := context.Background()
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	taskID := "shellexec"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
	}
	err = s.a.resetLogging(ctx, tc)
	s.NoError(err)
	err = s.a.runTask(ctx, tc)
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
	if err != nil {
		panic(err)
	}
	s.Equal("shell.exec test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

func (s *CommandTestSuite) TestS3Copy() {
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
	err := s.a.resetLogging(ctx, tc)
	s.NoError(err)
	err = s.a.runTask(ctx, tc)
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
