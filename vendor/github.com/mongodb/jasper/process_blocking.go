package jasper

import (
	"context"
	"math/rand"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	uuid "github.com/satori/go.uuid"
)

type blockingProcess struct {
	id       string
	opts     CreateOptions
	ops      chan func(*exec.Cmd)
	complete chan struct{}
	err      error

	mu             sync.RWMutex
	tags           map[string]struct{}
	triggers       ProcessTriggerSequence
	signalTriggers SignalTriggerSequence
	info           ProcessInfo
}

func newBlockingProcess(ctx context.Context, opts *CreateOptions) (Process, error) {
	id := uuid.Must(uuid.NewV4()).String()
	opts.AddEnvVar(EnvironID, id)

	cmd, err := opts.Resolve(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem building command from options")
	}

	p := &blockingProcess{
		id:       id,
		opts:     *opts,
		tags:     make(map[string]struct{}),
		ops:      make(chan func(*exec.Cmd)),
		complete: make(chan struct{}),
	}

	for _, t := range opts.Tags {
		p.Tag(t)
	}

	if err = p.RegisterTrigger(ctx, makeOptionsCloseTrigger()); err != nil {
		return nil, errors.Wrap(err, "problem registering options closer trigger")
	}

	if err = cmd.Start(); err != nil {
		return nil, errors.Wrap(err, "problem starting command")
	}

	p.info.Options.started = true
	p.opts.started = true
	opts.started = true

	p.info = ProcessInfo{
		ID:        id,
		PID:       cmd.Process.Pid,
		Options:   *opts,
		IsRunning: true,
	}
	p.info.Host, _ = os.Hostname()

	go p.reactor(ctx, cmd)

	return p, nil
}

func (p *blockingProcess) setInfo(info ProcessInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.info = info
}

func (p *blockingProcess) hasCompleteInfo() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.info.Complete
}

func (p *blockingProcess) getInfo() ProcessInfo {
	p.mu.RLock()
	defer p.mu.RUnlock()
	return p.info
}

func (p *blockingProcess) setErr(err error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.err = err
}

func (p *blockingProcess) getErr() error {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.err
}

func (p *blockingProcess) reactor(ctx context.Context, cmd *exec.Cmd) {
	signal := make(chan error)
	go func() {
		defer close(signal)
		signal <- cmd.Wait()
	}()
	defer close(p.complete)

	for {
		select {
		case err := <-signal:
			var info ProcessInfo

			func() {
				p.mu.RLock()
				defer p.mu.RUnlock()

				info = p.info
				info.Complete = true
				info.IsRunning = false

				if cmd.ProcessState != nil {
					info.Successful = cmd.ProcessState.Success()
					procWaitStatus := cmd.ProcessState.Sys().(syscall.WaitStatus)
					if procWaitStatus.Signaled() {
						info.ExitCode = int(procWaitStatus.Signal())
					} else {
						info.ExitCode = procWaitStatus.ExitStatus()
					}
				} else {
					info.Successful = (err == nil)
				}

				grip.Debug(message.WrapError(err, message.Fields{
					"id":           p.ID,
					"cmd":          strings.Join(p.opts.Args, " "),
					"success":      info.Successful,
					"num_triggers": len(p.triggers),
				}))
			}()

			p.mu.RLock()
			p.triggers.Run(info)
			p.mu.RUnlock()
			p.setErr(err)
			p.setInfo(info)
			return
		case <-ctx.Done():
			// note, the process might take a moment to
			// die when it gets here.
			info := p.getInfo()
			info.Complete = true
			info.IsRunning = false
			info.Successful = false

			p.mu.RLock()
			p.triggers.Run(info)
			p.mu.RUnlock()
			p.setInfo(info)
			return
		case op := <-p.ops:
			if op != nil {
				op(cmd)
			}
		}
	}
}

func (p *blockingProcess) ID() string { return p.id }
func (p *blockingProcess) Info(ctx context.Context) ProcessInfo {
	if p.hasCompleteInfo() {
		return p.getInfo()
	}

	out := make(chan ProcessInfo)
	operation := func(cmd *exec.Cmd) {
		out <- p.getInfo()
		close(out)
	}

	select {
	case p.ops <- operation:
		select {
		case res := <-out:
			return res
		case <-ctx.Done():
			return p.getInfo()
		case <-p.complete:
			return p.getInfo()
		}
	case <-ctx.Done():
		return p.getInfo()
	case <-p.complete:
		return p.getInfo()
	}
}

