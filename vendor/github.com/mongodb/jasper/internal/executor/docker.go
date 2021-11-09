package executor

import (
	"context"
	"io"
	"io/ioutil"
	"sync"
	"syscall"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/client"
	"github.com/docker/docker/pkg/stdcopy"
	"github.com/google/uuid"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type docker struct {
	execOpts types.ExecConfig
	stdin    io.Reader
	stdout   io.Writer
	stderr   io.Writer

	image    string
	platform string

	client *client.Client
	ctx    context.Context

	containerID    string
	containerMutex sync.RWMutex

	status      Status
	statusMutex sync.RWMutex

	pid      int
	exitCode int
	exitErr  error
	signal   syscall.Signal
}

// NewDocker returns an Executor that creates a process within a Docker
// container. Callers are expected to clean up resources by calling Close.
func NewDocker(ctx context.Context, client *client.Client, platform, image string, args []string) Executor {
	return &docker{
		ctx: ctx,
		execOpts: types.ExecConfig{
			Cmd: args,
		},
		platform: platform,
		image:    image,
		client:   client,
		status:   Unstarted,
		pid:      -1,
		exitCode: -1,
		signal:   syscall.Signal(-1),
	}
}

func (e *docker) Args() []string {
	return e.execOpts.Cmd
}

func (e *docker) SetEnv(env []string) {
	e.execOpts.Env = env
}

func (e *docker) Env() []string {
	return e.execOpts.Env
}

func (e *docker) SetDir(dir string) {
	e.execOpts.WorkingDir = dir
}

func (e *docker) Dir() string {
	return e.execOpts.WorkingDir
}

func (e *docker) SetStdin(stdin io.Reader) {
	e.stdin = stdin
	e.execOpts.AttachStdin = stdin != nil
}

func (e *docker) SetStdout(stdout io.Writer) {
	e.stdout = stdout
	e.execOpts.AttachStdout = stdout != nil
}

func (e *docker) Stdout() io.Writer {
	return e.stdout
}

func (e *docker) SetStderr(stderr io.Writer) {
	e.stderr = stderr
	e.execOpts.AttachStderr = stderr != nil
}

func (e *docker) Stderr() io.Writer {
	return e.stderr
}

func (e *docker) Start() error {
	if e.getStatus().After(Unstarted) {
		return errors.New("cannot start a process that has already started, exited, or closed")
	}

	if err := e.setupContainer(); err != nil {
		return errors.Wrap(err, "could not set up container for process")
	}

	if err := e.startContainer(); err != nil {
		return errors.Wrap(err, "could not start process within container")
	}

	e.setStatus(Running)

	_ = e.getPID()

	return nil
}

// setupContainer creates a container for the process without starting it.
func (e *docker) setupContainer() error {
	containerName := uuid.New().String()
	createResp, err := e.client.ContainerCreate(e.ctx, &container.Config{
		Image:        e.image,
		Cmd:          e.execOpts.Cmd,
		Env:          e.execOpts.Env,
		WorkingDir:   e.execOpts.WorkingDir,
		AttachStdin:  e.execOpts.AttachStdin,
		StdinOnce:    e.execOpts.AttachStdin,
		OpenStdin:    e.execOpts.AttachStdin,
		AttachStdout: e.execOpts.AttachStdout,
		AttachStderr: e.execOpts.AttachStderr,
	}, &container.HostConfig{}, &network.NetworkingConfig{}, containerName)
	if err != nil {
		return errors.Wrap(err, "problem creating container for process")
	}
	grip.WarningWhen(len(createResp.Warnings) != 0, message.Fields{
		"message":  "warnings during container creation for process",
		"warnings": createResp.Warnings,
	})

	e.setContainerID(createResp.ID)

	return nil
}

// startContainer attaches any I/O stream to the process and starts the
// container.
func (e *docker) startContainer() error {
	if err := e.setupIOStream(); err != nil {
		return e.withRemoveContainer(errors.Wrap(err, "problem setting up I/O streaming to process in container"))
	}

	if err := e.client.ContainerStart(e.ctx, e.getContainerID(), types.ContainerStartOptions{}); err != nil {
		return e.withRemoveContainer(errors.Wrap(err, "problem starting container for process"))
	}

	return nil
}

// setupIOStream sets up the process to read standard input and write to
// standard output and standard error. This is a no-op if there are no
// configured inputs or outputs.
func (e *docker) setupIOStream() error {
	if e.stdin == nil && e.stdout == nil && e.stderr == nil {
		return nil
	}

	stream, err := e.client.ContainerAttach(e.ctx, e.getContainerID(), types.ContainerAttachOptions{
		Stream: true,
		Stdin:  e.execOpts.AttachStdin,
		Stdout: e.execOpts.AttachStdout,
		Stderr: e.execOpts.AttachStderr,
	})
	if err != nil {
		return errors.Wrap(err, "could not set attach I/O to process in container")
	}

	go e.runIOStream(stream)

	return nil
}

// runIOStream starts the goroutines to handle standard I/O and waits until the
// stream is done.
func (e *docker) runIOStream(stream types.HijackedResponse) {
	defer stream.Close()
	var wg sync.WaitGroup

	if e.stdin != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := io.Copy(stream.Conn, e.stdin)
			grip.Error(errors.Wrap(err, "problem streaming input to process"))
			grip.Error(errors.Wrap(stream.CloseWrite(), "problem closing process input stream"))
		}()
	}

	if e.stdout != nil || e.stderr != nil {
		wg.Add(1)
		go func() {
			defer wg.Done()
			stdout := e.stdout
			stderr := e.stderr
			if stdout == nil {
				stdout = ioutil.Discard
			}
			if stderr == nil {
				stderr = ioutil.Discard
			}
			if _, err := stdcopy.StdCopy(stdout, stderr, stream.Reader); err != nil {
				grip.Error(errors.Wrap(err, "problem streaming output from process"))
			}
		}()
	}

	wg.Wait()
}

