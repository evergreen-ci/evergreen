// +build windows

package jasper

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func makeTracker() (*windowsProcessTracker, error) {
	tracker, err := newProcessTracker("foo" + uuid.Must(uuid.NewV4()).String())
	if err != nil {
		return nil, err
	}

	windowsTracker, ok := tracker.(*windowsProcessTracker)
	if !ok {
		return nil, errors.New("not a Windows process tracker")
	}
	return windowsTracker, nil
}

func makeAndStartYesCommand(ctx context.Context) (*exec.Cmd, error) {
	cmd := exec.CommandContext(ctx, "yes", "yes")
	err := cmd.Start()
	return cmd, err
}

func TestWindowsProcessTracker(t *testing.T) {
	for testName, testCase := range map[string]func(context.Context, *testing.T, *windowsProcessTracker){
		"NewWindowsProcessTrackerCreatesJob": func(_ context.Context, t *testing.T, tracker *windowsProcessTracker) {
			info, err := QueryInformationJobObjectProcessIdList(tracker.job.handle)
			assert.NoError(t, err)
			assert.Equal(t, 0, int(info.NumberOfAssignedProcesses))

			assert.NoError(t, tracker.job.Close())
		},
		"AddProcessToTrackerAssignsPid": func(ctx context.Context, t *testing.T, tracker *windowsProcessTracker) {
			cmd1, err := makeAndStartYesCommand(ctx)
			require.NoError(t, err)
			pid1 := uint(cmd1.Process.Pid)
			assert.NoError(t, tracker.add(pid1))

			cmd2, err := makeAndStartYesCommand(ctx)
			require.NoError(t, err)
			pid2 := uint(cmd2.Process.Pid)
			assert.NoError(t, tracker.add(pid2))

			info, err := QueryInformationJobObjectProcessIdList(tracker.job.handle)
			assert.NoError(t, err)
			assert.Equal(t, 2, int(info.NumberOfAssignedProcesses))
			assert.Equal(t, info.ProcessIdList[0], uint64(pid1))
			assert.Equal(t, info.ProcessIdList[1], uint64(pid2))

			assert.NoError(t, tracker.job.Close())
		},
		"AddedProcessIsTerminatedOnCleanup": func(ctx context.Context, t *testing.T, tracker *windowsProcessTracker) {
			cmd, err := makeAndStartYesCommand(ctx)
			require.NoError(t, err)
			pid := uint(cmd.Process.Pid)

			assert.NoError(t, tracker.add(pid))

			info, err := QueryInformationJobObjectProcessIdList(tracker.job.handle)
			assert.NoError(t, err)
			assert.Equal(t, 1, int(info.NumberOfAssignedProcesses))

			procHandle, err := OpenProcess(PROCESS_ALL_ACCESS, false, uint32(pid))
			assert.NoError(t, err)

			assert.NoError(t, tracker.cleanup())

			waitEvent, err := WaitForSingleObject(procHandle, 60*time.Second)
			assert.NoError(t, err)
			assert.Equal(t, WAIT_OBJECT_0, waitEvent)
			assert.NoError(t, CloseHandle(procHandle))

			assert.NoError(t, cmd.Wait())
			assert.NotNil(t, cmd.ProcessState)
			waitStatus := cmd.ProcessState.Sys().(syscall.WaitStatus)
			assert.True(t, waitStatus.Exited())
		},
	} {
		t.Run(testName, func(t *testing.T) {
			if _, runningInEvgAgent := os.LookupEnv("EVR_TASK_ID"); runningInEvgAgent {
				t.Skip("Evergreen makes its own job object, so these will not pass in Evergreen tests ",
					"(although they will pass if locally run).")
			}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			tracker, err := makeTracker()
			require.NoError(t, err)
			require.NotNil(t, tracker)

			testCase(ctx, t, tracker)
		})
	}
}
