package jasper

import (
	"context"
	"syscall"
	"testing"

	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCleanTerminationSignalTrigger(t *testing.T) {
	for procName, makeProc := range map[string]ProcessConstructor{
		"BasicProcess":    newBasicProcess,
		"BlockingProcess": newBlockingProcess,
	} {
		t.Run(procName, func(t *testing.T) {
			for testName, testCase := range map[string]func(context.Context, *options.Create, ProcessConstructor){
				"CleanTerminationRunsForSIGTERM": func(ctx context.Context, opts *options.Create, makep ProcessConstructor) {
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
				"CleanTerminationIgnoresNonSIGTERM": func(ctx context.Context, opts *options.Create, makep ProcessConstructor) {
					proc, err := makep(ctx, opts)
					require.NoError(t, err)
					trigger := makeCleanTerminationSignalTrigger()
					assert.False(t, trigger(proc.Info(ctx), syscall.SIGHUP))

					assert.True(t, proc.Running(ctx))

					assert.NoError(t, proc.Signal(ctx, syscall.SIGKILL))
				},
				"CleanTerminationFailsForExitedProcess": func(ctx context.Context, opts *options.Create, makep ProcessConstructor) {
					opts = testoptions.TrueCreateOpts()
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
					ctx, cancel := context.WithTimeout(context.Background(), testutil.TestTimeout)
					defer cancel()

					testCase(ctx, testoptions.SleepCreateOpts(1), makeProc)
				})
			}
		})
	}
}