// withRemoveContainer returns the error as well as any error from cleaning up
// the container.
func (e *docker) withRemoveContainer(err error) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(err)
	catcher.Add(e.removeContainer())
	return catcher.Resolve()
}

// removeContainer cleans up the container running this process.
func (e *docker) removeContainer() error {
	containerID := e.getContainerID()
	if containerID == "" {
		return nil
	}

	// We must ensure the container is cleaned up, so do not reuse the
	// Executor's context, which may already be done.
	rmCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := e.client.ContainerRemove(rmCtx, e.containerID, types.ContainerRemoveOptions{
		Force: true,
	}); err != nil {
		return errors.Wrap(err, "problem cleaning up container for process")
	}

	e.setContainerID("")

	return nil
}

func (e *docker) Wait() error {
	if e.getStatus().Before(Running) {
		return errors.New("cannot wait on unstarted process")
	}
	if e.getStatus().After(Running) {
		return e.exitErr
	}

	containerID := e.getContainerID()

	handleContextError := func() (handled bool) {
		// If the Docker container is killed because the context is done,
		// treat it the same as the exec implementation that kills it with a
		// signal.
		if e.ctx.Err() == nil {
			return false
		}
		e.exitErr = e.ctx.Err()
		e.signal = syscall.SIGKILL
		e.setStatus(Exited)
		return true
	}

	waitDone, errs := e.client.ContainerWait(e.ctx, containerID, container.WaitConditionNotRunning)
	select {
	case err := <-errs:
		if handled := handleContextError(); handled {
			return e.exitErr
		}
		e.exitErr = err
		e.setStatus(Exited)
		return errors.Wrap(err, "error waiting for container to finish running")
	case <-e.ctx.Done():
		_ = handleContextError()
		return e.exitErr
	case waitResult := <-waitDone:
		if handled := handleContextError(); handled {
			return e.exitErr
		}
		if waitResult.Error != nil && waitResult.Error.Message != "" {
			e.exitErr = errors.New(waitResult.Error.Message)
		} else if waitResult.StatusCode != 0 {
			// In order to maintain the same semantics as exec.Command, we have
			// to return an error for non-zero exit codes.
			e.exitErr = errors.Errorf("exit status %d", waitResult.StatusCode)
		}
		e.exitCode = int(waitResult.StatusCode)
	}

	e.setStatus(Exited)

	return e.exitErr
}

