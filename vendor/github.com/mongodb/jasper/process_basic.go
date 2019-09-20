package jasper

import (
	"context"
	"os"
	"os/exec"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

type basicProcess struct {
	info           ProcessInfo
	cmd            *exec.Cmd
	err            error
	id             string
	opts           options.Create
	tags           map[string]struct{}
	triggers       ProcessTriggerSequence
	signalTriggers SignalTriggerSequence
	waitProcessed  chan struct{}
	sync.RWMutex
}

func newBasicProcess(ctx context.Context, opts *options.Create) (Process, error) {
	id := uuid.Must(uuid.NewV4()).String()
	opts.AddEnvVar(EnvironID, id)

	cmd, deadline, err := opts.Resolve(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem building command from options")
	}

	p := &basicProcess{
		id:            id,
		opts:          *opts,
		cmd:           cmd,
		tags:          make(map[string]struct{}),
		waitProcessed: make(chan struct{}),
	}

	for _, t := range opts.Tags {
		p.tags[t] = struct{}{}
	}

	if err = p.RegisterTrigger(ctx, makeOptionsCloseTrigger()); err != nil {
		return nil, errors.Wrap(err, "problem registering options closer trigger")
	}

	if err = cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "problem starting command")
	}

	p.info.StartAt = time.Now()
	p.info.ID = p.id
	p.info.Options = p.opts
	p.info.Host, _ = os.Hostname()
	p.info.IsRunning = true
	p.info.PID = cmd.Process.Pid

	go p.transition(ctx, deadline, cmd)

	return p, nil
}

func (p *basicProcess) transition(ctx context.Context, deadline time.Time, cmd *exec.Cmd) {
	waitFinished := make(chan error)

	go func() {
		defer close(waitFinished)
		waitFinished <- cmd.Wait()
	}()

	finish := func(err error) {
		p.Lock()
		defer p.Unlock()
		defer close(p.waitProcessed)
		finishTime := time.Now()
		p.err = err
		p.info.EndAt = finishTime
		p.info.IsRunning = false
		p.info.Complete = true
		procWaitStatus := p.cmd.ProcessState.Sys().(syscall.WaitStatus)
		if procWaitStatus.Signaled() {
			p.info.ExitCode = int(procWaitStatus.Signal())
			if !deadline.IsZero() {
				p.info.Timeout = procWaitStatus.Signal() == syscall.SIGKILL && finishTime.After(deadline)
			}
		} else {
			p.info.ExitCode = procWaitStatus.ExitStatus()
			if runtime.GOOS == "windows" && !deadline.IsZero() {
				p.info.Timeout = procWaitStatus.ExitStatus() == 1 && finishTime.After(deadline)
			}
		}
		p.info.Successful = p.cmd.ProcessState.Success()
		p.triggers.Run(p.info)
	}
	finish(<-waitFinished)
}

func (p *basicProcess) ID() string {
	return p.id
}
func (p *basicProcess) Info(_ context.Context) ProcessInfo {
	p.RLock()
	defer p.RUnlock()

	return p.info
}

func (p *basicProcess) Complete(_ context.Context) bool {
	return !p.Running(nil)
}

func (p *basicProcess) Running(_ context.Context) bool {
	p.RLock()
	defer p.RUnlock()
	return p.info.IsRunning
}

func (p *basicProcess) Signal(_ context.Context, sig syscall.Signal) error {
	p.RLock()
	defer p.RUnlock()

	if p.info.Complete {
		return errors.New("cannot signal a process that has terminated")
	}

	if skipSignal := p.signalTriggers.Run(p.info, sig); !skipSignal {
		sig = makeCompatible(sig)
		return errors.Wrapf(p.cmd.Process.Signal(sig), "problem sending signal '%s' to '%s'", sig, p.id)
	}
	return nil
}

func (p *basicProcess) Respawn(ctx context.Context) (Process, error) {
	p.RLock()
	defer p.RUnlock()

	optsCopy := p.info.Options.Copy()
	return newBasicProcess(ctx, optsCopy)
}

func (p *basicProcess) Wait(ctx context.Context) (int, error) {
	if p.Complete(ctx) {
		p.RLock()
		defer p.RUnlock()

		return p.info.ExitCode, p.err
	}

	select {
	case <-ctx.Done():
		return -1, errors.New("operation canceled")
	case <-p.waitProcessed:
	}

	return p.info.ExitCode, p.err
}

func (p *basicProcess) RegisterTrigger(_ context.Context, trigger ProcessTrigger) error {
	if trigger == nil {
		return errors.New("cannot register nil trigger")
	}

	p.Lock()
	defer p.Unlock()

	if p.info.Complete {
		return errors.New("cannot register trigger after process exits")
	}

	p.triggers = append(p.triggers, trigger)

	return nil
}

func (p *basicProcess) RegisterSignalTrigger(_ context.Context, trigger SignalTrigger) error {
	if trigger == nil {
		return errors.New("cannot register nil trigger")
	}

	p.Lock()
	defer p.Unlock()

	if p.info.Complete {
		return errors.New("cannot register signal trigger after process exits")
	}

	p.signalTriggers = append(p.signalTriggers, trigger)

	return nil
}

func (p *basicProcess) RegisterSignalTriggerID(ctx context.Context, id SignalTriggerID) error {
	makeTrigger, ok := GetSignalTriggerFactory(id)
	if !ok {
		return errors.Errorf("could not find signal trigger with id '%s'", id)
	}
	return errors.Wrap(p.RegisterSignalTrigger(ctx, makeTrigger()), "failed to register signal trigger")
}

func (p *basicProcess) Tag(t string) {
	_, ok := p.tags[t]
	if ok {
		return
	}

	p.tags[t] = struct{}{}
	p.opts.Tags = append(p.opts.Tags, t)
}

func (p *basicProcess) ResetTags() {
	p.tags = make(map[string]struct{})
	p.opts.Tags = []string{}
}

func (p *basicProcess) GetTags() []string {
	out := []string{}
	for t := range p.tags {
		out = append(out, t)
	}
	return out
}
