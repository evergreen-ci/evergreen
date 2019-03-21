package jasper

import (
	"context"
	"syscall"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanTerminationSignalTrigger(t *testing.T) {
	for procName, makeProc := range map[string]ProcessConstructor{
		"Basic":    newBasicProcess,
		"Blocking": newBlockingProcess,
	} {
		t.Run(procName, func(t *testing.T) {
			for testName, testCase := range map[string]func(context.Context, *CreateOptions, ProcessConstructor){
				"CleanTerminationRunsForSIGTERM": func(ctx context.Context, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					trigger := makeCleanTerminationSignalTrigger()
					assert.True(t, trigger(proc.Info(ctx), syscall.SIGTERM))

					exitCode, err := proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Zero(t, exitCode)
					assert.False(t, proc.Running(ctx))

					// Subsequent executions of trigger should fail.
					assert.False(t, trigger(proc.Info(ctx), syscall.SIGTERM))
				},
				"CleanTerminationIgnoresNonSIGTERM": func(ctx context.Context, opts *CreateOptions, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					trigger := makeCleanTerminationSignalTrigger()
					assert.False(t, trigger(proc.Info(ctx), syscall.SIGHUP))

					assert.True(t, proc.Running(ctx))

					assert.NoError(t, proc.Signal(ctx, syscall.SIGKILL))
				},
				"CleanTerminationFailsForExitedProcess": func(ctx context.Context, opts *CreateOptions, makep ProcessConstructor) {
					opts = trueCreateOpts()
					proc, err := makep(ctx, opts)
					require.NoError(t, err)

					exitCode, err := proc.Wait(ctx)
					assert.NoError(t, err)
					assert.Zero(t, exitCode)

					trigger := makeCleanTerminationSignalTrigger()
					assert.False(t, trigger(proc.Info(ctx), syscall.SIGTERM))
				},
			} {
				t.Run(testName, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()

					opts := yesCreateOpts(0)
					testCase(ctx, &opts, makeProc)
				})
			}
		})
	}
}
