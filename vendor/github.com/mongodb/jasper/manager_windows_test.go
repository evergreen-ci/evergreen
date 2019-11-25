// +build windows

package jasper

import (
	"context"
	"os"
	"syscall"
	"testing"

	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBasicManagerWithTrackedProcesses(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for managerName, makeManager := range map[string]func(ctx context.Context, t *testing.T) *basicProcessManager{
		"Basic/NoLock/BasicProcs": func(ctx context.Context, t *testing.T) *basicProcessManager {
			basicManager, err := newBasicProcessManager(map[string]Process{}, false, true, false)
			require.NoError(t, err)
			return basicManager.(*basicProcessManager)
		},
		"Basic/NoLock/BlockingProcs": func(ctx context.Context, t *testing.T) *basicProcessManager {
			basicBlockingManager, err := newBasicProcessManager(map[string]Process{}, true, true, false)
			require.NoError(t, err)
			return basicBlockingManager.(*basicProcessManager)
		},
	} {
		t.Run(managerName, func(t *testing.T) {
			for testName, testCase := range map[string]func(context.Context, *testing.T, *basicProcessManager, *windowsProcessTracker, *options.Create){
				"ProcessTrackerCreatedEmpty": func(_ context.Context, t *testing.T, m *basicProcessManager, tracker *windowsProcessTracker, _ *options.Create) {
					require.NotNil(t, tracker.job)

					info, err := QueryInformationJobObjectProcessIdList(tracker.job.handle)
					assert.NoError(t, err)
					assert.Zero(t, info.NumberOfAssignedProcesses)
				},
				"CreateAddsProcess": func(ctx context.Context, t *testing.T, m *basicProcessManager, tracker *windowsProcessTracker, opts *options.Create) {
					proc, err := m.CreateProcess(ctx, opts)
					require.NoError(t, err)

					info, err := QueryInformationJobObjectProcessIdList(tracker.job.handle)
					assert.NoError(t, err)
					assert.Equal(t, 1, int(info.NumberOfAssignedProcesses))
					assert.Equal(t, proc.Info(ctx).PID, int(info.ProcessIdList[0]))
					assert.NoError(t, m.Close(ctx))
				},
				"RegisterAddsProcess": func(ctx context.Context, t *testing.T, m *basicProcessManager, tracker *windowsProcessTracker, opts *options.Create) {
					proc, err := newBasicProcess(ctx, opts)
					require.NoError(t, err)
					assert.NoError(t, m.Register(ctx, proc))

					info, err := QueryInformationJobObjectProcessIdList(tracker.job.handle)
					assert.NoError(t, err)
					assert.Equal(t, 1, int(info.NumberOfAssignedProcesses))
					assert.Equal(t, proc.Info(ctx).PID, int(info.ProcessIdList[0]))
					assert.NoError(t, m.Close(ctx))
				},
				"ClosePerformsProcessTrackingCleanup": func(ctx context.Context, t *testing.T, m *basicProcessManager, tracker *windowsProcessTracker, opts *options.Create) {
					proc, err := m.CreateProcess(ctx, opts)
					require.NoError(t, err)

					info, err := QueryInformationJobObjectProcessIdList(tracker.job.handle)
					assert.NoError(t, err)
					assert.Equal(t, 1, int(info.NumberOfAssignedProcesses))
					assert.Equal(t, proc.Info(ctx).PID, int(info.ProcessIdList[0]))
					assert.NoError(t, m.Close(ctx))

					exitCode, err := proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Zero(t, exitCode)
					assert.False(t, proc.Running(ctx))
					assert.True(t, proc.Complete(ctx))
				},
				"CloseOnTerminatedProcessSucceeds": func(ctx context.Context, t *testing.T, m *basicProcessManager, tracker *windowsProcessTracker, opts *options.Create) {
					proc, err := m.CreateProcess(ctx, opts)
					require.NoError(t, err)

					info, err := QueryInformationJobObjectProcessIdList(tracker.job.handle)
					assert.NoError(t, err)
					assert.Equal(t, 1, int(info.NumberOfAssignedProcesses))
					assert.Equal(t, proc.Info(ctx).PID, int(info.ProcessIdList[0]))

					assert.NoError(t, proc.Signal(ctx, syscall.SIGKILL))
					assert.NoError(t, m.Close(ctx))
				},
			} {
				t.Run(testName, func(t *testing.T) {
					if _, runningInEvgAgent := os.LookupEnv("EVR_TASK_ID"); runningInEvgAgent {
						t.Skip("Evergreen makes its own job object, so these will not pass in Evergreen tests ",
							"(although they will pass if locally run).")
					}
					tctx, cancel := context.WithTimeout(ctx, testutil.LongTestTimeout)
					defer cancel()
					manager := makeManager(tctx, t)
					tracker, ok := manager.tracker.(*windowsProcessTracker)
					require.True(t, ok)
					testCase(tctx, t, manager, tracker, testutil.YesCreateOpts(0))
				})
			}
		})
	}
}
