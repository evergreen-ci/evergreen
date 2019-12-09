package wire

import (
	"context"
	"runtime"
	"strings"
	"syscall"
	"testing"
	"time"

	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Note: these tests are largely copied directly from the top level package into
// this package to avoid an import cycle.

func TestWireManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	factory := func(ctx context.Context, t *testing.T) jasper.RemoteClient {
		mngr, err := jasper.NewSynchronizedManager(false)
		require.NoError(t, err)

		client, err := makeTestServiceAndClient(ctx, mngr)
		require.NoError(t, err)
		return client
	}

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
			for name, test := range map[string]func(context.Context, *testing.T, jasper.RemoteClient){
				"ValidateFixture": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					assert.NotNil(t, ctx)
					assert.NotNil(t, client)
				},
				"IDReturnsNonempty": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					assert.NotEmpty(t, client.ID())
				},
				"ProcEnvVarMatchesManagerID": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := testutil.TrueCreateOpts()
					modify.Options(opts)

					proc, err := client.CreateProcess(ctx, opts)
					require.NoError(t, err)
					info := proc.Info(ctx)
					require.NotEmpty(t, info.Options.Environment)
					assert.Equal(t, client.ID(), info.Options.Environment[jasper.ManagerEnvironID])
				},
				"ListDoesNotErrorWhenEmpty": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					all, err := client.List(ctx, options.All)
					require.NoError(t, err)
					assert.Len(t, all, 0)
				},
				"CreateProcessFails": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := &options.Create{}
					modify.Options(opts)
					proc, err := client.CreateProcess(ctx, opts)
					require.Error(t, err)
					assert.Nil(t, proc)
				},
				"ListAllReturnsErrorWithCanceledContext": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					cctx, cancel := context.WithCancel(ctx)
					opts := testutil.TrueCreateOpts()
					modify.Options(opts)
					created, err := createProcs(ctx, opts, client, 10)
					require.NoError(t, err)
					assert.Len(t, created, 10)
					cancel()
					output, err := client.List(cctx, options.All)
					require.Error(t, err)
					assert.Nil(t, output)
				},
				"LongRunningOperationsAreListedAsRunning": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := testutil.SleepCreateOpts(20)
					modify.Options(opts)
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
				"ListReturnsOneSuccessfulCommand": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := testutil.TrueCreateOpts()
					modify.Options(opts)
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
				"GetMethodErrorsWithNoResponse": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					proc, err := client.Get(ctx, "foo")
					require.Error(t, err)
					assert.Nil(t, proc)
				},
				"GetMethodReturnsMatchingDoc": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := testutil.TrueCreateOpts()
					modify.Options(opts)
					proc, err := client.CreateProcess(ctx, opts)
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
					opts := testutil.TrueCreateOpts()
					modify.Options(opts)
					_, err := client.CreateProcess(ctx, opts)
					require.NoError(t, err)

					cctx, cancel := context.WithCancel(ctx)
					cancel()
					procs, err := client.Group(cctx, "foo")
					require.Error(t, err)
					assert.Len(t, procs, 0)
					assert.Contains(t, err.Error(), "canceled")
				},
				"GroupPropagatesMatching": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := testutil.TrueCreateOpts()
					modify.Options(opts)
					proc, err := client.CreateProcess(ctx, opts)
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
					opts := testutil.SleepCreateOpts(100)
					modify.Options(opts)
					_, err := createProcs(ctx, opts, client, 10)
					require.NoError(t, err)
					assert.NoError(t, client.Close(ctx))
				},
				"CloseErrorsWithCanceledContext": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := testutil.SleepCreateOpts(100)
					modify.Options(opts)

					_, err := createProcs(ctx, opts, client, 10)
					require.NoError(t, err)

					cctx, cancel := context.WithCancel(ctx)
					cancel()

					err = client.Close(cctx)
					require.Error(t, err)
					assert.Contains(t, err.Error(), "canceled")
				},
				"CloseSucceedsWithTerminatedProcesses": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					procs, err := createProcs(ctx, testutil.TrueCreateOpts(), client, 10)
					for _, p := range procs {
						_, err = p.Wait(ctx)
						require.NoError(t, err)
					}

					require.NoError(t, err)
					assert.NoError(t, client.Close(ctx))
				},
				"WaitingOnNonExistentProcessErrors": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					proc, err := client.CreateProcess(ctx, testutil.TrueCreateOpts())
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					client.Clear(ctx)

					_, err = proc.Wait(ctx)
					require.Error(t, err)
					assert.Contains(t, err.Error(), "could not get process")
				},
				"ClearCausesDeletionOfProcesses": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := testutil.TrueCreateOpts()
					modify.Options(opts)
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
					opts := testutil.SleepCreateOpts(20)
					modify.Options(opts)
					proc, err := client.CreateProcess(ctx, opts)
					require.NoError(t, err)
					client.Clear(ctx)
					sameProc, err := client.Get(ctx, proc.ID())
					require.NoError(t, err)
					assert.Equal(t, proc.ID(), sameProc.ID())
					require.NoError(t, jasper.Terminate(ctx, proc)) // Clean up
				},
				"ClearSelectivelyDeletesOnlyDeadProcesses": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					trueOpts := testutil.TrueCreateOpts()
					modify.Options(trueOpts)
					lsProc, err := client.CreateProcess(ctx, trueOpts)
					require.NoError(t, err)

					sleepOpts := testutil.SleepCreateOpts(20)
					modify.Options(sleepOpts)
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
				// "": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {},

				///////////////////////////////////
				//
				// The following test cases are added
				// specifically for the MongoDB wire protocol case

				"RegisterIsDisabled": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					err := client.Register(ctx, nil)
					require.Error(t, err)
					assert.Contains(t, err.Error(), "cannot register")
				},
				"CreateProcessReturnsCorrectExample": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := testutil.TrueCreateOpts()
					modify.Options(opts)
					proc, err := client.CreateProcess(ctx, opts)
					require.NoError(t, err)
					assert.NotNil(t, proc)
					assert.NotZero(t, proc.ID())

					fetched, err := client.Get(ctx, proc.ID())
					assert.NoError(t, err)
					assert.NotNil(t, fetched)
					assert.Equal(t, proc.ID(), fetched.ID())
				},
				"WaitOnSigKilledProcessReturnsProperExitCode": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {
					opts := testutil.SleepCreateOpts(100)
					modify.Options(opts)
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
				// "": func(ctx context.Context, t *testing.T, client jasper.RemoteClient) {},
			} {
				t.Run(name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, testutil.RPCTestTimeout)
					defer cancel()
					test(tctx, t, factory(tctx, t))
				})
			}
		})
	}
}

