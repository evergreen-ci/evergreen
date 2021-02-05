package command

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type execCmdSuite struct {
	cancel func()
	conf   *internal.TaskConfig
	comm   client.Communicator
	logger client.LoggerProducer
	jasper jasper.Manager
	ctx    context.Context

	suite.Suite
}

func TestExecCmdSuite(t *testing.T) {
	suite.Run(t, new(execCmdSuite))
}

func (s *execCmdSuite) SetupSuite() {
	var err error
	s.jasper, err = jasper.NewSynchronizedManager(false)
	s.Require().NoError(err)
}

func (s *execCmdSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	var err error

	s.comm = client.NewMock("http://localhost.com")
	s.conf = &internal.TaskConfig{Expansions: &util.Expansions{}, Task: &task.Task{}, Project: &model.Project{}}
	s.logger, err = s.comm.GetLoggerProducer(s.ctx, client.TaskData{ID: s.conf.Task.Id, Secret: s.conf.Task.Secret}, nil)
	s.NoError(err)
}

func (s *execCmdSuite) TearDownTest() {
	s.cancel()
}

func (s *execCmdSuite) TestNoopExpansion() {
	cmd := &subprocessExec{
		WorkingDir: "foo",
		Binary:     "bar",
		Args:       []string{"a", "b"},
	}

	s.NoError(cmd.doExpansions(s.conf.Expansions))
	s.Equal("foo", cmd.WorkingDir)
	s.Equal("bar", cmd.Binary)
	s.Equal("a", cmd.Args[0])
	s.Equal("b", cmd.Args[1])
}

func (s *execCmdSuite) TestExpansionOfArgs() {

	cmd := &subprocessExec{
		Args: []string{
			"${foo|a}", "${foo|b}",
		},
	}

	s.NotEqual("a", cmd.Args[0])
	s.NotEqual("b", cmd.Args[1])

	s.NoError(cmd.doExpansions(s.conf.Expansions))
	s.Len(cmd.Args, 2)
	s.Equal("a", cmd.Args[0])
	s.Equal("b", cmd.Args[1])

}

func (s *execCmdSuite) TestExpansionOfEnvVarValues() {
	cmd := &subprocessExec{
		Env: map[string]string{
			"${foo|a}": "${foo|a}",
		},
	}

	s.NoError(cmd.doExpansions(s.conf.Expansions))
	v, ok := cmd.Env["${foo|a}"]
	s.True(ok)
	s.Equal("a", v)
}

func (s *execCmdSuite) TestWeirdAndBadExpansions() {
	cmd := &subprocessExec{
		WorkingDir: "fo${o",
		Binary:     "ba${sfdf${bar}f}}r",
		Args:       []string{"${foo|a}", "${bar|b}"},
	}

	s.Error(cmd.doExpansions(s.conf.Expansions))
	s.Equal("fo${o", cmd.WorkingDir)
	s.Equal("baf}}r", cmd.Binary)
	s.Equal("a", cmd.Args[0])
	s.Equal("b", cmd.Args[1])

}

func (s *execCmdSuite) TestParseParamsInitializesEnvMap() {
	cmd := &subprocessExec{}
	s.Nil(cmd.Env)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NotNil(cmd.Env)
}

func (s *execCmdSuite) TestErrorToIgnoreAndRedirectToStdOut() {
	cmd := &subprocessExec{}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	cmd.IgnoreStandardOutput = true
	cmd.RedirectStandardErrorToOutput = true
	s.Error(cmd.ParseParams(map[string]interface{}{}))

	cmd = &subprocessExec{}
	cmd.Silent = true
	cmd.RedirectStandardErrorToOutput = true
	s.Error(cmd.ParseParams(map[string]interface{}{}))
}

func (s *execCmdSuite) TestCommandParsing() {
	cmd := &subprocessExec{
		Command: "/bin/bash -c 'foo bar'",
	}
	s.Zero(cmd.Binary)
	s.Zero(cmd.Args)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.Len(cmd.Args, 2)
	s.NotZero(cmd.Binary)
	s.Equal("/bin/bash", cmd.Binary)
	s.Equal("-c", cmd.Args[0])
	s.Equal("foo bar", cmd.Args[1])
}

