package jasper

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
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ProcessTestCase represents a test case including a constructor and options to
// create a process.
type ProcessTestCase struct {
	Name string
	Case func(context.Context, *testing.T, *options.Create, ProcessConstructor)
}

// ProcessTests returns the common test suite for the Process interface. This
// should be used for testing purposes only.
func ProcessTests() []ProcessTestCase {
	return []ProcessTestCase{
		{
			Name: "WithPopulatedArgsCommandCreationPasses",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				assert.NotZero(t, opts.Args)
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				assert.NotNil(t, proc)
			},
		},
		{
			Name: "ErrorToCreateWithInvalidArgs",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Args = []string{}
				proc, err := makeProc(ctx, opts)
				assert.Error(t, err)
				assert.Nil(t, proc)
			},
		},
		{
			Name: "WithCanceledContextProcessCreationFails",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				pctx, pcancel := context.WithCancel(ctx)
				pcancel()
				proc, err := makeProc(pctx, opts)
				assert.Error(t, err)
				assert.Nil(t, proc)
			},
		},
		{
			Name: "ProcessLacksTagsByDefault",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				tags := proc.GetTags()
				assert.Empty(t, tags)
			},
		},
		{
			Name: "ProcessTagsPersist",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Tags = []string{"foo"}
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				tags := proc.GetTags()
				assert.Contains(t, tags, "foo")
			},
		},
		{
			Name: "InfoHasMatchingID",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				_, err = proc.Wait(ctx)
				require.NoError(t, err)
				assert.Equal(t, proc.ID(), proc.Info(ctx).ID)
			},
		},
		{
			Name: "InfoHasTimeoutWhenProcessTimesOut",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Args = testoptions.SleepCreateOpts(5).Args
				opts.Timeout = time.Second
				opts.TimeoutSecs = 1
				proc, err := makeProc(ctx, opts)
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
		},
		{
			Name: "InfoTagsMatchGetTags",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Tags = []string{"foo"}
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				tags := proc.GetTags()
				assert.Contains(t, tags, "foo")
				assert.Equal(t, tags, proc.Info(ctx).Options.Tags)

				proc.ResetTags()
				tags = proc.GetTags()
				assert.Empty(t, tags)
				assert.Empty(t, proc.Info(ctx).Options.Tags)
			},
		},
		{
			Name: "ResetTagsClearsAllTags",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				proc.Tag("foo")
				assert.Contains(t, proc.GetTags(), "foo")
				proc.ResetTags()
				assert.Len(t, proc.GetTags(), 0)
			},
		},
		{
			Name: "TagsHaveSetSemantics",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)

				for i := 0; i < 10; i++ {
					proc.Tag("foo")
				}

				assert.Len(t, proc.GetTags(), 1)
				proc.Tag("bar")
				assert.Len(t, proc.GetTags(), 2)
			},
		},
		{
			Name: "CompleteIsTrueAfterWait",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				_, err = proc.Wait(ctx)
				assert.NoError(t, err)
				assert.True(t, proc.Complete(ctx))
			},
		},
		{
			Name: "WaitReturnsErrorWithCanceledContext",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Args = testoptions.SleepCreateOpts(20).Args
				pctx, pcancel := context.WithCancel(ctx)
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				assert.True(t, proc.Running(ctx))
				pcancel()
				_, err = proc.Wait(pctx)
				assert.Error(t, err)
			},
		},
		{
			Name: "RegisterTriggerErrorsWithNilTrigger",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				assert.Error(t, proc.RegisterTrigger(ctx, nil))
			},
		},
		{
			Name: "RegisterSignalTriggerErrorsWithNilTrigger",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				assert.Error(t, proc.RegisterSignalTrigger(ctx, nil))
			},
		},
		{
			Name: "RegisterSignalTriggerErrorsWithExitedProcess",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				_, err = proc.Wait(ctx)
				assert.NoError(t, err)
				assert.Error(t, proc.RegisterSignalTrigger(ctx, func(ProcessInfo, syscall.Signal) bool { return false }))
			},
		},
		{
			Name: "RegisterSignalTriggerIDErrorsWithExitedProcess",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				_, err = proc.Wait(ctx)
				assert.NoError(t, err)
				assert.Error(t, proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger))
			},
		},
		{
			Name: "RegisterSignalTriggerIDFailsWithInvalidTriggerID",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Args = testoptions.SleepCreateOpts(3).Args
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				assert.Error(t, proc.RegisterSignalTriggerID(ctx, SignalTriggerID("foo")))
			},
		},
		{
			Name: "RegisterSignalTriggerIDPassesWithValidTriggerID",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Args = testoptions.SleepCreateOpts(3).Args
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				assert.NoError(t, proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger))
			},
		},
		{
			Name: "WaitOnRespawnedProcessDoesNotError",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				require.NotNil(t, proc)
				_, err = proc.Wait(ctx)
				require.NoError(t, err)

				newProc, err := proc.Respawn(ctx)
				require.NoError(t, err)
				_, err = newProc.Wait(ctx)
				assert.NoError(t, err)
			},
		},
		{
			Name: "RespawnedProcessGivesSameResult",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				require.NotNil(t, proc)

				_, err = proc.Wait(ctx)
				require.NoError(t, err)
				exitCode := proc.Info(ctx).ExitCode

				newProc, err := proc.Respawn(ctx)
				require.NoError(t, err)
				_, err = newProc.Wait(ctx)
				require.NoError(t, err)
				assert.Equal(t, exitCode, proc.Info(ctx).ExitCode)
			},
		},
		{
			Name: "RespawningCompletedProcessIsOK",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
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
		},
		{
			Name: "RespawningRunningProcessIsOK",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Args = testoptions.SleepCreateOpts(2).Args
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				require.NotNil(t, proc)

				newProc, err := proc.Respawn(ctx)
				require.NoError(t, err)
				require.NotNil(t, newProc)
				_, err = newProc.Wait(ctx)
				require.NoError(t, err)
				assert.True(t, newProc.Info(ctx).Successful)
			},
		},
		{
			Name: "RespawnShowsConsistentStateValues",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Args = testoptions.SleepCreateOpts(2).Args
				proc, err := makeProc(ctx, opts)
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
		},
		{
			Name: "WaitGivesSuccessfulExitCode",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Args = testoptions.TrueCreateOpts().Args
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				require.NotNil(t, proc)
				exitCode, err := proc.Wait(ctx)
				assert.NoError(t, err)
				assert.Equal(t, 0, exitCode)
			},
		},
		{
			Name: "WaitGivesFailureExitCode",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				opts.Args = testoptions.FalseCreateOpts().Args
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)
				require.NotNil(t, proc)
				exitCode, err := proc.Wait(ctx)
				assert.Error(t, err)
				assert.Equal(t, 1, exitCode)
			},
		},
		{
			Name: "WaitGivesProperExitCodeOnSignalTerminate",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, testoptions.SleepCreateOpts(5))
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
		},
		{
			Name: "WaitGivesNegativeOneOnAlternativeError",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, testoptions.SleepCreateOpts(5))
				require.NoError(t, err)
				require.NotNil(t, proc)

				var exitCode int
				waitFinished := make(chan bool)
				cctx, cancel := context.WithCancel(ctx)
				cancel()
				go func() {
					exitCode, err = proc.Wait(cctx)
					select {
					case waitFinished <- true:
					case <-ctx.Done():
					}
				}()
				select {
				case <-waitFinished:
					assert.Error(t, err)
					assert.Equal(t, -1, exitCode)
				case <-ctx.Done():
					assert.Fail(t, "call to Wait() took too long to finish")
				}
			},
		},
		{
			Name: "SignalingCompletedProcessErrors",
			Case: func(ctx context.Context, t *testing.T, opts *options.Create, makeProc ProcessConstructor) {
				proc, err := makeProc(ctx, opts)
				require.NoError(t, err)

				_, err = proc.Wait(ctx)
				assert.NoError(t, err)

				err = proc.Signal(ctx, syscall.SIGTERM)
				require.Error(t, err)
				assert.True(t, strings.Contains(err.Error(), "cannot signal a process that has terminated"))
			},
		},
	}
}

