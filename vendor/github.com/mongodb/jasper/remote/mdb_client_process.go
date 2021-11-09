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

type mdbProcess struct {
	info      jasper.ProcessInfo
	doRequest func(context.Context, mongowire.Message) (mongowire.Message, error)
}

func (p *mdbProcess) ID() string { return p.info.ID }

func (p *mdbProcess) Info(ctx context.Context) jasper.ProcessInfo {
	if p.info.Complete {
		return p.info
	}
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, infoRequest{ID: p.ID()})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process info",
			"process": p.ID(),
		}))
		return jasper.ProcessInfo{}
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process info",
			"process": p.ID(),
		}))
		return jasper.ProcessInfo{}
	}
	var resp infoResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process info",
			"process": p.ID(),
		}))
		return jasper.ProcessInfo{}
	}
	if err := resp.SuccessOrError(); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process info",
			"process": p.ID(),
		}))
		return jasper.ProcessInfo{}
	}
	p.info = resp.Info
	return p.info
}

func (p *mdbProcess) Running(ctx context.Context) bool {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, runningRequest{ID: p.ID()})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process running status",
			"process": p.ID(),
		}))
		return false
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process running status",
			"process": p.ID(),
		}))
		return false
	}
	var resp runningResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process running status",
			"process": p.ID(),
		}))
		return false
	}
	grip.Warning(message.WrapError(err, message.Fields{
		"message": "failed to get process running status",
		"process": p.ID(),
	}))
	return resp.Running
}

func (p *mdbProcess) Complete(ctx context.Context) bool {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, completeRequest{ID: p.ID()})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process completion status",
			"process": p.ID(),
		}))
		return false
	}
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process completion status",
			"process": p.ID(),
		}))
		return false
	}
	var resp completeResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process completion status",
			"process": p.ID(),
		}))
		return false
	}
	grip.Warning(message.WrapError(resp.SuccessOrError(), message.Fields{
		"message": "failed to get process completion status",
		"process": p.ID(),
	}))
	return resp.Complete
}

func (p *mdbProcess) Signal(ctx context.Context, sig syscall.Signal) error {
	r := signalRequest{}
	r.Params.ID = p.ID()
	r.Params.Signal = int(sig)
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, r)
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

func (p *mdbProcess) Wait(ctx context.Context) (int, error) {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, waitRequest{p.ID()})
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

func (p *mdbProcess) Respawn(ctx context.Context) (jasper.Process, error) {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, respawnRequest{ID: p.ID()})
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
	return &mdbProcess{info: resp.Info, doRequest: p.doRequest}, nil
}

func (p *mdbProcess) RegisterTrigger(ctx context.Context, t jasper.ProcessTrigger) error {
	return errors.New("cannot register triggers on remote processes")
}

func (p *mdbProcess) RegisterSignalTrigger(ctx context.Context, t jasper.SignalTrigger) error {
	return errors.New("cannot register signal triggers on remote processes")
}

func (p *mdbProcess) RegisterSignalTriggerID(ctx context.Context, sigID jasper.SignalTriggerID) error {
	r := registerSignalTriggerIDRequest{}
	r.Params.ID = p.ID()
	r.Params.SignalTriggerID = sigID
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, r)
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

func (p *mdbProcess) Tag(tag string) {
	r := tagRequest{}
	r.Params.ID = p.ID()
	r.Params.Tag = tag
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, r)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to tag process",
			"process": p.ID(),
			"tag":     tag,
		}))
		return
	}
	msg, err := p.doRequest(context.Background(), req)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to tag process",
			"process": p.ID(),
			"tag":     tag,
		}))
		return
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to tag process",
			"process": p.ID(),
			"tag":     tag,
		}))
		return
	}
	if err := resp.SuccessOrError(); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to tag process",
			"process": p.ID(),
			"tag":     tag,
		}))
		return
	}
	p.info.Options.Tags = append(p.info.Options.Tags, tag)
}

func (p *mdbProcess) GetTags() []string {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, getTagsRequest{p.ID()})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get tags",
			"process": p.ID(),
		}))
		return nil
	}
	msg, err := p.doRequest(context.Background(), req)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get tags",
			"process": p.ID(),
		}))
		return nil
	}
	var resp getTagsResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get tags",
			"process": p.ID(),
		}))
		return nil
	}
	return resp.Tags
}

func (p *mdbProcess) ResetTags() {
	req, err := shell.RequestToMessage(mongowire.OP_QUERY, resetTagsRequest{p.ID()})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to reset tags",
			"process": p.ID(),
		}))
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	msg, err := p.doRequest(ctx, req)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to reset tags",
			"process": p.ID(),
		}))
		return
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to reset tags",
			"process": p.ID(),
		}))
		return
	}
	if err := resp.SuccessOrError(); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to reset tags",
			"process": p.ID(),
		}))
		return
	}
	p.info.Options.Tags = []string{}
}
