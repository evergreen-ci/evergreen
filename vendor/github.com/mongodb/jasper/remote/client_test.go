package remote

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/scripting"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	sender := grip.GetSender()
	grip.Error(sender.SetLevel(send.LevelInfo{Default: level.Info, Threshold: level.Info}))
	grip.Error(grip.SetSender(sender))
}

type clientTestCase struct {
	Name string
	Case func(context.Context, *testing.T, Manager)
}

func addBasicClientTests(modify testutil.OptsModify, tests ...clientTestCase) []clientTestCase {
	return append([]clientTestCase{
		{
			Name: "ValidateFixture",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				assert.NotNil(t, ctx)
				assert.NotNil(t, client)
			},
		},
		{
			Name: "IDReturnsNonempty",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				assert.NotEmpty(t, client.ID())
			},
		},
		{
			Name: "ProcEnvVarMatchesManagerID",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.TrueCreateOpts()
				modify(opts)
				proc, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)
				info := proc.Info(ctx)
				require.NotEmpty(t, info.Options.Environment)
				assert.Equal(t, client.ID(), info.Options.Environment[jasper.ManagerEnvironID])
			},
		},
		{
			Name: "CreateProcessFailsWithEmptyOptions",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := &options.Create{}
				modify(opts)
				proc, err := client.CreateProcess(ctx, opts)
				require.Error(t, err)
				assert.Nil(t, proc)
			},
		},
		{
			Name: "LongRunningOperationsAreListedAsRunning",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.SleepCreateOpts(20)
				modify(opts)
				procs, err := createProcs(ctx, opts, client, 10)
				require.NoError(t, err)
				assert.Len(t, procs, 10)

				procs, err = client.List(ctx, options.All)
				require.NoError(t, err)
				assert.Len(t, procs, 10)

				procs, err = client.List(ctx, options.Running)
				require.NoError(t, err)
				assert.Len(t, procs, 10)

				procs, err = client.List(ctx, options.Successful)
				require.NoError(t, err)
				assert.Len(t, procs, 0)
			},
		},
		{
			Name: "ListDoesNotErrorWhenEmptyResult",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				all, err := client.List(ctx, options.All)
				require.NoError(t, err)
				assert.Len(t, all, 0)
			},
		},
		{
			Name: "ListErrorsWithInvalidFilter",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				procs, err := client.List(ctx, options.Filter("foo"))
				assert.Error(t, err)
				assert.Nil(t, procs)
			},
		},
		{
			Name: "ListAllReturnsErrorWithCanceledContext",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				cctx, cancel := context.WithCancel(ctx)
				opts := testutil.TrueCreateOpts()
				modify(opts)
				created, err := createProcs(ctx, opts, client, 10)
				require.NoError(t, err)
				assert.Len(t, created, 10)
				cancel()
				output, err := client.List(cctx, options.All)
				require.Error(t, err)
				assert.Nil(t, output)
			},
		},
		{
			Name: "ListReturnsOneSuccessfulCommand",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.TrueCreateOpts()
				modify(opts)
				proc, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)

				_, err = proc.Wait(ctx)
				require.NoError(t, err)

				listOut, err := client.List(ctx, options.Successful)
				require.NoError(t, err)

				if assert.Len(t, listOut, 1) {
					assert.Equal(t, listOut[0].ID(), proc.ID())
				}
			},
		},
		{
			Name: "RegisterAlwaysErrors",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				proc, err := client.CreateProcess(ctx, &options.Create{Args: []string{"ls"}})
				assert.NotNil(t, proc)
				require.NoError(t, err)

				assert.Error(t, client.Register(ctx, nil))
				assert.Error(t, client.Register(ctx, proc))
			},
		},
		{
			Name: "GetMethodErrorsWithNoResponse",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				proc, err := client.Get(ctx, "foo")
				require.Error(t, err)
				assert.Nil(t, proc)
			},
		},
		{
			Name: "GetMethodReturnsMatchingProc",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.TrueCreateOpts()
				modify(opts)
				proc, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)

				ret, err := client.Get(ctx, proc.ID())
				require.NoError(t, err)
				assert.Equal(t, ret.ID(), proc.ID())
			},
		},
		{
			Name: "GroupDoesNotErrorWhenEmptyResult",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				procs, err := client.Group(ctx, "foo")
				require.NoError(t, err)
				assert.Len(t, procs, 0)
			},
		},
		{
			Name: "GroupErrorsForCanceledContexts",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.TrueCreateOpts()
				modify(opts)
				_, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)

				cctx, cancel := context.WithCancel(ctx)
				cancel()
				procs, err := client.Group(cctx, "foo")
				require.Error(t, err)
				assert.Len(t, procs, 0)
				assert.Contains(t, err.Error(), "canceled")
			},
		},
		{
			Name: "GroupPropagatesMatching",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.TrueCreateOpts()
				modify(opts)

				proc, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)

				proc.Tag("foo")

				procs, err := client.Group(ctx, "foo")
				require.NoError(t, err)
				require.Len(t, procs, 1)
				assert.Equal(t, procs[0].ID(), proc.ID())
			},
		},
		{
			Name: "CloseEmptyManagerNoops",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				require.NoError(t, client.Close(ctx))
			},
		},
		{
			Name: "ClosersWithoutTriggersTerminatesProcesses",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				if runtime.GOOS == "windows" {
					t.Skip("the sleep tests don't block correctly on windows")
				}
				opts := testutil.SleepCreateOpts(100)
				modify(opts)

				_, err := createProcs(ctx, opts, client, 10)
				require.NoError(t, err)
				assert.NoError(t, client.Close(ctx))
			},
		},
		{
			Name: "CloseErrorsWithCanceledContext",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.SleepCreateOpts(100)
				modify(opts)

				_, err := createProcs(ctx, opts, client, 10)
				require.NoError(t, err)

				cctx, cancel := context.WithCancel(ctx)
				cancel()

				err = client.Close(cctx)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "canceled")
			},
		},
		{
			Name: "CloseSucceedsWithTerminatedProcesses",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				procs, err := createProcs(ctx, testutil.TrueCreateOpts(), client, 10)
				for _, p := range procs {
					_, err = p.Wait(ctx)
					require.NoError(t, err)
				}

				require.NoError(t, err)
				assert.NoError(t, client.Close(ctx))
			},
		},
		{
			Name: "WaitingOnNonExistentProcessErrors",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.TrueCreateOpts()
				modify(opts)

				proc, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)

				_, err = proc.Wait(ctx)
				require.NoError(t, err)

				client.Clear(ctx)

				_, err = proc.Wait(ctx)
				require.Error(t, err)
				procs, err := client.List(ctx, options.All)
				require.NoError(t, err)
				assert.Len(t, procs, 0)
			},
		},
		{
			Name: "ClearCausesDeletionOfProcesses",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.TrueCreateOpts()
				modify(opts)
				proc, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)
				sameProc, err := client.Get(ctx, proc.ID())
				require.NoError(t, err)
				require.Equal(t, proc.ID(), sameProc.ID())
				_, err = proc.Wait(ctx)
				require.NoError(t, err)
				client.Clear(ctx)
				nilProc, err := client.Get(ctx, proc.ID())
				require.Error(t, err)
				assert.Nil(t, nilProc)
			},
		},
		{
			Name: "ClearIsANoopForActiveProcesses",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.SleepCreateOpts(20)
				modify(opts)
				proc, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)
				client.Clear(ctx)
				sameProc, err := client.Get(ctx, proc.ID())
				require.NoError(t, err)
				assert.Equal(t, proc.ID(), sameProc.ID())
				require.NoError(t, jasper.Terminate(ctx, proc)) // Clean up
			},
		},
		{
			Name: "ClearSelectivelyDeletesOnlyDeadProcesses",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				trueOpts := testutil.TrueCreateOpts()
				modify(trueOpts)
				lsProc, err := client.CreateProcess(ctx, trueOpts)
				require.NoError(t, err)

				sleepOpts := testutil.SleepCreateOpts(20)
				modify(sleepOpts)
				sleepProc, err := client.CreateProcess(ctx, sleepOpts)
				require.NoError(t, err)

				_, err = lsProc.Wait(ctx)
				require.NoError(t, err)

				client.Clear(ctx)

				sameSleepProc, err := client.Get(ctx, sleepProc.ID())
				require.NoError(t, err)
				assert.Equal(t, sleepProc.ID(), sameSleepProc.ID())

				nilProc, err := client.Get(ctx, lsProc.ID())
				require.Error(t, err)
				assert.Nil(t, nilProc)
				require.NoError(t, jasper.Terminate(ctx, sleepProc)) // Clean up
			},
		},
		{
			Name: "RegisterIsDisabled",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				err := client.Register(ctx, nil)
				require.Error(t, err)
				assert.Contains(t, err.Error(), "cannot register")
			},
		},
		{
			Name: "CreateProcessReturnsCorrectExample",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.TrueCreateOpts()
				modify(opts)
				proc, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)
				assert.NotNil(t, proc)
				assert.NotZero(t, proc.ID())

				fetched, err := client.Get(ctx, proc.ID())
				assert.NoError(t, err)
				assert.NotNil(t, fetched)
				assert.Equal(t, proc.ID(), fetched.ID())
			},
		},
		{
			Name: "WaitOnSigKilledProcessReturnsProperExitCode",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := testutil.SleepCreateOpts(100)
				modify(opts)
				proc, err := client.CreateProcess(ctx, opts)
				require.NoError(t, err)
				require.NotNil(t, proc)
				require.NotZero(t, proc.ID())

				require.NoError(t, proc.Signal(ctx, syscall.SIGKILL))

				exitCode, err := proc.Wait(ctx)
				require.Error(t, err)
				if runtime.GOOS == "windows" {
					assert.Equal(t, 1, exitCode)
				} else {
					assert.Equal(t, 9, exitCode)
				}
			},
		},
		{
			Name: "WriteFileFailsWithInvalidPath",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := options.WriteFile{Content: []byte("foo")}
				assert.Error(t, client.WriteFile(ctx, opts))
			},
		},
	}, tests...)
}

func TestManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for _, factory := range []struct {
		Name        string
		Constructor func(context.Context, *testing.T) Manager
	}{
		{
			Name: "MDB",
			Constructor: func(ctx context.Context, t *testing.T) Manager {
				mngr, err := jasper.NewSynchronizedManager(false)
				require.NoError(t, err)

				client, err := makeTestMDBServiceAndClient(ctx, mngr)
				require.NoError(t, err)
				return client
			},
		},
		{
			Name: "RPC/TLS",
			Constructor: func(ctx context.Context, t *testing.T) Manager {
				mngr, err := jasper.NewSynchronizedManager(false)
				require.NoError(t, err)

				client, err := makeTLSRPCServiceAndClient(ctx, mngr)
				require.NoError(t, err)
				return client
			},
		},
		{
			Name: "RPC/Insecure",
			Constructor: func(ctx context.Context, t *testing.T) Manager {
				assert.NotPanics(t, func() {
					newRPCClient(nil)
				})

				mngr, err := jasper.NewSynchronizedManager(false)
				require.NoError(t, err)

				client, err := makeInsecureRPCServiceAndClient(ctx, mngr)
				require.NoError(t, err)
				return client
			},
		},
	} {
		t.Run(factory.Name, func(t *testing.T) {
			for _, modify := range []struct {
				Name    string
				Options testutil.OptsModify
			}{
				{
					Name: "Blocking",
					Options: func(opts *options.Create) {
						opts.Implementation = options.ProcessImplementationBlocking
					},
				},
				{
					Name: "Basic",
					Options: func(opts *options.Create) {
						opts.Implementation = options.ProcessImplementationBasic
					},
				},
				{
					Name:    "Default",
					Options: func(opts *options.Create) {},
				},
			} {
				t.Run(modify.Name, func(t *testing.T) {
					for _, test := range addBasicClientTests(modify.Options,
						clientTestCase{
							Name: "StandardInput",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								for subTestName, subTestCase := range map[string]func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte){
									"ReaderIsIgnored": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte) {
										opts.StandardInput = bytes.NewBuffer(stdin)

										proc, err := client.CreateProcess(ctx, opts)
										require.NoError(t, err)

										_, err = proc.Wait(ctx)
										require.NoError(t, err)

										logs, err := client.GetLogStream(ctx, proc.ID(), 1)
										require.NoError(t, err)
										assert.Empty(t, logs.Logs)
									},
									"BytesSetsStandardInput": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte) {
										opts.StandardInputBytes = stdin

										proc, err := client.CreateProcess(ctx, opts)
										require.NoError(t, err)

										_, err = proc.Wait(ctx)
										require.NoError(t, err)

										logs, err := client.GetLogStream(ctx, proc.ID(), 1)
										require.NoError(t, err)

										require.Len(t, logs.Logs, 1)
										assert.Equal(t, expectedOutput, strings.TrimSpace(logs.Logs[0]))
									},
									"BytesCopiedByRespawnedProcess": func(ctx context.Context, t *testing.T, opts *options.Create, expectedOutput string, stdin []byte) {
										opts.StandardInputBytes = stdin

										proc, err := client.CreateProcess(ctx, opts)
										require.NoError(t, err)

										_, err = proc.Wait(ctx)
										require.NoError(t, err)

										logs, err := client.GetLogStream(ctx, proc.ID(), 1)
										require.NoError(t, err)

										require.Len(t, logs.Logs, 1)
										assert.Equal(t, expectedOutput, strings.TrimSpace(logs.Logs[0]))

										newProc, err := proc.Respawn(ctx)
										require.NoError(t, err)

										_, err = newProc.Wait(ctx)
										require.NoError(t, err)

										logs, err = client.GetLogStream(ctx, newProc.ID(), 1)
										require.NoError(t, err)

										require.Len(t, logs.Logs, 1)
										assert.Equal(t, expectedOutput, strings.TrimSpace(logs.Logs[0]))
									},
								} {
									t.Run(subTestName, func(t *testing.T) {
										inMemLogger, err := jasper.NewInMemoryLogger(1)
										require.NoError(t, err)

										opts := &options.Create{
											Args: []string{"bash", "-s"},
											Output: options.Output{
												Loggers: []*options.LoggerConfig{inMemLogger},
											},
										}
										modify.Options(opts)

										expectedOutput := "foobar"
										stdin := []byte("echo " + expectedOutput)
										subTestCase(ctx, t, opts, expectedOutput, stdin)
									})
								}
							},
						},
						clientTestCase{
							Name: "WriteFileSucceeds",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpFile, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tmpFile.Name()))
								}()
								require.NoError(t, tmpFile.Close())

								opts := options.WriteFile{Path: tmpFile.Name(), Content: []byte("foo")}
								require.NoError(t, client.WriteFile(ctx, opts))

								content, err := ioutil.ReadFile(tmpFile.Name())
								require.NoError(t, err)

								assert.Equal(t, opts.Content, content)
							},
						},
						clientTestCase{
							Name: "WriteFileAcceptsContentFromReader",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpFile, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tmpFile.Name()))
								}()
								require.NoError(t, tmpFile.Close())

								buf := []byte("foo")
								opts := options.WriteFile{Path: tmpFile.Name(), Reader: bytes.NewBuffer(buf)}
								require.NoError(t, client.WriteFile(ctx, opts))

								content, err := ioutil.ReadFile(tmpFile.Name())
								require.NoError(t, err)

								assert.Equal(t, buf, content)
							},
						},
						clientTestCase{
							Name: "WriteFileSucceedsWithLargeContent",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpFile, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tmpFile.Name()))
								}()
								require.NoError(t, tmpFile.Close())

								const mb = 1024 * 1024
								opts := options.WriteFile{Path: tmpFile.Name(), Content: bytes.Repeat([]byte("foo"), mb)}
								require.NoError(t, client.WriteFile(ctx, opts))

								content, err := ioutil.ReadFile(tmpFile.Name())
								require.NoError(t, err)

								assert.Equal(t, opts.Content, content)
							},
						},
						clientTestCase{
							Name: "WriteFileSucceedsWithLargeContentFromReader",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpFile, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, tmpFile.Close())
									assert.NoError(t, os.RemoveAll(tmpFile.Name()))
								}()

								const mb = 1024 * 1024
								buf := bytes.Repeat([]byte("foo"), 2*mb)
								opts := options.WriteFile{Path: tmpFile.Name(), Reader: bytes.NewBuffer(buf)}
								require.NoError(t, client.WriteFile(ctx, opts))

								content, err := ioutil.ReadFile(tmpFile.Name())
								require.NoError(t, err)

								assert.Equal(t, buf, content)
							},
						},
						clientTestCase{
							Name: "WriteFileSucceedsWithNoContent",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								path := filepath.Join(testutil.BuildDirectory(), filepath.Base(t.Name()))
								require.NoError(t, os.RemoveAll(path))
								defer func() {
									assert.NoError(t, os.RemoveAll(path))
								}()

								opts := options.WriteFile{Path: path}
								require.NoError(t, client.WriteFile(ctx, opts))

								stat, err := os.Stat(path)
								require.NoError(t, err)

								assert.Zero(t, stat.Size())
							},
						},
						clientTestCase{
							Name: "GetLogStreamFromNonexistentProcessFails",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								stream, err := client.GetLogStream(ctx, "foo", 1)
								assert.Error(t, err)
								assert.Zero(t, stream)
							},
						},
						clientTestCase{
							Name: "GetLogStreamFailsWithoutInMemoryLogger",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								opts := &options.Create{Args: []string{"echo", "foo"}}
								modify.Options(opts)
								proc, err := client.CreateProcess(ctx, opts)
								require.NoError(t, err)
								require.NotNil(t, proc)

								_, err = proc.Wait(ctx)
								require.NoError(t, err)

								stream, err := client.GetLogStream(ctx, proc.ID(), 1)
								assert.Error(t, err)
								assert.Zero(t, stream)
							},
						},
						clientTestCase{
							Name: "WithInMemoryLogger",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								inMemLogger, err := jasper.NewInMemoryLogger(100)
								require.NoError(t, err)
								output := "foo"
								opts := &options.Create{
									Args: []string{"echo", output},
									Output: options.Output{
										Loggers: []*options.LoggerConfig{inMemLogger},
									},
								}
								modify.Options(opts)

								for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, proc jasper.Process){
									"GetLogStreamFailsForInvalidCount": func(ctx context.Context, t *testing.T, proc jasper.Process) {
										stream, err := client.GetLogStream(ctx, proc.ID(), -1)
										assert.Error(t, err)
										assert.Zero(t, stream)
									},
									"GetLogStreamReturnsOutputOnSuccess": func(ctx context.Context, t *testing.T, proc jasper.Process) {
										logs := []string{}
										for stream, err := client.GetLogStream(ctx, proc.ID(), 1); !stream.Done; stream, err = client.GetLogStream(ctx, proc.ID(), 1) {
											require.NoError(t, err)
											require.NotEmpty(t, stream.Logs)
											logs = append(logs, stream.Logs...)
										}
										assert.Contains(t, logs, output)
									},
								} {
									t.Run(testName, func(t *testing.T) {
										proc, err := client.CreateProcess(ctx, opts)
										require.NoError(t, err)
										require.NotNil(t, proc)

										_, err = proc.Wait(ctx)
										require.NoError(t, err)
										testCase(ctx, t, proc)
									})
								}
							},
						},
						clientTestCase{
							Name: "DownloadFile",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, client Manager, tempDir string){
									"CreatesFileIfNonexistent": func(ctx context.Context, t *testing.T, client Manager, tempDir string) {
										opts := options.Download{
											URL:  "https://example.com",
											Path: filepath.Join(tempDir, filepath.Base(t.Name())),
										}
										require.NoError(t, client.DownloadFile(ctx, opts))
										defer func() {
											assert.NoError(t, os.RemoveAll(opts.Path))
										}()

										fileInfo, err := os.Stat(opts.Path)
										require.NoError(t, err)
										assert.NotZero(t, fileInfo.Size())
									},
									"WritesFileIfExists": func(ctx context.Context, t *testing.T, client Manager, tempDir string) {
										file, err := ioutil.TempFile(tempDir, "out.txt")
										require.NoError(t, err)
										defer func() {
											assert.NoError(t, os.RemoveAll(file.Name()))
										}()
										require.NoError(t, file.Close())

										opts := options.Download{
											URL:  "https://example.com",
											Path: file.Name(),
										}
										require.NoError(t, client.DownloadFile(ctx, opts))
										defer func() {
											assert.NoError(t, os.RemoveAll(opts.Path))
										}()

										fileInfo, err := os.Stat(file.Name())
										require.NoError(t, err)
										assert.NotZero(t, fileInfo.Size())
									},
									"CreatesFileAndExtracts": func(ctx context.Context, t *testing.T, client Manager, tempDir string) {
										downloadDir, err := ioutil.TempDir(tempDir, "out")
										require.NoError(t, err)
										defer func() {
											assert.NoError(t, os.RemoveAll(downloadDir))
										}()

										fileServerDir, err := ioutil.TempDir(tempDir, "file_server")
										require.NoError(t, err)
										defer func() {
											assert.NoError(t, os.RemoveAll(fileServerDir))
										}()

										fileName := "foo.zip"
										fileContents := "foo"
										require.NoError(t, testutil.AddFileToDirectory(fileServerDir, fileName, fileContents))

										absDownloadDir, err := filepath.Abs(downloadDir)
										require.NoError(t, err)
										destFilePath := filepath.Join(absDownloadDir, fileName)
										destExtractDir := filepath.Join(absDownloadDir, "extracted")

										port := testutil.GetPortNumber()
										fileServerAddr := fmt.Sprintf("localhost:%d", port)
										fileServer := &http.Server{Addr: fileServerAddr, Handler: http.FileServer(http.Dir(fileServerDir))}
										defer func() {
											assert.NoError(t, fileServer.Close())
										}()
										listener, err := net.Listen("tcp", fileServerAddr)
										require.NoError(t, err)
										go func() {
											grip.Info(fileServer.Serve(listener))
										}()

										baseURL := fmt.Sprintf("http://%s", fileServerAddr)
										require.NoError(t, testutil.WaitForRESTService(ctx, baseURL))

										opts := options.Download{
											URL:  fmt.Sprintf("%s/%s", baseURL, fileName),
											Path: destFilePath,
											ArchiveOpts: options.Archive{
												ShouldExtract: true,
												Format:        options.ArchiveZip,
												TargetPath:    destExtractDir,
											},
										}
										require.NoError(t, client.DownloadFile(ctx, opts))

										fileInfo, err := os.Stat(destFilePath)
										require.NoError(t, err)
										assert.NotZero(t, fileInfo.Size())

										dirContents, err := ioutil.ReadDir(destExtractDir)
										require.NoError(t, err)

										assert.NotZero(t, len(dirContents))
									},
									"FailsForInvalidArchiveFormat": func(ctx context.Context, t *testing.T, client Manager, tempDir string) {
										file, err := ioutil.TempFile(tempDir, filepath.Base(t.Name()))
										require.NoError(t, err)
										defer func() {
											assert.NoError(t, os.RemoveAll(file.Name()))
										}()
										require.NoError(t, file.Close())
										extractDir, err := ioutil.TempDir(tempDir, filepath.Base(t.Name())+"_extract")
										require.NoError(t, err)
										defer func() {
											assert.NoError(t, os.RemoveAll(file.Name()))
										}()

										opts := options.Download{
											URL:  "https://example.com",
											Path: file.Name(),
											ArchiveOpts: options.Archive{
												ShouldExtract: true,
												Format:        options.ArchiveFormat("foo"),
												TargetPath:    extractDir,
											},
										}
										assert.Error(t, client.DownloadFile(ctx, opts))
									},
									"FailsForUnarchivedFile": func(ctx context.Context, t *testing.T, client Manager, tempDir string) {
										extractDir, err := ioutil.TempDir(tempDir, filepath.Base(t.Name())+"_extract")
										require.NoError(t, err)
										defer func() {
											assert.NoError(t, os.RemoveAll(extractDir))
										}()
										opts := options.Download{
											URL:  "https://example.com",
											Path: filepath.Join(tempDir, filepath.Base(t.Name())),
											ArchiveOpts: options.Archive{
												ShouldExtract: true,
												Format:        options.ArchiveAuto,
												TargetPath:    extractDir,
											},
										}
										assert.Error(t, client.DownloadFile(ctx, opts))

										dirContents, err := ioutil.ReadDir(extractDir)
										require.NoError(t, err)
										assert.Zero(t, len(dirContents))
									},
									"FailsForInvalidURL": func(ctx context.Context, t *testing.T, client Manager, tempDir string) {
										file, err := ioutil.TempFile(tempDir, filepath.Base(t.Name()))
										require.NoError(t, err)
										defer func() {
											assert.NoError(t, os.RemoveAll(file.Name()))
										}()
										require.NoError(t, file.Close())
										assert.Error(t, client.DownloadFile(ctx, options.Download{URL: "", Path: file.Name()}))
									},
									"FailsForNonexistentURL": func(ctx context.Context, t *testing.T, client Manager, tempDir string) {
										file, err := ioutil.TempFile(tempDir, "out.txt")
										require.NoError(t, err)
										defer func() {
											assert.NoError(t, os.RemoveAll(file.Name()))
										}()
										require.NoError(t, file.Close())
										assert.Error(t, client.DownloadFile(ctx, options.Download{URL: "https://example.com/foo", Path: file.Name()}))
									},
									"FailsForInsufficientPermissions": func(ctx context.Context, t *testing.T, client Manager, tempDir string) {
										if os.Geteuid() == 0 {
											t.Skip("cannot test download permissions as root")
										} else if runtime.GOOS == "windows" {
											t.Skip("cannot test download permissions on windows")
										}
										assert.Error(t, client.DownloadFile(ctx, options.Download{URL: "https://example.com", Path: "/foo/bar"}))
									},
								} {
									t.Run(testName, func(t *testing.T) {
										tempDir, err := ioutil.TempDir(testutil.BuildDirectory(), filepath.Base(t.Name()))
										require.NoError(t, err)
										defer func() {
											assert.NoError(t, os.RemoveAll(tempDir))
										}()
										testCase(ctx, t, client, tempDir)
									})
								}
							},
						},
						clientTestCase{
							Name: "GetBuildloggerURLsFailsWithoutBuildlogger",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								logger := &options.LoggerConfig{}
								require.NoError(t, logger.Set(&options.DefaultLoggerOptions{
									Base: options.BaseOptions{Format: options.LogFormatPlain},
								}))
								opts := &options.Create{
									Args: []string{"echo", "foobar"},
									Output: options.Output{
										Loggers: []*options.LoggerConfig{logger},
									},
								}

								info, err := client.CreateProcess(ctx, opts)
								require.NoError(t, err)
								id := info.ID()
								assert.NotEmpty(t, id)

								urls, err := client.GetBuildloggerURLs(ctx, id)
								assert.Error(t, err)
								assert.Nil(t, urls)
							},
						},
						clientTestCase{
							Name: "GetBuildloggerURLsFailsWithNonexistentProcess",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								urls, err := client.GetBuildloggerURLs(ctx, "foo")
								assert.Error(t, err)
								assert.Nil(t, urls)
							},
						},
						clientTestCase{
							Name: "CreateWithLogFile",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								file, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(file.Name()))
								}()
								require.NoError(t, file.Close())

								logger := &options.LoggerConfig{}
								require.NoError(t, logger.Set(&options.FileLoggerOptions{
									Filename: file.Name(),
									Base:     options.BaseOptions{Format: options.LogFormatPlain},
								}))
								output := "foobar"
								opts := &options.Create{
									Args: []string{"echo", output},
									Output: options.Output{
										Loggers: []*options.LoggerConfig{logger},
									},
								}

								proc, err := client.CreateProcess(ctx, opts)
								require.NoError(t, err)

								exitCode, err := proc.Wait(ctx)
								require.NoError(t, err)
								require.Zero(t, exitCode)

								info, err := os.Stat(file.Name())
								require.NoError(t, err)
								assert.NotZero(t, info.Size())

								fileContents, err := ioutil.ReadFile(file.Name())
								require.NoError(t, err)
								assert.Contains(t, string(fileContents), output)
							},
						},
						clientTestCase{
							Name: "RegisterSignalTriggerIDChecksForInvalidTriggerID",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								proc, err := client.CreateProcess(ctx, testutil.SleepCreateOpts(1))
								require.NoError(t, err)
								assert.True(t, proc.Running(ctx))

								assert.Error(t, proc.RegisterSignalTriggerID(ctx, jasper.SignalTriggerID("foo")))

								assert.NoError(t, proc.Signal(ctx, syscall.SIGTERM))
							},
						},
						clientTestCase{
							Name: "RegisterSignalTriggerIDPassesWithValidArgs",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								proc, err := client.CreateProcess(ctx, testutil.SleepCreateOpts(1))
								require.NoError(t, err)
								assert.True(t, proc.Running(ctx))

								assert.NoError(t, proc.RegisterSignalTriggerID(ctx, jasper.CleanTerminationSignalTrigger))

								assert.NoError(t, proc.Signal(ctx, syscall.SIGTERM))
							},
						},
						clientTestCase{
							Name: "LoggingCacheCreate",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								lc := client.LoggingCache(ctx)
								logger, err := lc.Create("new_logger", &options.Output{})
								require.NoError(t, err)
								assert.Equal(t, "new_logger", logger.ID)

								// should fail with existing logger
								_, err = lc.Create("new_logger", &options.Output{})
								assert.Error(t, err)
							},
						},
						clientTestCase{
							Name: "LoggingCachePutNotImplemented",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								lc := client.LoggingCache(ctx)
								assert.Error(t, lc.Put("logger", &options.CachedLogger{ID: "logger"}))
							},
						},
						clientTestCase{
							Name: "LoggingCacheGetExists",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								lc := client.LoggingCache(ctx)
								expectedLogger, err := lc.Create("new_logger", &options.Output{})
								require.NoError(t, err)

								logger := lc.Get(expectedLogger.ID)
								require.NotNil(t, logger)
								assert.Equal(t, expectedLogger.ID, logger.ID)
							},
						},
						clientTestCase{
							Name: "LoggingCacheGetDNE",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								lc := client.LoggingCache(ctx)
								logger := lc.Get("DNE")
								require.Nil(t, logger)
							},
						},
						clientTestCase{
							Name: "LoggingCacheRemove",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								lc := client.LoggingCache(ctx)
								logger1, err := lc.Create("logger1", &options.Output{})
								require.NoError(t, err)
								logger2, err := lc.Create("logger2", &options.Output{})
								require.NoError(t, err)

								require.NotNil(t, lc.Get(logger1.ID))
								require.NotNil(t, lc.Get(logger2.ID))
								lc.Remove(logger2.ID)
								require.NotNil(t, lc.Get(logger1.ID))
								require.Nil(t, lc.Get(logger2.ID))
							},
						},
						clientTestCase{
							Name: "LoggingCachePrune",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								lc := client.LoggingCache(ctx)
								logger1, err := lc.Create("logger1", &options.Output{})
								require.NoError(t, err)
								require.NotNil(t, lc.Get(logger1.ID))
								time.Sleep(2 * time.Second)

								logger2, err := lc.Create("logger2", &options.Output{})
								require.NoError(t, err)

								lc.Prune(time.Now().Add(-time.Second))
								require.Nil(t, lc.Get(logger1.ID))
								require.NotNil(t, lc.Get(logger2.ID))
							},
						},
						clientTestCase{
							Name: "LoggingCacheLenEmpty",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								lc := client.LoggingCache(ctx)
								_, err := lc.Create("logger1", &options.Output{})
								require.NoError(t, err)
								_, err = lc.Create("logger2", &options.Output{})
								require.NoError(t, err)

								assert.Equal(t, 2, lc.Len())
							},
						},
						clientTestCase{
							Name: "LoggingCacheLenNotEmpty",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								lc := client.LoggingCache(ctx)
								assert.Zero(t, lc.Len())
							},
						},
						clientTestCase{
							Name: "LoggingSendMessagesInvalid",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								payload := options.LoggingPayload{
									LoggerID: "DNE",
									Data:     "new log message",
									Priority: level.Warning,
									Format:   options.LoggingPayloadFormatString,
								}
								assert.Error(t, client.SendMessages(ctx, payload))
							},
						},
						clientTestCase{
							Name: "LoggingSendMessagesValid",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								lc := client.LoggingCache(ctx)
								logger1, err := lc.Create("logger1", &options.Output{})
								require.NoError(t, err)

								payload := options.LoggingPayload{
									LoggerID: logger1.ID,
									Data:     "new log message",
									Priority: level.Warning,
									Format:   options.LoggingPayloadFormatString,
								}
								assert.NoError(t, client.SendMessages(ctx, payload))
							},
						},
						clientTestCase{
							Name: "ScriptingGetDNE",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								_, err := client.GetScripting(ctx, "DNE")
								assert.Error(t, err)
							},
						},
						clientTestCase{
							Name: "ScriptingGetExists",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								expectedHarness := createTestScriptingHarness(ctx, t, client, ".")

								harness, err := client.GetScripting(ctx, expectedHarness.ID())
								require.NoError(t, err)
								assert.Equal(t, expectedHarness.ID(), harness.ID())
							},
						},
						clientTestCase{
							Name: "ScriptingSetup",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								harness := createTestScriptingHarness(ctx, t, client, ".")
								assert.NoError(t, harness.Setup(ctx))
							},
						},
						clientTestCase{
							Name: "ScriptingCleanup",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								harness := createTestScriptingHarness(ctx, t, client, ".")
								assert.NoError(t, harness.Cleanup(ctx))
							},
						},
						clientTestCase{
							Name: "ScriptingRunNoError",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpdir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tmpdir))
								}()
								harness := createTestScriptingHarness(ctx, t, client, tmpdir)

								require.NoError(t, err)
								tmpFile := filepath.Join(tmpdir, "fake_script.go")
								require.NoError(t, ioutil.WriteFile(tmpFile, []byte(`package main; import "os"; func main() { os.Exit(0) }`), 0755))
								assert.NoError(t, harness.Run(ctx, []string{tmpFile}))
							},
						},
						clientTestCase{
							Name: "ScriptingRunError",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpdir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tmpdir))
								}()
								harness := createTestScriptingHarness(ctx, t, client, tmpdir)

								tmpFile := filepath.Join(tmpdir, "fake_script.go")
								require.NoError(t, ioutil.WriteFile(tmpFile, []byte(`package main; import "os"; func main() { os.Exit(42) }`), 0755))
								assert.Error(t, harness.Run(ctx, []string{tmpFile}))
							},
						},
						clientTestCase{
							Name: "ScriptingRunScriptNoError",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpdir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tmpdir))
								}()

								harness := createTestScriptingHarness(ctx, t, client, tmpdir)
								assert.NoError(t, harness.RunScript(ctx, `package main; import "fmt"; func main() { fmt.Println("Hello World") }`))
							},
						},
						clientTestCase{
							Name: "ScriptingRunScriptError",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpdir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tmpdir))
								}()

								harness := createTestScriptingHarness(ctx, t, client, tmpdir)
								require.Error(t, harness.RunScript(ctx, `package main; import "os"; func main() { os.Exit(42) }`))
							},
						},
						clientTestCase{
							Name: "ScriptingBuild",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpdir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tmpdir))
								}()
								harness := createTestScriptingHarness(ctx, t, client, tmpdir)

								tmpFile := filepath.Join(tmpdir, "fake_script.go")
								require.NoError(t, ioutil.WriteFile(tmpFile, []byte(`package main; import "os"; func main() { os.Exit(0) }`), 0755))
								_, err = harness.Build(ctx, tmpdir, []string{
									"-o",
									filepath.Join(tmpdir, "fake_script"),
									tmpFile,
								})
								require.NoError(t, err)
								_, err = os.Stat(filepath.Join(tmpFile))
								require.NoError(t, err)
							},
						},
						clientTestCase{
							Name: "ScriptingTest",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpdir, err := ioutil.TempDir(testutil.BuildDirectory(), "scripting_tests")
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, os.RemoveAll(tmpdir))
								}()
								harness := createTestScriptingHarness(ctx, t, client, tmpdir)

								tmpFile := filepath.Join(tmpdir, "fake_script_test.go")
								require.NoError(t, ioutil.WriteFile(tmpFile, []byte(`package main; import "testing"; func TestMain(t *testing.T) { return }`), 0755))
								results, err := harness.Test(ctx, tmpdir, scripting.TestOptions{Name: "dummy"})
								require.NoError(t, err)
								require.Len(t, results, 1)
							},
						},
					) {
						t.Run(test.Name, func(t *testing.T) {
							tctx, cancel := context.WithTimeout(ctx, testutil.RPCTestTimeout)
							defer cancel()
							test.Case(tctx, t, factory.Constructor(tctx, t))
						})
					}
				})
			}
		})
	}
}

func createTestScriptingHarness(ctx context.Context, t *testing.T, client Manager, dir string) scripting.Harness {
	opts := options.NewGolangScriptingEnvironment(filepath.Join(dir, "gopath"), runtime.GOROOT())
	harness, err := client.CreateScripting(ctx, opts)
	require.NoError(t, err)

	return harness
}
