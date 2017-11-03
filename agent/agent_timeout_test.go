package agent

import (
	"context"
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/stretchr/testify/suite"
)

type TimeoutSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tmpFile          *os.File
	tmpFileName      string
	tmpDirName       string
}

func TestTimeoutSuite(t *testing.T) {
	suite.Run(t, new(TimeoutSuite))
}

func (s *TimeoutSuite) SetupTest() {
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

	s.tmpDirName, err = ioutil.TempDir("", "agent-timeout-suite-")
	s.Require().NoError(err)
	s.tmpFile, err = ioutil.TempFile(s.tmpDirName, "timeout")
	s.Require().NoError(err)

	s.tmpFileName = s.tmpFile.Name()
	s.mockCommunicator.TimeoutFilename = s.tmpFileName
	s.Require().NoError(s.tmpFile.Close())

}

func (s *TimeoutSuite) TearDownTest() {
	s.Require().NoError(os.Remove(s.tmpFileName))
	s.Require().NoError(os.RemoveAll(s.tmpDirName))
}

// TestExecTimeoutProject tests exec_timeout_secs set on a project.
// exec_timeout_secs has an effect only on a project or a task.
func (s *TimeoutSuite) TestExecTimeoutProject() {
	taskID := "exec_timeout_project"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 0

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	err := s.a.resetLogging(ctx, tc)
	s.NoError(err)
	err = s.a.runTask(ctx, tc)
	s.NoError(err)

	messages := s.mockCommunicator.GetMockMessages()
	s.Len(messages, 1)
	foundSuccessLogMessage := false
	foundShellLogMessage := false
	foundTimeoutMessage := false
	for _, msg := range messages[taskID] {
		if msg.Message == "Task completed - FAILURE." {
			foundSuccessLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Hit exec timeout (1s)") {
			foundTimeoutMessage = true
		}
		if strings.HasPrefix(msg.Message, "Running task-timeout commands") {
			foundShellLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Finished 'shell.exec' in \"timeout\"") {
			foundShellLogMessage = true
		}
	}
	s.True(foundSuccessLogMessage)
	s.True(foundShellLogMessage)
	s.True(foundTimeoutMessage)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("test", detail.Type)
	s.Contains(detail.Description, "shell.exec")
	s.True(detail.TimedOut)

	data, err := ioutil.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

// TestExecTimeoutTask tests exec_timeout_secs set on a task. exec_timeout_secs
// has an effect only on a project or a task.
func (s *TimeoutSuite) TestExecTimeoutTask() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "exec_timeout_task"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 1

	err := s.a.resetLogging(ctx, tc)
	s.NoError(err)
	err = s.a.runTask(ctx, tc)
	s.NoError(err)

	messages := s.mockCommunicator.GetMockMessages()
	s.Len(messages, 1)
	foundSuccessLogMessage := false
	foundShellLogMessage := false
	foundTimeoutMessage := false
	for _, msg := range messages[taskID] {
		if msg.Message == "Task completed - FAILURE." {
			foundSuccessLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Hit exec timeout (1s)") {
			foundTimeoutMessage = true
		}
		if strings.HasPrefix(msg.Message, "Running task-timeout commands") {
			foundShellLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Finished 'shell.exec' in \"timeout\"") {
			foundShellLogMessage = true
		}
	}
	s.True(foundSuccessLogMessage)
	s.True(foundShellLogMessage)
	s.True(foundTimeoutMessage)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("test", detail.Type)
	s.Contains(detail.Description, "shell.exec")
	s.True(detail.TimedOut)

	data, err := ioutil.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

// TestIdleTimeoutFunc tests timeout_secs set in a function.
func (s *TimeoutSuite) TestIdleTimeoutFunc() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "idle_timeout_func"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 2

	err := s.a.resetLogging(ctx, tc)
	s.NoError(err)
	err = s.a.runTask(ctx, tc)
	s.NoError(err)

	messages := s.mockCommunicator.GetMockMessages()
	s.Len(messages, 1)
	foundSuccessLogMessage := false
	foundShellLogMessage := false
	foundTimeoutMessage := false
	for _, msg := range messages[taskID] {
		if msg.Message == "Task completed - FAILURE." {
			foundSuccessLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Hit idle timeout (no message on stdout for more than 1s)") {
			foundTimeoutMessage = true
		}
		if strings.HasPrefix(msg.Message, "Running task-timeout commands") {
			foundShellLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Finished 'shell.exec' in \"timeout\"") {
			foundShellLogMessage = true
		}
	}
	s.True(foundSuccessLogMessage)
	s.True(foundShellLogMessage)
	s.True(foundTimeoutMessage)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("test", detail.Type)
	s.Contains(detail.Description, "shell.exec")
	s.True(detail.TimedOut)

	data, err := ioutil.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

// TestIdleTimeout tests timeout_secs set on a function in a command.
func (s *TimeoutSuite) TestIdleTimeoutCommand() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	taskID := "idle_timeout_task"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 3

	err := s.a.resetLogging(ctx, tc)
	s.NoError(err)
	err = s.a.runTask(ctx, tc)
	s.NoError(err)

	messages := s.mockCommunicator.GetMockMessages()
	s.Len(messages, 1)
	foundSuccessLogMessage := false
	foundShellLogMessage := false
	foundTimeoutMessage := false
	for _, msg := range messages[taskID] {
		if msg.Message == "Task completed - FAILURE." {
			foundSuccessLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Hit idle timeout (no message on stdout for more than 1s)") {
			foundTimeoutMessage = true
		}
		if strings.HasPrefix(msg.Message, "Running task-timeout commands") {
			foundShellLogMessage = true
		}
		if strings.HasPrefix(msg.Message, "Finished 'shell.exec' in \"timeout\"") {
			foundShellLogMessage = true
		}
	}
	s.True(foundSuccessLogMessage)
	s.True(foundShellLogMessage)
	s.True(foundTimeoutMessage)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("test", detail.Type)
	s.Contains(detail.Description, "shell.exec")
	s.True(detail.TimedOut)

	data, err := ioutil.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}
