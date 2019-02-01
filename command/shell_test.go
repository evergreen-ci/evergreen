package command

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/stretchr/testify/suite"
)

type shellExecuteCommandSuite struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *model.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer
	shells []string

	suite.Suite
}

func TestShellExecuteCommand(t *testing.T) {
	suite.Run(t, new(shellExecuteCommandSuite))
}

func (s *shellExecuteCommandSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.comm = client.NewMock("http://localhost.com")
	s.conf = &model.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	s.logger = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret}, nil)

	s.shells = []string{"bash", "python", "sh"}

	if runtime.GOOS != "windows" {
		s.shells = append(s.shells, "/bin/sh", "/bin/bash", "/usr/bin/python")
	}
}

func (s *shellExecuteCommandSuite) TearDownTest() {
	s.cancel()
}

func (s *shellExecuteCommandSuite) TestWorksWithEmptyShell() {
	cmd := &shellExec{WorkingDir: testutil.GetDirectoryOfFile()}
	s.Empty(cmd.Shell)
	s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}


func (s *shellExecuteCommandSuite) TestSilentAndRedirectToStdOutError() {
	cmd := &subprocessExec{}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.False(cmd.IgnoreStandardError)
	s.False(cmd.IgnoreStandardOutput)
	cmd.Silent = true

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.True(cmd.IgnoreStandardError)
	s.True(cmd.IgnoreStandardOutput)
}

func (s *shellExecuteCommandSuite) TestShellIsntChangedDuringExecution() {
	for _, sh := range s.shells {
		cmd := &shellExec{Shell: sh, WorkingDir: testutil.GetDirectoryOfFile()}

		s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
		s.Equal(sh, cmd.Shell)
	}
}

func (s *shellExecuteCommandSuite) TestUnsetWorkedDirectoryFails() {
	cmd := &shellExec{}
	s.Error(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *shellExecuteCommandSuite) TestErrorIfWorkingDirectoryDoesntExist() {
	path := "foo/bar/baz"
	_, err := os.Stat(path)
	s.Error(err)
	s.True(os.IsNotExist(err))

	_, err = os.Stat(filepath.Join(s.conf.WorkDir))
	s.Error(err)
	s.True(os.IsNotExist(err))

	cmd := &shellExec{WorkingDir: path}
	s.Error(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *shellExecuteCommandSuite) TestCancellingContextShouldCancelCommand() {
	cmd := &shellExec{
		Script:     "sleep 60",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	ctx, cancel := context.WithTimeout(s.ctx, time.Nanosecond)
	time.Sleep(time.Millisecond)
	defer cancel()

	err := cmd.Execute(ctx, s.comm, s.logger, s.conf)
	s.Contains("context deadline exceeded", err.Error())
	s.NotContains("error while stopping process", err.Error())
}
