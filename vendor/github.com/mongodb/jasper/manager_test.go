package jasper

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var echoSubCmd = []string{"echo", "foo"}

func TestManagerInterface(t *testing.T) {
	t.Parallel()

	httpClient := &http.Client{}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for mname, factory := range map[string]func(context.Context, *testing.T) Manager{
		"Basic/NoLock/BasicProcs": func(_ context.Context, _ *testing.T) Manager {
			return &basicProcessManager{
				id:       "id",
				procs:    map[string]Process{},
				blocking: false,
			}
		},
		"Basic/NoLock/BlockingProcs": func(_ context.Context, _ *testing.T) Manager {
			return &basicProcessManager{
				id:       "id",
				procs:    map[string]Process{},
				blocking: true,
			}
		},
		"Basic/Lock/BasicProcs": func(_ context.Context, t *testing.T) Manager {
			localManager, err := NewLocalManager(false)
			require.NoError(t, err)
			return localManager
		},
		"Basic/Lock/BlockingProcs": func(ctx context.Context, t *testing.T) Manager {
			localBlockingManager, err := NewLocalManagerBlockingProcesses(false)
			require.NoError(t, err)
			return localBlockingManager
		},
		"SelfClearing/BasicProcs": func(ctx context.Context, t *testing.T) Manager {
			selfClearingManager, err := NewSelfClearingProcessManager(10, false)
			require.NoError(t, err)
			return selfClearingManager
		},
		"SelfClearing/BlockingProcs": func(ctx context.Context, t *testing.T) Manager {
			selfClearingBlockingManager, err := NewSelfClearingProcessManagerBlockingProcesses(10, false)
			require.NoError(t, err)
			return selfClearingBlockingManager
		},
		"REST": func(ctx context.Context, t *testing.T) Manager {
			srv, port, err := startRESTService(ctx, httpClient)
			require.NoError(t, err)
			require.NotNil(t, srv)

			return &restClient{
				prefix: fmt.Sprintf("http://localhost:%d/jasper/v1", port),
				client: httpClient,
			}
		},
	} {
		t.Run(mname, func(t *testing.T) {
			for name, test := range map[string]func(context.Context, *testing.T, Manager){
				"ValidateFixture": func(ctx context.Context, t *testing.T, manager Manager) {
					assert.NotNil(t, ctx)
					assert.NotNil(t, manager)
				},
				"IDReturnsNonempty": func(ctx context.Context, t *testing.T, manager Manager) {
					assert.NotEmpty(t, manager.ID())
				},
				"ProcEnvVarMatchesManagerID": func(ctx context.Context, t *testing.T, manager Manager) {
					opts := trueCreateOpts()
					proc, err := manager.CreateProcess(ctx, opts)
					require.NoError(t, err)
					info := proc.Info(ctx)
					require.NotEmpty(t, info.Options.Environment)
					assert.Equal(t, manager.ID(), info.Options.Environment[ManagerEnvironID])
				},
				"ListDoesNotErrorWhenEmpty": func(ctx context.Context, t *testing.T, manager Manager) {
					all, err := manager.List(ctx, All)
					require.NoError(t, err)
					assert.Len(t, all, 0)
				},
				"CreateSimpleProcess": func(ctx context.Context, t *testing.T, manager Manager) {
					opts := trueCreateOpts()
					proc, err := manager.CreateProcess(ctx, opts)
					require.NoError(t, err)
					assert.NotNil(t, proc)
					info := proc.Info(ctx)
					assert.True(t, info.IsRunning || info.Complete)
				},
				"CreateProcessFails": func(ctx context.Context, t *testing.T, manager Manager) {
					proc, err := manager.CreateProcess(ctx, &CreateOptions{})
					require.Error(t, err)
					assert.Nil(t, proc)
				},
				"ListAllOperations": func(ctx context.Context, t *testing.T, manager Manager) {
					created, err := createProcs(ctx, trueCreateOpts(), manager, 10)
					require.NoError(t, err)
					assert.Len(t, created, 10)
					output, err := manager.List(ctx, All)
					require.NoError(t, err)
					assert.Len(t, output, 10)
				},
				"ListAllReturnsErrorWithCanceledContext": func(ctx context.Context, t *testing.T, manager Manager) {
					cctx, cancel := context.WithCancel(ctx)
					created, err := createProcs(ctx, trueCreateOpts(), manager, 10)
					require.NoError(t, err)
					assert.Len(t, created, 10)
					cancel()
					output, err := manager.List(cctx, All)
					require.Error(t, err)
					assert.Nil(t, output)
				},
				"LongRunningOperationsAreListedAsRunning": func(ctx context.Context, t *testing.T, manager Manager) {
					procs, err := createProcs(ctx, sleepCreateOpts(10), manager, 10)
					require.NoError(t, err)
					assert.Len(t, procs, 10)

					procs, err = manager.List(ctx, Running)
					require.NoError(t, err)
					assert.Len(t, procs, 10)

					procs, err = manager.List(ctx, Successful)
					require.NoError(t, err)
					assert.Len(t, procs, 0)
				},
				"ListReturnsOneSuccessfulCommand": func(ctx context.Context, t *testing.T, manager Manager) {
					proc, err := manager.CreateProcess(ctx, trueCreateOpts())
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					listOut, err := manager.List(ctx, Successful)
					require.NoError(t, err)

					if assert.Len(t, listOut, 1) {
						assert.Equal(t, listOut[0].ID(), proc.ID())
					}
				},
				"ListReturnsOneFailedCommand": func(ctx context.Context, t *testing.T, manager Manager) {
					proc, err := manager.CreateProcess(ctx, falseCreateOpts())
					require.NoError(t, err)
					_, err = proc.Wait(ctx)
					require.Error(t, err)

					listOut, err := manager.List(ctx, Failed)
					require.NoError(t, err)

					if assert.Len(t, listOut, 1) {
						assert.Equal(t, listOut[0].ID(), proc.ID())
					}
				},
				"GetMethodErrorsWithNoResponse": func(ctx context.Context, t *testing.T, manager Manager) {
					proc, err := manager.Get(ctx, "foo")
					require.Error(t, err)
					assert.Nil(t, proc)
				},
				"GetMethodReturnsMatchingDoc": func(ctx context.Context, t *testing.T, manager Manager) {
					proc, err := manager.CreateProcess(ctx, trueCreateOpts())
					require.NoError(t, err)

					ret, err := manager.Get(ctx, proc.ID())
					require.NoError(t, err)
					assert.Equal(t, ret.ID(), proc.ID())
				},
				"GroupDoesNotErrorWithoutResults": func(ctx context.Context, t *testing.T, manager Manager) {
					procs, err := manager.Group(ctx, "foo")
					require.NoError(t, err)
					assert.Len(t, procs, 0)
				},
				"GroupErrorsForCanceledContexts": func(ctx context.Context, t *testing.T, manager Manager) {
					_, err := manager.CreateProcess(ctx, trueCreateOpts())
					require.NoError(t, err)

					cctx, cancel := context.WithCancel(ctx)
					cancel()
					procs, err := manager.Group(cctx, "foo")
					require.Error(t, err)
					assert.Len(t, procs, 0)
					assert.Contains(t, err.Error(), context.Canceled.Error())
				},
				"GroupPropagatesMatching": func(ctx context.Context, t *testing.T, manager Manager) {
					proc, err := manager.CreateProcess(ctx, trueCreateOpts())
					require.NoError(t, err)

					proc.Tag("foo")

					procs, err := manager.Group(ctx, "foo")
					require.NoError(t, err)
					require.Len(t, procs, 1)
					assert.Equal(t, procs[0].ID(), proc.ID())
				},
				"CloseEmptyManagerNoops": func(ctx context.Context, t *testing.T, manager Manager) {
					assert.NoError(t, manager.Close(ctx))
				},
				"CloseErrorsWithCanceledContext": func(ctx context.Context, t *testing.T, manager Manager) {
					_, err := createProcs(ctx, sleepCreateOpts(100), manager, 10)
					require.NoError(t, err)

					cctx, cancel := context.WithCancel(ctx)
					cancel()

					err = manager.Close(cctx)
					require.Error(t, err)
					assert.Contains(t, err.Error(), context.Canceled.Error())
				},
				"CloseSucceedsWithTerminatedProcesses": func(ctx context.Context, t *testing.T, manager Manager) {
					procs, err := createProcs(ctx, trueCreateOpts(), manager, 10)
					for _, p := range procs {
						_, err := p.Wait(ctx)
						require.NoError(t, err)
					}

					require.NoError(t, err)
					assert.NoError(t, manager.Close(ctx))
				},
				"ClosersWithoutTriggersTerminatesProcesses": func(ctx context.Context, t *testing.T, manager Manager) {
					if runtime.GOOS == "windows" {
						t.Skip("manager close tests will error due to process termination on Windows")
					}

					_, err := createProcs(ctx, sleepCreateOpts(100), manager, 10)
					require.NoError(t, err)
					assert.NoError(t, manager.Close(ctx))
				},
				"CloseExecutesClosersForProcesses": func(ctx context.Context, t *testing.T, manager Manager) {
					if mname == "REST" {
						t.Skip("not supported on rest interfaces")
					}

					if runtime.GOOS == "windows" {
						t.Skip("manager close tests will error due to process termination on Windows")
					}

					opts := sleepCreateOpts(5)
					count := 0
					countIncremented := make(chan bool, 1)
					opts.closers = append(opts.closers, func() (_ error) {
						count++
						countIncremented <- true
						close(countIncremented)
						return
					})

					_, err := manager.CreateProcess(ctx, opts)
					require.NoError(t, err)

					assert.Equal(t, count, 0)
					require.NoError(t, manager.Close(ctx))
					select {
					case <-ctx.Done():
						assert.Fail(t, "process took too long to run closers")
					case <-countIncremented:
						assert.Equal(t, 1, count)
					}
				},
				"RegisterProcessErrorsForNilProcess": func(ctx context.Context, t *testing.T, manager Manager) {
					if mname == "REST" {
						t.Skip("not supported on rest interfaces")
					}

					err := manager.Register(ctx, nil)
					require.Error(t, err)
					assert.Contains(t, err.Error(), "not defined")
				},
				"RegisterProcessErrorsForCanceledContext": func(ctx context.Context, t *testing.T, manager Manager) {
					if mname == "REST" {
						t.Skip("not supported on rest interfaces")
					}

					cctx, cancel := context.WithCancel(ctx)
					cancel()
					proc, err := newBlockingProcess(ctx, trueCreateOpts())
					require.NoError(t, err)
					err = manager.Register(cctx, proc)
					require.Error(t, err)
					assert.Contains(t, err.Error(), context.Canceled.Error())
				},
				"RegisterProcessErrorsWhenMissingID": func(ctx context.Context, t *testing.T, manager Manager) {
					if mname == "REST" {
						t.Skip("not supported on rest interfaces")
					}

					proc := &blockingProcess{}
					assert.Equal(t, proc.ID(), "")
					err := manager.Register(ctx, proc)
					require.Error(t, err)
					assert.Contains(t, err.Error(), "malformed")
				},
				"RegisterProcessModifiesManagerState": func(ctx context.Context, t *testing.T, manager Manager) {
					if mname == "REST" {
						t.Skip("not supported on rest interfaces")
					}

					proc, err := newBlockingProcess(ctx, trueCreateOpts())
					require.NoError(t, err)
					err = manager.Register(ctx, proc)
					require.NoError(t, err)

					procs, err := manager.List(ctx, All)
					require.NoError(t, err)
					require.True(t, len(procs) >= 1)

					assert.Equal(t, procs[0].ID(), proc.ID())
				},
				"RegisterProcessErrorsForDuplicateProcess": func(ctx context.Context, t *testing.T, manager Manager) {
					if mname == "REST" {
						t.Skip("not supported on rest interfaces")
					}

					proc, err := newBlockingProcess(ctx, trueCreateOpts())
					require.NoError(t, err)
					assert.NotEmpty(t, proc)
					err = manager.Register(ctx, proc)
					require.NoError(t, err)
					err = manager.Register(ctx, proc)
					assert.Error(t, err)
				},
				"ManagerCallsOptionsCloseByDefault": func(ctx context.Context, t *testing.T, manager Manager) {
					if mname == "REST" {
						t.Skip("cannot register trigger on rest interfaces")
					}

					opts := &CreateOptions{}
					opts.Args = []string{"echo", "foobar"}
					count := 0
					countIncremented := make(chan bool, 1)
					opts.closers = append(opts.closers, func() (_ error) {
						count++
						countIncremented <- true
						close(countIncremented)
						return
					})

					proc, err := manager.CreateProcess(ctx, opts)
					require.NoError(t, err)
					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					select {
					case <-ctx.Done():
						assert.Fail(t, "process took too long to run closers")
					case <-countIncremented:
						assert.Equal(t, 1, count)
					}
				},
				"ClearCausesDeletionOfProcesses": func(ctx context.Context, t *testing.T, manager Manager) {
					opts := trueCreateOpts()
					proc, err := manager.CreateProcess(ctx, opts)
					require.NoError(t, err)
					sameProc, err := manager.Get(ctx, proc.ID())
					require.NoError(t, err)
					require.Equal(t, proc.ID(), sameProc.ID())
					_, err = proc.Wait(ctx)
					require.NoError(t, err)
					manager.Clear(ctx)
					nilProc, err := manager.Get(ctx, proc.ID())
					require.Error(t, err)
					assert.Nil(t, nilProc)
				},
				"ClearIsANoopForActiveProcesses": func(ctx context.Context, t *testing.T, manager Manager) {
					opts := sleepCreateOpts(20)
					proc, err := manager.CreateProcess(ctx, opts)
					require.NoError(t, err)
					manager.Clear(ctx)
					sameProc, err := manager.Get(ctx, proc.ID())
					require.NoError(t, err)
					assert.Equal(t, proc.ID(), sameProc.ID())
					require.NoError(t, Terminate(ctx, proc)) // Clean up
				},
				"ClearSelectivelyDeletesOnlyDeadProcesses": func(ctx context.Context, t *testing.T, manager Manager) {
					trueOpts := trueCreateOpts()
					lsProc, err := manager.CreateProcess(ctx, trueOpts)
					require.NoError(t, err)

					sleepOpts := sleepCreateOpts(20)
					sleepProc, err := manager.CreateProcess(ctx, sleepOpts)
					require.NoError(t, err)

					_, err = lsProc.Wait(ctx)
					require.NoError(t, err)

					manager.Clear(ctx)

					sameSleepProc, err := manager.Get(ctx, sleepProc.ID())
					require.NoError(t, err)
					assert.Equal(t, sleepProc.ID(), sameSleepProc.ID())

					nilProc, err := manager.Get(ctx, lsProc.ID())
					require.Error(t, err)
					assert.Nil(t, nilProc)
					require.NoError(t, Terminate(ctx, sleepProc)) // Clean up
				},
				"CreateCommandPasses": func(ctx context.Context, t *testing.T, manager Manager) {
					cmd := manager.CreateCommand(ctx)
					cmd.Add(echoSubCmd)
					assert.NoError(t, cmd.Run(ctx))
				},
				"RunningCommandCreatesNewProcesses": func(ctx context.Context, t *testing.T, manager Manager) {
					procList, err := manager.List(ctx, All)
					require.NoError(t, err)
					originalProcCount := len(procList) // zero
					cmd := manager.CreateCommand(ctx)
					subCmds := [][]string{echoSubCmd, echoSubCmd, echoSubCmd}
					cmd.Extend(subCmds)
					require.NoError(t, cmd.Run(ctx))
					newProcList, err := manager.List(ctx, All)
					require.NoError(t, err)

					assert.Len(t, newProcList, originalProcCount+len(subCmds))
				},
				"CommandProcIDsMatchManagerIDs": func(ctx context.Context, t *testing.T, manager Manager) {
					cmd := manager.CreateCommand(ctx)
					cmd.Extend([][]string{echoSubCmd, echoSubCmd, echoSubCmd})
					require.NoError(t, cmd.Run(ctx))
					newProcList, err := manager.List(ctx, All)
					require.NoError(t, err)

					findIDInProcList := func(procID string) bool {
						for _, proc := range newProcList {
							if proc.ID() == procID {
								return true
							}
						}
						return false
					}

					for _, procID := range cmd.GetProcIDs() {
						assert.True(t, findIDInProcList(procID))
					}
				},
				// "": func(ctx context.Context, t *testing.T, manager Manager) {},
			} {
				t.Run(name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, managerTestTimeout)
					defer cancel()
					test(tctx, t, factory(tctx, t))
				})
			}
		})
	}
}

