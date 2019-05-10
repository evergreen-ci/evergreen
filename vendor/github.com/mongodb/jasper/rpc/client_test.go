package rpc

import (
	"context"
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

// Note: these tests are largely copied directly from the top level
// package into this package to avoid an import cycle.

func TestRPCManager(t *testing.T) {
	assert.NotPanics(t, func() {
		NewRPCManager(nil)
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for mname, factory := range map[string]func(ctx context.Context, t *testing.T) jasper.Manager{
		"Basic": func(ctx context.Context, t *testing.T) jasper.Manager {
			mngr, err := jasper.NewLocalManager(false)
			require.NoError(t, err)
			addr, err := startRPC(ctx, mngr)
			require.NoError(t, err)

			client, err := getClient(ctx, addr)
			require.NoError(t, err)

			return client
		},
		"Blocking": func(ctx context.Context, t *testing.T) jasper.Manager {
			mngr, err := jasper.NewLocalManagerBlockingProcesses(false)
			require.NoError(t, err)
			addr, err := startRPC(ctx, mngr)
			require.NoError(t, err)

			client, err := getClient(ctx, addr)
			require.NoError(t, err)

			return client
		},
	} {
		t.Run(mname, func(t *testing.T) {
			for name, test := range map[string]func(context.Context, *testing.T, jasper.Manager){
				"ValidateFixture": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					assert.NotNil(t, ctx)
					assert.NotNil(t, manager)
				},
				"ListErrorsWhenEmpty": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					all, err := manager.List(ctx, jasper.All)
					assert.Error(t, err)
					assert.Len(t, all, 0)
					assert.Contains(t, err.Error(), "no processes")
				},
				"CreateProcessFails": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					proc, err := manager.CreateProcess(ctx, &jasper.CreateOptions{})
					assert.Error(t, err)
					assert.Nil(t, proc)
				},
				"ListAllReturnsErrorWithCanceledContext": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					cctx, cancel := context.WithCancel(ctx)
					created, err := createProcs(ctx, trueCreateOpts(), manager, 10)
					assert.NoError(t, err)
					assert.Len(t, created, 10)
					cancel()
					output, err := manager.List(cctx, jasper.All)
					assert.Error(t, err)
					assert.Nil(t, output)
				},
				"LongRunningOperationsAreListedAsRunning": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					procs, err := createProcs(ctx, sleepCreateOpts(20), manager, 10)
					assert.NoError(t, err)
					assert.Len(t, procs, 10)

					procs, err = manager.List(ctx, jasper.All)
					assert.NoError(t, err)
					assert.Len(t, procs, 10)

					procs, err = manager.List(ctx, jasper.Running)
					assert.NoError(t, err)
					assert.Len(t, procs, 10)

					procs, err = manager.List(ctx, jasper.Successful)
					assert.Error(t, err)
					assert.Len(t, procs, 0)
				},
				"ListReturnsOneSuccessfulCommand": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					proc, err := manager.CreateProcess(ctx, trueCreateOpts())
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					assert.NoError(t, err)

					listOut, err := manager.List(ctx, jasper.Successful)
					assert.NoError(t, err)

					if assert.Len(t, listOut, 1) {
						assert.Equal(t, listOut[0].ID(), proc.ID())
					}
				},
				"GetMethodErrorsWithNoResponse": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					proc, err := manager.Get(ctx, "foo")
					assert.Error(t, err)
					assert.Nil(t, proc)
				},
				"GetMethodReturnsMatchingDoc": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					t.Skip("for now,")
					proc, err := manager.CreateProcess(ctx, trueCreateOpts())
					require.NoError(t, err)

					ret, err := manager.Get(ctx, proc.ID())
					if assert.NoError(t, err) {
						assert.Equal(t, ret.ID(), proc.ID())
					}
				},
				"GroupErrorsWithoutResults": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					procs, err := manager.Group(ctx, "foo")
					assert.Error(t, err)
					assert.Len(t, procs, 0)
					assert.Contains(t, err.Error(), "no jobs")
				},
				"GroupErrorsForCanceledContexts": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					_, err := manager.CreateProcess(ctx, trueCreateOpts())
					assert.NoError(t, err)

					cctx, cancel := context.WithCancel(ctx)
					cancel()
					procs, err := manager.Group(cctx, "foo")
					assert.Error(t, err)
					assert.Len(t, procs, 0)
					assert.Contains(t, err.Error(), "canceled")
				},
				"GroupPropagatesMatching": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					proc, err := manager.CreateProcess(ctx, trueCreateOpts())
					require.NoError(t, err)

					proc.Tag("foo")

					procs, err := manager.Group(ctx, "foo")
					require.NoError(t, err)
					require.Len(t, procs, 1)
					assert.Equal(t, procs[0].ID(), proc.ID())
				},
				"CloseEmptyManagerNoops": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					assert.NoError(t, manager.Close(ctx))
				},
				"ClosersWithoutTriggersTerminatesProcesses": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					if runtime.GOOS == "windows" {
						t.Skip("the sleep tests don't block correctly on windows")
					}

					_, err := createProcs(ctx, sleepCreateOpts(100), manager, 10)
					assert.NoError(t, err)
					assert.NoError(t, manager.Close(ctx))
				},
				"CloseErrorsWithCanceledContext": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					_, err := createProcs(ctx, sleepCreateOpts(100), manager, 10)
					assert.NoError(t, err)

					cctx, cancel := context.WithCancel(ctx)
					cancel()

					err = manager.Close(cctx)
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "canceled")
				},
				"CloseErrorsWithTerminatedProcesses": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					if runtime.GOOS == "windows" {
						t.Skip("context times out on windows")
					}

					procs, err := createProcs(ctx, trueCreateOpts(), manager, 10)
					for _, p := range procs {
						_, err := p.Wait(ctx)
						assert.NoError(t, err)
					}

					assert.NoError(t, err)
					assert.Error(t, manager.Close(ctx))
				},
				"WaitingOnNonExistentProcessErrors": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					proc, err := manager.CreateProcess(ctx, trueCreateOpts())

					_, err = proc.Wait(ctx)
					assert.NoError(t, err)

					manager.Clear(ctx)

					_, err = proc.Wait(ctx)
					assert.Error(t, err)
					assert.True(t, strings.Contains(err.Error(), "problem finding process"))
				},
				"ClearCausesDeletionOfProcesses": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
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
					assert.Error(t, err)
					assert.Nil(t, nilProc)
				},
				"ClearIsANoopForActiveProcesses": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					opts := sleepCreateOpts(20)
					proc, err := manager.CreateProcess(ctx, opts)
					require.NoError(t, err)
					manager.Clear(ctx)
					sameProc, err := manager.Get(ctx, proc.ID())
					require.NoError(t, err)
					assert.Equal(t, proc.ID(), sameProc.ID())
					require.NoError(t, jasper.Terminate(ctx, proc)) // Clean up
				},
				"ClearSelectivelyDeletesOnlyDeadProcesses": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
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
					assert.Error(t, err)
					assert.Nil(t, nilProc)
					require.NoError(t, jasper.Terminate(ctx, sleepProc)) // Clean up
				},
				// "": func(ctx context.Context, t *testing.T, manager jasper.Manager) {},
				// "": func(ctx context.Context, t *testing.T, manager jasper.Manager) {},

				///////////////////////////////////
				//
				// The following test cases are added
				// specifically for the rpc case

				"RegisterIsDisabled": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					err := manager.Register(ctx, nil)
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "cannot register")
				},
				"ListErrorsWithEmpty": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					procs, err := manager.List(ctx, jasper.All)
					assert.Error(t, err)
					assert.Contains(t, err.Error(), "no processes")
					assert.Len(t, procs, 0)
				},
				"CreateProcessReturnsCorrectExample": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					proc, err := manager.CreateProcess(ctx, trueCreateOpts())
					assert.NoError(t, err)
					assert.NotNil(t, proc)
					assert.NotZero(t, proc.ID())

					fetched, err := manager.Get(ctx, proc.ID())
					assert.NoError(t, err)
					assert.NotNil(t, fetched)
					assert.Equal(t, proc.ID(), fetched.ID())
				},
				"WaitOnSigKilledProcessReturnsProperExitCode": func(ctx context.Context, t *testing.T, manager jasper.Manager) {
					proc, err := manager.CreateProcess(ctx, sleepCreateOpts(100))
					require.NoError(t, err)
					require.NotNil(t, proc)
					require.NotZero(t, proc.ID())

					require.NoError(t, proc.Signal(ctx, syscall.SIGKILL))

					exitCode, err := proc.Wait(ctx)
					assert.Error(t, err)
					if runtime.GOOS == "windows" {
						assert.Equal(t, 1, exitCode)
					} else {
						assert.Equal(t, 9, exitCode)
					}
				},
				// "": func(ctx context.Context, t *testing.T, manager jasper.Manager) {},
			} {
				t.Run(name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, taskTimeout)
					defer cancel()
					test(tctx, t, factory(tctx, t))
				})
			}
		})
	}
}

