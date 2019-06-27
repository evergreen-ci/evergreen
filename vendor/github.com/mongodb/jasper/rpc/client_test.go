package rpc

import (
	"context"
	"fmt"
	"io/ioutil"
	"net"
	"path/filepath"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeInsecureServiceAndClient(ctx context.Context, mngr jasper.Manager) (jasper.RemoteClient, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", getPortNumber()))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := startTestService(ctx, mngr, addr, nil); err != nil {
		return nil, errors.WithStack(err)
	}

	return newTestClient(ctx, addr, nil)
}

func makeTLSServiceAndClient(ctx context.Context, mngr jasper.Manager) (jasper.RemoteClient, error) {
	addr, err := net.ResolveTCPAddr("tcp", fmt.Sprintf("localhost:%d", getPortNumber()))
	if err != nil {

		return nil, errors.WithStack(err)
	}
	caCertFile := filepath.Join("testdata", "ca.crt")

	serverCertFile := filepath.Join("testdata", "server.crt")
	serverKeyFile := filepath.Join("testdata", "server.key")

	clientCertFile := filepath.Join("testdata", "client.crt")
	clientKeyFile := filepath.Join("testdata", "client.key")

	// Make CA credentials
	caCert, err := ioutil.ReadFile(caCertFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cert file")
	}

	// Make server credentials
	serverCert, err := ioutil.ReadFile(serverCertFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cert file")
	}
	serverKey, err := ioutil.ReadFile(serverKeyFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read key file")
	}
	serverCreds, err := NewCredentials(caCert, serverCert, serverKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize test server credentials")
	}

	if err := startTestService(ctx, mngr, addr, serverCreds); err != nil {
		return nil, errors.Wrap(err, "failed to start test server")
	}

	clientCert, err := ioutil.ReadFile(clientCertFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read cert file")
	}
	clientKey, err := ioutil.ReadFile(clientKeyFile)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read key file")
	}
	clientCreds, err := NewCredentials(caCert, clientCert, clientKey)
	if err != nil {
		return nil, errors.Wrap(err, "failed to initialize test client credentials")
	}

	return newTestClient(ctx, addr, clientCreds)
}

// Note: these tests are largely copied directly from the top level
// package into this package to avoid an import cycle.