func TestWireProcess(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	makeProc := func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
		mngr, err := jasper.NewSynchronizedManager(false)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		client, err := makeTestServiceAndClient(ctx, mngr)
		if err != nil {
			return nil, errors.WithStack(err)
		}

		return client.CreateProcess(ctx, opts)
	}

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
			for name, testCase := range map[string]func(context.Context, *testing.T, *options.Create, jasper.ProcessConstructor){
				"WithPopulatedArgsCommandCreationPasses": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					assert.NotZero(t, opts.Args)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.NotNil(t, proc)
				},
				"ErrorToCreateWithInvalidArgs": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					opts.Args = []string{}
					proc, err := makep(ctx, opts)
					require.Error(t, err)
					assert.Nil(t, proc)
				},
				"WithCanceledContextProcessCreationFails": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					pctx, pcancel := context.WithCancel(ctx)
					pcancel()
					proc, err := makep(pctx, opts)
					require.Error(t, err)
					assert.Nil(t, proc)
				},
				"CanceledContextTimesOutEarly": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
					defer pcancel()
					startAt := time.Now()
					opts := testutil.SleepCreateOpts(20)
					modify.Options(opts)
					proc, err := makep(pctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)

					time.Sleep(5 * time.Millisecond) // let time pass...
					assert.False(t, proc.Info(ctx).Successful)
					assert.True(t, time.Since(startAt) < 20*time.Second)
				},
				"ProcessLacksTagsByDefault": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					tags := proc.GetTags()
					assert.Empty(t, tags)
				},
				"ProcessTagsPersist": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					opts.Tags = []string{"foo"}
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					tags := proc.GetTags()
					assert.Contains(t, tags, "foo")
				},
				"InfoHasMatchingID": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					_, err = proc.Wait(ctx)
					require.NoError(t, err)
					assert.Equal(t, proc.ID(), proc.Info(ctx).ID)
				},
				"ResetTags": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					proc.Tag("foo")
					assert.Contains(t, proc.GetTags(), "foo")
					proc.ResetTags()
					assert.Len(t, proc.GetTags(), 0)
				},
				"TagsAreSetLike": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)

					for i := 0; i < 100; i++ {
						proc.Tag("foo")
					}

					assert.Len(t, proc.GetTags(), 1)
					proc.Tag("bar")
					assert.Len(t, proc.GetTags(), 2)
				},
				"CompleteIsTrueAfterWait": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					time.Sleep(10 * time.Millisecond) // give the process time to start background machinery
					_, err = proc.Wait(ctx)
					assert.NoError(t, err)
					assert.True(t, proc.Complete(ctx))
				},
				"WaitReturnsWithCanceledContext": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
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
				"RegisterTriggerErrorsForNil": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.Error(t, proc.RegisterTrigger(ctx, nil))
				},
				"RegisterSignalTriggerIDErrorsForExitedProcess": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					_, err = proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Error(t, proc.RegisterSignalTriggerID(ctx, jasper.CleanTerminationSignalTrigger))
				},
				"RegisterSignalTriggerIDFailsWithInvalidTriggerID": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(3)
					modify.Options(opts)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.Error(t, proc.RegisterSignalTriggerID(ctx, jasper.SignalTriggerID(-1)))
				},
				"RegisterSignalTriggerIDPassesWithValidTriggerID": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(3)
					modify.Options(opts)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.NoError(t, proc.RegisterSignalTriggerID(ctx, jasper.CleanTerminationSignalTrigger))
				},
				"WaitOnRespawnedProcessDoesNotError": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
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
				"RespawnedProcessGivesSameResult": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
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
				"RespawningFinishedProcessIsOK": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
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
				"RespawningRunningProcessIsOK": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(2)
					modify.Options(opts)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)

					newProc, err := proc.Respawn(ctx)
					assert.NoError(t, err)
					_, err = newProc.Wait(ctx)
					require.NoError(t, err)
					assert.True(t, newProc.Info(ctx).Successful)
				},
				"RespawnShowsConsistentStateValues": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(3)
					modify.Options(opts)
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
				"WaitGivesSuccessfulExitCode": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.TrueCreateOpts()
					modify.Options(opts)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)
					exitCode, err := proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Equal(t, 0, exitCode)
				},
				"WaitGivesFailureExitCode": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.FalseCreateOpts()
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)
					exitCode, err := proc.Wait(ctx)
					require.Error(t, err)
					assert.Equal(t, 1, exitCode)
				},
				"WaitGivesProperExitCodeOnSignalDeath": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(100)
					modify.Options(opts)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)
					sig := syscall.SIGTERM
					require.NoError(t, proc.Signal(ctx, sig))
					exitCode, err := proc.Wait(ctx)
					require.Error(t, err)
					if runtime.GOOS == "windows" {
						assert.Equal(t, 1, exitCode)
					} else {
						assert.Equal(t, int(sig), exitCode)
					}
				},
				"WaitGivesNegativeOneOnAlternativeError": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					cctx, cancel := context.WithCancel(ctx)
					opts := testutil.SleepCreateOpts(100)
					modify.Options(opts)
					proc, err := makep(ctx, opts)
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
					assert.NoError(t, jasper.Terminate(ctx, proc)) // Clean up.
				},
				"InfoHasTimeoutWhenProcessTimesOut": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(100)
					modify.Options(opts)
					opts.Timeout = time.Second
					opts.TimeoutSecs = 1
					proc, err := makep(ctx, opts)
					require.NoError(t, err)

					exitCode, err := proc.Wait(ctx)
					assert.Error(t, err)
					if runtime.GOOS == "windows" {
						assert.Equal(t, 1, exitCode)
					} else {
						assert.Equal(t, int(syscall.SIGKILL), exitCode)
					}
					info := proc.Info(ctx)
					assert.True(t, info.Timeout)
				},
				"CallingSignalOnDeadProcessDoesError": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)

					_, err = proc.Wait(ctx)
					assert.NoError(t, err)

					err = proc.Signal(ctx, syscall.SIGTERM)
					require.Error(t, err)
					assert.True(t, strings.Contains(err.Error(), "cannot signal a process that has terminated"))
				},
			} {
				t.Run(name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, testutil.RPCTestTimeout)
					defer cancel()

					opts := &options.Create{Args: []string{"ls"}}
					modify.Options(opts)
					testCase(tctx, t, opts, makeProc)
				})
			}
		})
	}
}
