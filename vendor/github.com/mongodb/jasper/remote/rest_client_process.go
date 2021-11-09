package remote

import (
	"context"
	"net/http"
	"syscall"

	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/pkg/errors"
)

// restProcess is the client-side representation of a jasper.Process for making
// requests to the remote REST service.
type restProcess struct {
	id     string
	client *restClient
}

func (p *restProcess) ID() string { return p.id }

func (p *restProcess) Info(ctx context.Context) jasper.ProcessInfo {
	info, err := p.client.getProcessInfo(ctx, p.id)
	grip.Warning(message.WrapError(err, message.Fields{
		"message": "failed to get process info",
		"process": p.id,
	}))
	return info
}

func (p *restProcess) Running(ctx context.Context) bool {
	info, err := p.client.getProcessInfo(ctx, p.id)
	grip.Warning(message.WrapError(err, message.Fields{
		"message": "failed to get process running status",
		"process": p.id,
	}))
	return info.IsRunning
}

func (p *restProcess) Complete(ctx context.Context) bool {
	info, err := p.client.getProcessInfo(ctx, p.id)
	grip.Warning(message.WrapError(err, message.Fields{
		"message": "failed to get process completion status",
		"process": p.id,
	}))
	return info.Complete
}

func (p *restProcess) Signal(ctx context.Context, sig syscall.Signal) error {
	resp, err := p.client.doRequest(ctx, http.MethodPatch, p.client.getURL("/process/%s/signal/%d", p.id, sig), nil)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	return nil
}

func (p *restProcess) Wait(ctx context.Context) (int, error) {
	resp, err := p.client.doRequest(ctx, http.MethodGet, p.client.getURL("/process/%s/wait", p.id), nil)
	if err != nil {
		return -1, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	var waitResp restWaitResponse
	if err = gimlet.GetJSON(resp.Body, &waitResp); err != nil {
		return -1, errors.Wrap(err, "reading response from wait")
	}
	if waitResp.Error != "" {
		return waitResp.ExitCode, errors.New(waitResp.Error)
	}
	return waitResp.ExitCode, nil
}

func (p *restProcess) Respawn(ctx context.Context) (jasper.Process, error) {
	resp, err := p.client.doRequest(ctx, http.MethodGet, p.client.getURL("/process/%s/respawn", p.id), nil)
	if err != nil {
		return nil, errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	info := jasper.ProcessInfo{}
	if err = gimlet.GetJSON(resp.Body, &info); err != nil {
		return nil, errors.WithStack(err)
	}

	return &restProcess{
		id:     info.ID,
		client: p.client,
	}, nil
}

func (p *restProcess) RegisterTrigger(_ context.Context, _ jasper.ProcessTrigger) error {
	return errors.New("cannot register triggers on remote processes")
}

func (p *restProcess) RegisterSignalTrigger(_ context.Context, _ jasper.SignalTrigger) error {
	return errors.New("cannot register signal trigger on remote processes")
}

func (p *restProcess) RegisterSignalTriggerID(ctx context.Context, triggerID jasper.SignalTriggerID) error {
	resp, err := p.client.doRequest(ctx, http.MethodPatch, p.client.getURL("/process/%s/trigger/signal/%s", p.id, triggerID), nil)
	if err != nil {
		return errors.Wrap(err, "request returned error")
	}
	defer resp.Body.Close()

	return nil
}

func (p *restProcess) Tag(tag string) {
	resp, err := p.client.doRequest(context.Background(), http.MethodPost, p.client.getURL("/process/%s/tags?add=%s", p.id, tag), nil)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to tag process",
			"process": p.ID(),
			"tag":     tag,
		}))
		return
	}
	defer resp.Body.Close()
}

func (p *restProcess) GetTags() []string {
	resp, err := p.client.doRequest(context.Background(), http.MethodGet, p.client.getURL("/process/%s/tags", p.id), nil)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to get tags",
			"process": p.ID(),
		}))
		return nil
	}
	defer resp.Body.Close()

	out := []string{}
	if err = gimlet.GetJSON(resp.Body, &out); err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "failed to read tags from response",
			"process": p.ID(),
		}))

		return nil
	}
	return out
}

func (p *restProcess) ResetTags() {
	resp, err := p.client.doRequest(context.Background(), http.MethodDelete, p.client.getURL("/process/%s/tags", p.id), nil)
	if err != nil {
		grip.Warning(message.WrapError(err, message.Fields{
			"message": "request returned error",
			"process": p.id,
		}))
		return
	}
	defer resp.Body.Close()
}
