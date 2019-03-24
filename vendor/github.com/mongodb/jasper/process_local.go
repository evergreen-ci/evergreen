package jasper

import (
	"context"
	"sync"
	"syscall"

	"github.com/pkg/errors"
)

type localProcess struct {
	proc  Process
	mutex sync.RWMutex
}

func (p *localProcess) ID() string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.ID()
}

func (p *localProcess) Info(ctx context.Context) ProcessInfo {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.Info(ctx)
}

func (p *localProcess) Running(ctx context.Context) bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.Running(ctx)
}

func (p *localProcess) Complete(ctx context.Context) bool {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.Complete(ctx)
}

func (p *localProcess) Signal(ctx context.Context, sig syscall.Signal) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return errors.WithStack(p.proc.Signal(ctx, sig))
}

func (p *localProcess) Tag(t string) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.proc.Tag(t)
}

func (p *localProcess) ResetTags() {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	p.proc.ResetTags()
}

func (p *localProcess) GetTags() []string {
	p.mutex.RLock()
	defer p.mutex.RUnlock()

	return p.proc.GetTags()
}

func (p *localProcess) RegisterTrigger(ctx context.Context, trigger ProcessTrigger) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return errors.WithStack(p.proc.RegisterTrigger(ctx, trigger))
}

func (p *localProcess) RegisterSignalTrigger(ctx context.Context, trigger SignalTrigger) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return errors.WithStack(p.proc.RegisterSignalTrigger(ctx, trigger))
}

func (p *localProcess) RegisterSignalTriggerID(ctx context.Context, trigger SignalTriggerID) error {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	return errors.WithStack(p.proc.RegisterSignalTriggerID(ctx, trigger))
}

func (p *localProcess) Wait(ctx context.Context) (int, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	exitCode, err := p.proc.Wait(ctx)
	return exitCode, errors.WithStack(err)
}

func (p *localProcess) Respawn(ctx context.Context) (Process, error) {
	p.mutex.Lock()
	defer p.mutex.Unlock()

	newProc, err := p.proc.Respawn(ctx)
	return newProc, errors.WithStack(err)
}
