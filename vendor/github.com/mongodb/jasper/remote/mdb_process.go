package remote

import (
	"context"
	"syscall"

	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

type process struct {
	info      jasper.ProcessInfo
	doRequest func(context.Context, mongowire.Message) (mongowire.Message, error)
}

func (p *process) ID() string { return p.info.ID }

func (p *process) Info(ctx context.Context) jasper.ProcessInfo {
	if p.info.Complete {
		return p.info
	}
	req, err := shell.RequestToMessage(infoRequest{ID: p.ID()})
	if err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process info for process %s", p.ID()))
		return jasper.ProcessInfo{}
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process info for process %s", p.ID()))
		return jasper.ProcessInfo{}
	}
	var resp infoResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process info for process %s", p.ID()))
		return jasper.ProcessInfo{}
	}
	if err := resp.SuccessOrError(); err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process info for process %s", p.ID()))
		return jasper.ProcessInfo{}
	}
	p.info = resp.Info
	return p.info
}

func (p *process) Running(ctx context.Context) bool {
	if p.info.Complete {
		return false
	}
	req, err := shell.RequestToMessage(runningRequest{ID: p.ID()})
	if err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process running status for process %s", p.ID()))
		return false
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process running status for process %s", p.ID()))
		return false
	}
	var resp runningResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process running status for process %s", p.ID()))
		return false
	}
	grip.Warning(message.WrapErrorf(resp.SuccessOrError(), "failed to get process running status for process %s", p.ID()))
	return resp.Running
}

func (p *process) Complete(ctx context.Context) bool {
	if p.info.Complete {
		return true
	}
	req, err := shell.RequestToMessage(completeRequest{ID: p.ID()})
	if err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process completion status for process %s", p.ID()))
		return false
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process completion status for process %s", p.ID()))
		return false
	}
	var resp completeResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get process completion status for process %s", p.ID()))
		return false
	}
	grip.Warning(message.WrapErrorf(resp.SuccessOrError(), "failed to get process completion status for process %s", p.ID()))
	return resp.Complete
}

func (p *process) Signal(ctx context.Context, sig syscall.Signal) error {
	r := signalRequest{}
	r.Params.ID = p.ID()
	r.Params.Signal = int(sig)
	req, err := shell.RequestToMessage(r)
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed during request")
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return errors.Wrapf(err, "failed to get signal response for process %s", p.ID())
	}
	return errors.Wrap(resp.SuccessOrError(), "error in response")
}

func (p *process) Wait(ctx context.Context) (int, error) {
	req, err := shell.RequestToMessage(waitRequest{p.ID()})
	if err != nil {
		return -1, errors.Wrap(err, "could not create request")
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		return -1, errors.Wrap(err, "failed during request")
	}
	var resp waitResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return -1, errors.Wrap(err, "failed to get wait response for process %s")
	}
	return resp.ExitCode, errors.Wrap(resp.SuccessOrError(), "error in response")
}

func (p *process) Respawn(ctx context.Context) (jasper.Process, error) {
	req, err := shell.RequestToMessage(respawnRequest{ID: p.ID()})
	if err != nil {
		return nil, errors.Wrap(err, "could not create request")
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed during request")
	}
	var resp infoResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return nil, errors.Wrap(err, "problem with received response")
	}
	if err := resp.SuccessOrError(); err != nil {
		return nil, errors.Wrap(err, "error in response")
	}
	return &process{info: resp.Info, doRequest: p.doRequest}, nil
}

func (p *process) RegisterTrigger(ctx context.Context, t jasper.ProcessTrigger) error {
	return errors.New("cannot register triggers on remote processes")
}

func (p *process) RegisterSignalTrigger(ctx context.Context, t jasper.SignalTrigger) error {
	return errors.New("cannot register signal triggers on remote processes")
}

func (p *process) RegisterSignalTriggerID(ctx context.Context, sigID jasper.SignalTriggerID) error {
	r := registerSignalTriggerIDRequest{}
	r.Params.ID = p.ID()
	r.Params.SignalTriggerID = sigID
	req, err := shell.RequestToMessage(r)
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed during request")
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return errors.Wrap(err, "problem with received response")
	}
	return errors.Wrap(resp.SuccessOrError(), "error in response")
}

func (p *process) Tag(tag string) {
	r := tagRequest{}
	r.Params.ID = p.ID()
	r.Params.Tag = tag
	req, err := shell.RequestToMessage(r)
	if err != nil {
		grip.Warningf("failed to tag process %s with tag %s", p.ID(), tag)
		return
	}
	msg, err := p.doRequest(context.Background(), req)
	if err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to tag process %s with tag %s", p.ID(), tag))
		return
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get tags for process %s", p.ID()))
		return
	}
	grip.Warning(message.WrapErrorf(resp.SuccessOrError(), "failed to tag process %s with tag %s", p.ID(), tag))
}

func (p *process) GetTags() []string {
	req, err := shell.RequestToMessage(getTagsRequest{p.ID()})
	if err != nil {
		grip.Warningf("failed to get tags for process %s", p.ID())
		return nil
	}
	msg, err := p.doRequest(context.Background(), req)
	if err != nil {
		grip.Warningf("failed to get tags for process %s", p.ID())
		return nil
	}
	var resp getTagsResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to get tags for process %s", p.ID()))
		return nil
	}
	return resp.Tags
}

func (p *process) ResetTags() {
	req, err := shell.RequestToMessage(resetTagsRequest{p.ID()})
	if err != nil {
		grip.Warningf("failed to reset tags for process %s", p.ID())
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		grip.Warningf("failed to reset tags for process %s", p.ID())
		return
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapErrorf(err, "failed to reset tags for process %s", p.ID()))
		return
	}
	grip.Warning(message.WrapErrorf(resp.SuccessOrError(), "failed to reset tags for process %s", p.ID()))
}