func TestTrackedManager(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for managerName, makeManager := range map[string]func() *basicProcessManager{
		"Basic/NoLock/BasicProcs": func() *basicProcessManager {
			return &basicProcessManager{
				procs:    map[string]Process{},
				blocking: false,
				tracker:  newMockProcessTracker(),
			}
		},
		"Basic/NoLock/BlockingProcs": func() *basicProcessManager {
			return &basicProcessManager{
				procs:    map[string]Process{},
				blocking: true,
				tracker:  newMockProcessTracker(),
			}
		},
	} {
		t.Run(managerName, func(t *testing.T) {
			for name, test := range map[string]func(context.Context, *testing.T, *basicProcessManager, *CreateOptions){
				"ValidateFixtureSetup": func(ctx context.Context, t *testing.T, manager *basicProcessManager, opts *CreateOptions) {
					assert.NotNil(t, manager.tracker)
					assert.Len(t, manager.procs, 0)
				},
				"CreateProcessTracksProcess": func(ctx context.Context, t *testing.T, manager *basicProcessManager, opts *CreateOptions) {
					proc, err := manager.CreateProcess(ctx, opts)
					require.NoError(t, err)
					assert.Len(t, manager.procs, 1)

					mockTracker, ok := manager.tracker.(*mockProcessTracker)
					require.True(t, ok)
					require.Len(t, mockTracker.Infos, 1)
					assert.Equal(t, proc.Info(ctx), mockTracker.Infos[0])
				},
				"CreateCommandTracksCommandAfterRun": func(ctx context.Context, t *testing.T, manager *basicProcessManager, opts *CreateOptions) {
					err := manager.CreateCommand(ctx).Add(opts.Args).Background(true).Run(ctx)
					require.NoError(t, err)
					assert.Len(t, manager.procs, 1)

					mockTracker, ok := manager.tracker.(*mockProcessTracker)
					require.True(t, ok)
					require.Len(t, mockTracker.Infos, 1)
					assert.NotZero(t, mockTracker.Infos[0])
				},
				"DoNotTrackProcessIfCreateProcessDoesNotMakeProcess": func(ctx context.Context, t *testing.T, manager *basicProcessManager, opts *CreateOptions) {
					opts.Args = []string{"foo"}
					_, err := manager.CreateProcess(ctx, opts)
					require.Error(t, err)
					assert.Len(t, manager.procs, 0)

					mockTracker, ok := manager.tracker.(*mockProcessTracker)
					require.True(t, ok)
					assert.Len(t, mockTracker.Infos, 0)
				},
				"DoNotTrackProcessIfCreateCommandDoesNotMakeProcess": func(ctx context.Context, t *testing.T, manager *basicProcessManager, opts *CreateOptions) {
					opts.Args = []string{"foo"}
					err := manager.CreateCommand(ctx).Add(opts.Args).Background(true).Run(ctx)
					require.Error(t, err)
					assert.Len(t, manager.procs, 0)

					mockTracker, ok := manager.tracker.(*mockProcessTracker)
					require.True(t, ok)
					assert.Len(t, mockTracker.Infos, 0)
				},
				"CloseCleansUpProcesses": func(ctx context.Context, t *testing.T, manager *basicProcessManager, opts *CreateOptions) {
					require.NoError(t, manager.CreateCommand(ctx).Add(opts.Args).Background(true).Run(ctx))
					assert.Len(t, manager.procs, 1)

					mockTracker, ok := manager.tracker.(*mockProcessTracker)
					require.True(t, ok)
					require.Len(t, mockTracker.Infos, 1)
					assert.NotZero(t, mockTracker.Infos[0])

					require.NoError(t, manager.Close(ctx))
					assert.Len(t, mockTracker.Infos, 0)
					require.NoError(t, manager.Close(ctx))
				},
				"CloseWithNoProcessesIsNotError": func(ctx context.Context, t *testing.T, manager *basicProcessManager, opts *CreateOptions) {
					mockTracker, ok := manager.tracker.(*mockProcessTracker)
					require.True(t, ok)

					require.NoError(t, manager.Close(ctx))
					assert.Len(t, mockTracker.Infos, 0)
					require.NoError(t, manager.Close(ctx))
					assert.Len(t, mockTracker.Infos, 0)
				},
				"DoubleCloseIsNotError": func(ctx context.Context, t *testing.T, manager *basicProcessManager, opts *CreateOptions) {
					require.NoError(t, manager.CreateCommand(ctx).Add(opts.Args).Background(true).Run(ctx))
					assert.Len(t, manager.procs, 1)

					mockTracker, ok := manager.tracker.(*mockProcessTracker)
					require.True(t, ok)
					require.Len(t, mockTracker.Infos, 1)
					assert.NotZero(t, mockTracker.Infos[0])

					require.NoError(t, manager.Close(ctx))
					assert.Len(t, mockTracker.Infos, 0)
					require.NoError(t, manager.Close(ctx))
					assert.Len(t, mockTracker.Infos, 0)
				},
				// "": func(ctx context.Context, t *testing.T, manager Manager) {},
			} {
				t.Run(name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, managerTestTimeout)
					defer cancel()
					opts := yesCreateOpts(managerTestTimeout)
					test(tctx, t, makeManager(), &opts)
				})
			}
		})
	}
}