func TestRPCClient(t *testing.T) {
	assert.NotPanics(t, func() {
		newRPCClient(nil)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for setupMethod, makeTestServiceAndClient := range map[string]func(ctx context.Context, mngr jasper.Manager) (jasper.RemoteClient, error){
		"Insecure": makeInsecureServiceAndClient,
		"TLS":      makeTLSServiceAndClient,
	} {
		t.Run(setupMethod, func(t *testing.T) {
			for mname, factory := range map[string]func(ctx context.Context, t *testing.T) jasper.RemoteClient{
				"Basic": func(ctx context.Context, t *testing.T) jasper.RemoteClient {
					mngr, err := jasper.NewLocalManager(false)
					require.NoError(t, err)

					client, err := makeTestServiceAndClient(ctx, mngr)
					require.NoError(t, err)
					return client
				},
				"Blocking": func(ctx context.Context, t *testing.T) jasper.RemoteClient {
					mngr, err := jasper.NewLocalManagerBlockingProcesses(false)
					require.NoError(t, err)

					client, err := makeTestServiceAndClient(ctx, mngr)
					require.NoError(t, err)
					return client
				},
			} {
				t.Run(mname, func(t *testing.T) {
					for name, test := range map[string]func(context.Context, *testing.T, jasper.RemoteClient){
						"ValidateFixture": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							assert.NotNil(t, ctx)
							assert.NotNil(t, client)
						},
						"ListDoesNotErrorWhenEmpty": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							all, err := client.List(ctx, jasper.All)
							require.NoError(t, err)
							assert.Len(t, all, 0)
						},
						"CreateProcessFails": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							proc, err := client.CreateProcess(ctx, &jasper.CreateOptions{})
							require.Error(t, err)
							assert.Nil(t, proc)
						},
						"ListAllReturnsErrorWithCanceledContext": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							cctx, cancel := context.WithCancel(ctx)
							created, err := createProcs(ctx, trueCreateOpts(), client, 10)
							require.NoError(t, err)
							assert.Len(t, created, 10)
							cancel()
							output, err := client.List(cctx, jasper.All)
							require.Error(t, err)
							assert.Nil(t, output)
						},
						"LongRunningOperationsAreListedAsRunning": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							procs, err := createProcs(ctx, sleepCreateOpts(20), client, 10)
							require.NoError(t, err)
							assert.Len(t, procs, 10)

							procs, err = client.List(ctx, jasper.All)
							require.NoError(t, err)
							assert.Len(t, procs, 10)

							procs, err = client.List(ctx, jasper.Running)
							require.NoError(t, err)
							assert.Len(t, procs, 10)

							procs, err = client.List(ctx, jasper.Successful)
							require.NoError(t, err)
							assert.Len(t, procs, 0)
						},
						"ListReturnsOneSuccessfulCommand": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							proc, err := client.CreateProcess(ctx, trueCreateOpts())
							require.NoError(t, err)

							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							listOut, err := client.List(ctx, jasper.Successful)
							require.NoError(t, err)

							if assert.Len(t, listOut, 1) {
								assert.Equal(t, listOut[0].ID(), proc.ID())
							}
						},
						"GetMethodErrorsWithNoResponse": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							proc, err := client.Get(ctx, "foo")
							require.Error(t, err)
							assert.Nil(t, proc)
						},
						"GetMethodReturnsMatchingDoc": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							proc, err := client.CreateProcess(ctx, trueCreateOpts())
							require.NoError(t, err)

							ret, err := client.Get(ctx, proc.ID())
							require.NoError(t, err)
							assert.Equal(t, ret.ID(), proc.ID())
						},
						"GroupDoesNotErrorWithoutResults": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							procs, err := client.Group(ctx, "foo")
							require.NoError(t, err)
							assert.Len(t, procs, 0)
						},
						"GroupErrorsForCanceledContexts": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							_, err := client.CreateProcess(ctx, trueCreateOpts())
							require.NoError(t, err)

							cctx, cancel := context.WithCancel(ctx)
							cancel()
							procs, err := client.Group(cctx, "foo")
							require.Error(t, err)
							assert.Len(t, procs, 0)
							assert.Contains(t, err.Error(), "canceled")
						},
						"GroupPropagatesMatching": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							proc, err := client.CreateProcess(ctx, trueCreateOpts())
							require.NoError(t, err)

							proc.Tag("foo")

							procs, err := client.Group(ctx, "foo")
							require.NoError(t, err)
							require.Len(t, procs, 1)
							assert.Equal(t, procs[0].ID(), proc.ID())
						},
						"CloseEmptyManagerNoops": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							require.NoError(t, client.Close(ctx))
						},
						"ClosersWithoutTriggersTerminatesProcesses": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							if runtime.GOOS == "windows" {
								t.Skip("the sleep tests don't block correctly on windows")
							}

							_, err := createProcs(ctx, sleepCreateOpts(100), client, 10)
							require.NoError(t, err)
							assert.NoError(t, client.Close(ctx))
						},
						"CloseErrorsWithCanceledContext": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							_, err := createProcs(ctx, sleepCreateOpts(100), client, 10)
							require.NoError(t, err)

							cctx, cancel := context.WithCancel(ctx)
							cancel()

							err = client.Close(cctx)
							require.Error(t, err)
							assert.Contains(t, err.Error(), "canceled")
						},
						"CloseSucceedsWithTerminatedProcesses": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							if runtime.GOOS == "windows" {
								t.Skip("context times out on windows")
							}

							procs, err := createProcs(ctx, trueCreateOpts(), client, 10)
							for _, p := range procs {
								_, err := p.Wait(ctx)
								require.NoError(t, err)
							}

							require.NoError(t, err)
							assert.NoError(t, client.Close(ctx))
						},
						"WaitingOnNonExistentProcessErrors": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							proc, err := client.CreateProcess(ctx, trueCreateOpts())

							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							client.Clear(ctx)

							_, err = proc.Wait(ctx)
							require.Error(t, err)
							assert.True(t, strings.Contains(err.Error(), "problem finding process"))
						},
						"ClearCausesDeletionOfProcesses": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							opts := trueCreateOpts()
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
						"ClearIsANoopForActiveProcesses": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							opts := sleepCreateOpts(20)
							proc, err := client.CreateProcess(ctx, opts)
							require.NoError(t, err)
							client.Clear(ctx)
							sameProc, err := client.Get(ctx, proc.ID())
							require.NoError(t, err)
							assert.Equal(t, proc.ID(), sameProc.ID())
							require.NoError(t, jasper.Terminate(ctx, proc)) // Clean up
						},
						"ClearSelectivelyDeletesOnlyDeadProcesses": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							trueOpts := trueCreateOpts()
							lsProc, err := client.CreateProcess(ctx, trueOpts)
							require.NoError(t, err)

							sleepOpts := sleepCreateOpts(20)
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

						// The following test cases are added specifically for the
						// RemoteClient.

						"WithInMemoryLogger": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							output := "foo"
							opts := &jasper.CreateOptions{
								Args: []string{"echo", output},
								Output: jasper.OutputOptions{
									Loggers: []jasper.Logger{
										jasper.Logger{
											Type:    jasper.LogInMemory,
											Options: jasper.LogOptions{InMemoryCap: 100, Format: jasper.LogFormatPlain},
										},
									},
								},
							}

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
						"GetLogStreamFromNonexistentProcessFails": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							stream, err := client.GetLogStream(ctx, "foo", 1)
							assert.Error(t, err)
							assert.Zero(t, stream)
						},
						"GetLogStreamFailsWithoutInMemoryLogger": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							opts := &jasper.CreateOptions{Args: []string{"echo", "foo"}}

							proc, err := client.CreateProcess(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)

							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							stream, err := client.GetLogStream(ctx, proc.ID(), 1)
							assert.Error(t, err)
							assert.Zero(t, stream)
						},
						// "": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {},

						///////////////////////////////////
						//
						// The following test cases are added
						// specifically for the rpc case

						"RegisterIsDisabled": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							err := client.Register(ctx, nil)
							require.Error(t, err)
							assert.Contains(t, err.Error(), "cannot register")
						},
						"CreateProcessReturnsCorrectExample": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							proc, err := client.CreateProcess(ctx, trueCreateOpts())
							require.NoError(t, err)
							assert.NotNil(t, proc)
							assert.NotZero(t, proc.ID())

							fetched, err := client.Get(ctx, proc.ID())
							assert.NoError(t, err)
							assert.NotNil(t, fetched)
							assert.Equal(t, proc.ID(), fetched.ID())
						},
						"WaitOnSigKilledProcessReturnsProperExitCode": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
							proc, err := client.CreateProcess(ctx, sleepCreateOpts(100))
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
						// "": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {},
					} {
						t.Run(name, func(t *testing.T) {
							tctx, cancel := context.WithTimeout(ctx, taskTimeout)
							defer cancel()
							test(tctx, t, factory(tctx, t))
						})
					}
				})
			}
		})
	}
}

