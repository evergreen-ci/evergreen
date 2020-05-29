package executor

import (
	"context"
	"fmt"
	"io"
	"strings"
	"syscall"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ssh"
)

// execSSH runs processes on a remote machine via SSH.
type execSSH struct {
	session   *ssh.Session
	client    *ssh.Client
	args      []string
	dir       string
	env       []string
	exited    bool
	exitErr   error
	closeConn context.CancelFunc
}

// MakeSSH returns an Executor that creates processes over SSH. Callers are
// expected to clean up resources by either cancelling the context or explicitly
// calling Close.
func MakeSSH(ctx context.Context, client *ssh.Client, session *ssh.Session, args []string) Executor {
	ctx, cancel := context.WithCancel(ctx)
	e := &execSSH{session: session, client: client, args: args}
	e.closeConn = cancel
	go func() {
		<-ctx.Done()
		if err := e.session.Close(); err != nil && err != io.EOF {
			grip.Warning(errors.Wrap(err, "error closing SSH session"))
		}
		if err := e.client.Close(); err != nil && err != io.EOF {
			grip.Warning(errors.Wrap(err, "error closing SSH client"))
		}
	}()
	return e
}

// Args returns the arguments to the process.
func (e *execSSH) Args() []string {
	return e.args
}

// SetEnv sets the process environment.
func (e *execSSH) SetEnv(env []string) {
	e.env = env
}

// Env returns the process environment.
func (e *execSSH) Env() []string {
	return e.env
}

// SetDir sets the process working directory.
func (e *execSSH) SetDir(dir string) {
	e.dir = dir
}

// Dir returns the process working directory.
func (e *execSSH) Dir() string {
	return e.dir
}

// SetStdin sets the process standard input.
func (e *execSSH) SetStdin(stdin io.Reader) {
	e.session.Stdin = stdin
}

// SetStdout sets the process standard output.
func (e *execSSH) SetStdout(stdout io.Writer) {
	e.session.Stdout = stdout
}

// SetStderr sets the process standard error.
func (e *execSSH) SetStderr(stderr io.Writer) {
	e.session.Stderr = stderr
}

// Start begins running the process.
func (e *execSSH) Start() error {
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
func (e *execSSH) Wait() error {
	catcher := grip.NewBasicCatcher()
	e.exitErr = e.session.Wait()
	catcher.Add(e.exitErr)
	e.exited = true
	return catcher.Resolve()
}

// Signal sends a signal to the remote process.
func (e *execSSH) Signal(sig syscall.Signal) error {
	return e.session.Signal(syscallToSSHSignal(sig))
}

// PID is not implemented.
func (e *execSSH) PID() int {
	// TODO: there is no simple way of retrieving the PID of the remote process.
	return -1
}

// ExitCode returns the exit code of the process, or -1 if the process is not
// finished.
func (e *execSSH) ExitCode() int {
	if !e.exited {
		return -1
	}
	if e.exitErr == nil {
		return 0
	}
	sshExitErr, ok := e.exitErr.(*ssh.ExitError)
	if !ok {
		return -1
	}
	return sshExitErr.Waitmsg.ExitStatus()
}

// Success returns whether or not the process ran successfully.
func (e *execSSH) Success() bool {
	if !e.exited {
		return false
	}
	return e.exitErr == nil
}

// SignalInfo returns information about signals the process has received.
func (e *execSSH) SignalInfo() (sig syscall.Signal, signaled bool) {
	if e.exitErr == nil {
		return syscall.Signal(-1), false
	}
	sshExitErr, ok := e.exitErr.(*ssh.ExitError)
	if !ok {
		return syscall.Signal(-1), false
	}
	sshSig := ssh.Signal(sshExitErr.Waitmsg.Signal())
	return sshToSyscallSignal(sshSig), sshSig != ""
}

// Close closes the SSH connection.
func (e *execSSH) Close() {
	e.closeConn()
}

// syscallToSSHSignal converts a syscall.Signal to its equivalent ssh.Signal
func syscallToSSHSignal(sig syscall.Signal) ssh.Signal {
	switch sig {
	case syscall.SIGABRT:
		return ssh.SIGABRT
	case syscall.SIGALRM:
		return ssh.SIGALRM
	case syscall.SIGFPE:
		return ssh.SIGFPE
	case syscall.SIGHUP:
		return ssh.SIGHUP
	case syscall.SIGILL:
		return ssh.SIGILL
	case syscall.SIGINT:
		return ssh.SIGINT
	case syscall.SIGKILL:
		return ssh.SIGKILL
	case syscall.SIGPIPE:
		return ssh.SIGPIPE
	case syscall.SIGQUIT:
		return ssh.SIGQUIT
	case syscall.SIGSEGV:
		return ssh.SIGSEGV
	case syscall.SIGTERM:
		return ssh.SIGTERM
	}
	return ssh.Signal("")
}

func sshToSyscallSignal(sig ssh.Signal) syscall.Signal {
	switch sig {
	case ssh.SIGABRT:
		return syscall.SIGABRT
	case ssh.SIGALRM:
		return syscall.SIGALRM
	case ssh.SIGFPE:
		return syscall.SIGFPE
	case ssh.SIGHUP:
		return syscall.SIGHUP
	case ssh.SIGILL:
		return syscall.SIGILL
	case ssh.SIGINT:
		return syscall.SIGINT
	case ssh.SIGKILL:
		return syscall.SIGKILL
	case ssh.SIGPIPE:
		return syscall.SIGPIPE
	case ssh.SIGQUIT:
		return syscall.SIGQUIT
	case ssh.SIGSEGV:
		return syscall.SIGSEGV
	case ssh.SIGTERM:
		return syscall.SIGTERM
	}
	return syscall.Signal(-1)
}
