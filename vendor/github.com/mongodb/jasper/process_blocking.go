package jasper

import (
	"context"
	"math/rand"
	"os"
	"runtime"
	"sync"
	"syscall"
	"time"

	"github.com/google/uuid"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper/internal/executor"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

type blockingProcess struct {
	id       string
	ops      chan func(executor.Executor)
	complete chan struct{}
	err      error

	mu             sync.RWMutex
	tags           map[string]struct{}
	triggers       ProcessTriggerSequence
	signalTriggers SignalTriggerSequence
	info           ProcessInfo
}

func newBlockingProcess(ctx context.Context, opts *options.Create) (Process, error) {
	id := uuid.New().String()
	opts.AddEnvVar(EnvironID, id)

	exec, deadline, err := opts.Resolve(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem building command from options")
	}

	p := &blockingProcess{
		id:       id,
		tags:     make(map[string]struct{}),
		ops:      make(chan func(executor.Executor)),
		complete: make(chan struct{}),
	}

	for _, t := range opts.Tags {
		p.tags[t] = struct{}{}
	}

	if err = p.RegisterTrigger(ctx, makeOptionsCloseTrigger()); err != nil {
		catcher := grip.NewBasicCatcher()
		catcher.Wrap(opts.Close(), "problem closing options")
		catcher.Add(err)
		return nil, errors.Wrap(catcher.Resolve(), "problem registering options close trigger")
	}

	if err = exec.Start(); err != nil {
		catcher := grip.NewBasicCatcher()
		catcher.Wrap(opts.Close(), "problem closing options")
		catcher.Add(err)
		return nil, errors.Wrap(catcher.Resolve(), "problem starting command")
	}

	p.info = ProcessInfo{
		ID:        id,
		PID:       exec.PID(),
		Options:   *opts,
		IsRunning: true,
		StartAt:   time.Now(),
	}
	if opts.Remote != nil {
		p.info.Host = opts.Remote.Host
	} else {
		p.info.Host, _ = os.Hostname()
	}

	go p.reactor(ctx, deadline, exec)

	return p, nil
}

func (p *blockingProcess) hasCompleteInfo() bool {
	p.mu.RLock()
	defer p.mu.RUnlock()

	return p.info.Complete
}

func (p *blockingProcess) setInfo(info ProcessInfo) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.info = info
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

func (p *blockingProcess) reactor(ctx context.Context, deadline time.Time, exec executor.Executor) {
	defer exec.Close()

	signal := make(chan error)
	go func() {
		defer close(signal)
		select {
		case signal <- exec.Wait():
		case <-ctx.Done():
		}
	}()
	defer close(p.complete)

	for {
		select {
		case err := <-signal:
			var info ProcessInfo
			finishTime := time.Now()

			func() {
				p.mu.RLock()
				defer p.mu.RUnlock()
				p.info.EndAt = finishTime

				info = p.info
				info.Complete = true
				info.IsRunning = false

				info.Successful = exec.Success()
				if sig, signaled := exec.SignalInfo(); signaled {
					info.ExitCode = int(sig)
					if !deadline.IsZero() {
						info.Timeout = sig == syscall.SIGKILL && finishTime.After(deadline)
					}
				} else {
					exitCode := exec.ExitCode()
					info.ExitCode = exitCode
					if runtime.GOOS == "windows" && !deadline.IsZero() {
						info.Timeout = exitCode == 1 && finishTime.After(deadline)
					}
				}
			}()

			p.mu.RLock()
			p.triggers.Run(info)
			info.Options = p.info.Options
			p.mu.RUnlock()
			p.setErr(err)
			p.mu.Lock()
			// Set the options now because our view of the process info may be
			// stale. The options could have been modified in between the above
			// info assignment and setting the information here (e.g. by calling
			// ResetTags()).
			info.Options = p.info.Options
			p.info = info
			p.mu.Unlock()

			return
		case <-ctx.Done():
			// note, the process might take a moment to
			// die when it gets here.
			info := p.getInfo()
			info.Complete = true
			info.IsRunning = false
			info.Successful = false
			info.EndAt = time.Now()

			p.mu.RLock()
			p.triggers.Run(info)
			p.mu.RUnlock()
			p.setErr(ctx.Err())
			p.mu.Lock()
			// Set the options now because our view of the process info may be
			// stale. The options could have been modified in between the above
			// info assignment and setting the information here (e.g. by calling
			// ResetTags()).
			info.Options = p.info.Options
			p.info = info
			p.mu.Unlock()

			return
		case op := <-p.ops:
			if op != nil {
				op(exec)
			}
		}
	}
}

func (p *blockingProcess) ID() string { return p.id }
func (p *blockingProcess) Info(ctx context.Context) ProcessInfo {
	if p.hasCompleteInfo() {
		return p.getInfo()
	}

	out := make(chan ProcessInfo, 1)
	operation := func(exec executor.Executor) {
		defer close(out)
		out <- p.getInfo()
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

	out := make(chan bool, 1)
	operation := func(exec executor.Executor) {
		defer close(out)

		if exec == nil {
			out <- false
			return
		}

		if exec.PID() <= 0 {
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

	out := make(chan error, 1)
	operation := func(exec executor.Executor) {
		defer close(out)

		if exec == nil {
			out <- errors.New("cannot signal nil process")
			return
		}

		if skipSignal := p.signalTriggers.Run(p.getInfo(), sig); !skipSignal {
			sig = makeCompatible(sig)
			out <- errors.Wrapf(exec.Signal(sig), "problem sending signal '%s' to '%s'",
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
			// If the process is complete because the operations channel
			// signaled the process, the signal was successful.
			if p.Info(ctx).ExitCode == int(sig) {
				return nil
			}
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

	out := make(chan error, 1)
	waiter := func(exec executor.Executor) {
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
	p.info.Options.Tags = append(p.info.Options.Tags, t)
}

func (p *blockingProcess) ResetTags() {
	p.mu.Lock()
	defer p.mu.Unlock()

	p.tags = make(map[string]struct{})
	p.info.Options.Tags = []string{}
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
