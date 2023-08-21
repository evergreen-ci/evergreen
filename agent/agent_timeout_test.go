package agent

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/mock"
	"github.com/stretchr/testify/suite"
	"go.opentelemetry.io/otel"
)

type TimeoutSuite struct {
	suite.Suite
	a                *Agent
	mockCommunicator *client.Mock
	tmpFile          *os.File
	tmpFileName      string
	tmpDirName       string
	ctx              context.Context
	cancel           context.CancelFunc
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
			LogOutput:  LogOutputStdout,
			LogPrefix:  "agent",
		},
		comm:   client.NewMock("url"),
		tracer: otel.GetTracerProvider().Tracer("noop_tracer"),
	}
	s.mockCommunicator = s.a.comm.(*client.Mock)
	var err error

	s.tmpDirName = s.T().TempDir()
	s.tmpFile, err = os.CreateTemp(s.tmpDirName, "timeout")
	s.Require().NoError(err)

	s.tmpFileName = s.tmpFile.Name()
	s.mockCommunicator.TimeoutFilename = s.tmpFileName
	s.Require().NoError(s.tmpFile.Close())
	s.a.jasper, err = jasper.NewSynchronizedManager(false)
	s.Require().NoError(err)
	s.ctx, s.cancel = context.WithCancel(context.Background())
}

