package scripting

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

func makeScriptingEnv(ctx context.Context, t *testing.T, mgr Manager, opts options.ScriptingHarness) ScriptingHarness {
	se, err := mgr.CreateScripting(ctx, opts)
	require.NoError(t, err)
	return se
}

func TestScriptingHarness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager, err := NewSynchronizedManager(false)
	require.NoError(t, err)
	defer manager.Close(ctx)

	tmpdir, err := ioutil.TempDir(filepath.Join(testutil.GetDirectoryOfFile(), "build"), "scripting_tests")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tmpdir))
	}()

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
		Case func(*testing.T, options.ScriptingHarness)
	}

	for _, env := range []struct {
		Name           string
		Supported      bool
		DefaultOptions options.ScriptingHarness
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
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						require.Equal(t, "ros", opts.Interpreter())
						require.NotZero(t, opts.ID())
					},
				},
				{
					Name: "HelloWorldScript",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.RunScript(ctx, `(defun main () (print "hello world"))`))
					},
				},
				{
					Name: "RunHelloWorld",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.Run(ctx, []string{`(print "hello world")`}))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
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
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						require.True(t, strings.HasSuffix(opts.Interpreter(), "python"))
						require.NotZero(t, opts.ID())
					},
				},
				{
					Name: "HelloWorldScript",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.RunScript(ctx, `print("hello world")`))
					},
				},
				{
					Name: "RunHelloWorld",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.Run(ctx, []string{"-c", `print("hello world")`}))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
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
				Packages:              []string{"wheel"},
				Output:                output,
			},
			Tests: []seTest{
				{
					Name: "Options",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						require.True(t, strings.HasSuffix(opts.Interpreter(), "python"))
						require.NotZero(t, opts.ID())
					},
				},
				{
					Name: "HelloWorldScript",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.RunScript(ctx, `print("hello world")`))
					},
				},
				{
					Name: "RunHelloWorld",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.Run(ctx, []string{"-c", `print("hello world")`}))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
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
					"github.com/pkg/errors",
				},
				Output: output,
			},
			Tests: []seTest{
				{
					Name: "Options",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						require.True(t, strings.HasSuffix(opts.Interpreter(), "go"))
						require.NotZero(t, opts.ID())
					},
				},
				{
					Name: "HelloWorldScript",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.NoError(t, se.RunScript(ctx, `package main; import "fmt"; func main() { fmt.Println("Hello World")}`))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						require.Error(t, se.RunScript(ctx, `package main; import "os"; func main() { os.Exit(42) }`))
					},
				},
				{
					Name: "Dependencies",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						se := makeScriptingEnv(ctx, t, manager, opts)
						tmpFile := filepath.Join(tmpdir, "fake_script.go")
						require.NoError(t, ioutil.WriteFile(tmpFile, []byte(`package main; import ("fmt"; "github.com/pkg/errors"); func main() { fmt.Println(errors.New("error")) }`), 0755))
						defer func() {
							assert.NoError(t, os.Remove(tmpFile))
						}()
						err = se.Run(ctx, []string{tmpFile})
						require.NoError(t, err)
					},
				},
				{
					Name: "RunFile",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						if runtime.GOOS == "windows" {
							t.Skip("windows paths")
						}
						se := makeScriptingEnv(ctx, t, manager, opts)
						tmpFile := filepath.Join(tmpdir, "fake_script.go")
						require.NoError(t, ioutil.WriteFile(tmpFile, []byte(`package main; import "os"; func main() { os.Exit(0) }`), 0755))
						defer func() {
							assert.NoError(t, os.Remove(tmpFile))
						}()
						err = se.Run(ctx, []string{tmpFile})
						require.NoError(t, err)
					},
				},
				{
					Name: "Build",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						if runtime.GOOS == "windows" {
							t.Skip("windows paths")
						}
						se := makeScriptingEnv(ctx, t, manager, opts)
						tmpFile := filepath.Join(tmpdir, "fake_script.go")
						require.NoError(t, ioutil.WriteFile(tmpFile, []byte(`package main; import "os"; func main() { os.Exit(0) }`), 0755))
						defer func() {
							assert.NoError(t, os.Remove(tmpFile))
						}()
						_, err := se.Build(ctx, testutil.GetDirectoryOfFile(), []string{
							"-o", filepath.Join(tmpdir, "fake_script"),
							tmpFile,
						})
						require.NoError(t, err)
						_, err = os.Stat(filepath.Join(tmpFile))
						require.NoError(t, err)
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
				se := makeScriptingEnv(ctx, t, manager, env.DefaultOptions)
				require.NoError(t, se.Cleanup(ctx))
			})

		})
	}
}
