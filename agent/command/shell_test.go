package command

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	agentutil "github.com/evergreen-ci/evergreen/agent/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/suite"
)

type shellExecuteCommandSuite struct {
	ctx    context.Context
	cancel context.CancelFunc
	conf   *internal.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer

	jasper jasper.Manager

	suite.Suite
}

func TestShellExecuteCommand(t *testing.T) {
	suite.Run(t, new(shellExecuteCommandSuite))
}

func (s *shellExecuteCommandSuite) SetupSuite() {
	var err error
	s.jasper, err = jasper.NewSynchronizedManager(false)
	s.Require().NoError(err)
}

func (s *shellExecuteCommandSuite) SetupTest() {
	var err error
	s.ctx, s.cancel = context.WithCancel(context.Background())

	s.comm = client.NewMock("http://localhost.com")
	s.conf = &internal.TaskConfig{
		Expansions: util.Expansions{},
		Task: task.Task{
			Id:     "task_id",
			Secret: "task_secret",
		},
		Project: model.Project{},
	}
	s.logger, err = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret}, nil)
	s.NoError(err)
}

func (s *shellExecuteCommandSuite) TearDownTest() {
	s.cancel()
}

func (s *shellExecuteCommandSuite) TestWorksWithEmptyShell() {
	cmd := &shellExec{
		WorkingDir: testutil.GetDirectoryOfFile(),
		Script:     "exit 0",
	}
	cmd.SetJasperManager(s.jasper)
	s.Empty(cmd.Shell)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NotEmpty(cmd.Shell)
	s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *shellExecuteCommandSuite) TestSilentAndRedirectToStdOutError() {
	cmd := &shellExec{
		Script: "exit 0",
	}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.False(cmd.IgnoreStandardError)
	s.False(cmd.IgnoreStandardOutput)
	cmd.Silent = true

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.True(cmd.IgnoreStandardError)
	s.True(cmd.IgnoreStandardOutput)
}

func (s *shellExecuteCommandSuite) TestTerribleQuotingIsHandledProperly() {
	for idx, script := range []string{
		"echo \"hi\"; exit 0",
		"echo '\"hi\"'; exit 0",
		`echo '"'; exit 0`,
		`echo "'"; exit 0`,
		`echo \'; exit 0`,
		`process_kill_list="(^cl\.exe$|bsondump|java|lein|lldb|mongo|python|_test$|_test\.exe$)"
		process_exclude_list="(main|tuned|evergreen|go|godoc|gocode|make)"

		if [ "Windows_NT" = "$OS" ]; then
			processes=$(tasklist /fo:csv | awk -F'","' '{x=$1; gsub("\"","",x); print $2, x}' | grep -iE "$process_kill_list" | grep -ivE "$process_exclude_list")
			kill_process () { pid=$(echo $1 | cut -f1 -d ' '); echo "Killing process $1"; taskkill /pid "$pid" /f; }
		else
			pgrep -f --list-full ".*" 2>&1 | grep -qE "(illegal|invalid|unrecognized) option"
			if [ $? -ne 0 ]; then
				pgrep_list=$(pgrep -f --list-full "$process_kill_list")
			else
				pgrep_list=$(pgrep -f -l "$process_kill_list")
			fi

			processes=$(echo "$pgrep_list" | grep -ivE "$process_exclude_list" | sed -e '/^ *[0-9]/!d; s/^ *//; s/[[:cntrl:]]//g;')
			kill_process () { pid=$(echo $1 | cut -f1 -d ' '); echo "Killing process $1"; kill -9 $pid; }
		fi
		IFS=$(printf "\n\r")
		for process in $processes
		do
			kill_process "$process"
		done

		exit 0
`,
	} {
		cmd := &shellExec{
			WorkingDir: testutil.GetDirectoryOfFile(),
			Shell:      "bash",
		}
		cmd.SetJasperManager(s.jasper)
		cmd.Script = script
		s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf), "%d: %s", idx, script)
	}
}

func (s *shellExecuteCommandSuite) TestShellIsNotChangedDuringExecution() {
	shells := []string{"bash", "python", "sh"}
	if runtime.GOOS != "windows" {
		shells = append(shells, "/bin/sh", "/bin/bash", "/usr/bin/python")
	}
	for _, sh := range shells {
		cmd := &shellExec{Shell: sh, WorkingDir: testutil.GetDirectoryOfFile()}
		cmd.SetJasperManager(s.jasper)
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
		Shell:      "bash",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)
	ctx, cancel := context.WithTimeout(s.ctx, time.Nanosecond)
	defer cancel()

	time.Sleep(100 * time.Millisecond)

	err := cmd.Execute(ctx, s.comm, s.logger, s.conf)
	s.Require().NotNil(err)
	s.True(utility.IsContextError(errors.Cause(err)))
}

func (s *shellExecuteCommandSuite) TestEnvIsSetAndDefaulted() {
	cmd := &shellExec{
		Script:     "echo hello world",
		Shell:      "bash",
		Env:        map[string]string{"foo": "bar"},
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)
	ctx, cancel := context.WithTimeout(s.ctx, time.Second)
	defer cancel()
	s.Require().NoError(cmd.Execute(ctx, s.comm, s.logger, s.conf))
	s.Len(cmd.Env, 8)
	s.Contains(cmd.Env, agentutil.MarkerTaskID)
	s.Contains(cmd.Env, agentutil.MarkerAgentPID)
	s.Contains(cmd.Env, "TEMP")
	s.Contains(cmd.Env, "TMP")
	s.Contains(cmd.Env, "TMPDIR")
	s.Contains(cmd.Env, "GOCACHE")
	s.Contains(cmd.Env, "CI")
	s.Contains(cmd.Env, "foo")
}

func (s *shellExecuteCommandSuite) TestEnvAddsExpansionsAndDefaults() {
	cmd := &shellExec{
		Script:             "echo hello world",
		Shell:              "bash",
		AddExpansionsToEnv: true,
		WorkingDir:         testutil.GetDirectoryOfFile(),
	}
	s.conf.Expansions = *util.NewExpansions(map[string]string{
		"expansion1": "foo",
		"expansion2": "bar",
	})
	cmd.SetJasperManager(s.jasper)
	ctx, cancel := context.WithTimeout(s.ctx, time.Second)
	defer cancel()
	s.Require().NoError(cmd.Execute(ctx, s.comm, s.logger, s.conf))
	s.Len(cmd.Env, 9)
	s.Contains(cmd.Env, agentutil.MarkerTaskID)
	s.Contains(cmd.Env, agentutil.MarkerAgentPID)
	s.Contains(cmd.Env, "TEMP")
	s.Contains(cmd.Env, "TMP")
	s.Contains(cmd.Env, "TMPDIR")
	s.Contains(cmd.Env, "GOCACHE")
	s.Contains(cmd.Env, "CI")
	for k, v := range s.conf.Expansions.Map() {
		s.Equal(v, cmd.Env[k])
	}
}

func (s *shellExecuteCommandSuite) TestFailingShellCommandErrors() {
	cmd := &shellExec{
		Script:     "exit 1",
		Shell:      "bash",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)
	err := cmd.Execute(s.ctx, s.comm, s.logger, s.conf)
	s.Require().NotNil(err)
	s.Contains(err.Error(), "shell script encountered problem: exit code 1")
}