func (s *TimeoutSuite) TearDownTest() {
	s.cancel()
	s.Require().NoError(os.Remove(s.tmpFileName))
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
		taskConfig: &internal.TaskConfig{
			Task: &task.Task{
				Id:        taskID,
				Execution: 0,
			},
		},
		taskModel:     &task.Task{},
		ranSetupGroup: false,
		oomTracker:    &mock.OOMTracker{},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 0

	s.NoError(s.a.startLogging(s.ctx, tc))
	defer s.a.removeTaskDirectory(tc)
	_, err := s.a.runTask(s.ctx, tc)
	s.NoError(err)

	s.Require().NoError(tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, taskID, []string{
		"Hit exec timeout (1s)",
		"Running task-timeout commands",
		"Finished command 'shell.exec' in function 'timeout' (step 1 of 1) in block 'timeout'",
	}, nil)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal(evergreen.CommandTypeTest, detail.Type)
	s.Equal("'shell.exec' in function 'task' (step 1 of 1)", detail.Description)
	s.True(detail.TimedOut)
	s.Equal(1*time.Second, detail.TimeoutDuration)
	s.EqualValues(execTimeout, detail.TimeoutType)

	data, err := os.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

// TestExecTimeoutTask tests exec_timeout_secs set on a task. exec_timeout_secs
// has an effect only on a project or a task.
func (s *TimeoutSuite) TestExecTimeoutTask() {
	taskID := "exec_timeout_task"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		taskConfig: &internal.TaskConfig{
			Task: &task.Task{
				Id:        taskID,
				Execution: 0,
			},
		},
		taskModel:     &task.Task{},
		ranSetupGroup: false,
		oomTracker:    &mock.OOMTracker{},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 1

	s.NoError(s.a.startLogging(s.ctx, tc))
	defer s.a.removeTaskDirectory(tc)
	_, err := s.a.runTask(s.ctx, tc)
	s.NoError(err)

	s.Require().NoError(tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, taskID, []string{
		"Task completed - FAILURE.",
		"Hit exec timeout (1s).",
		"Running task-timeout commands.",
		"Finished command 'shell.exec' in function 'timeout' (step 1 of 1) in block 'timeout'",
	}, nil)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal(evergreen.CommandTypeTest, detail.Type)
	s.Equal("'shell.exec' in function 'task' (step 1 of 1)", detail.Description)
	s.True(detail.TimedOut)
	s.Equal(1*time.Second, detail.TimeoutDuration)
	s.EqualValues(execTimeout, detail.TimeoutType)

	data, err := os.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

// TestIdleTimeoutFunc tests timeout_secs set in a function.
func (s *TimeoutSuite) TestIdleTimeoutFunc() {
	taskID := "idle_timeout_func"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		taskConfig: &internal.TaskConfig{
			Task: &task.Task{
				Id:        taskID,
				Execution: 0,
			},
		},
		taskModel:     &task.Task{},
		ranSetupGroup: false,
		oomTracker:    &mock.OOMTracker{},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 2

	s.NoError(s.a.startLogging(s.ctx, tc))
	defer s.a.removeTaskDirectory(tc)
	_, err := s.a.runTask(s.ctx, tc)
	s.NoError(err)

	s.Require().NoError(tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, taskID, []string{
		"Task completed - FAILURE.",
		"Hit idle timeout (no message on stdout for more than 1s).",
		"Running task-timeout commands.",
		"Finished command 'shell.exec' in function 'timeout' (step 1 of 1) in block 'timeout'",
	}, nil)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("test", detail.Type)
	s.Equal("'shell.exec' in function 'task' (step 1 of 1)", detail.Description)
	s.True(detail.TimedOut)
	s.Equal(1*time.Second, detail.TimeoutDuration)
	s.EqualValues(idleTimeout, detail.TimeoutType)

	data, err := os.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

// TestIdleTimeout tests timeout_secs set on a function in a command.
func (s *TimeoutSuite) TestIdleTimeoutCommand() {
	taskID := "idle_timeout_task"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		taskConfig: &internal.TaskConfig{
			Task: &task.Task{
				Id:        taskID,
				Execution: 0,
			},
		},
		taskModel:     &task.Task{},
		ranSetupGroup: false,
		oomTracker:    &mock.OOMTracker{},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 3

	s.NoError(s.a.startLogging(s.ctx, tc))
	defer s.a.removeTaskDirectory(tc)
	_, err := s.a.runTask(s.ctx, tc)
	s.NoError(err)

	s.Require().NoError(tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, taskID, []string{
		"Task completed - FAILURE.",
		"Hit idle timeout (no message on stdout for more than 1s).",
		"Running task-timeout commands.",
		"Finished command 'shell.exec' in function 'timeout' (step 1 of 1) in block 'timeout'",
	}, nil)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("test", detail.Type)
	s.Equal("'shell.exec' in function 'task' (step 1 of 1)", detail.Description)
	s.True(detail.TimedOut)
	s.Equal(1*time.Second, detail.TimeoutDuration)
	s.EqualValues(idleTimeout, detail.TimeoutType)

	data, err := os.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

// TestDynamicIdleTimeout tests that the `update.timeout` command sets timeout_secs.
func (s *TimeoutSuite) TestDynamicIdleTimeout() {
	taskID := "dynamic_idle_timeout_task"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		taskConfig: &internal.TaskConfig{
			Task: &task.Task{
				Id:        taskID,
				Execution: 0,
			},
		},
		taskModel:     &task.Task{},
		ranSetupGroup: false,
		oomTracker:    &mock.OOMTracker{},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 3

	s.NoError(s.a.startLogging(s.ctx, tc))
	defer s.a.removeTaskDirectory(tc)
	_, err := s.a.runTask(s.ctx, tc)
	s.NoError(err)

	s.Require().NoError(tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, taskID, []string{
		"Hit idle timeout (no message on stdout for more than 2s).",
		"Running task-timeout commands",
		"Finished command 'shell.exec' in function 'timeout' (step 1 of 1) in block 'timeout'",
	}, nil)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("test", detail.Type)
	s.Equal("'shell.exec' in function 'task' (step 2 of 2)", detail.Description)
	s.True(detail.TimedOut)
	s.Equal(2*time.Second, detail.TimeoutDuration)
	s.EqualValues(idleTimeout, detail.TimeoutType)

	data, err := os.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}

// TestDynamicExecTimeout tests that the `update.timeout` command sets exec_timeout_secs.
func (s *TimeoutSuite) TestDynamicExecTimeoutTask() {
	taskID := "dynamic_exec_timeout_task"
	taskSecret := "mock_task_secret"
	tc := &taskContext{
		task: client.TaskData{
			ID:     taskID,
			Secret: taskSecret,
		},
		taskConfig: &internal.TaskConfig{
			Task: &task.Task{
				Id:        taskID,
				Execution: 0,
			},
		},
		taskModel:     &task.Task{},
		ranSetupGroup: false,
		oomTracker:    &mock.OOMTracker{},
	}
	// Windows may not have finished deleting task directories when
	// os.RemoveAll returns. Setting TaskExecution in this suite causes the
	// tests in this suite to create differently-named task directories.
	s.mockCommunicator.TaskExecution = 1

	s.NoError(s.a.startLogging(s.ctx, tc))
	defer s.a.removeTaskDirectory(tc)
	_, err := s.a.runTask(s.ctx, tc)
	s.NoError(err)

	s.Require().NoError(tc.logger.Close())
	checkMockLogs(s.T(), s.mockCommunicator, taskID, []string{
		"Hit exec timeout (2s)",
		"Task completed - FAILURE",
		"Running task-timeout commands",
		"Finished command 'shell.exec' in function 'timeout' (step 1 of 1) in block 'timeout'",
	}, nil)

	detail := s.mockCommunicator.GetEndTaskDetail()
	s.Equal(evergreen.TaskFailed, detail.Status)
	s.Equal("test", detail.Type)
	s.Equal("'shell.exec' in function 'task' (step 2 of 2)", detail.Description)
	s.True(detail.TimedOut)
	s.Equal(2*time.Second, detail.TimeoutDuration)
	s.EqualValues(execTimeout, detail.TimeoutType)

	data, err := os.ReadFile(s.tmpFileName)
	s.Require().NoError(err)
	s.Equal("timeout test message", strings.Trim(string(data), "\r\n"))

	taskData := s.mockCommunicator.EndTaskResult.TaskData
	s.Equal(taskID, taskData.ID)
	s.Equal(taskSecret, taskData.Secret)
}
