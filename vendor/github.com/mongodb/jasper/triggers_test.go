package jasper

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/jasper/options"
	"github.com/mongodb/jasper/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultTrigger(t *testing.T) {
	const parentID = "parent-trigger-id"

	for name, testcase := range map[string]func(context.Context, *testing.T, Manager){
		"VerifyFixtures": func(ctx context.Context, t *testing.T, manager Manager) {
			require.NotNil(t, manager)
			require.NotNil(t, ctx)
			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
			assert.NotNil(t, makeDefaultTrigger(ctx, manager, testutil.TrueCreateOpts(), parentID))
			assert.NotNil(t, makeDefaultTrigger(ctx, manager, nil, ""))
		},
		"OneOnFailure": func(ctx context.Context, t *testing.T, manager Manager) {
			opts := testutil.FalseCreateOpts()
			tcmd := testutil.TrueCreateOpts()
			opts.OnFailure = append(opts.OnFailure, tcmd)
			trigger := makeDefaultTrigger(ctx, manager, opts, parentID)
			trigger(ProcessInfo{})

			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			require.Len(t, out, 1)
			_, err = out[0].Wait(ctx)
			require.NoError(t, err)
			info := out[0].Info(ctx)
			assert.True(t, info.IsRunning || info.Complete)
		},
		"OneOnSuccess": func(ctx context.Context, t *testing.T, manager Manager) {
			opts := testutil.TrueCreateOpts()
			tcmd := testutil.FalseCreateOpts()
			opts.OnSuccess = append(opts.OnSuccess, tcmd)
			trigger := makeDefaultTrigger(ctx, manager, opts, parentID)
			trigger(ProcessInfo{Successful: true})

			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			require.Len(t, out, 1)
			info := out[0].Info(ctx)
			assert.True(t, info.IsRunning || info.Complete)
		},
		"FailureTriggerDoesNotWorkWithCanceledContext": func(ctx context.Context, t *testing.T, manager Manager) {
			cctx, cancel := context.WithCancel(ctx)
			cancel()
			opts := testutil.FalseCreateOpts()
			tcmd := testutil.TrueCreateOpts()
			opts.OnFailure = append(opts.OnFailure, tcmd)
			trigger := makeDefaultTrigger(cctx, manager, opts, parentID)
			trigger(ProcessInfo{})

			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"SuccessTriggerDoesNotWorkWithCanceledContext": func(ctx context.Context, t *testing.T, manager Manager) {
			cctx, cancel := context.WithCancel(ctx)
			cancel()
			opts := testutil.FalseCreateOpts()
			tcmd := testutil.TrueCreateOpts()
			opts.OnSuccess = append(opts.OnSuccess, tcmd)
			trigger := makeDefaultTrigger(cctx, manager, opts, parentID)
			trigger(ProcessInfo{Successful: true})

			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"SuccessOutcomeWithNoTriggers": func(ctx context.Context, t *testing.T, manager Manager) {
			trigger := makeDefaultTrigger(ctx, manager, testutil.TrueCreateOpts(), parentID)
			trigger(ProcessInfo{})
			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"FailureOutcomeWithNoTriggers": func(ctx context.Context, t *testing.T, manager Manager) {
			trigger := makeDefaultTrigger(ctx, manager, testutil.TrueCreateOpts(), parentID)
			trigger(ProcessInfo{Successful: true})
			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"TimeoutWithTimeout": func(ctx context.Context, t *testing.T, manager Manager) {
			opts := testutil.FalseCreateOpts()
			tcmd := testutil.TrueCreateOpts()
			opts.OnTimeout = append(opts.OnTimeout, tcmd)

			tctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			trigger := makeDefaultTrigger(tctx, manager, opts, parentID)
			trigger(ProcessInfo{Timeout: true})

			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			require.Len(t, out, 1)
			_, err = out[0].Wait(ctx)
			assert.NoError(t, err)
			info := out[0].Info(ctx)
			assert.True(t, info.IsRunning || info.Complete)
		},
		"TimeoutWithoutTimeout": func(ctx context.Context, t *testing.T, manager Manager) {
			opts := testutil.FalseCreateOpts()
			tcmd := testutil.TrueCreateOpts()
			opts.OnTimeout = append(opts.OnTimeout, tcmd)

			trigger := makeDefaultTrigger(ctx, manager, opts, parentID)
			trigger(ProcessInfo{Timeout: true})

			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			require.Len(t, out, 1)
			_, err = out[0].Wait(ctx)
			assert.NoError(t, err)
			info := out[0].Info(ctx)
			assert.True(t, info.IsRunning || info.Complete)
		},
		"TimeoutWithCanceledContext": func(ctx context.Context, t *testing.T, manager Manager) {
			cctx, cancel := context.WithCancel(ctx)
			cancel()

			opts := testutil.FalseCreateOpts()
			tcmd := testutil.TrueCreateOpts()
			opts.OnTimeout = append(opts.OnTimeout, tcmd)

			trigger := makeDefaultTrigger(cctx, manager, opts, parentID)
			trigger(ProcessInfo{Timeout: true})

			out, err := manager.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"OptionsCloseTriggerCallsClosers": func(ctx context.Context, t *testing.T, manager Manager) {
			count := 0
			opts := options.Create{}
			opts.RegisterCloser(func() (_ error) { count++; return })
			info := ProcessInfo{Options: opts}

			trigger := makeOptionsCloseTrigger()
			trigger(info)
			assert.Equal(t, 1, count)
		},
		// "": func(ctx context.Context, t *testing.T, manager Manager) {},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			testcase(ctx, t, &synchronizedProcessManager{
				manager: &basicProcessManager{
					procs: map[string]Process{},
				},
			})
		})
	}
}
