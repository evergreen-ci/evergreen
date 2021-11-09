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
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func isInPath(binary string) bool {
	_, err := exec.LookPath(binary)
	return err == nil
}

func makeScriptingHarness(ctx context.Context, t *testing.T, mgr jasper.Manager, opts options.ScriptingHarness) Harness {
	se, err := NewHarness(mgr, opts)
	require.NoError(t, err)
	return se
}

func TestScriptingHarness(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	manager, err := jasper.NewSynchronizedManager(false)
	require.NoError(t, err)
	defer manager.Close(ctx)

	tmpdir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, os.RemoveAll(tmpdir))
	}()

	opts := &options.DefaultLoggerOptions{
		Base: options.BaseOptions{
			Format: options.LogFormatDefault,
			Level: send.LevelInfo{
				Threshold: level.Debug,
				Default:   level.Info,
			},
		},
	}
	logger := &options.LoggerConfig{}
	require.NoError(t, logger.Set(opts))
	output := options.Output{
		SendErrorToOutput: true,
		Loggers:           []*options.LoggerConfig{logger},
	}

	type shTest struct {
		Name string
		Case func(*testing.T, options.ScriptingHarness)
	}

	for _, harnessType := range []struct {
		Name           string
		Supported      bool
		DefaultOptions options.ScriptingHarness
		Tests          []shTest
	}{
		{
			Name:      "Roswell",
			Supported: isInPath("ros"),
			DefaultOptions: &options.ScriptingRoswell{
				Path:   filepath.Join(tmpdir, "roswell"),
				Lisp:   "sbcl-bin",
				Output: output,
			},
			Tests: []shTest{
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
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.NoError(t, sh.RunScript(ctx, `(defun main () (print "hello world"))`))
					},
				},
				{
					Name: "RunHelloWorld",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.NoError(t, sh.Run(ctx, []string{`(print "hello world")`}))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.Error(t, sh.RunScript(ctx, `(sb-ext:exit :code 42)`))
					},
				},
			},
		},
		// TODO (EVG-13209): fix tests for Windows.
		// TODO (EVG-13210): fix tests for Ubuntu.
		{
			Name:      "Python3",
			Supported: isInPath("python3") && !strings.Contains(os.Getenv("EVR_TASK_ID"), "ubuntu"),
			DefaultOptions: &options.ScriptingPython{
				VirtualEnvPath:    filepath.Join(tmpdir, "python3"),
				LegacyPython:      false,
				InterpreterBinary: "python3",
				Output:            output,
			},
			Tests: []shTest{
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
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.NoError(t, sh.RunScript(ctx, `print("hello world")`))
					},
				},
				{
					Name: "RunHelloWorld",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.NoError(t, sh.Run(ctx, []string{"-c", `print("hello world")`}))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.Error(t, sh.RunScript(ctx, `exit(42)`))
					},
				},
			},
		},
		// TODO (EVG-13209): fix tests for Windows.
		// TODO (EVG-13212): fix tests for Arch (race detector).
		{
			Name:      "Python2",
			Supported: isInPath("python2") && runtime.GOOS != "windows" && !strings.Contains(os.Getenv("EVR_TASK_ID"), "race"),
			DefaultOptions: &options.ScriptingPython{
				VirtualEnvPath:    filepath.Join(tmpdir, "python2"),
				LegacyPython:      true,
				InterpreterBinary: "python2",
				Packages:          []string{"wheel"},
				Output:            output,
			},
			Tests: []shTest{
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
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.NoError(t, sh.RunScript(ctx, `print("hello world")`))
					},
				},
				{
					Name: "RunHelloWorld",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.NoError(t, sh.Run(ctx, []string{"-c", `print("hello world")`}))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.Error(t, sh.RunScript(ctx, `exit(42)`))
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
			Tests: []shTest{
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
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.NoError(t, sh.RunScript(ctx, testutil.GolangMainSuccess()))
					},
				},
				{
					Name: "ScriptExitError",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						require.Error(t, sh.RunScript(ctx, testutil.GolangMainFail()))
					},
				},
				{
					Name: "Dependencies",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						tmpFile := filepath.Join(tmpdir, "fake_script.go")
						require.NoError(t, ioutil.WriteFile(tmpFile, []byte(`package main; import ("fmt"; "github.com/pkg/errors"); func main() { fmt.Println(errors.New("error")) }`), 0755))
						defer func() {
							assert.NoError(t, os.Remove(tmpFile))
						}()
						err = sh.Run(ctx, []string{tmpFile})
						require.NoError(t, err)
					},
				},
				{
					Name: "RunFile",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						tmpFile := filepath.Join(tmpdir, "fake_script.go")
						require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangMainSuccess()), 0755))
						defer func() {
							assert.NoError(t, os.Remove(tmpFile))
						}()
						err = sh.Run(ctx, []string{tmpFile})
						require.NoError(t, err)
					},
				},
				{
					Name: "Build",
					Case: func(t *testing.T, opts options.ScriptingHarness) {
						sh := makeScriptingHarness(ctx, t, manager, opts)
						tmpFile := filepath.Join(tmpdir, "fake_script.go")
						require.NoError(t, ioutil.WriteFile(tmpFile, []byte(testutil.GolangMainSuccess()), 0755))
						defer func() {
							assert.NoError(t, os.Remove(tmpFile))
						}()
						_, err := sh.Build(ctx, testutil.BuildDirectory(), []string{
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
		t.Run(harnessType.Name, func(t *testing.T) {
			if !harnessType.Supported {
				t.Skipf("%s is not supported in the current system", harnessType.Name)
				return
			}
			require.NoError(t, harnessType.DefaultOptions.Validate())
			t.Run("Config", func(t *testing.T) {
				start := time.Now()
				sh := makeScriptingHarness(ctx, t, manager, harnessType.DefaultOptions)
				require.NoError(t, sh.Setup(ctx))
				dur := time.Since(start)
				require.NotNil(t, sh)

				t.Run("ID", func(t *testing.T) {
					require.Equal(t, harnessType.DefaultOptions.ID(), sh.ID())
					assert.Len(t, sh.ID(), 40)
				})
				t.Run("Caching", func(t *testing.T) {
					start := time.Now()
					require.NoError(t, sh.Setup(ctx))

					// Since Setup() was previously called, it should already be
					// cached.
					assert.True(t, time.Since(start) < dur, "%s < %s",
						time.Since(start), dur)
				})
			})
			for _, test := range harnessType.Tests {
				t.Run(test.Name, func(t *testing.T) {
					test.Case(t, harnessType.DefaultOptions)
				})
			}
			t.Run("Testing", func(t *testing.T) {
				sh := makeScriptingHarness(ctx, t, manager, harnessType.DefaultOptions)
				res, err := sh.Test(ctx, tmpdir)
				require.NoError(t, err)
				require.Len(t, res, 0)
			})
			t.Run("Cleanup", func(t *testing.T) {
				sh := makeScriptingHarness(ctx, t, manager, harnessType.DefaultOptions)
				require.NoError(t, sh.Cleanup(ctx))
			})

		})
	}
}