func (e *docker) Signal(sig syscall.Signal) error {
	if e.getStatus() != Running {
		return errors.New("cannot signal a non-running process")
	}

	dsig, err := syscallToDockerSignal(sig, e.platform)
	if err != nil {
		return errors.Wrapf(err, "could not get Docker equivalent of signal '%d'", sig)
	}
	if err := e.client.ContainerKill(e.ctx, e.getContainerID(), dsig); err != nil {
		return errors.Wrap(err, "could not signal process within container")
	}

	e.signal = sig

	return nil
}

// PID returns the PID of the process in the container, or -1 if the PID cannot
// be retrieved.
func (e *docker) PID() int {
	if e.pid > -1 || !e.getStatus().BetweenInclusive(Running, Exited) {
		return e.pid
	}

	return e.getPID()
}

// getPID makes a request to get the process's runtime PID, which is only
// available while the container is running.
func (e *docker) getPID() int {
	state, err := e.getProcessState()
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "could not get container PID",
			"op":        "pid",
			"container": e.getContainerID(),
			"executor":  "docker",
		}))
		return -1
	}

	e.pid = state.Pid

	return e.pid
}

// ExitCode returns the exit code of the process in the container, or -1 if the
// exit code cannot be retrieved.
func (e *docker) ExitCode() int {
	if e.exitCode > -1 || !e.getStatus().BetweenInclusive(Running, Exited) {
		return e.exitCode
	}
	if e.getStatus() == Running {
		return -1
	}

	state, err := e.getProcessState()
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":   "could not get container exit code",
			"container": e.getContainerID(),
			"executor":  "docker",
		}))
		return e.exitCode
	}

	e.exitCode = state.ExitCode

	return e.exitCode
}

func (e *docker) Success() bool {
	if e.getStatus().Before(Exited) {
		return false
	}
	return e.exitErr == nil
}

// SignalInfo returns information about signals that were sent to the process in
// the container. This will only return information about received signals if
// Signal is called.
func (e *docker) SignalInfo() (sig syscall.Signal, signaled bool) {
	return e.signal, e.signal != -1
}

// Close cleans up the container associated with this process executor and
// closes the connection to the Docker daemon.
func (e *docker) Close() error {
	catcher := grip.NewBasicCatcher()
	catcher.Wrap(e.removeContainer(), "error removing Docker container")
	catcher.Wrap(e.client.Close(), "error closing Docker client")
	e.setStatus(Closed)
	return catcher.Resolve()
}

// getProcessState returns information about the state of the process that ran
// inside the container.
func (e *docker) getProcessState() (*types.ContainerState, error) {
	resp, err := e.client.ContainerInspect(e.ctx, e.getContainerID())
	if err != nil {
		return nil, errors.Wrap(err, "could not inspect container")
	}
	if resp.ContainerJSONBase == nil || resp.ContainerJSONBase.State == nil {
		return nil, errors.Wrap(err, "introspection of container's process is missing state information")
	}
	return resp.ContainerJSONBase.State, nil
}

func (e *docker) getContainerID() string {
	e.containerMutex.RLock()
	defer e.containerMutex.RUnlock()
	return e.containerID
}

func (e *docker) setContainerID(id string) {
	e.containerMutex.Lock()
	defer e.containerMutex.Unlock()
	e.containerID = id
}

func (e *docker) getStatus() Status {
	e.statusMutex.RLock()
	defer e.statusMutex.RUnlock()
	return e.status
}

func (e *docker) setStatus(status Status) {
	e.statusMutex.Lock()
	defer e.statusMutex.Unlock()
	if status < e.status && status != Unknown {
		return
	}
	e.status = status
}