type processConstructor func(context.Context, *jasper.CreateOptions) (jasper.Process, error)

func TestRPCProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for setupMethod, makeTestServiceAndClient := range map[string]func(ctx context.Context, mngr jasper.Manager) (jasper.RemoteClient, error){
		"Insecure": makeInsecureServiceAndClient,
		"TLS":      makeTLSServiceAndClient,
	} {
		t.Run(setupMethod, func(t *testing.T) {
			for cname, makeProc := range map[string]processConstructor{
				"Basic": func(ctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
					mngr, err := jasper.NewLocalManager(false)
					if err != nil {
						return nil, errors.WithStack(err)
					}

					client, err := makeTestServiceAndClient(ctx, mngr)
					if err != nil {
						return nil, errors.WithStack(err)
					}

					return client.CreateProcess(ctx, opts)
				},
				"Blocking": func(ctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
					mngr, err := jasper.NewLocalManagerBlockingProcesses(false)
					if err != nil {
						return nil, errors.WithStack(err)
					}

					client, err := makeTestServiceAndClient(ctx, mngr)
					if err != nil {
						return nil, errors.WithStack(err)
					}

					return client.CreateProcess(ctx, opts)
				},
			} {
				t.Run(cname, func(t *testing.T) {
					for name, testCase := range map[string]func(context.Context, *testing.T, *jasper.CreateOptions, processConstructor){
						"WithPopulatedArgsCommandCreationPasses": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							assert.NotZero(t, opts.Args)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.NotNil(t, proc)
						},
						"ErrorToCreateWithInvalidArgs": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							opts.Args = []string{}
							proc, err := makep(ctx, opts)
							require.Error(t, err)
							assert.Nil(t, proc)
						},
						"WithCanceledContextProcessCreationFails": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							pctx, pcancel := context.WithCancel(ctx)
							pcancel()
							proc, err := makep(pctx, opts)
							require.Error(t, err)
							assert.Nil(t, proc)
						},
						"CanceledContextTimesOutEarly": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
							defer pcancel()
							startAt := time.Now()
							opts = sleepCreateOpts(20)
							proc, err := makep(pctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)

							time.Sleep(5 * time.Millisecond) // let time pass...
							assert.False(t, proc.Info(ctx).Successful)
							assert.True(t, time.Since(startAt) < 20*time.Second)
						},
						"ProcessLacksTagsByDefault": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							tags := proc.GetTags()
							assert.Empty(t, tags)
						},
						"ProcessTagsPersist": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							opts.Tags = []string{"foo"}
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							tags := proc.GetTags()
							assert.Contains(t, tags, "foo")
						},
						"InfoHasMatchingID": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							_, err = proc.Wait(ctx)
							require.NoError(t, err)
							assert.Equal(t, proc.ID(), proc.Info(ctx).ID)
						},
						"ResetTags": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							proc.Tag("foo")
							assert.Contains(t, proc.GetTags(), "foo")
							proc.ResetTags()
							assert.Len(t, proc.GetTags(), 0)
						},
						"TagsAreSetLike": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							for i := 0; i < 100; i++ {
								proc.Tag("foo")
							}

							assert.Len(t, proc.GetTags(), 1)
							proc.Tag("bar")
							assert.Len(t, proc.GetTags(), 2)
						},
						"CompleteIsTrueAfterWait": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							time.Sleep(10 * time.Millisecond) // give the process time to start background machinery
							_, err = proc.Wait(ctx)
							assert.NoError(t, err)
							assert.True(t, proc.Complete(ctx))
						},
						"WaitReturnsWithCanceledContext": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							opts.Args = []string{"sleep", "10"}
							pctx, pcancel := context.WithCancel(ctx)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.True(t, proc.Running(ctx))
							assert.NoError(t, err)
							pcancel()
							_, err = proc.Wait(pctx)
							assert.Error(t, err)
						},
						"RegisterTriggerErrorsForNil": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.Error(t, proc.RegisterTrigger(ctx, nil))
						},
						"RegisterSignalTriggerIDErrorsForExitedProcess": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							_, err = proc.Wait(ctx)
							assert.NoError(t, err)
							assert.Error(t, proc.RegisterSignalTriggerID(ctx, jasper.CleanTerminationSignalTrigger))
						},
						"RegisterSignalTriggerIDFailsWithInvalidTriggerID": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							opts = sleepCreateOpts(3)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.Error(t, proc.RegisterSignalTriggerID(ctx, jasper.SignalTriggerID(-1)))
						},
						"RegisterSignalTriggerIDPassesWithValidTriggerID": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							opts = sleepCreateOpts(3)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.NoError(t, proc.RegisterSignalTriggerID(ctx, jasper.CleanTerminationSignalTrigger))
						},
						"WaitOnRespawnedProcessDoesNotError": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)
							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							newProc, err := proc.Respawn(ctx)
							require.NoError(t, err)
							_, err = newProc.Wait(ctx)
							assert.NoError(t, err)
						},
						"RespawnedProcessGivesSameResult": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)

							_, err = proc.Wait(ctx)
							require.NoError(t, err)
							procExitCode := proc.Info(ctx).ExitCode

							newProc, err := proc.Respawn(ctx)
							require.NoError(t, err)
							_, err = newProc.Wait(ctx)
							require.NoError(t, err)
							assert.Equal(t, procExitCode, newProc.Info(ctx).ExitCode)
						},
						"RespawningFinishedProcessIsOK": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)
							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							newProc, err := proc.Respawn(ctx)
							assert.NoError(t, err)
							_, err = newProc.Wait(ctx)
							require.NoError(t, err)
							assert.True(t, newProc.Info(ctx).Successful)
						},
						"RespawningRunningProcessIsOK": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							opts = sleepCreateOpts(2)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)

							newProc, err := proc.Respawn(ctx)
							assert.NoError(t, err)
							_, err = newProc.Wait(ctx)
							require.NoError(t, err)
							assert.True(t, newProc.Info(ctx).Successful)
						},
						"RespawnShowsConsistentStateValues": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							opts = sleepCreateOpts(3)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							require.NotNil(t, proc)
							_, err = proc.Wait(ctx)
							require.NoError(t, err)

							newProc, err := proc.Respawn(ctx)
							require.NoError(t, err)
							assert.True(t, newProc.Running(ctx))
							_, err = newProc.Wait(ctx)
							require.NoError(t, err)
							assert.True(t, proc.Complete(ctx))
						},
						"WaitGivesSuccessfulExitCode": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, trueCreateOpts())
							require.NoError(t, err)
							require.NotNil(t, proc)
							exitCode, err := proc.Wait(ctx)
							assert.NoError(t, err)
							assert.Equal(t, 0, exitCode)
						},
						"WaitGivesFailureExitCode": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, falseCreateOpts())
							require.NoError(t, err)
							require.NotNil(t, proc)
							exitCode, err := proc.Wait(ctx)
							require.Error(t, err)
							assert.Equal(t, 1, exitCode)
						},
						"WaitGivesProperExitCodeOnSignalDeath": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, sleepCreateOpts(100))
							require.NoError(t, err)
							require.NotNil(t, proc)
							sig := syscall.SIGTERM
							proc.Signal(ctx, sig)
							exitCode, err := proc.Wait(ctx)
							require.Error(t, err)
							if runtime.GOOS == "windows" {
								assert.Equal(t, 1, exitCode)
							} else {
								assert.Equal(t, int(sig), exitCode)
							}
						},
						"WaitGivesNegativeOneOnAlternativeError": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							cctx, cancel := context.WithCancel(ctx)
							proc, err := makep(ctx, sleepCreateOpts(100))
							require.NoError(t, err)
							require.NotNil(t, proc)

							var exitCode int
							waitFinished := make(chan bool)
							go func() {
								exitCode, err = proc.Wait(cctx)
								waitFinished <- true
							}()
							cancel()
							select {
							case <-waitFinished:
								require.Error(t, err)
								assert.Equal(t, -1, exitCode)
							case <-ctx.Done():
								assert.Fail(t, "call to Wait() took too long to finish")
							}
							require.NoError(t, jasper.Terminate(ctx, proc)) // Clean up.
						},
						"CallingSignalOnDeadProcessDoesError": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							_, err = proc.Wait(ctx)
							assert.NoError(t, err)

							err = proc.Signal(ctx, syscall.SIGTERM)
							require.Error(t, err)
							assert.True(t, strings.Contains(err.Error(), "cannot signal a process that has terminated"))
						},

						// "": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {},

						///////////////////////////////////
						//
						// The following test cases are added
						// specifically for the rpc case

						"CompleteReturnsFalseForProcessThatDoesntExist": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							firstID := proc.ID()
							_, err = proc.Wait(ctx)
							assert.NoError(t, err)
							assert.True(t, proc.Complete(ctx))
							proc.(*rpcProcess).info.Id += "_foo"
							proc.(*rpcProcess).info.Complete = false
							require.NotEqual(t, firstID, proc.ID())
							assert.False(t, proc.Complete(ctx), proc.ID())
						},
						"RunningReturnsFalseForProcessThatDoesntExist": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							firstID := proc.ID()
							_, err = proc.Wait(ctx)
							assert.NoError(t, err)
							proc.(*rpcProcess).info.Id += "_foo"
							proc.(*rpcProcess).info.Complete = false
							require.NotEqual(t, firstID, proc.ID())
							assert.False(t, proc.Running(ctx), proc.ID())
						},
						"CompleteAlwaysReturnsTrueWhenProcessIsComplete": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							proc, err := makep(ctx, opts)
							require.NoError(t, err)

							_, err = proc.Wait(ctx)
							assert.NoError(t, err)

							assert.True(t, proc.Complete(ctx))
						},
						"RegisterSignalTriggerFails": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
							opts = sleepCreateOpts(3)
							proc, err := makep(ctx, opts)
							require.NoError(t, err)
							assert.Error(t, proc.RegisterSignalTrigger(ctx, func(_ jasper.ProcessInfo, _ syscall.Signal) bool {
								return false
							}))
						},
						// "": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {},
					} {
						t.Run(name, func(t *testing.T) {
							tctx, cancel := context.WithTimeout(ctx, taskTimeout)
							defer cancel()

							opts := &jasper.CreateOptions{Args: []string{"ls"}}
							testCase(tctx, t, opts, makeProc)
						})
					}
				})
			}

		})
	}
}
