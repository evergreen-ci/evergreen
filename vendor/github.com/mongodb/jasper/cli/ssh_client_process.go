package cli

import (
	"context"
	"encoding/json"
	"syscall"

	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// sshProcess uses SSH to access a remote machine's Jasper CLI, which has access
// to methods in the Process interface.
type sshProcess struct {
	client *sshRunner
	info   jasper.ProcessInfo
}

// newSSHProcess creates a new process that runs using a Jasper CLI over SSH.
// The caller should pass in the function that will run CLI client commands over
// SSH.
func newSSHProcess(client *sshRunner, info jasper.ProcessInfo) (jasper.Process, error) {
	if client == nil {
		return nil, errors.New("SSH process needs an SSH client to run CLI commands")
	}
	return &sshProcess{
		client: client,
		info:   info,
	}, nil
}

func (p *sshProcess) ID() string {
	return p.info.ID
}

func (p *sshProcess) Info(ctx context.Context) jasper.ProcessInfo {
	if p.info.Complete {
		return p.info
	}

	output, err := p.runCommand(ctx, InfoCommand, &IDInput{ID: p.info.ID})
	if err != nil {
		return jasper.ProcessInfo{}
	}

	resp, err := ExtractInfoResponse(output)
	if err != nil {
		return jasper.ProcessInfo{}
	}
	p.info = resp.Info

	return p.info
}

func (p *sshProcess) Running(ctx context.Context) bool {
	if p.info.Complete {
		return false
	}

	output, err := p.runCommand(ctx, RunningCommand, &IDInput{ID: p.info.ID})
	if err != nil {
		return false
	}

	resp, err := ExtractRunningResponse(output)
	if err != nil {
		return false
	}
	p.info.IsRunning = resp.Running

	return p.info.IsRunning
}

func (p *sshProcess) Complete(ctx context.Context) bool {
	if p.info.Complete {
		return true
	}

	output, err := p.runCommand(ctx, CompleteCommand, &IDInput{ID: p.info.ID})
	if err != nil {
		return false
	}

	resp, err := ExtractCompleteResponse(output)
	if err != nil {
		return false
	}
	p.info.Complete = resp.Complete

	return p.info.Complete
}

func (p *sshProcess) Signal(ctx context.Context, sig syscall.Signal) error {
	output, err := p.runCommand(ctx, SignalCommand, &SignalInput{ID: p.info.ID, Signal: int(sig)})
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err = ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *sshProcess) Wait(ctx context.Context) (int, error) {
	output, err := p.runCommand(ctx, WaitCommand, &IDInput{ID: p.info.ID})
	if err != nil {
		return -1, errors.WithStack(err)
	}

	resp, err := ExtractWaitResponse(output)
	if err != nil {
		return resp.ExitCode, errors.WithStack(err)
	}

	if resp.Error != "" {
		return resp.ExitCode, errors.New(resp.Error)
	}

	return resp.ExitCode, nil
}

func (p *sshProcess) Respawn(ctx context.Context) (jasper.Process, error) {
	output, err := p.runCommand(ctx, RespawnCommand, &IDInput{ID: p.info.ID})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	resp, err := ExtractInfoResponse(output)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return newSSHProcess(p.client, resp.Info)
}

func (p *sshProcess) RegisterTrigger(ctx context.Context, t jasper.ProcessTrigger) error {
	return errors.New("cannot register triggers on remote processes")
}

func (p *sshProcess) RegisterSignalTrigger(ctx context.Context, t jasper.SignalTrigger) error {
	return errors.New("cannot register signal triggers on remote processes")
}

func (p *sshProcess) RegisterSignalTriggerID(ctx context.Context, sigID jasper.SignalTriggerID) error {
	output, err := p.runCommand(ctx, RegisterSignalTriggerIDCommand, &SignalTriggerIDInput{
		ID:              p.info.ID,
		SignalTriggerID: sigID,
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if _, err = ExtractOutcomeResponse(output); err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (p *sshProcess) Tag(tag string) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _ = p.runCommand(ctx, TagCommand, &TagIDInput{
		ID:  p.info.ID,
		Tag: tag,
	})
}

func (p *sshProcess) GetTags() []string {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	output, err := p.runCommand(ctx, GetTagsCommand, &IDInput{
		ID: p.info.ID,
	})
	if err != nil {
		return nil
	}
	resp, err := ExtractTagsResponse(output)
	if err != nil {
		return nil
	}
	return resp.Tags
}

func (p *sshProcess) ResetTags() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	_, _ = p.runCommand(ctx, ResetTagsCommand, &IDInput{
		ID: p.info.ID,
	})
}

func (p *sshProcess) runCommand(ctx context.Context, processSubcommand string, subcommandInput interface{}) (json.RawMessage, error) {
	return p.client.runClientCommand(ctx, []string{ProcessCommand, processSubcommand}, subcommandInput)
}