func (s *execCmdSuite) TestParseErrorIfTypeMismatch() {
	cmd := &subprocessExec{}
	s.Error(cmd.ParseParams(map[string]interface{}{"args": 1, "silent": "false"}))
	s.False(cmd.Background)
}

func (s *execCmdSuite) TestInvalidToSpecifyCommandInMultipleWays() {
	cmd := &subprocessExec{
		Command: "/bin/bash -c 'echo foo'",
		Binary:  "bash",
		Args: []string{
			"-c",
			"echo foo",
		},
	}
	s.Error(cmd.ParseParams(map[string]interface{}{}))
}

func (s *execCmdSuite) TestRunCommand() {
	cmd := &subprocessExec{
		Binary: "bash",
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec := cmd.getProc(s.ctx, "foo", s.logger)
	s.NoError(cmd.runCommand(s.ctx, "foo", exec, s.logger))
}

func (s *execCmdSuite) TestRunCommandPropgatesError() {
	cmd := &subprocessExec{
		Command: "bash -c 'exit 1'",
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec := cmd.getProc(s.ctx, "foo", s.logger)
	s.Error(cmd.runCommand(s.ctx, "foo", exec, s.logger))
}

func (s *execCmdSuite) TestRunCommandContinueOnErrorNoError() {
	cmd := &subprocessExec{
		Command:         "bash -c 'exit 1'",
		ContinueOnError: true,
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec := cmd.getProc(s.ctx, "foo", s.logger)
	s.NoError(cmd.runCommand(s.ctx, "foo", exec, s.logger))
}

func (s *execCmdSuite) TestRunCommandBackgroundAlwaysNil() {
	cmd := &subprocessExec{
		Command:    "bash -c 'exit 1'",
		Background: true,
		Silent:     true,
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	exec := cmd.getProc(s.ctx, "foo", s.logger)
	s.NoError(cmd.runCommand(s.ctx, "foo", exec, s.logger))
}

func (s *execCmdSuite) TestCommandFailsWithoutWorkingDirectorySet() {
	// this is a situation that won't happen in production code,
	// but should happen logicaly, but means if you don't specify
	// a directory and there's not one configured on the distro,
	// then you're in trouble.
	cmd := &subprocessExec{
		Command: "bash -c 'echo hello world!'",
	}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.Error(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *execCmdSuite) TestCommandIntegrationSimple() {
	cmd := &subprocessExec{
		Command:    "bash -c 'echo hello world!'",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *execCmdSuite) TestCommandIntegrationFailureExpansion() {
	cmd := &subprocessExec{
		Command:    "bash -c 'echo hello wor${ld!'",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	err := cmd.Execute(s.ctx, s.comm, s.logger, s.conf)
	if s.Error(err) {
		s.Contains(err.Error(), "problem expanding")
	}
}

func (s *execCmdSuite) TestCommandIntegrationFailureCase() {
	cmd := &subprocessExec{
		// just set up enough so that we don't fail parse params
		Env:        map[string]string{},
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.Error(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
}

func (s *execCmdSuite) TestExecuteErrorsIfCommandAborts() {
	cmd := &subprocessExec{
		Command:    "bash -c 'echo hello world!'",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)

	s.cancel()

	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	err := cmd.Execute(s.ctx, s.comm, s.logger, s.conf)
	if s.Error(err) {
		s.Contains(err.Error(), "aborted")
	}
}

func (s *execCmdSuite) TestKeepEmptyArgs() {
	// by default empty args should be stripped
	cmd := &subprocessExec{
		Command:    "echo ${foo|} bar",
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(cmd.Args, 1)

	// empty args should not be stripped if set
	cmd = &subprocessExec{
		Command:       "echo ${foo|} bar",
		WorkingDir:    testutil.GetDirectoryOfFile(),
		KeepEmptyArgs: true,
	}
	cmd.SetJasperManager(s.jasper)
	s.NoError(cmd.ParseParams(map[string]interface{}{}))
	s.NoError(cmd.Execute(s.ctx, s.comm, s.logger, s.conf))
	s.Len(cmd.Args, 2)
}

func (s *execCmdSuite) TestPathSetting() {
	cmd := &subprocessExec{
		// just set up enough so that we don't fail parse params
		Env:        map[string]string{},
		WorkingDir: testutil.GetDirectoryOfFile(),
		Path:       []string{"foo", "bar"},
	}
	exp := util.NewExpansions(map[string]string{})
	s.Len(cmd.Env, 0)
	s.NoError(cmd.doExpansions(exp))
	s.Len(cmd.Env, 1)

	path, ok := cmd.Env["PATH"]
	s.True(ok)
	s.Len(filepath.SplitList(path), len(filepath.SplitList(os.Getenv("PATH")))+2)
}

func (s *execCmdSuite) TestNoPathSetting() {
	cmd := &subprocessExec{
		// just set up enough so that we don't fail parse params
		Env:        map[string]string{},
		WorkingDir: testutil.GetDirectoryOfFile(),
	}
	exp := util.NewExpansions(map[string]string{})
	s.Len(cmd.Env, 0)
	s.NoError(cmd.doExpansions(exp))
	s.Len(cmd.Env, 0)

	path, ok := cmd.Env["PATH"]
	s.False(ok)
	s.Zero(path)
}

func (s *execCmdSuite) TestExpansionsEnvOptionDisabled() {
	cmd := &subprocessExec{
		Env:        map[string]string{},
		WorkingDir: testutil.GetDirectoryOfFile(),
	}

	s.NoError(cmd.doExpansions(util.NewExpansions(map[string]string{})))
	s.Len(cmd.Env, 0)
	cmd.Env["one"] = "one"
	s.NoError(cmd.doExpansions(util.NewExpansions(map[string]string{"two": "two"})))
	s.Len(cmd.Env, 1)
	s.NotEqual("two", cmd.Env["two"])
	s.Equal("one", cmd.Env["one"])
}

func (s *execCmdSuite) TestEnvIsSetAndDefaulted() {
	cmd := &subprocessExec{
		Binary:     "echo",
		Args:       []string{"hello", "world"},
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

func (s *execCmdSuite) TestEnvAddsExpansionsAndDefaults() {
	cmd := &subprocessExec{
		Binary:             "echo",
		Args:               []string{"hello", "world"},
		AddExpansionsToEnv: true,
		WorkingDir:         testutil.GetDirectoryOfFile(),
	}
	s.conf.Expansions = util.NewExpansions(map[string]string{
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

func TestAddTemp(t *testing.T) {
	for name, test := range map[string]func(*testing.T, map[string]string){
		"Empty": func(t *testing.T, env map[string]string) {
			addTempDirs(env, "")
			assert.Len(t, env, 3)
		},
		"WithAnExistingOther": func(t *testing.T, env map[string]string) {
			env["foo"] = "one"
			addTempDirs(env, "")
			assert.Len(t, env, 4)
		},
		"WithExistingTmp": func(t *testing.T, env map[string]string) {
			env["TMP"] = "foo"
			addTempDirs(env, "bar")
			assert.Len(t, env, 3)
			assert.Equal(t, "foo", env["TMP"])
			assert.Equal(t, "bar", env["TMPDIR"])
		},
		"CorrectKeys": func(t *testing.T, env map[string]string) {
			addTempDirs(env, "bar")
			assert.Len(t, env, 3)
			assert.Equal(t, "bar", env["TEMP"])
			assert.Equal(t, "bar", env["TMP"])
			assert.Equal(t, "bar", env["TMPDIR"])
		},
	} {
		t.Run(name, func(t *testing.T) {
			env := make(map[string]string)
			require.Len(t, env, 0)
			test(t, env)
		})
	}
}

func TestDefaultAndApplyExpansionsToEnv(t *testing.T) {
	for testName, testCase := range map[string]func(t *testing.T, exp util.Expansions){
		"SetsDefaultAndRequiredEnvWhenStandardValuesAreGiven": func(t *testing.T, exp util.Expansions) {
			opts := modifyEnvOptions{
				taskID:     "task_id",
				workingDir: "working_dir",
				tmpDir:     "tmp_dir",
			}
			env := defaultAndApplyExpansionsToEnv(map[string]string{}, opts)
			assert.Len(t, env, 7)
			assert.Equal(t, opts.taskID, env[agentutil.MarkerTaskID])
			assert.Contains(t, strconv.Itoa(os.Getpid()), env[agentutil.MarkerAgentPID])
			assert.Contains(t, opts.tmpDir, env["TEMP"])
			assert.Contains(t, opts.tmpDir, env["TMP"])
			assert.Contains(t, opts.tmpDir, env["TMPDIR"])
			assert.Equal(t, filepath.Join(opts.workingDir, ".gocache"), env["GOCACHE"])
			assert.Equal(t, "true", env["CI"])
		},
		"SetsDefaultAndRequiredEnvEvenWhenStandardValuesAreZero": func(t *testing.T, exp util.Expansions) {
			env := defaultAndApplyExpansionsToEnv(map[string]string{}, modifyEnvOptions{})
			assert.Len(t, env, 7)
			assert.Contains(t, env, agentutil.MarkerTaskID)
			assert.Contains(t, env, agentutil.MarkerAgentPID)
			assert.Contains(t, env, "TEMP")
			assert.Contains(t, env, "TMP")
			assert.Contains(t, env, "TMPDIR")
			assert.Equal(t, ".gocache", env["GOCACHE"])
			assert.Equal(t, "true", env["CI"])
		},
		"AgentEnvVarsOverridesExplicitlySetEnvVars": func(t *testing.T, exp util.Expansions) {
			opts := modifyEnvOptions{
				taskID: "real_task_id",
			}
			env := defaultAndApplyExpansionsToEnv(map[string]string{
				agentutil.MarkerAgentPID: "12345",
				agentutil.MarkerTaskID:   "fake_task_id",
			}, opts)
			assert.Equal(t, strconv.Itoa(os.Getpid()), env[agentutil.MarkerAgentPID])
			assert.Equal(t, opts.taskID, env[agentutil.MarkerTaskID])
		},
		"ExplicitlySetEnVVarsOverrideDefaultEnvVars": func(t *testing.T, exp util.Expansions) {
			gocache := "/path/to/gocache"
			ci := "definitely not Jenkins"
			tmpDir := "/some/tmpdir"
			env := defaultAndApplyExpansionsToEnv(map[string]string{
				"GOCACHE": gocache,
				"CI":      ci,
				"TEMP":    tmpDir,
				"TMP":     "/some/tmpdir",
				"TMPDIR":  tmpDir,
			}, modifyEnvOptions{tmpDir: "/tmp"})
			assert.Equal(t, gocache, env["GOCACHE"])
			assert.Equal(t, ci, env["CI"])
		},
		"AddExpansionsToEnvAddsAllExpansions": func(t *testing.T, exp util.Expansions) {
			env := defaultAndApplyExpansionsToEnv(map[string]string{
				"key1": "val1",
				"key2": "val2",
			}, modifyEnvOptions{
				expansions:         exp,
				addExpansionsToEnv: true,
			})
			assert.Equal(t, "val1", env["key1"])
			assert.Equal(t, "val2", env["key2"])
			for k, v := range exp.Map() {
				assert.Equal(t, v, env[k])
			}
		},
		"AddExpansionsToEnvOverridesDefaultEnvVars": func(t *testing.T, exp util.Expansions) {
			exp.Put("CI", "actually it's Jenkins")
			exp.Put("GOCACHE", "/path/to/gocache")
			env := defaultAndApplyExpansionsToEnv(map[string]string{}, modifyEnvOptions{
				expansions:         exp,
				addExpansionsToEnv: true,
			})
			for k, v := range exp.Map() {
				assert.Equal(t, v, env[k])
			}
		},
		"AgentEnvVarsOverrideExpansionsAddedToEnv": func(t *testing.T, exp util.Expansions) {
			agentEnvVars := map[string]string{
				agentutil.MarkerAgentPID: "12345",
				agentutil.MarkerTaskID:   "fake_task_id",
			}
			exp.Update(agentEnvVars)
			opts := modifyEnvOptions{
				taskID:             "task_id",
				expansions:         exp,
				addExpansionsToEnv: true,
			}
			env := defaultAndApplyExpansionsToEnv(map[string]string{}, opts)
			for k, v := range exp.Map() {
				if _, ok := agentEnvVars[k]; ok {
					continue
				}
				assert.Equal(t, v, env[k])
			}
			assert.Equal(t, opts.taskID, env[agentutil.MarkerTaskID])
			assert.Equal(t, strconv.Itoa(os.Getpid()), env[agentutil.MarkerAgentPID])
		},
		"IncludeExpansionsToEnvSelectivelyIncludesExpansions": func(t *testing.T, exp util.Expansions) {
			var include []string
			for k := range exp.Map() {
				include = append(include, k)
				break
			}
			opts := modifyEnvOptions{
				taskID:                 "task_id",
				expansions:             exp,
				includeExpansionsInEnv: include,
			}
			env := defaultAndApplyExpansionsToEnv(map[string]string{}, opts)
			for k, v := range exp.Map() {
				if utility.StringSliceContains(include, k) {
					assert.Equal(t, v, env[k])
				} else {
					_, ok := env[k]
					assert.False(t, ok)
				}
			}
		},
		"IncludeExpansionsInEnvOverridesDefaultEnvVars": func(t *testing.T, exp util.Expansions) {
			exp.Put("CI", "Travis")
			exp.Put("GOCACHE", "/path/to/gocache")
			include := []string{"GOCACHE", "CI"}
			opts := modifyEnvOptions{
				expansions:             exp,
				includeExpansionsInEnv: include,
			}
			env := defaultAndApplyExpansionsToEnv(map[string]string{}, opts)
			for k, v := range exp.Map() {
				if utility.StringSliceContains(include, k) {
					assert.Equal(t, v, env[k])
				} else {
					_, ok := env[k]
					assert.False(t, ok)
				}
			}
		},
		"AgentEnvVarsOverrideExpansionsIncludedInEnv": func(t *testing.T, exp util.Expansions) {
			agentEnvVars := map[string]string{
				agentutil.MarkerAgentPID: "12345",
				agentutil.MarkerTaskID:   "fake_task_id",
			}
			exp.Update(agentEnvVars)
			opts := modifyEnvOptions{
				taskID:                 "task_id",
				expansions:             exp,
				includeExpansionsInEnv: []string{agentutil.MarkerAgentPID, agentutil.MarkerTaskID},
			}
			env := defaultAndApplyExpansionsToEnv(map[string]string{}, opts)
			assert.Equal(t, opts.taskID, env[agentutil.MarkerTaskID])
			assert.Equal(t, strconv.Itoa(os.Getpid()), env[agentutil.MarkerAgentPID])
		},
		"IncludeExpansionsToEnvIgnoresNonexistentExpansions": func(t *testing.T, exp util.Expansions) {
			include := []string{"nonexistent1", "nonexistent2"}
			opts := modifyEnvOptions{
				expansions:             exp,
				includeExpansionsInEnv: include,
			}
			env := defaultAndApplyExpansionsToEnv(map[string]string{}, opts)
			for _, expName := range include {
				assert.NotContains(t, env, expName)
			}
		},
	} {
		t.Run(testName, func(t *testing.T) {
			exp := util.NewExpansions(map[string]string{
				"expansion1": "foo",
				"expansion2": "bar",
			})
			testCase(t, *exp)
		})
	}
}
