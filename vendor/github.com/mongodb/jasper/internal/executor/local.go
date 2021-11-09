package executor

import (
	"context"
	"io"
	"os/exec"
	"syscall"

	"github.com/pkg/errors"
)

// local runs processes on a local machine via exec.
type local struct {
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
	return &local{cmd: cmd}
}

// MakeLocal wraps an existing local process.
func MakeLocal(cmd *exec.Cmd) Executor {
	return &local{
		cmd: cmd,
	}
}

// Args returns the arguments to the process.
func (e *local) Args() []string {
	return e.cmd.Args
}

// SetEnv sets the process environment.
func (e *local) SetEnv(env []string) {
	e.cmd.Env = env
}

// Env returns the environment process.
func (e *local) Env() []string {
	return e.cmd.Env
}

// SetDir sets the process working directory.
func (e *local) SetDir(dir string) {
	e.cmd.Dir = dir
}

// Dir returns the process working directory.
func (e *local) Dir() string {
	return e.cmd.Dir
}

// SetStdin sets process standard input.
func (e *local) SetStdin(stdin io.Reader) {
	e.cmd.Stdin = stdin
}

// SetStdin sets the process standard output.
func (e *local) SetStdout(stdout io.Writer) {
	e.cmd.Stdout = stdout
}

func (e *local) Stdout() io.Writer {
	return e.cmd.Stdout
}

// SetStdin sets the process standard error.
func (e *local) SetStderr(stderr io.Writer) {
	e.cmd.Stderr = stderr
}

func (e *local) Stderr() io.Writer {
	return e.cmd.Stderr
}

// Start begins running the process.
func (e *local) Start() error {
	return e.cmd.Start()
}

// Wait returns the result for waiting for the process to finish.
func (e *local) Wait() error {
	return e.cmd.Wait()
}

// Signal sends a signal to the process.
func (e *local) Signal(sig syscall.Signal) error {
	if e.cmd.Process == nil {
		return errors.New("cannot signal an unstarted process")
	}
	return e.cmd.Process.Signal(sig)
}

// PID returns the PID of the process.
func (e *local) PID() int {
	if e.cmd.Process == nil {
		return -1
	}
	return e.cmd.Process.Pid
}

// ExitCode returns the exit code of the process, or -1 if the process is not
// finished.
func (e *local) ExitCode() int {
	if e.cmd.ProcessState == nil {
		return -1
	}
	// TODO: this can be just replaced with ProcessState.ExitCode, but requires
	// go1.12.
	status := e.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return status.ExitStatus()
}

// Success returns whether or not the process ran successfully.
func (e *local) Success() bool {
	if e.cmd.ProcessState == nil {
		return false
	}
	return e.cmd.ProcessState.Success()
}

// SignalInfo returns information about the signals the process has received.
func (e *local) SignalInfo() (sig syscall.Signal, signaled bool) {
	if e.cmd.ProcessState == nil {
		return syscall.Signal(-1), false
	}
	status := e.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return status.Signal(), status.Signaled()
}

// Close is a no-op.
func (e *local) Close() error {
	return nil
}
