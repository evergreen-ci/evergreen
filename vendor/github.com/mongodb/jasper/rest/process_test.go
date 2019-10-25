package rest

import (
	"context"
	"fmt"
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

func TestProcessImplementations(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	httpClient := testutil.GetHTTPClient()
	defer testutil.PutHTTPClient(httpClient)

	for cname, makeProc := range map[string]jasper.ProcessConstructor{
		"REST": func(ctx context.Context, opts *options.Create) (jasper.Process, error) {
			_, port, err := startRESTService(ctx, httpClient)
			if err != nil {
				return nil, errors.WithStack(err)
			}

			client := &restClient{
				prefix: fmt.Sprintf("http://localhost:%d/jasper/v1", port),
				client: httpClient,
			}

			return client.CreateProcess(ctx, opts)
		},
	} {
		t.Run(cname, func(t *testing.T) {
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
					assert.Error(t, err)
					assert.Nil(t, proc)
				},
				"CanceledContextTimesOutEarly": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					pctx, pcancel := context.WithTimeout(ctx, 5*time.Second)
					defer pcancel()
					startAt := time.Now()
					opts := testutil.SleepCreateOpts(20)
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

					for i := 0; i < 10; i++ {
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
					opts.Args = []string{"sleep", "20"}
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
				"RegisterSignalTriggerErrorsForNil": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.Error(t, proc.RegisterSignalTrigger(ctx, nil))
				},
				"RegisterSignalTriggerErrorsForExitedProcess": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					_, err = proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Error(t, proc.RegisterSignalTrigger(ctx, func(_ jasper.ProcessInfo, _ syscall.Signal) bool { return false }))
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
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					assert.Error(t, proc.RegisterSignalTriggerID(ctx, jasper.SignalTriggerID("foo")))
				},
				"RegisterSignalTriggerIDPassesWithValidTriggerID": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(3)
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
					assert.Equal(t, procExitCode, proc.Info(ctx).ExitCode)
				},
				"RespawningFinishedProcessIsOK": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)
					_, err = proc.Wait(ctx)
					require.NoError(t, err)

					newProc, err := proc.Respawn(ctx)
					require.NoError(t, err)
					require.NotNil(t, newProc)
					_, err = newProc.Wait(ctx)
					require.NoError(t, err)
					assert.True(t, newProc.Info(ctx).Successful)
				},
				"RespawningRunningProcessIsOK": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(2)
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					require.NotNil(t, proc)

					newProc, err := proc.Respawn(ctx)
					require.NoError(t, err)
					require.NotNil(t, newProc)
					_, err = newProc.Wait(ctx)
					require.NoError(t, err)
					assert.True(t, newProc.Info(ctx).Successful)
				},
				"RespawnShowsConsistentStateValues": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(2)
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
					assert.True(t, newProc.Complete(ctx))
				},
				"WaitGivesSuccessfulExitCode": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, testutil.TrueCreateOpts())
					require.NoError(t, err)
					require.NotNil(t, proc)
					exitCode, err := proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Equal(t, 0, exitCode)
				},
				"WaitGivesFailureExitCode": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, testutil.FalseCreateOpts())
					require.NoError(t, err)
					require.NotNil(t, proc)
					exitCode, err := proc.Wait(ctx)
					assert.Error(t, err)
					assert.Equal(t, 1, exitCode)
				},
				"WaitGivesProperExitCodeOnSignalDeath": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					proc, err := makep(ctx, testutil.SleepCreateOpts(100))
					require.NoError(t, err)
					require.NotNil(t, proc)
					sig := syscall.SIGTERM
					assert.NoError(t, proc.Signal(ctx, sig))
					exitCode, err := proc.Wait(ctx)
					assert.Error(t, err)
					if runtime.GOOS == "windows" {
						assert.Equal(t, 1, exitCode)
					} else {
						assert.Equal(t, int(sig), exitCode)
					}
				},
				"WaitGivesNegativeOneOnAlternativeError": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {
					cctx, cancel := context.WithCancel(ctx)
					proc, err := makep(ctx, testutil.SleepCreateOpts(100))
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
				"InfoHasTimeoutWhenProcessTimesOut": func(ctx context.Context, t *testing.T, _ *options.Create, makep jasper.ProcessConstructor) {
					opts := testutil.SleepCreateOpts(100)
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
					assert.True(t, proc.Info(ctx).Timeout)
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
				// "": func(ctx context.Context, t *testing.T, opts *options.Create, makep jasper.ProcessConstructor) {},
			} {
				t.Run(name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, testutil.ProcessTestTimeout)
					defer cancel()

					opts := &options.Create{Args: []string{"ls"}}
					testCase(tctx, t, opts, makeProc)
				})
			}
		})
	}
}
