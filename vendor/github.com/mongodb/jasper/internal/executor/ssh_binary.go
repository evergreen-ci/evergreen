package executor

import (
	"context"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"syscall"

	"github.com/pkg/errors"
)

// execSSHBinary runs remote processes using the SSH binary.
type execSSHBinary struct {
	cmd         *exec.Cmd
	destination string
	remoteOpts  []string
	cmdArgs     []string
	dir         string
	env         []string
}

// NewSSHBinary returns an Executor that creates processes using the SSH binary.
func NewSSHBinary(ctx context.Context, destination string, opts []string, args []string) Executor {
	return &execSSHBinary{
		destination: destination,
		remoteOpts:  opts,
		cmdArgs:     args,
		// The actual arguments to SSH will be resolved when Start is called.
		cmd: exec.CommandContext(ctx, "ssh"),
	}
}

// Args returns the arguments to the process.
func (e *execSSHBinary) Args() []string {
	return e.cmdArgs
}

// SetEnv sets the remote process environment.
func (e *execSSHBinary) SetEnv(env []string) {
	e.env = env
}

// Env returns the remote process environment.
func (e *execSSHBinary) Env() []string {
	return e.env
}

// SetDir sets the remote process working directory.
func (e *execSSHBinary) SetDir(dir string) {
	e.dir = dir
}

// Dir returns the remote process working directory.
func (e *execSSHBinary) Dir() string {
	return e.dir
}

// SetStdin sets the remote process standard input.
func (e *execSSHBinary) SetStdin(stdin io.Reader) {
	e.cmd.Stdin = stdin
}

// SetStdin sets the remote process standard output.
func (e *execSSHBinary) SetStdout(stdout io.Writer) {
	e.cmd.Stdout = stdout
}

// SetStdin sets the remote process standard error.
func (e *execSSHBinary) SetStderr(stderr io.Writer) {
	e.cmd.Stderr = stderr
}

func (e *execSSHBinary) Stdout() io.Writer {
	return e.cmd.Stdout
}

func (e *execSSHBinary) Stderr() io.Writer {
	return e.cmd.Stderr
}

// Start begins running the remote process using the SSH binary.
func (e *execSSHBinary) Start() error {
	var resolvedArgs string
	if e.dir != "" {
		resolvedArgs += fmt.Sprintf("cd '%s' && ", e.dir)
	}
	if len(e.env) != 0 {
		resolvedArgs += strings.Join(e.env, " ") + " "
	}
	resolvedArgs += strings.Join(e.cmdArgs, " ")
	path, err := exec.LookPath("ssh")
	if err != nil {
		return errors.Wrap(err, "could not find SSH binary")
	}
	e.cmd.Path = path
	e.cmd.Args = []string{"ssh"}
	e.cmd.Args = append(e.cmd.Args, e.remoteOpts...)
	e.cmd.Args = append(e.cmd.Args, e.destination)
	e.cmd.Args = append(e.cmd.Args, resolvedArgs)

	return e.cmd.Start()
}

// Wait returns the reuslt of waiting for the remote process to finish.
func (e *execSSHBinary) Wait() error {
	if e.cmd == nil {
		return errors.New("cannot wait on an unstarted process")
	}
	return e.cmd.Wait()
}

// Signal sends a signal to the SSH binary.
func (e *execSSHBinary) Signal(sig syscall.Signal) error {
	if e.cmd == nil || e.cmd.Process == nil {
		return errors.New("cannot signal an unstarted process")
	}
	return e.cmd.Process.Signal(sig)
}

// PID returns the PID of the local SSH binary process.
func (e *execSSHBinary) PID() int {
	if e.cmd == nil || e.cmd.Process == nil {
		return -1
	}
	return e.cmd.Process.Pid
}

// ExitCode returns the exit code of the SSH binary process, or -1 if the
// process is not finished.
func (e *execSSHBinary) ExitCode() int {
	if e.cmd == nil || e.cmd.ProcessState == nil {
		return -1
	}
	status := e.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return status.ExitStatus()
}

// Success returns whether or not the process ran successfully.
func (e *execSSHBinary) Success() bool {
	if e.cmd == nil || e.cmd.ProcessState == nil {
		return false
	}
	return e.cmd.ProcessState.Success()
}

// SignalInfo returns information about the signals the SSH binary process has
// received.
func (e *execSSHBinary) SignalInfo() (sig syscall.Signal, signaled bool) {
	if e.cmd == nil || e.cmd.ProcessState == nil {
		return syscall.Signal(-1), false
	}
	status := e.cmd.ProcessState.Sys().(syscall.WaitStatus)
	return status.Signal(), status.Signaled()
}

// Close is a no-op.
func (e *execSSHBinary) Close() error {
	return nil
}
