package jasper

import (
	"context"
	"testing"

	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func registerBasedCreate(ctx context.Context, m *selfClearingProcessManager, t *testing.T, opts *options.Create) (Process, error) {
	sleep, err := newBlockingProcess(ctx, testoptions.SleepCreateOpts(10))
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

func fillUp(ctx context.Context, t *testing.T, manager *selfClearingProcessManager, numProcs int, opts *options.Create) {
	procs, err := createProcs(ctx, opts, manager, numProcs)
	require.NoError(t, err)
	require.Len(t, procs, numProcs)
}

func TestSelfClearingManager(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for createTestName, createFunc := range map[string]func(context.Context, *selfClearingProcessManager, *testing.T, *options.Create) (Process, error){
		"Create":   pureCreate,
		"Register": registerBasedCreate,
	} {
		t.Run(createTestName, func(t *testing.T) {
			for testName, testCase := range map[string]func(context.Context, *testing.T, *selfClearingProcessManager, *options.Create){
				"SucceedsWhenFree": func(ctx context.Context, t *testing.T, manager *selfClearingProcessManager, opts *options.Create) {
					opts.Args = testoptions.TrueCreateOpts().Args
					proc, err := createFunc(ctx, manager, t, opts)
					assert.NoError(t, err)
					assert.NotNil(t, proc)
				},
				"ErrorsWhenFull": func(ctx context.Context, t *testing.T, manager *selfClearingProcessManager, opts *options.Create) {
					fillUp(ctx, t, manager, manager.maxProcs, opts)

					sleep, err := createFunc(ctx, manager, t, testoptions.SleepCreateOpts(10))
					assert.Error(t, err)
					assert.Nil(t, sleep)
				},
				"PartiallySucceedsWhenAlmostFull": func(ctx context.Context, t *testing.T, manager *selfClearingProcessManager, opts *options.Create) {
					fillUp(ctx, t, manager, manager.maxProcs-1, opts)
					firstSleep, err := createFunc(ctx, manager, t, testoptions.SleepCreateOpts(10))
					assert.NoError(t, err)
					assert.NotNil(t, firstSleep)
					secondSleep, err := createFunc(ctx, manager, t, testoptions.SleepCreateOpts(10))
					assert.Error(t, err)
					assert.Nil(t, secondSleep)
				},
				"InitialFailureIsResolvedByWaiting": func(ctx context.Context, t *testing.T, manager *selfClearingProcessManager, opts *options.Create) {
					fillUp(ctx, t, manager, manager.maxProcs, opts)
					sleepOpts := testoptions.SleepCreateOpts(100)
					sleepProc, err := createFunc(ctx, manager, t, sleepOpts)
					assert.Error(t, err)
					assert.Nil(t, sleepProc)
					otherSleepProcs, err := manager.List(ctx, options.All)
					require.NoError(t, err)
					for _, otherSleepProc := range otherSleepProcs {
						_, err = otherSleepProc.Wait(ctx)
						require.NoError(t, err)
					}
					sleepProc, err = createFunc(ctx, manager, t, sleepOpts)
					assert.NoError(t, err)
					assert.NotNil(t, sleepProc)
				},
			} {

				t.Run(testName, func(t *testing.T) {
					for procName, modifyOpts := range map[string]testoptions.ModifyOpts{
						"BlockingProcess": func(opts *options.Create) *options.Create {
							opts.Implementation = options.ProcessImplementationBlocking
							return opts
						},
						"BasicProcess": func(opts *options.Create) *options.Create {
							opts.Implementation = options.ProcessImplementationBasic
							return opts
						},
					} {
						t.Run(procName, func(t *testing.T) {
							tctx, cancel := context.WithTimeout(ctx, testutil.ManagerTestTimeout)
							defer cancel()

							mngr, err := NewSelfClearingProcessManager(5, false)
							require.NoError(t, err)
							selfClearingMngr, ok := mngr.(*selfClearingProcessManager)
							require.True(t, ok)
							opts := modifyOpts(testoptions.SleepCreateOpts(5))
							testCase(tctx, t, selfClearingMngr, opts)
							assert.NoError(t, mngr.Close(ctx))
						})
					}
				})
			}
		})
	}
}