func (p *blockingProcess) Running(ctx context.Context) bool {
	if p.hasCompleteInfo() {
		return false
	}

	out := make(chan bool)
	operation := func(cmd *exec.Cmd) {
		defer close(out)

		if cmd == nil || cmd.Process == nil {
			out <- false
			return
		}

		if cmd.Process.Pid <= 0 {
			out <- false
			return
		}

		out <- true
	}

	select {
	case p.ops <- operation:
		select {
		case res := <-out:
			return res
		case <-ctx.Done():
			return p.getInfo().IsRunning
		case <-p.complete:
			return p.getInfo().IsRunning
		}
	case <-ctx.Done():
		return p.getInfo().IsRunning
	case <-p.complete:
		return p.getInfo().IsRunning
	}
}

func (p *blockingProcess) Complete(_ context.Context) bool {
	return p.hasCompleteInfo()
}

func (p *blockingProcess) Signal(ctx context.Context, sig syscall.Signal) error {
	if p.hasCompleteInfo() {
		return errors.New("cannot signal a process that has terminated")
	}

	out := make(chan error)
	operation := func(cmd *exec.Cmd) {
		defer close(out)

		if cmd == nil {
			out <- errors.New("cannot signal nil process")
			return
		}

		if skipSignal := p.signalTriggers.Run(p.getInfo(), sig); !skipSignal {
			sig = makeCompatible(sig)
			out <- errors.Wrapf(cmd.Process.Signal(sig), "problem sending signal '%s' to '%s'",
				sig, p.id)
		} else {
			out <- nil
		}

	}
	select {
	case p.ops <- operation:
		select {
		case res := <-out:
			return res
		case <-ctx.Done():
			return errors.New("context canceled")
		case <-p.complete:
			return errors.New("cannot signal after process is complete")
		}
	case <-ctx.Done():
		return errors.New("context canceled")
	case <-p.complete:
		return errors.New("cannot signal after process is complete")
	}
}

func (p *blockingProcess) RegisterTrigger(_ context.Context, trigger ProcessTrigger) error {
	if trigger == nil {
		return errors.New("cannot register nil trigger")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.info.Complete {
		return errors.New("cannot register trigger after process exits")
	}

	p.triggers = append(p.triggers, trigger)

	return nil
}

func (p *blockingProcess) RegisterSignalTrigger(_ context.Context, trigger SignalTrigger) error {
	if trigger == nil {
		return errors.New("cannot register nil trigger")
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.info.Complete {
		return errors.New("cannot register trigger after process exits")
	}

	p.signalTriggers = append(p.signalTriggers, trigger)

	return nil
}

func (p *blockingProcess) RegisterSignalTriggerID(ctx context.Context, id SignalTriggerID) error {
	makeTrigger, ok := GetSignalTriggerFactory(id)
	if !ok {
		return errors.Errorf("could not find signal trigger with id '%s'", id)
	}
	return errors.Wrap(p.RegisterSignalTrigger(ctx, makeTrigger()), "failed to register signal trigger")
}

func (p *blockingProcess) Wait(ctx context.Context) (int, error) {
	if p.hasCompleteInfo() {
		return p.getInfo().ExitCode, p.getErr()
	}

	out := make(chan error)
	waiter := func(cmd *exec.Cmd) {
		if !p.hasCompleteInfo() {
			return
		}

		out <- p.getErr()
	}

	timer := time.NewTimer(0)
	defer timer.Stop()
	for {
		select {
		case <-timer.C:
			timer.Reset(time.Duration(rand.Int63n(50)) * time.Millisecond)
		case p.ops <- waiter:
			continue
		case <-ctx.Done():
			return -1, errors.New("wait operation canceled")
		case err := <-out:
			return p.getInfo().ExitCode, errors.WithStack(err)
		case <-p.complete:
			return p.getInfo().ExitCode, p.getErr()
		}
	}
}

func (p *blockingProcess) Respawn(ctx context.Context) (Process, error) {
	opts := p.Info(ctx).Options
	optsCopy := opts.Copy()
	return newBlockingProcess(ctx, optsCopy)
}

func (p *blockingProcess) Tag(t string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	_, ok := p.tags[t]
	if ok {
		return
	}

	p.tags[t] = struct{}{}
	p.opts.Tags = append(p.opts.Tags, t)
}

func (p *blockingProcess) ResetTags() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.tags = make(map[string]struct{})
	p.opts.Tags = []string{}
}

func (p *blockingProcess) GetTags() []string {
	p.mu.RLock()
	defer p.mu.RUnlock()

	out := []string{}
	for t := range p.tags {
		out = append(out, t)
	}
	return out
}
