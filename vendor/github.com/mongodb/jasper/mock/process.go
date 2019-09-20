package mock

import (
	"context"
	"syscall"

	"github.com/mongodb/jasper"
)

// Process implements the Process interface with exported fields to
// configure and introspect the mock's behavior.
type Process struct {
	ProcInfo jasper.ProcessInfo

	FailRespawn bool

	FailRegisterTrigger bool
	Triggers            jasper.ProcessTriggerSequence

	FailRegisterSignalTrigger bool
	SignalTriggers            jasper.SignalTriggerSequence

	FailRegisterSignalTriggerID bool
	SignalTriggerIDs            []jasper.SignalTriggerID

	FailSignal bool
	Signals    []syscall.Signal

	Tags []string

	FailWait     bool
	WaitExitCode int
}

// ID returns the ID set in ProcInfo set by the user.
func (p *Process) ID() string {
	return p.ProcInfo.ID
}

// Info returns the ProcInfo set by the user.
func (p *Process) Info(ctx context.Context) jasper.ProcessInfo {
	return p.ProcInfo
}

// Running returns the IsRunning field set by the user.
func (p *Process) Running(ctx context.Context) bool {
	return p.ProcInfo.IsRunning
}

// Complete returns the Complete field set by the user.
func (p *Process) Complete(ctx context.Context) bool {
	return p.ProcInfo.Complete
}

// GetTags returns all tags set by the user or using Tag.
func (p *Process) GetTags() []string {
	return p.Tags
}

// Tag adds to the Tags slice.
func (p *Process) Tag(tag string) {
	p.Tags = append(p.Tags, tag)
}

// ResetTags removes all tags stored in Tags.
func (p *Process) ResetTags() {
	p.Tags = []string{}
}

// Signal records the signals sent to the process in Signals. If FailSignal is
// set, it returns an error.
func (p *Process) Signal(ctx context.Context, sig syscall.Signal) error {
	if p.FailSignal {
		return mockFail()
	}

	p.Signals = append(p.Signals, sig)

	return nil
}

// Wait returns the ExitCode set by the user in ProcInfo. If FailWait is set, it
// returns exit code -1 and an error.
func (p *Process) Wait(ctx context.Context) (int, error) {
	if p.FailWait {
		return -1, mockFail()
	}

	return p.ProcInfo.ExitCode, nil
}

// Respawn creates a new Process, which has a copy of all the fields in the
// current Process.
func (p *Process) Respawn(ctx context.Context) (jasper.Process, error) {
	if p.FailRespawn {
		return nil, mockFail()
	}

	newProc := Process(*p)

	return &newProc, nil
}

// RegisterTrigger records the trigger in Triggers. If FailRegisterTrigger is
// set, it returns an error.
func (p *Process) RegisterTrigger(ctx context.Context, t jasper.ProcessTrigger) error {
	if p.FailRegisterTrigger {
		return mockFail()
	}

	p.Triggers = append(p.Triggers, t)

	return nil
}

// RegisterSignalTrigger records the signal trigger in SignalTriggers. If
// FailRegisterSignalTrigger is set, it returns an error.
func (p *Process) RegisterSignalTrigger(ctx context.Context, t jasper.SignalTrigger) error {
	if p.FailRegisterSignalTrigger {
		return mockFail()
	}

	p.SignalTriggers = append(p.SignalTriggers, t)

	return nil
}

// RegisterSignalTriggerID records the ID of the signal trigger in
// SignalTriggers. If FailRegisterSignalTriggerID is set, it returns an error.
func (p *Process) RegisterSignalTriggerID(ctx context.Context, sigID jasper.SignalTriggerID) error {
	if p.FailRegisterSignalTriggerID {
		return mockFail()
	}

	p.SignalTriggerIDs = append(p.SignalTriggerIDs, sigID)

	return nil
}