// ManagerTestCase represents a test case including a manager and
// options.ModifyOpts to modify process creation options.
type ManagerTestCase struct {
	Name string
	Case func(context.Context, *testing.T, Manager, testoptions.ModifyOpts)
}

// ManagerTests returns the common test suite for the Manager interface. This
// should be used for testing purposes only.
func ManagerTests() []ManagerTestCase {
	return []ManagerTestCase{
		{
			Name: "ValidateFixture",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				assert.NotNil(t, ctx)
				assert.NotNil(t, mngr)
				assert.NotNil(t, mngr.LoggingCache(ctx))
			},
		},
		{
			Name: "IDReturnsNonempty",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				assert.NotEmpty(t, mngr.ID())
			},
		},
		{
			Name: "ProcEnvVarMatchesManagerID",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())
				proc, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)
				info := proc.Info(ctx)
				require.NotEmpty(t, info.Options.Environment)
				assert.Equal(t, mngr.ID(), info.Options.Environment[ManagerEnvironID])
			},
		},
		{
			Name: "CreateProcessFailsWithEmptyOptions",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(&options.Create{})
				proc, err := mngr.CreateProcess(ctx, opts)
				require.Error(t, err)
				assert.Nil(t, proc)
			},
		},
		{
			Name: "CreateSimpleProcess",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())
				proc, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)
				assert.NotNil(t, proc)
				info := proc.Info(ctx)
				assert.True(t, info.IsRunning || info.Complete)
			},
		},
		{
			Name: "ListWithoutResultsDoesNotError",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				procs, err := mngr.List(ctx, options.All)
				require.NoError(t, err)
				assert.Len(t, procs, 0)
			},
		},
		{
			Name: "ListAllProcesses",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())
				created, err := createProcs(ctx, opts, mngr, 10)
				require.NoError(t, err)
				assert.Len(t, created, 10)
				all, err := mngr.List(ctx, options.All)
				require.NoError(t, err)
				assert.Len(t, all, 10)
			},
		},
		{
			Name: "ListAllErrorsWithCanceledContext",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())
				created, err := createProcs(ctx, opts, mngr, 10)
				require.NoError(t, err)
				assert.Len(t, created, 10)

				cctx, cancel := context.WithCancel(ctx)
				cancel()
				procs, err := mngr.List(cctx, options.All)
				require.Error(t, err)
				assert.Nil(t, procs)
			},
		},
		{
			Name: "LongRunningProcessesAreListedAsRunning",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.SleepCreateOpts(20))
				procs, err := createProcs(ctx, opts, mngr, 10)
				require.NoError(t, err)
				assert.Len(t, procs, 10)

				procs, err = mngr.List(ctx, options.All)
				require.NoError(t, err)
				assert.Len(t, procs, 10)

				procs, err = mngr.List(ctx, options.Running)
				require.NoError(t, err)
				assert.Len(t, procs, 10)

				procs, err = mngr.List(ctx, options.Successful)
				require.NoError(t, err)
				assert.Len(t, procs, 0)
			},
		},
		{
			Name: "ListReturnsOneSuccessfulProcess",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())

				proc, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)

				_, err = proc.Wait(ctx)
				require.NoError(t, err)

				procs, err := mngr.List(ctx, options.Successful)
				require.NoError(t, err)

				require.Len(t, procs, 1)
				assert.Equal(t, proc.ID(), procs[0].ID())
			},
		},
		{
			Name: "ListReturnsOneFailedProcess",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.FalseCreateOpts())

				proc, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)
				_, err = proc.Wait(ctx)
				require.Error(t, err)

				procs, err := mngr.List(ctx, options.Failed)
				require.NoError(t, err)

				require.Len(t, procs, 1)
				assert.Equal(t, proc.ID(), procs[0].ID())
			},
		},
		{
			Name: "ListErrorsWithInvalidFilter",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				procs, err := mngr.List(ctx, options.Filter("foo"))
				assert.Error(t, err)
				assert.Empty(t, procs)
			},
		},
		{
			Name: "GetProcessErrorsWithNonexistentProcess",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				proc, err := mngr.Get(ctx, "foo")
				require.Error(t, err)
				assert.Nil(t, proc)
			},
		},
		{
			Name: "GetProcessReturnsMatchingProcess",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())
				proc, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)

				getProc, err := mngr.Get(ctx, proc.ID())
				require.NoError(t, err)
				assert.Equal(t, proc.ID(), getProc.ID())
			},
		},
		{
			Name: "GroupWithoutResultsDoesNotError",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				procs, err := mngr.Group(ctx, "foo")
				require.NoError(t, err)
				assert.Len(t, procs, 0)
			},
		},
		{
			Name: "GroupErrorsWithCanceledContext",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())
				_, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)

				cctx, cancel := context.WithCancel(ctx)
				cancel()
				procs, err := mngr.Group(cctx, "foo")
				require.Error(t, err)
				assert.Len(t, procs, 0)
			},
		},
		{
			Name: "GroupReturnsMatchingProcesses",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())
				proc, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)
				proc.Tag("foo")

				procs, err := mngr.Group(ctx, "foo")
				require.NoError(t, err)
				require.Len(t, procs, 1)
				assert.Equal(t, proc.ID(), procs[0].ID())
			},
		},
		{
			Name: "CloseEmptyManagerNoops",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, moidfyOpts testoptions.ModifyOpts) {
				assert.NoError(t, mngr.Close(ctx))
			},
		},
		{
			Name: "CloseErrorsWithCanceledContext",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.SleepCreateOpts(5))
				_, err := createProcs(ctx, opts, mngr, 10)
				require.NoError(t, err)

				cctx, cancel := context.WithCancel(ctx)
				cancel()

				assert.Error(t, mngr.Close(cctx))
			},
		},
		{
			Name: "CloseSucceedsWithTerminatedProcesses",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())
				procs, err := createProcs(ctx, opts, mngr, 10)
				for _, proc := range procs {
					_, err = proc.Wait(ctx)
					require.NoError(t, err)
				}

				require.NoError(t, err)
				assert.NoError(t, mngr.Close(ctx))
			},
		},
		{
			Name: "CloseSucceedsOnRunningProcesses",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				if runtime.GOOS == "windows" {
					t.Skip("manager close tests will error due to process termination on Windows")
				}
				opts := modifyOpts(testoptions.SleepCreateOpts(5))

				_, err := createProcs(ctx, opts, mngr, 10)
				require.NoError(t, err)
				assert.NoError(t, mngr.Close(ctx))
			},
		},
		{
			Name: "ClearCausesDeletionOfCompletedProcesses",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.TrueCreateOpts())
				proc, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)

				sameProc, err := mngr.Get(ctx, proc.ID())
				require.NoError(t, err)
				require.Equal(t, proc.ID(), sameProc.ID())

				_, err = proc.Wait(ctx)
				require.NoError(t, err)

				mngr.Clear(ctx)
				nilProc, err := mngr.Get(ctx, proc.ID())
				require.Error(t, err)
				assert.Nil(t, nilProc)
			},
		},
		{
			Name: "ClearIsANoopForRunningProcesses",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := modifyOpts(testoptions.SleepCreateOpts(5))
				proc, err := mngr.CreateProcess(ctx, opts)
				require.NoError(t, err)

				mngr.Clear(ctx)
				sameProc, err := mngr.Get(ctx, proc.ID())
				require.NoError(t, err)
				assert.Equal(t, proc.ID(), sameProc.ID())
			},
		},
		{
			Name: "ClearSelectivelyDeletesOnlyCompletedProcesses",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				trueOpts := modifyOpts(testoptions.TrueCreateOpts())
				trueProc, err := mngr.CreateProcess(ctx, trueOpts)
				require.NoError(t, err)

				sleepOpts := modifyOpts(testoptions.SleepCreateOpts(5))
				sleepProc, err := mngr.CreateProcess(ctx, sleepOpts)
				require.NoError(t, err)

				_, err = trueProc.Wait(ctx)
				require.NoError(t, err)

				require.True(t, sleepProc.Running(ctx))

				mngr.Clear(ctx)

				sameSleepProc, err := mngr.Get(ctx, sleepProc.ID())
				require.NoError(t, err)
				assert.Equal(t, sleepProc.ID(), sameSleepProc.ID())

				nilProc, err := mngr.Get(ctx, trueProc.ID())
				require.Error(t, err)
				assert.Nil(t, nilProc)
			},
		},
		{
			Name: "CreateCommandPasses",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				cmd := mngr.CreateCommand(ctx)
				args := testoptions.TrueCreateOpts().Args
				cmd.ApplyFromOpts(modifyOpts(&options.Create{})).Add(args)
				assert.NoError(t, cmd.Run(ctx))
			},
		},
		{
			Name: "RunningCommandCreatesNewProcesses",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				cmd := mngr.CreateCommand(ctx)
				trueCmd := testoptions.TrueCreateOpts().Args
				subCmds := [][]string{trueCmd, trueCmd, trueCmd}
				cmd.ApplyFromOpts(modifyOpts(&options.Create{})).Extend(subCmds)
				require.NoError(t, cmd.Run(ctx))

				allProcs, err := mngr.List(ctx, options.All)
				require.NoError(t, err)

				assert.Len(t, allProcs, len(subCmds))
			},
		},
		{

			Name: "CommandProcessIDsMatchManagedProcessIDs",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				cmd := mngr.CreateCommand(ctx)
				trueCmd := testoptions.TrueCreateOpts().Args
				subCmds := [][]string{trueCmd, trueCmd, trueCmd}
				cmd.ApplyFromOpts(modifyOpts(&options.Create{})).Extend(subCmds)
				require.NoError(t, cmd.Run(ctx))

				allProcs, err := mngr.List(ctx, options.All)
				require.NoError(t, err)

				procsContainID := func(procs []Process, procID string) bool {
					for _, proc := range procs {
						if proc.ID() == procID {
							return true
						}
					}
					return false
				}

				for _, procID := range cmd.GetProcIDs() {
					assert.True(t, procsContainID(allProcs, procID))
				}
			},
		},
		{
			Name: "WriteFileSucceeds",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				tmpFile, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
				require.NoError(t, err)
				defer func() {
					assert.NoError(t, os.RemoveAll(tmpFile.Name()))
				}()
				require.NoError(t, tmpFile.Close())

				opts := options.WriteFile{Path: tmpFile.Name(), Content: []byte("foo")}
				require.NoError(t, mngr.WriteFile(ctx, opts))

				content, err := ioutil.ReadFile(tmpFile.Name())
				require.NoError(t, err)

				assert.Equal(t, opts.Content, content)
			},
		},
		{
			Name: "WriteFileAcceptsContentFromReader",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				tmpFile, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
				require.NoError(t, err)
				defer func() {
					assert.NoError(t, os.RemoveAll(tmpFile.Name()))
				}()
				require.NoError(t, tmpFile.Close())

				buf := []byte("foo")
				opts := options.WriteFile{Path: tmpFile.Name(), Reader: bytes.NewBuffer(buf)}
				require.NoError(t, mngr.WriteFile(ctx, opts))

				content, err := ioutil.ReadFile(tmpFile.Name())
				require.NoError(t, err)

				assert.Equal(t, buf, content)
			},
		},
		{
			Name: "WriteFileSucceedsWithLargeContent",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				tmpFile, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
				require.NoError(t, err)
				defer func() {
					assert.NoError(t, os.RemoveAll(tmpFile.Name()))
				}()
				require.NoError(t, tmpFile.Close())

				const mb = 1024 * 1024
				opts := options.WriteFile{Path: tmpFile.Name(), Content: bytes.Repeat([]byte("foo"), mb)}
				require.NoError(t, mngr.WriteFile(ctx, opts))

				content, err := ioutil.ReadFile(tmpFile.Name())
				require.NoError(t, err)

				assert.Equal(t, opts.Content, content)
			},
		},
		{
			Name: "WriteFileSucceedsWithLargeContentFromReader",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				tmpFile, err := ioutil.TempFile(testutil.BuildDirectory(), filepath.Base(t.Name()))
				require.NoError(t, err)
				defer func() {
					assert.NoError(t, tmpFile.Close())
					assert.NoError(t, os.RemoveAll(tmpFile.Name()))
				}()

				const mb = 1024 * 1024
				buf := bytes.Repeat([]byte("foo"), 2*mb)
				opts := options.WriteFile{Path: tmpFile.Name(), Reader: bytes.NewBuffer(buf)}
				require.NoError(t, mngr.WriteFile(ctx, opts))

				content, err := ioutil.ReadFile(tmpFile.Name())
				require.NoError(t, err)

				assert.Equal(t, buf, content)
			},
		},
		{
			Name: "WriteFileSucceedsWithNoContent",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				path := filepath.Join(testutil.BuildDirectory(), filepath.Base(t.Name()))
				require.NoError(t, os.RemoveAll(path))
				defer func() {
					assert.NoError(t, os.RemoveAll(path))
				}()

				opts := options.WriteFile{Path: path}
				require.NoError(t, mngr.WriteFile(ctx, opts))

				stat, err := os.Stat(path)
				require.NoError(t, err)

				assert.Zero(t, stat.Size())
			},
		},
		{
			Name: "WriteFileFailsWithInvalidPath",
			Case: func(ctx context.Context, t *testing.T, mngr Manager, modifyOpts testoptions.ModifyOpts) {
				opts := options.WriteFile{Content: []byte("foo")}
				assert.Error(t, mngr.WriteFile(ctx, opts))
			},
		},
	}
}

// createProcs makes N identical processes with the given manager.
func createProcs(ctx context.Context, opts *options.Create, mngr Manager, num int) ([]Process, error) {
	catcher := grip.NewBasicCatcher()
	var procs []Process
	for i := 0; i < num; i++ {
		optsCopy := *opts

		proc, err := mngr.CreateProcess(ctx, &optsCopy)
		catcher.Add(err)
		if proc != nil {
			procs = append(procs, proc)
		}
	}

	return procs, catcher.Resolve()
}
