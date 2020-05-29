// +build linux

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

func TestLinuxProcessTrackerWithCgroups(t *testing.T) {
	if os.Geteuid() != 0 {
		t.Skip("cannot run Linux process tracker tests with cgroups without admin privileges")
	}
	for procName, makeProc := range map[string]ProcessConstructor{
		"Blocking": newBlockingProcess,
		"Basic":    newBasicProcess,
	} {
		t.Run(procName, func(t *testing.T) {

			for name, testCase := range map[string]func(context.Context, *testing.T, *linuxProcessTracker, Process){
				"VerifyInitialSetup": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					require.NotNil(t, tracker.cgroup)
					require.True(t, tracker.validCgroup())
					pids, err := tracker.listCgroupPIDs()
					require.NoError(t, err)
					assert.Len(t, pids, 0)
				},
				"NilCgroupIsInvalid": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					tracker.cgroup = nil
					assert.False(t, tracker.validCgroup())
				},
				"DeletedCgroupIsInvalid": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					require.NoError(t, tracker.cgroup.Delete())
					assert.False(t, tracker.validCgroup())
				},
				"SetDefaultCgroupIfInvalidNoopsIfCgroupIsValid": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					cgroup := tracker.cgroup
					assert.NotNil(t, cgroup)
					assert.NoError(t, tracker.setDefaultCgroupIfInvalid())
					assert.Equal(t, cgroup, tracker.cgroup)
				},
				"SetDefaultCgroupIfNilSetsIfCgroupIsInvalid": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					tracker.cgroup = nil
					assert.NoError(t, tracker.setDefaultCgroupIfInvalid())
					assert.NotNil(t, tracker.cgroup)
				},
				"AddNewProcessSucceeds": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					assert.NoError(t, tracker.Add(proc.Info(ctx)))

					pids, err := tracker.listCgroupPIDs()
					require.NoError(t, err)
					assert.Len(t, pids, 1)
					assert.Contains(t, pids, proc.Info(ctx).PID)
				},
				"DoubleAddProcessSucceedsButDoesNotDuplicateProcessInCgroup": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					pid := proc.Info(ctx).PID
					assert.NoError(t, tracker.Add(proc.Info(ctx)))
					assert.NoError(t, tracker.Add(proc.Info(ctx)))

					pids, err := tracker.listCgroupPIDs()
					require.NoError(t, err)
					assert.Len(t, pids, 1)
					assert.Contains(t, pids, pid)
				},
				"ListCgroupPIDsDoesNotSeeTerminatedProcesses": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					require.NoError(t, tracker.Add(proc.Info(ctx)))

					assert.NoError(t, proc.RegisterSignalTriggerID(ctx, CleanTerminationSignalTrigger))
					err := proc.Signal(ctx, syscall.SIGTERM)
					assert.NoError(t, err)
					exitCode, err := proc.Wait(ctx)
					assert.Error(t, err)
					assert.Equal(t, exitCode, int(syscall.SIGTERM))

					pids, err := tracker.listCgroupPIDs()
					require.NoError(t, err)
					assert.Len(t, pids, 0)
				},
				"ListCgroupPIDsDoesNotErrorIfCgroupDeleted": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					assert.NoError(t, tracker.cgroup.Delete())
					pids, err := tracker.listCgroupPIDs()
					assert.NoError(t, err)
					assert.Len(t, pids, 0)
				},
				"CleanupNoProcsSucceeds": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					pids, err := tracker.listCgroupPIDs()
					require.NoError(t, err)
					assert.Len(t, pids, 0)
					assert.NoError(t, tracker.Cleanup())
				},
				"CleanupTerminatesProcessInCgroup": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					assert.NoError(t, tracker.Add(proc.Info(ctx)))
					assert.NoError(t, tracker.Cleanup())

					procTerminated := make(chan struct{})
					go func() {
						defer close(procTerminated)
						_, _ = proc.Wait(ctx)
					}()

					select {
					case <-procTerminated:
					case <-ctx.Done():
						assert.Fail(t, "context timed out before process was complete")
					}
				},
				"CleanupAfterDoubleAddDoesNotError": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					assert.NoError(t, tracker.Add(proc.Info(ctx)))
					assert.NoError(t, tracker.Add(proc.Info(ctx)))
					assert.NoError(t, tracker.Cleanup())
				},
				"DoubleCleanupDoesNotError": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					assert.NoError(t, tracker.Add(proc.Info(ctx)))
					assert.NoError(t, tracker.Cleanup())
					assert.NoError(t, tracker.Cleanup())
				},
				"AddProcessAfterCleanupSucceeds": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, proc Process) {
					require.NoError(t, tracker.Add(proc.Info(ctx)))
					require.NoError(t, tracker.Cleanup())

					newProc, err := makeProc(ctx, testutil.YesCreateOpts(0))
					require.NoError(t, err)

					require.NoError(t, tracker.Add(newProc.Info(ctx)))
					pids, err := tracker.listCgroupPIDs()
					require.NoError(t, err)
					assert.Len(t, pids, 1)
					assert.Contains(t, pids, newProc.Info(ctx).PID)
				},
			} {
				t.Run(name, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
					defer cancel()

					opts := testutil.YesCreateOpts(testutil.TestTimeout)
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)

					tracker, err := NewProcessTracker("test")
					require.NoError(t, err)
					require.NotNil(t, tracker)
					linuxTracker, ok := tracker.(*linuxProcessTracker)
					require.True(t, ok)
					defer func() {
						// Ensure that the cgroup is cleaned up.
						assert.NoError(t, tracker.Cleanup())
					}()

					testCase(ctx, t, linuxTracker, proc)
				})
			}
		})
	}
}

