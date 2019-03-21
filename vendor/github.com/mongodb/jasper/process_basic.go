package jasper

import (
	"context"
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

type basicProcess struct {
	info           ProcessInfo
	cmd            *exec.Cmd
	err            error
	id             string
	opts           CreateOptions
	tags           map[string]struct{}
	triggers       ProcessTriggerSequence
	signalTriggers SignalTriggerSequence
	waitProcessed  chan struct{}
	initialized    chan struct{}
	sync.RWMutex
}

func newBasicProcess(ctx context.Context, opts *CreateOptions) (Process, error) {
	id := uuid.Must(uuid.NewV4()).String()
	opts.AddEnvVar(EnvironID, id)
	opts.Hostname, _ = os.Hostname()

	cmd, err := opts.Resolve(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem building command from options")
	}

	p := &basicProcess{
		id:            id,
		opts:          *opts,
		cmd:           cmd,
		tags:          make(map[string]struct{}),
		waitProcessed: make(chan struct{}),
		initialized:   make(chan struct{}),
	}

	for _, t := range opts.Tags {
		p.Tag(t)
	}

	if err = p.RegisterTrigger(ctx, makeOptionsCloseTrigger()); err != nil {
		return nil, errors.Wrap(err, "problem registering options closer trigger")
	}

	err = cmd.Start()
	if err != nil {
		return nil, errors.Wrap(err, "problem creating command")
	}

	p.info.ID = p.id
	p.info.Options = p.opts
	p.info.Host = p.opts.Hostname

	go p.transition(ctx, cmd)

	opts.started = true

	return p, nil
}

func (p *basicProcess) transition(ctx context.Context, cmd *exec.Cmd) {
	waitFinished := make(chan error)

	go func() {
		defer close(waitFinished)
		waitFinished <- cmd.Wait()
	}()

	initialize := func() {
		p.Lock()
		defer p.Unlock()
		defer close(p.initialized)
		p.opts.started = true
		p.info.IsRunning = true
		p.info.PID = p.cmd.Process.Pid
		p.cmd = cmd
	}
	initialize()

	finish := func(err error) {
		p.Lock()
		defer p.Unlock()
		defer close(p.waitProcessed)
		p.err = err
		p.info.IsRunning = false
		p.info.Complete = true
		procWaitStatus := p.cmd.ProcessState.Sys().(syscall.WaitStatus)
		if procWaitStatus.Signaled() {
			p.info.ExitCode = int(procWaitStatus.Signal())
		} else {
			p.info.ExitCode = procWaitStatus.ExitStatus()
		}
		p.info.Successful = p.cmd.ProcessState.Success()
		p.triggers.Run(p.info)
	}
	finish(<-waitFinished)
}

func (p *basicProcess) ID() string {
	p.RLock()
	defer p.RUnlock()

	return p.id
}
func (p *basicProcess) Info(_ context.Context) ProcessInfo {
	<-p.initialized
	p.RLock()
	defer p.RUnlock()

	return p.info
}

func (p *basicProcess) Complete(_ context.Context) bool {
	return !p.Running(nil)
}

func (p *basicProcess) Running(_ context.Context) bool {
	<-p.initialized
	p.RLock()
	defer p.RUnlock()
	return p.info.IsRunning
}

func (p *basicProcess) Signal(_ context.Context, sig syscall.Signal) error {
	<-p.initialized
	p.RLock()
	defer p.RUnlock()

	if p.Running(nil) {
		if skipSignal := p.signalTriggers.Run(p.Info(nil), sig); !skipSignal {
			sig = makeCompatible(sig)
			return errors.Wrapf(p.cmd.Process.Signal(sig), "problem sending signal '%s' to '%s'", sig, p.id)
		}
		return nil
	}

	return errors.New("cannot signal a process that has terminated")
}

func (p *basicProcess) Respawn(ctx context.Context) (Process, error) {
	<-p.initialized
	p.RLock()
	defer p.RUnlock()

	opts := p.info.Options
	opts.closers = []func() error{}

	return newBasicProcess(ctx, &opts)
}

func (p *basicProcess) Wait(ctx context.Context) (int, error) {
	if !p.Running(ctx) {
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
		return errors.New("cannot register trigger after process exits")
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
