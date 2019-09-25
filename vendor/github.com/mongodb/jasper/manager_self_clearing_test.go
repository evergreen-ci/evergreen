package jasper

import (
	"context"
	"testing"

	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registerBasedCreate(ctx context.Context, m *selfClearingProcessManager, t *testing.T, opts *options.Create) (Process, error) {
	sleep, err := newBlockingProcess(ctx, testutil.SleepCreateOpts(10))
	require.NoError(t, err)
	require.NotNil(t, sleep)
	err = m.Register(ctx, sleep)
	if err != nil {
		// Mimic the behavior of Create()'s error return.
		return nil, err
	}

	return sleep, err
}

func pureCreate(ctx context.Context, m *selfClearingProcessManager, t *testing.T, opts *options.Create) (Process, error) {
	return m.CreateProcess(ctx, opts)
}

func fillUp(ctx context.Context, t *testing.T, manager *selfClearingProcessManager, numProcs int) {
	procs, err := createProcs(ctx, testutil.SleepCreateOpts(5), manager, numProcs)
	require.NoError(t, err)
	require.Len(t, procs, numProcs)
}

func TestSelfClearingManager(t *testing.T) {
	for mname, createFunc := range map[string]func(context.Context, *selfClearingProcessManager, *testing.T, *options.Create) (Process, error){
		"Create":   pureCreate,
		"Register": registerBasedCreate,
	} {
		t.Run(mname, func(t *testing.T) {
			for name, test := range map[string]func(context.Context, *testing.T, *selfClearingProcessManager){
				"SucceedsWhenFree": func(ctx context.Context, t *testing.T, manager *selfClearingProcessManager) {
					proc, err := createFunc(ctx, manager, t, testutil.TrueCreateOpts())
					assert.NoError(t, err)
					assert.NotNil(t, proc)
				},
				"ErrorsWhenFull": func(ctx context.Context, t *testing.T, manager *selfClearingProcessManager) {
					fillUp(ctx, t, manager, manager.maxProcs)
					sleep, err := createFunc(ctx, manager, t, testutil.SleepCreateOpts(10))
					assert.Error(t, err)
					assert.Nil(t, sleep)
				},
				"PartiallySucceedsWhenAlmostFull": func(ctx context.Context, t *testing.T, manager *selfClearingProcessManager) {
					fillUp(ctx, t, manager, manager.maxProcs-1)
					firstSleep, err := createFunc(ctx, manager, t, testutil.SleepCreateOpts(10))
					assert.NoError(t, err)
					assert.NotNil(t, firstSleep)
					secondSleep, err := createFunc(ctx, manager, t, testutil.SleepCreateOpts(10))
					assert.Error(t, err)
					assert.Nil(t, secondSleep)
				},
				"InitialFailureIsResolvedByWaiting": func(ctx context.Context, t *testing.T, manager *selfClearingProcessManager) {
					fillUp(ctx, t, manager, manager.maxProcs)
					sleepOpts := testutil.SleepCreateOpts(100)
					sleepProc, err := createFunc(ctx, manager, t, sleepOpts)
					assert.Error(t, err)
					assert.Nil(t, sleepProc)
					otherSleepProcs, err := manager.List(ctx, options.All)
					require.NoError(t, err)
					for _, otherSleepProc := range otherSleepProcs {
						_, err := otherSleepProc.Wait(ctx)
						require.NoError(t, err)
					}
					sleepProc, err = createFunc(ctx, manager, t, sleepOpts)
					assert.NoError(t, err)
					assert.NotNil(t, sleepProc)
				},
				//"": func(ctx context.Context, t *testing.T, manager *selfClearingProcessManager) {},
			} {
				t.Run(name, func(t *testing.T) {
					tctx, cancel := context.WithTimeout(context.Background(), testutil.ManagerTestTimeout)
					defer cancel()

					selfClearingManager, err := NewSelfClearingProcessManager(5, false)
					require.NoError(t, err)
					test(tctx, t, selfClearingManager.(*selfClearingProcessManager))
					selfClearingManager.Close(tctx)
				})
			}
		})
	}
}
