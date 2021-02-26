package jasper

import (
	"context"
	"testing"
	"time"

	"github.com/mongodb/jasper/options"
	testoptions "github.com/mongodb/jasper/testutil/options"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultTrigger(t *testing.T) {
	const parentID = "parent-trigger-id"

	for name, testcase := range map[string]func(context.Context, *testing.T, Manager){
		"VerifyFixtures": func(ctx context.Context, t *testing.T, mngr Manager) {
			require.NotNil(t, mngr)
			require.NotNil(t, ctx)
			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
			assert.NotNil(t, makeDefaultTrigger(ctx, mngr, testoptions.TrueCreateOpts(), parentID))
			assert.NotNil(t, makeDefaultTrigger(ctx, mngr, nil, ""))
		},
		"OneOnFailure": func(ctx context.Context, t *testing.T, mngr Manager) {
			opts := testoptions.FalseCreateOpts()
			tcmd := testoptions.TrueCreateOpts()
			opts.OnFailure = append(opts.OnFailure, tcmd)
			trigger := makeDefaultTrigger(ctx, mngr, opts, parentID)
			trigger(ProcessInfo{})

			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			require.Len(t, out, 1)
			_, err = out[0].Wait(ctx)
			require.NoError(t, err)
			info := out[0].Info(ctx)
			assert.True(t, info.IsRunning || info.Complete)
		},
		"OneOnSuccess": func(ctx context.Context, t *testing.T, mngr Manager) {
			opts := testoptions.TrueCreateOpts()
			tcmd := testoptions.FalseCreateOpts()
			opts.OnSuccess = append(opts.OnSuccess, tcmd)
			trigger := makeDefaultTrigger(ctx, mngr, opts, parentID)
			trigger(ProcessInfo{Successful: true})

			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			require.Len(t, out, 1)
			info := out[0].Info(ctx)
			assert.True(t, info.IsRunning || info.Complete)
		},
		"FailureTriggerDoesNotWorkWithCanceledContext": func(ctx context.Context, t *testing.T, mngr Manager) {
			cctx, cancel := context.WithCancel(ctx)
			cancel()
			opts := testoptions.FalseCreateOpts()
			tcmd := testoptions.TrueCreateOpts()
			opts.OnFailure = append(opts.OnFailure, tcmd)
			trigger := makeDefaultTrigger(cctx, mngr, opts, parentID)
			trigger(ProcessInfo{})

			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"SuccessTriggerDoesNotWorkWithCanceledContext": func(ctx context.Context, t *testing.T, mngr Manager) {
			cctx, cancel := context.WithCancel(ctx)
			cancel()
			opts := testoptions.FalseCreateOpts()
			tcmd := testoptions.TrueCreateOpts()
			opts.OnSuccess = append(opts.OnSuccess, tcmd)
			trigger := makeDefaultTrigger(cctx, mngr, opts, parentID)
			trigger(ProcessInfo{Successful: true})

			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"SuccessOutcomeWithNoTriggers": func(ctx context.Context, t *testing.T, mngr Manager) {
			trigger := makeDefaultTrigger(ctx, mngr, testoptions.TrueCreateOpts(), parentID)
			trigger(ProcessInfo{})
			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"FailureOutcomeWithNoTriggers": func(ctx context.Context, t *testing.T, mngr Manager) {
			trigger := makeDefaultTrigger(ctx, mngr, testoptions.TrueCreateOpts(), parentID)
			trigger(ProcessInfo{Successful: true})
			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"TimeoutWithTimeout": func(ctx context.Context, t *testing.T, mngr Manager) {
			opts := testoptions.FalseCreateOpts()
			tcmd := testoptions.TrueCreateOpts()
			opts.OnTimeout = append(opts.OnTimeout, tcmd)

			tctx, cancel := context.WithTimeout(ctx, time.Second)
			defer cancel()
			trigger := makeDefaultTrigger(tctx, mngr, opts, parentID)
			trigger(ProcessInfo{Timeout: true})

			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			require.Len(t, out, 1)
			_, err = out[0].Wait(ctx)
			assert.NoError(t, err)
			info := out[0].Info(ctx)
			assert.True(t, info.IsRunning || info.Complete)
		},
		"TimeoutWithoutTimeout": func(ctx context.Context, t *testing.T, mngr Manager) {
			opts := testoptions.FalseCreateOpts()
			tcmd := testoptions.TrueCreateOpts()
			opts.OnTimeout = append(opts.OnTimeout, tcmd)

			trigger := makeDefaultTrigger(ctx, mngr, opts, parentID)
			trigger(ProcessInfo{Timeout: true})

			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			require.Len(t, out, 1)
			_, err = out[0].Wait(ctx)
			assert.NoError(t, err)
			info := out[0].Info(ctx)
			assert.True(t, info.IsRunning || info.Complete)
		},
		"TimeoutWithCanceledContext": func(ctx context.Context, t *testing.T, mngr Manager) {
			cctx, cancel := context.WithCancel(ctx)
			cancel()

			opts := testoptions.FalseCreateOpts()
			tcmd := testoptions.TrueCreateOpts()
			opts.OnTimeout = append(opts.OnTimeout, tcmd)

			trigger := makeDefaultTrigger(cctx, mngr, opts, parentID)
			trigger(ProcessInfo{Timeout: true})

			out, err := mngr.List(ctx, options.All)
			require.NoError(t, err)
			assert.Empty(t, out)
		},
		"OptionsCloseTriggerCallsClosers": func(ctx context.Context, t *testing.T, mngr Manager) {
			count := 0
			opts := options.Create{}
			opts.RegisterCloser(func() (_ error) { count++; return })
			info := ProcessInfo{Options: opts}

			trigger := makeOptionsCloseTrigger()
			trigger(info)
			assert.Equal(t, 1, count)
		},
		// "": func(ctx context.Context, t *testing.T, mngr Manager) {},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			testcase(ctx, t, &synchronizedProcessManager{
				manager: &basicProcessManager{
					loggers: NewLoggingCache(),
					procs:   map[string]Process{},
				},
			})
		})
	}
}