func TestLinuxProcessTrackerWithEnvironmentVariables(t *testing.T) {
	for procName, makeProc := range map[string]ProcessConstructor{
		"Blocking": newBlockingProcess,
		"Basic":    newBasicProcess,
	} {
		t.Run(procName, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, opts *options.Create, envVarName string, envVarValue string){
				"CleanupFindsProcessesByEnvironmentVariable": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, opts *options.Create, envVarName string, envVarValue string) {
					opts.AddEnvVar(envVarName, envVarValue)
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)
					assert.NoError(t, tracker.Add(proc.Info(ctx)))
					// Cgroup might be re-initialized in Add(), so invalidate
					// it.
					tracker.cgroup = nil
					assert.NoError(t, tracker.Cleanup())

					procTerminated := make(chan struct{})
					go func() {
						defer close(procTerminated)
						_, _ = proc.Wait(ctx)
					}()

					select {
					case <-procTerminated:
					case <-ctx.Done():
						assert.Fail(t, "context timed out before process was complete")
					}
				},
				"CleanupIgnoresAddedProcessesWithoutEnvironmentVariable": func(ctx context.Context, t *testing.T, tracker *linuxProcessTracker, opts *options.Create, envVarName string, envVarValue string) {
					proc, err := makeProc(ctx, opts)
					require.NoError(t, err)
					_, ok := proc.Info(ctx).Options.Environment[envVarName]
					require.False(t, ok)

					assert.NoError(t, tracker.Add(proc.Info(ctx)))
					// Cgroup might be re-initialized in Add(), so invalidate
					// it.
					tracker.cgroup = nil
					assert.NoError(t, tracker.Cleanup())
					assert.True(t, proc.Running(ctx))
				},
				// "": func(ctx, context.Context, t *testing.T, tracker *linuxProcessTracker, envVarName string, envVarValue string) {},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
					defer cancel()

					envVarValue := "bar"

					tracker, err := NewProcessTracker(envVarValue)
					require.NoError(t, err)
					require.NotNil(t, tracker)
					linuxTracker, ok := tracker.(*linuxProcessTracker)
					require.True(t, ok)
					defer func() {
						// Ensure that the cgroup is cleaned up.
						assert.NoError(t, tracker.Cleanup())
					}()
					// Override default cgroup behavior.
					linuxTracker.cgroup = nil

					testCase(ctx, t, linuxTracker, testutil.YesCreateOpts(testutil.TestTimeout), ManagerEnvironID, envVarValue)
				})
			}
		})
	}
}

func TestManagerSetsEnvironmentVariables(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for managerName, makeManager := range map[string]func() *basicProcessManager{
		"Basic": func() *basicProcessManager {
			return &basicProcessManager{
				procs: map[string]Process{},
				tracker: &mockProcessTracker{
					Infos: []ProcessInfo{},
				},
			}
		},
	} {
		t.Run(managerName, func(t *testing.T) {
			for testName, testCase := range map[string]func(context.Context, *testing.T, *basicProcessManager){
				"CreateProcessSetsManagerEnvironmentVariables": func(ctx context.Context, t *testing.T, manager *basicProcessManager) {
					proc, err := manager.CreateProcess(ctx, testutil.YesCreateOpts(testutil.ManagerTestTimeout))
					require.NoError(t, err)

					env := proc.Info(ctx).Options.Environment
					require.NotNil(t, env)
					value, ok := env[ManagerEnvironID]
					require.True(t, ok)
					assert.Equal(t, value, manager.id, "process should have manager environment variable set")
				},
				"CreateCommandAddsEnvironmentVariables": func(ctx context.Context, t *testing.T, manager *basicProcessManager) {
					envVar := ManagerEnvironID
					value := manager.id

					cmdArgs := []string{"yes"}
					cmd := manager.CreateCommand(ctx).AddEnv(ManagerEnvironID, manager.id).Add(cmdArgs).Background(true)
					require.NoError(t, cmd.Run(ctx))

					ids := cmd.GetProcIDs()
					require.Len(t, ids, 1)
					proc, err := manager.Get(ctx, ids[0])
					require.NoError(t, err)
					env := proc.Info(ctx).Options.Environment
					require.NotNil(t, env)
					actualValue, ok := env[envVar]
					require.True(t, ok)
					assert.Equal(t, value, actualValue)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(ctx, testutil.ManagerTestTimeout)
					defer cancel()
					testCase(tctx, t, makeManager())
				})
			}
		})
	}
}
