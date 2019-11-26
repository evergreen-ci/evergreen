package jasper

import (
	"context"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isInPath(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

func evgTaskContains(subs string) bool {
	return strings.Contains(os.Getenv("EVR_TASK_ID"), subs)
}

const doCleanTesting = false

func makeScriptingEnv(ctx context.Context, t *testing.T, mgr Manager, opts options.ScriptingEnvironment) ScriptingEnvironment {
	se, err := mgr.CreateScripting(ctx, opts)
	require.NoError(t, err)
	return se
}

func TestScriptingEnvironment(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager, err := NewSynchronizedManager(false)
	require.NoError(t, err)
	defer manager.Close(ctx)

	var tmpdir string
	if doCleanTesting {
		tmpdir, err = ioutil.TempDir("", "scripting_tests")
		require.NoError(t, err)
		defer func() {
			grip.Error(os.RemoveAll(tmpdir))
		}()
	} else {
		tmpdir = filepath.Join(testutil.GetDirectoryOfFile(), "build", "scripting-test")
	}

	output := options.Output{
		SendErrorToOutput: true,
		Loggers: []options.Logger{
			{
				Type: options.LogDefault,
				Options: options.Log{
					Format: options.LogFormatDefault,
					Level: send.LevelInfo{
						Threshold: level.Debug,
						Default:   level.Info,
					},
				},
			},
		},
	}

	type seTest struct {
		Name string
		Case func(*testing.T, options.ScriptingEnvironment)
	}

	for _, env := range []struct {
		Name           string
		Supported      bool
		DefaultOptions options.ScriptingEnvironment
		Tests          []seTest
	}{
		{
			Name:      "Roswell",
			Supported: isInPath("ros"),
			DefaultOptions: &options.ScriptingRoswell{
				Path:   filepath.Join(tmpdir, "roswell"),
				Lisp:   "sbcl-bin",
				Output: output,
			},
			Tests: []seTest{
				{
					Name: "Options",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						require.Equal(t, "ros", opts.Interpreter())
						require.NotZero(t, opts.ID())
					},
				},
				{
					Name: "HelloWorldScript",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.RunScript(ctx, `(defun main () (print "hello world"))`))
					},
				},
				{
					Name: "RunHelloWorld",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.Run(ctx, []string{`(print "hello world")`}))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.Error(t, se.RunScript(ctx, `(sb-ext:exit :code 42)`))
					},
				},
			},
		},
		{
			Name:      "Python3",
			Supported: isInPath("python3") && !evgTaskContains("ubuntu"),
			DefaultOptions: &options.ScriptingPython{
				VirtualEnvPath:        filepath.Join(tmpdir, "python3"),
				LegacyPython:          false,
				HostPythonInterpreter: "python3",
				Output:                output,
			},
			Tests: []seTest{
				{
					Name: "Options",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						require.True(t, strings.HasSuffix(opts.Interpreter(), "python"))
						require.NotZero(t, opts.ID())
					},
				},
				{
					Name: "HelloWorldScript",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.RunScript(ctx, `print("hello world")`))
					},
				},
				{
					Name: "RunHelloWorld",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.Run(ctx, []string{"-c", `print("hello world")`}))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.Error(t, se.RunScript(ctx, `exit(42)`))
					},
				},
			},
		},
		{
			Name:      "Python2",
			Supported: isInPath("python") && !evgTaskContains("windows"),
			DefaultOptions: &options.ScriptingPython{
				VirtualEnvPath:        filepath.Join(tmpdir, "python2"),
				LegacyPython:          true,
				HostPythonInterpreter: "python",
				Output:                output,
			},
			Tests: []seTest{
				{
					Name: "Options",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						require.True(t, strings.HasSuffix(opts.Interpreter(), "python"))
						require.NotZero(t, opts.ID())
					},
				},
				{
					Name: "HelloWorldScript",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.RunScript(ctx, `print("hello world")`))
					},
				},
				{
					Name: "RunHelloWorld",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.Run(ctx, []string{"-c", `print("hello world")`}))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.Error(t, se.RunScript(ctx, `exit(42)`))
					},
				},
			},
		},
		{
			Name:      "Golang",
			Supported: isInPath("go"),
			DefaultOptions: &options.ScriptingGolang{
				Gopath: filepath.Join(tmpdir, "gopath"),
				Goroot: runtime.GOROOT(),
				Packages: []string{
					"github.com/tychoish/tarjan",
					"github.com/alecthomas/gometalinter",
				},
				Output: output,
			},
			Tests: []seTest{
				{
					Name: "Options",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						require.True(t, strings.HasSuffix(opts.Interpreter(), "go"))
						require.NotZero(t, opts.ID())
					},
				},
				{
					Name: "HelloWorldScript",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.RunScript(ctx, `package main; import "fmt"; func main() { fmt.Println("Hello World")}`))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.Error(t, se.RunScript(ctx, `package main; import "os"; func main() { os.Exit(42) }`))
					},
				},
				{
					Name: "RunScript",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						if runtime.GOOS == "windows" {
							t.Skip("windows paths")
						}
						se := makeScriptingEnv(ctx, t, manager, opts)
						err := manager.CreateCommand(ctx).
							AddEnv("GOPATH", filepath.Join(tmpdir, "gopath")).
							Append("go install github.com/alecthomas/gometalinter").
							SetOutputOptions(output).Run(ctx)

						require.NoError(t, err)
						err = se.Run(ctx, []string{
							filepath.Join("cmd", "run-linter", "run-linter.go"),
							"--packages=mock",
							"--lintArgs='--disable-all'",
						})
						require.NoError(t, err)
					},
				},
				{
					Name: "Build",
					Case: func(t *testing.T, opts options.ScriptingEnvironment) {
						if runtime.GOOS == "windows" {
							t.Skip("windows paths")
						}
						se := makeScriptingEnv(ctx, t, manager, opts)
						err := se.Build(ctx, testutil.GetDirectoryOfFile(), []string{
							"-o", filepath.Join(tmpdir, "gopath", "bin", "run-linter"),
							"./cmd/run-linter",
						})
						require.NoError(t, err)
						_, err = os.Stat(filepath.Join(tmpdir, "gopath", "bin", "run-linter"))
						require.True(t, !os.IsNotExist(err))
					},
				},
			},
		},
	} {
		t.Run(env.Name, func(t *testing.T) {
			if !env.Supported {
				t.Skipf("%s is not supported in the current system", env.Name)
				return
			}
			require.NoError(t, env.DefaultOptions.Validate())
			t.Run("Config", func(t *testing.T) {
				start := time.Now()
				se := makeScriptingEnv(ctx, t, manager, env.DefaultOptions)
				dur := time.Since(start)
				require.NotNil(t, se)

				t.Run("ID", func(t *testing.T) {
					require.Equal(t, env.DefaultOptions.ID(), se.ID())
					assert.Len(t, se.ID(), 40)
				})
				t.Run("Caching", func(t *testing.T) {
					start := time.Now()
					require.NoError(t, se.Setup(ctx))
					assert.True(t, time.Since(start) < dur)
				})
			})
			for _, test := range env.Tests {
				t.Run(test.Name, func(t *testing.T) {
					test.Case(t, env.DefaultOptions)
				})
			}
			t.Run("Cleanup", func(t *testing.T) {
				if doCleanTesting {
					se := makeScriptingEnv(ctx, t, manager, env.DefaultOptions)
					require.NoError(t, se.Cleanup(ctx))
				}
			})

		})
	}
}
