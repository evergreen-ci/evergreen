package remote

import (
	"context"
	"syscall"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	internal "github.com/mongodb/jasper/remote/internal"
	"github.com/pkg/errors"
)

// rpcProcess is the client-side representation of a jasper.Process for making
// requests to the remote gRPC service.
type rpcProcess struct {
	client internal.JasperProcessManagerClient
	info   *internal.ProcessInfo
}

func (p *rpcProcess) ID() string { return p.info.Id }

func (p *rpcProcess) Info(ctx context.Context) jasper.ProcessInfo {
	info, err := p.client.Get(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process info",
			"process": p.ID(),
		}))
		return jasper.ProcessInfo{}
	}
	p.info = info

	exportedInfo, err := p.info.Export()
	grip.Warning(message.WrapError(err, message.Fields{
		"message": "failed to convert info for process",
		"process": p.ID(),
	}))

	return exportedInfo
}
func (p *rpcProcess) Running(ctx context.Context) bool {
	info, err := p.client.Get(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process running status",
			"process": p.ID(),
		}))
		return false
	}
	p.info = info

	return info.Running
}

func (p *rpcProcess) Complete(ctx context.Context) bool {
	info, err := p.client.Get(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get process completion status",
			"process": p.ID(),
		}))
		return false
	}
	p.info = info

	return info.Complete
}

func (p *rpcProcess) Signal(ctx context.Context, sig syscall.Signal) error {
	resp, err := p.client.Signal(ctx, &internal.SignalProcess{
		ProcessID: &internal.JasperProcessID{Value: p.info.Id},
		Signal:    internal.ConvertSignal(sig),
	})

	if err != nil {
		return errors.WithStack(err)
	}

	if !resp.Success {
		return errors.New(resp.Text)
	}

	return nil
}

func (p *rpcProcess) Wait(ctx context.Context) (int, error) {
	resp, err := p.client.Wait(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		return -1, errors.WithStack(err)
	}

	if !resp.Success {
		return int(resp.ExitCode), errors.Wrapf(errors.New(resp.Text), "process exited with error")
	}

	return int(resp.ExitCode), nil
}

func (p *rpcProcess) Respawn(ctx context.Context) (jasper.Process, error) {
	newProc, err := p.client.Respawn(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &rpcProcess{client: p.client, info: newProc}, nil
}

func (p *rpcProcess) RegisterTrigger(ctx context.Context, _ jasper.ProcessTrigger) error {
	return errors.New("cannot register triggers on remote processes")
}

func (p *rpcProcess) RegisterSignalTrigger(ctx context.Context, _ jasper.SignalTrigger) error {
	return errors.New("cannot register signal triggers on remote processes")
}

func (p *rpcProcess) RegisterSignalTriggerID(ctx context.Context, sigID jasper.SignalTriggerID) error {
	resp, err := p.client.RegisterSignalTriggerID(ctx, &internal.SignalTriggerParams{
		ProcessID:       &internal.JasperProcessID{Value: p.info.Id},
		SignalTriggerID: internal.ConvertSignalTriggerID(sigID),
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if !resp.Success {
		return errors.New(resp.Text)
	}

	return nil
}

func (p *rpcProcess) Tag(tag string) {
	resp, err := p.client.TagProcess(context.Background(), &internal.ProcessTags{
		ProcessID: p.info.Id,
		Tags:      []string{tag},
	})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to tag process",
			"process": p.ID(),
			"tag":     tag,
		}))
		return
	}
	if !resp.Success {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to tag process",
			"process": p.ID(),
			"tag":     tag,
		}))
		return
	}
	p.info.Options.Tags = append(p.info.Options.Tags, tag)
}

func (p *rpcProcess) GetTags() []string {
	resp, err := p.client.GetTags(context.Background(), &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get tags",
			"process": p.ID(),
		}))
		return nil
	}

	return resp.Tags
}

func (p *rpcProcess) ResetTags() {
	resp, err := p.client.ResetTags(context.Background(), &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to reset tags",
			"process": p.ID(),
		}))
		return
	}
	if !resp.Success {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to reset tags",
			"process": p.ID(),
			"reason":  resp.Text,
		}))
		return
	}

	p.info.Options.Tags = []string{}
}
