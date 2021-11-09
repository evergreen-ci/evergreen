package executor

import (
	"context"
	"fmt"
	"io"
	"strings"
	"syscall"

	"github.com/mongodb/grip"
	cryptossh "golang.org/x/crypto/ssh"
)

// ssh runs processes on a remote machine via SSH.
type ssh struct {
	session *cryptossh.Session
	client  *cryptossh.Client
	args    []string
	dir     string
	env     []string
	exited  bool
	exitErr error
	ctx     context.Context
}

// NewSSH returns an Executor that creates processes over SSH. Callers are
// expected to clean up resources by explicitly calling Close.
func NewSSH(ctx context.Context, client *cryptossh.Client, session *cryptossh.Session, args []string) Executor {
	return &ssh{
		ctx:     ctx,
		session: session,
		client:  client,
		args:    args,
	}
}

// Args returns the arguments to the process.
func (e *ssh) Args() []string {
	return e.args
}

// SetEnv sets the process environment.
func (e *ssh) SetEnv(env []string) {
	e.env = env
}

// Env returns the process environment.
func (e *ssh) Env() []string {
	return e.env
}

// SetDir sets the process working directory.
func (e *ssh) SetDir(dir string) {
	e.dir = dir
}

// Dir returns the process working directory.
func (e *ssh) Dir() string {
	return e.dir
}

// SetStdin sets the process standard input.
func (e *ssh) SetStdin(stdin io.Reader) {
	e.session.Stdin = stdin
}

// SetStdout sets the process standard output.
func (e *ssh) SetStdout(stdout io.Writer) {
	e.session.Stdout = stdout
}

// Stdout returns the standard output of the process.
func (e *ssh) Stdout() io.Writer { return e.session.Stdout }

// SetStderr sets the process standard error.
func (e *ssh) SetStderr(stderr io.Writer) {
	e.session.Stderr = stderr
}

// Stderr returns the standard error of the process.
func (e *ssh) Stderr() io.Writer { return e.session.Stderr }

// Start begins running the process.
func (e *ssh) Start() error {
	args := []string{}
	for _, entry := range e.env {
		args = append(args, fmt.Sprintf("export %s", entry))
	}
	if e.dir != "" {
		args = append(args, fmt.Sprintf("cd %s", e.dir))
	}
	args = append(args, strings.Join(e.args, " "))
	return e.session.Start(strings.Join(args, "\n"))
}

// Wait returns the result of waiting for the remote process to finish.
func (e *ssh) Wait() error {
	catcher := grip.NewBasicCatcher()
	e.exitErr = e.session.Wait()
	catcher.Add(e.exitErr)
	e.exited = true
	return catcher.Resolve()
}

// Signal sends a signal to the remote process.
func (e *ssh) Signal(sig syscall.Signal) error {
	return e.session.Signal(syscallToSSHSignal(sig))
}

// PID is not implemented since there is no simple way to get the remote
// process's PID.
func (e *ssh) PID() int {
	return -1
}

// ExitCode returns the exit code of the process, or -1 if the process is not
// finished.
func (e *ssh) ExitCode() int {
	if !e.exited {
		return -1
	}
	if e.exitErr == nil {
		return 0
	}
	sshExitErr, ok := e.exitErr.(*cryptossh.ExitError)
	if !ok {
		return -1
	}
	return sshExitErr.Waitmsg.ExitStatus()
}

// Success returns whether or not the process ran successfully.
func (e *ssh) Success() bool {
	if !e.exited {
		return false
	}
	return e.exitErr == nil
}

// SignalInfo returns information about signals the process has received.
func (e *ssh) SignalInfo() (sig syscall.Signal, signaled bool) {
	if e.exitErr == nil {
		return syscall.Signal(-1), false
	}
	sshExitErr, ok := e.exitErr.(*cryptossh.ExitError)
	if !ok {
		return syscall.Signal(-1), false
	}
	sshSig := cryptossh.Signal(sshExitErr.Waitmsg.Signal())
	return sshToSyscallSignal(sshSig), sshSig != ""
}

// Close closes the SSH connection resources.
func (e *ssh) Close() error {
	catcher := grip.NewBasicCatcher()
	if err := e.session.Close(); err != nil && err != io.EOF {
		catcher.Wrap(err, "error closing SSH session")
	}
	if err := e.client.Close(); err != nil && err != io.EOF {
		catcher.Wrap(err, "error closing SSH client")
	}
	return catcher.Resolve()
}

// syscallToSSHSignal converts a syscall.Signal to its equivalent
// cryptossh.Signal.
func syscallToSSHSignal(sig syscall.Signal) cryptossh.Signal {
	switch sig {
	case syscall.SIGABRT:
		return cryptossh.SIGABRT
	case syscall.SIGALRM:
		return cryptossh.SIGALRM
	case syscall.SIGFPE:
		return cryptossh.SIGFPE
	case syscall.SIGHUP:
		return cryptossh.SIGHUP
	case syscall.SIGILL:
		return cryptossh.SIGILL
	case syscall.SIGINT:
		return cryptossh.SIGINT
	case syscall.SIGKILL:
		return cryptossh.SIGKILL
	case syscall.SIGPIPE:
		return cryptossh.SIGPIPE
	case syscall.SIGQUIT:
		return cryptossh.SIGQUIT
	case syscall.SIGSEGV:
		return cryptossh.SIGSEGV
	case syscall.SIGTERM:
		return cryptossh.SIGTERM
	}
	return cryptossh.Signal("")
}

// sshToSyscallSignal converts a cryptossh.Signal to its equivalent
// syscall.Signal.
func sshToSyscallSignal(sig cryptossh.Signal) syscall.Signal {
	switch sig {
	case cryptossh.SIGABRT:
		return syscall.SIGABRT
	case cryptossh.SIGALRM:
		return syscall.SIGALRM
	case cryptossh.SIGFPE:
		return syscall.SIGFPE
	case cryptossh.SIGHUP:
		return syscall.SIGHUP
	case cryptossh.SIGILL:
		return syscall.SIGILL
	case cryptossh.SIGINT:
		return syscall.SIGINT
	case cryptossh.SIGKILL:
		return syscall.SIGKILL
	case cryptossh.SIGPIPE:
		return syscall.SIGPIPE
	case cryptossh.SIGQUIT:
		return syscall.SIGQUIT
	case cryptossh.SIGSEGV:
		return syscall.SIGSEGV
	case cryptossh.SIGTERM:
		return syscall.SIGTERM
	}
	return syscall.Signal(-1)
}
