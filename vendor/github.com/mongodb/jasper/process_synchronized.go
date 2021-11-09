package jasper

import (
	"context"
	"sync"
	"syscall"

	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

type synchronizedProcess struct {
	proc  Process
	mutex sync.RWMutex
}

// makeSynchronizedProcess wraps the given process in a thread-safe Process.
func makeSynchronizedProcess(proc Process) Process {
	return &synchronizedProcess{proc: proc}
}

// newSynchronizedProcess is a constructor for a thread-safe Process.
func newSynchronizedProcess(ctx context.Context, opts *options.Create) (Process, error) {
	opts.Synchronized = true
	return NewProcess(ctx, opts)
}

func (p *synchronizedProcess) ID() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.ID()
}

func (p *synchronizedProcess) Info(ctx context.Context) ProcessInfo {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.Info(ctx)
}

func (p *synchronizedProcess) Running(ctx context.Context) bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.Running(ctx)
}

func (p *synchronizedProcess) Complete(ctx context.Context) bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.Complete(ctx)
}

func (p *synchronizedProcess) Signal(ctx context.Context, sig syscall.Signal) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return errors.WithStack(p.proc.Signal(ctx, sig))
}

func (p *synchronizedProcess) Tag(t string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.proc.Tag(t)
}

func (p *synchronizedProcess) ResetTags() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.proc.ResetTags()
}

func (p *synchronizedProcess) GetTags() []string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.GetTags()
}

func (p *synchronizedProcess) RegisterTrigger(ctx context.Context, trigger ProcessTrigger) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return errors.WithStack(p.proc.RegisterTrigger(ctx, trigger))
}

func (p *synchronizedProcess) RegisterSignalTrigger(ctx context.Context, trigger SignalTrigger) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return errors.WithStack(p.proc.RegisterSignalTrigger(ctx, trigger))
}

func (p *synchronizedProcess) RegisterSignalTriggerID(ctx context.Context, trigger SignalTriggerID) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return errors.WithStack(p.proc.RegisterSignalTriggerID(ctx, trigger))
}

func (p *synchronizedProcess) Wait(ctx context.Context) (int, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	exitCode, err := p.proc.Wait(ctx)
	return exitCode, errors.WithStack(err)
}

func (p *synchronizedProcess) Respawn(ctx context.Context) (Process, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	newProc, err := p.proc.Respawn(ctx)
	return newProc, errors.WithStack(err)
}
