package executor

import (
	"context"
	"io"
	"os/exec"
	"syscall"

	"github.com/pkg/errors"
)

// execLocal runs processes on a local machine via exec.
type execLocal struct {
	cmd *exec.Cmd
}

// NewLocal returns an Executor that creates processes locally.
func NewLocal(ctx context.Context, args []string) Executor {
	executable := args[0]
	var execArgs []string
	if len(args) > 1 {
		execArgs = args[1:]
	}
	cmd := exec.CommandContext(ctx, executable, execArgs...)
	return &execLocal{cmd: cmd}
}

// MakeLocal wraps an existing local process.
func MakeLocal(cmd *exec.Cmd) Executor {
	return &execLocal{
		cmd: cmd,
	}
}

// Args returns the arguments to the process.
func (e *execLocal) Args() []string {
	return e.cmd.Args
}

// SetEnv sets the process environment.
func (e *execLocal) SetEnv(env []string) {
	e.cmd.Env = env
}

// Env returns the environment process.
func (e *execLocal) Env() []string {
	return e.cmd.Env
}

// SetDir sets the process working directory.
func (e *execLocal) SetDir(dir string) {
	e.cmd.Dir = dir
}

// Dir returns the process working directory.
func (e *execLocal) Dir() string {
	return e.cmd.Dir
}

// SetStdin sets process standard input.
func (e *execLocal) SetStdin(stdin io.Reader) {
	e.cmd.Stdin = stdin
}

// SetStdin sets the process standard output.
func (e *execLocal) SetStdout(stdout io.Writer) {
	e.cmd.Stdout = stdout
}

// SetStdin sets the process standard error.
func (e *execLocal) SetStderr(stderr io.Writer) {
	e.cmd.Stderr = stderr
}

// Start begins running the process.
func (e *execLocal) Start() error {
	return e.cmd.Start()
}

// Wait returns the result for waiting for the process to finish.
func (e *execLocal) Wait() error {
	return e.cmd.Wait()
}

// Signal sends a signal to the process.
func (e *execLocal) Signal(sig syscall.Signal) error {
	if e.cmd.Process == nil {
		return errors.New("cannot signal an unstarted process")
	}
	return e.cmd.Process.Signal(sig)
}

// PID returns the PID of the process.
func (e *execLocal) PID() int {
	if e.cmd.Process == nil {
		return -1
	}
	return e.cmd.Process.Pid
}

// ExitCode returns the exit code of the process, or -1 if the process is not
// finished.
func (e *execLocal) ExitCode() int {
	if e.cmd.ProcessState == nil {
		return -1
	}
	status := e.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return status.ExitStatus()
}

// Success returns whether or not the process ran successfully.
func (e *execLocal) Success() bool {
	if e.cmd.ProcessState == nil {
		return false
	}
	return e.cmd.ProcessState.Success()
}

// SignalInfo returns information about the signals the process has received.
func (e *execLocal) SignalInfo() (sig syscall.Signal, signaled bool) {
	if e.cmd.ProcessState == nil {
		return syscall.Signal(-1), false
	}
	status := e.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return status.Signal(), status.Signaled()
}

// Close is a no-op.
func (e *execLocal) Close() {}
