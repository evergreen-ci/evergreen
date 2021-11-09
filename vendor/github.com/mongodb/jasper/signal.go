package jasper

import (
	"context"
	"syscall"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// Terminate sends a SIGTERM signal to the given process under the given
// context. This does not guarantee that the process will actually die. This
// function does not Wait() on the given process upon sending the signal.
func Terminate(ctx context.Context, p Process) error {
	return errors.WithStack(p.Signal(ctx, syscall.SIGTERM))
}

// Kill sends a SIGKILL signal to the given process under the given context.
// This guarantees that the process will die. This function does not Wait() on
// the given process upon sending the signal.
func Kill(ctx context.Context, p Process) error {
	return errors.WithStack(p.Signal(ctx, syscall.SIGKILL))
}

// TerminateAll sends a SIGTERM signal to each of the given processes under the
// given context. This does not guarantee that each process will actually die.
// This function calls Wait() on each process after sending them SIGTERM
// signals. On Windows, this function sends a SIGKILL instead of SIGTERM. Use
// Terminate() in a loop if you do not wish to potentially hang on Wait().
func TerminateAll(ctx context.Context, procs []Process) error {
	catcher := grip.NewBasicCatcher()

	for _, proc := range procs {
		if proc.Running(ctx) {
			catcher.Add(Terminate(ctx, proc))
		}
	}

	for _, proc := range procs {
		_, _ = proc.Wait(ctx)
	}

	return catcher.Resolve()
}

// KillAll sends a SIGKILL signal to each of the given processes under the
// given context. This guarantees that each process will actually die. This
// function calls Wait() on each process after sending them SIGKILL signals.
// Use Kill() in a loop if you do not wish to potentially hang on Wait().
func KillAll(ctx context.Context, procs []Process) error {
	catcher := grip.NewBasicCatcher()

	for _, proc := range procs {
		if proc.Running(ctx) {
			catcher.Add(Kill(ctx, proc))
		}
	}

	for _, proc := range procs {
		_, _ = proc.Wait(ctx)
	}

	return catcher.Resolve()
}