type processConstructor func(context.Context, *jasper.CreateOptions) (jasper.Process, error)

func TestRPCProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for cname, makeProc := range map[string]processConstructor{
		"Basic": func(ctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
			mngr, err := jasper.NewLocalManager(false)
			require.NoError(t, err)
			addr, err := startRPC(ctx, mngr)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			client, err := getClient(ctx, addr)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			return client.CreateProcess(ctx, opts)

		},
		"Blocking": func(ctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
			mngr, err := jasper.NewLocalManagerBlockingProcesses(false)
			require.NoError(t, err)
			addr, err := startRPC(ctx, mngr)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			client, err := getClient(ctx, addr)
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
					assert.Error(t, err)
					assert.Nil(t, proc)
				},
				"WithCanceledContextProcessCreationFails": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
					pctx, pcancel := context.WithCancel(ctx)
					pcancel()
					proc, err := makep(pctx, opts)
					assert.Error(t, err)
					assert.Nil(t, proc)
				},
				"CanceledContextTimesOutEarly": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
					if runtime.GOOS == "windows" {
						t.Skip("the sleep tests don't block correctly on windows")
					}

					pctx, pcancel := context.WithTimeout(ctx, 200*time.Millisecond)
					defer pcancel()
					startAt := time.Now()
					opts.Args = []string{"sleep", "10"}
					proc, err := makep(pctx, opts)
					assert.NoError(t, err)

					time.Sleep(100 * time.Millisecond) // let time pass...
					require.NotNil(t, proc)
					assert.False(t, proc.Info(ctx).Successful)
					assert.True(t, time.Since(startAt) < 400*time.Millisecond)
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
					assert.Error(t, err)
					assert.Equal(t, 1, exitCode)
				},
				"WaitGivesProperExitCodeOnSignalDeath": func(ctx context.Context, t *testing.T, opts *jasper.CreateOptions, makep processConstructor) {
					proc, err := makep(ctx, sleepCreateOpts(100))
					require.NoError(t, err)
					require.NotNil(t, proc)
					sig := syscall.SIGTERM
					proc.Signal(ctx, sig)
					exitCode, err := proc.Wait(ctx)
					assert.Error(t, err)
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
						assert.Error(t, err)
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
}
