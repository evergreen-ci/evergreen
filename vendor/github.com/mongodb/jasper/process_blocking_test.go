package jasper

import (
	"context"
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"

	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const gracefulTimeout = 1000 * time.Millisecond

func TestBlockingProcess(t *testing.T) {
	t.Parallel()
	// we run the suite multiple times given that implementation
	// is heavily threaded, there are timing concerns that require
	// multiple executions.
	for _, attempt := range []string{"First", "Second", "Third", "Fourth", "Fifth"} {
		t.Run(attempt, func(t *testing.T) {
			t.Parallel()
			for name, testCase := range map[string]func(context.Context, *testing.T, *blockingProcess){
				"VerifyTestCaseConfiguration": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					assert.NotNil(t, proc)
					assert.NotNil(t, ctx)
					assert.NotZero(t, proc.ID())
					assert.False(t, proc.Complete(ctx))
					assert.NotNil(t, makeDefaultTrigger(ctx, nil, &proc.opts, "foo"))
				},
				"InfoIDPopulatedInBasicCase": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					infoReturned := make(chan struct{})
					go func() {
						assert.Equal(t, proc.Info(ctx).ID, proc.ID())
						close(infoReturned)
					}()

					op := <-proc.ops
					op(nil)
					<-infoReturned
				},
				"InfoReturnsNotCompleteForCanceledCase": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					signal := make(chan struct{})
					go func() {
						cctx, cancel := context.WithCancel(ctx)
						cancel()

						assert.False(t, proc.Info(cctx).Complete)
						close(signal)
					}()

					gracefulCtx, cancel := context.WithTimeout(ctx, gracefulTimeout)
					defer cancel()

					select {
					case <-signal:
					case <-gracefulCtx.Done():
						t.Error("reached timeout")
					}
				},
				"SignalErrorsForCanceledContext": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					signal := make(chan struct{})
					go func() {
						cctx, cancel := context.WithCancel(ctx)
						cancel()

						assert.Error(t, proc.Signal(cctx, syscall.SIGTERM))
						close(signal)
					}()

					gracefulCtx, cancel := context.WithTimeout(ctx, gracefulTimeout)
					defer cancel()

					select {
					case <-signal:
					case <-gracefulCtx.Done():
						t.Error("reached timeout")
					}
				},
				"TestRegisterTriggerAfterComplete": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					proc.info.Complete = true
					assert.True(t, proc.Complete(ctx))
					assert.Error(t, proc.RegisterTrigger(ctx, nil))
					assert.Error(t, proc.RegisterTrigger(ctx, makeDefaultTrigger(ctx, nil, &proc.opts, "foo")))
					assert.Len(t, proc.triggers, 0)
				},
				"TestRegisterPopulatedTrigger": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					assert.False(t, proc.Complete(ctx))
					assert.Error(t, proc.RegisterTrigger(ctx, nil))
					assert.NoError(t, proc.RegisterTrigger(ctx, makeDefaultTrigger(ctx, nil, &proc.opts, "foo")))
					assert.Len(t, proc.triggers, 1)
				},
				"RunningIsFalseWhenCompleteIsSatisfied": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					proc.info.Complete = true
					assert.True(t, proc.Complete(ctx))
					assert.False(t, proc.Running(ctx))
				},
				"RunningIsFalseWithEmptyPid": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					signal := make(chan struct{})
					go func() {
						assert.False(t, proc.Running(ctx))
						close(signal)
					}()

					op := <-proc.ops

					op(&exec.Cmd{
						Process: &os.Process{},
					})
					<-signal
				},
				"RunningIsFalseWithNilCmd": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					signal := make(chan struct{})
					go func() {
						assert.False(t, proc.Running(ctx))
						close(signal)
					}()

					op := <-proc.ops
					op(nil)

					<-signal
				},
				"RunningIsTrueWithValidPid": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					signal := make(chan struct{})
					go func() {
						assert.True(t, proc.Running(ctx))
						close(signal)
					}()

					op := <-proc.ops
					op(&exec.Cmd{
						Process: &os.Process{Pid: 42},
					})

					<-signal
				},
				"RunningIsFalseWithCanceledContext": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					proc.ops <- func(_ *exec.Cmd) {}
					cctx, cancel := context.WithCancel(ctx)
					cancel()
					assert.False(t, proc.Running(cctx))
				},
				"SignalIsErrorAfterComplete": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					proc.info = ProcessInfo{Complete: true}
					assert.True(t, proc.Complete(ctx))

					assert.Error(t, proc.Signal(ctx, syscall.SIGTERM))
				},
				"SignalNilProcessIsError": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					signal := make(chan struct{})
					go func() {
						assert.False(t, proc.Complete(ctx))
						assert.Error(t, proc.Signal(ctx, syscall.SIGTERM))
						close(signal)
					}()

					op := <-proc.ops
					op(nil)

					<-signal
				},
				"SignalCanceledProcessIsError": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					cctx, cancel := context.WithCancel(ctx)
					cancel()

					assert.Error(t, proc.Signal(cctx, syscall.SIGTERM))
				},
				"SignalErrorsInvalidProcess": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					signal := make(chan struct{})
					go func() {
						assert.False(t, proc.Complete(ctx))
						assert.Error(t, proc.Signal(ctx, syscall.SIGTERM))
						close(signal)
					}()

					op := <-proc.ops
					op(&exec.Cmd{
						Process: &os.Process{Pid: -42},
					})

					<-signal
				},
				"WaitSomeBeforeCanceling": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					proc.opts = *sleepCreateOpts(10)
					proc.complete = make(chan struct{})
					cctx, cancel := context.WithTimeout(ctx, 600*time.Millisecond)
					defer cancel()

					cmd, deadline, err := proc.opts.Resolve(ctx)
					require.NoError(t, err)
					assert.NoError(t, cmd.Start())

					go proc.reactor(ctx, deadline, cmd)
					_, err = proc.Wait(cctx)
					require.Error(t, err)
					assert.Contains(t, err.Error(), "operation canceled")
				},
				"WaitShouldReturnNilForSuccessfulCommandsWithoutIDs": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					proc.opts.Args = []string{"sleep", "10"}
					proc.ops = make(chan func(*exec.Cmd))

					cmd, _, err := proc.opts.Resolve(ctx)
					assert.NoError(t, err)
					assert.NoError(t, cmd.Start())
					signal := make(chan struct{})
					go func() {
						// this is the crucial
						// assertion of this tests
						_, err := proc.Wait(ctx)
						assert.NoError(t, err)
						close(signal)
					}()

					go func() {
						for {
							select {
							case op := <-proc.ops:
								proc.setInfo(ProcessInfo{
									Complete:   true,
									Successful: true,
								})
								if op != nil {
									op(cmd)
								}
							}
						}
					}()
					<-signal
				},
				"WaitShouldReturnNilForSuccessfulCommands": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					proc.opts.Args = []string{"sleep", "10"}
					proc.ops = make(chan func(*exec.Cmd))

					cmd, _, err := proc.opts.Resolve(ctx)
					assert.NoError(t, err)
					assert.NoError(t, cmd.Start())
					signal := make(chan struct{})
					go func() {
						// this is the crucial
						// assertion of this tests
						_, err := proc.Wait(ctx)
						assert.NoError(t, err)
						close(signal)
					}()

					go func() {
						for {
							select {
							case op := <-proc.ops:
								proc.setInfo(ProcessInfo{
									ID:         "foo",
									Complete:   true,
									Successful: true,
								})
								if op != nil {
									op(cmd)
								}
							}
						}
					}()
					<-signal
				},
				"WaitShouldReturnErrorForFailedCommands": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					proc.opts.Args = []string{"sleep", "10"}
					proc.ops = make(chan func(*exec.Cmd))

					cmd, _, err := proc.opts.Resolve(ctx)
					assert.NoError(t, err)
					assert.NoError(t, cmd.Start())
					signal := make(chan struct{})
					go func() {
						// this is the crucial assertion
						// of this tests.
						_, err := proc.Wait(ctx)
						assert.Error(t, err)
						close(signal)
					}()

					go func() {
						for {
							select {
							case op := <-proc.ops:
								proc.err = errors.New("signal: killed")
								proc.setInfo(ProcessInfo{
									ID:         "foo",
									Complete:   true,
									Successful: false,
								})
								if op != nil {
									op(cmd)
								}
							}
						}
					}()
					<-signal
				},
				"InfoDoesNotWaitForContextTimeoutAfterProcessCompletes": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					opts := &CreateOptions{
						Args: []string{"ls"},
					}

					process, err := newBlockingProcess(ctx, opts)
					require.NoError(t, err)

					opCompleted := make(chan struct{})

					go func() {
						defer close(opCompleted)
						_ = process.Info(ctx)
					}()

					_, err = process.Wait(ctx)
					require.NoError(t, err)

					longCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
					defer cancel()

					select {
					case <-opCompleted:
					case <-longCtx.Done():
						assert.Fail(t, "context timed out waiting for op to return")
					}
				},
				"RunningDoesNotWaitForContextTimeoutAfterProcessCompletes": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					opts := &CreateOptions{
						Args: []string{"ls"},
					}

					process, err := newBlockingProcess(ctx, opts)
					require.NoError(t, err)

					opCompleted := make(chan struct{})

					go func() {
						defer close(opCompleted)
						_ = process.Running(ctx)
					}()

					_, err = process.Wait(ctx)
					require.NoError(t, err)

					longCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
					defer cancel()

					select {
					case <-opCompleted:
					case <-longCtx.Done():
						assert.Fail(t, "context timed out waiting for op to return")
					}
				},
				"SignalDoesNotWaitForContextTimeoutAfterProcessCompletes": func(ctx context.Context, t *testing.T, proc *blockingProcess) {
					opts := &CreateOptions{
						Args: []string{"ls"},
					}

					process, err := newBlockingProcess(ctx, opts)
					require.NoError(t, err)

					opCompleted := make(chan struct{})

					go func() {
						defer close(opCompleted)
						_ = process.Signal(ctx, syscall.SIGKILL)
					}()

					_, _ = process.Wait(ctx)

					longCtx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
					defer cancel()

					select {
					case <-opCompleted:
					case <-longCtx.Done():
						assert.Fail(t, "context timed out waiting for op to return")
					}
				},
				// "": func(ctx context.Context, t *testing.T, proc *blockingProcess) {},
			} {
				t.Run(name, func(t *testing.T) {
					ctx, cancel := context.WithCancel(context.Background())
					defer cancel()

					id := uuid.Must(uuid.NewV4()).String()
					proc := &blockingProcess{
						id:   id,
						ops:  make(chan func(*exec.Cmd), 1),
						opts: CreateOptions{},
						info: ProcessInfo{ID: id},
					}

					testCase(ctx, t, proc)

					close(proc.ops)
				})
			}
		})
	}
}
