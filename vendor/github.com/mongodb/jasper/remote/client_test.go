package remote

import (
	"bytes"
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/send"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	sender := grip.GetSender()
	grip.Error(sender.SetLevel(send.LevelInfo{Threshold: level.Info}))
	grip.Error(grip.SetSender(sender))
}

type ClientTestCase struct {
	Name       string
	Case       func(context.Context, *testing.T, Manager)
	ShouldSkip bool
}

func AddBasicClientTests(modify testutil.OptsModify, tests ...ClientTestCase) []ClientTestCase {
	return append([]ClientTestCase{
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
			Name: "ListDoesNotErrorWhenEmpty",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				all, err := client.List(ctx, options.All)
				require.NoError(t, err)
				assert.Len(t, all, 0)
			},
		},
		{
			Name: "CreateProcessFails",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				opts := &options.Create{}
				modify(opts)
				proc, err := client.CreateProcess(ctx, opts)
				require.Error(t, err)
				assert.Nil(t, proc)
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
			Name: "GetMethodErrorsWithNoResponse",
			Case: func(ctx context.Context, t *testing.T, client Manager) {
				proc, err := client.Get(ctx, "foo")
				require.Error(t, err)
				assert.Nil(t, proc)
			},
		},
		{
			Name: "GetMethodReturnsMatchingDoc",
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
			Name: "GroupDoesNotErrorWithoutResults",
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
					for _, test := range AddBasicClientTests(modify.Options,
						ClientTestCase{
							Name:       "StandardInput",
							ShouldSkip: factory.Name == "MDB",
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
										opts := &options.Create{
											Args: []string{"bash", "-s"},
											Output: options.Output{
												Loggers: []options.Logger{jasper.NewInMemoryLogger(1)},
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
						ClientTestCase{
							Name:       "WriteFileSucceeds",
							ShouldSkip: factory.Name == "MDB",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpFile, err := ioutil.TempFile(buildDir(t), filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, tmpFile.Close())
									assert.NoError(t, os.RemoveAll(tmpFile.Name()))
								}()

								opts := options.WriteFile{Path: tmpFile.Name(), Content: []byte("foo")}
								require.NoError(t, client.WriteFile(ctx, opts))

								content, err := ioutil.ReadFile(tmpFile.Name())
								require.NoError(t, err)

								assert.Equal(t, opts.Content, content)
							},
						},
						ClientTestCase{
							Name:       "WriteFileAcceptsContentFromReader",
							ShouldSkip: factory.Name == "MDB",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpFile, err := ioutil.TempFile(buildDir(t), filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, tmpFile.Close())
									assert.NoError(t, os.RemoveAll(tmpFile.Name()))
								}()

								buf := []byte("foo")
								opts := options.WriteFile{Path: tmpFile.Name(), Reader: bytes.NewBuffer(buf)}
								require.NoError(t, client.WriteFile(ctx, opts))

								content, err := ioutil.ReadFile(tmpFile.Name())
								require.NoError(t, err)

								assert.Equal(t, buf, content)
							},
						},
						ClientTestCase{
							Name:       "WriteFileSucceedsWithLargeContent",
							ShouldSkip: factory.Name == "MDB",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								tmpFile, err := ioutil.TempFile(buildDir(t), filepath.Base(t.Name()))
								require.NoError(t, err)
								defer func() {
									assert.NoError(t, tmpFile.Close())
									assert.NoError(t, os.RemoveAll(tmpFile.Name()))
								}()

								const mb = 1024 * 1024
								opts := options.WriteFile{Path: tmpFile.Name(), Content: bytes.Repeat([]byte("foo"), mb)}
								require.NoError(t, client.WriteFile(ctx, opts))

								content, err := ioutil.ReadFile(tmpFile.Name())
								require.NoError(t, err)

								assert.Equal(t, opts.Content, content)
							},
						},
						ClientTestCase{
							Name:       "WriteFileSucceedsWithNoContent",
							ShouldSkip: factory.Name == "MDB",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								path := filepath.Join(buildDir(t), filepath.Base(t.Name()))
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

						ClientTestCase{
							Name:       "GetLogStreamFromNonexistentProcessFails",
							ShouldSkip: factory.Name == "MDB",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								stream, err := client.GetLogStream(ctx, "foo", 1)
								assert.Error(t, err)
								assert.Zero(t, stream)
							},
						},
						ClientTestCase{
							Name:       "GetLogStreamFailsWithoutInMemoryLogger",
							ShouldSkip: factory.Name == "MDB",
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
						ClientTestCase{
							Name:       "WithInMemoryLogger",
							ShouldSkip: factory.Name == "MDB",
							Case: func(ctx context.Context, t *testing.T, client Manager) {
								output := "foo"
								opts := &options.Create{
									Args: []string{"echo", output},
									Output: options.Output{
										Loggers: []options.Logger{
											{
												Type:    options.LogInMemory,
												Options: options.Log{InMemoryCap: 100, Format: options.LogFormatPlain},
											},
										},
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
					) {
						if test.ShouldSkip {
							continue
						}
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
