package rpc

import (
	"context"
	"io"
	"syscall"

	empty "github.com/golang/protobuf/ptypes/empty"
	"github.com/mongodb/jasper"
	internal "github.com/mongodb/jasper/rpc/internal"
	"github.com/pkg/errors"
	grpc "google.golang.org/grpc"
)

type rpcManager struct {
	client internal.JasperProcessManagerClient
}

// TODO provide some better way of constructing this object

// NewRPCManager is a constructor for a rpcManager.
func NewRPCManager(cc *grpc.ClientConn) jasper.Manager {
	return &rpcManager{
		client: internal.NewJasperProcessManagerClient(cc),
	}
}

func (m *rpcManager) CreateProcess(ctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
	proc, err := m.client.Create(ctx, internal.ConvertCreateOptions(opts))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &rpcProcess{client: m.client, info: proc}, nil
}

func (m *rpcManager) CreateCommand(ctx context.Context) *jasper.Command {
	return jasper.NewCommand().ProcConstructor(m.CreateProcess)
}

func (m *rpcManager) Register(ctx context.Context, proc jasper.Process) error {
	return errors.New("cannot register extant processes on remote systms")
}

func (m *rpcManager) List(ctx context.Context, f jasper.Filter) ([]jasper.Process, error) {
	procs, err := m.client.List(ctx, internal.ConvertFilter(f))
	if err != nil {
		return nil, errors.Wrap(err, "problem getting streaming client")
	}

	out := []jasper.Process{}
	for {
		info, err := procs.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, errors.Wrap(err, "problem getting list")
		}

		out = append(out, &rpcProcess{
			client: m.client,
			info:   info,
		})
	}

	return out, nil
}

func (m *rpcManager) Group(ctx context.Context, name string) ([]jasper.Process, error) {
	procs, err := m.client.Group(ctx, &internal.TagName{Value: name})
	if err != nil {
		return nil, errors.Wrap(err, "problem getting streaming client")
	}

	out := []jasper.Process{}
	for {
		info, err := procs.Recv()
		if err == io.EOF {
			break
		} else if err != nil {
			return nil, errors.Wrap(err, "problem getting group")
		}

		out = append(out, &rpcProcess{
			client: m.client,
			info:   info,
		})
	}

	return out, nil
}

func (m *rpcManager) Get(ctx context.Context, name string) (jasper.Process, error) {
	info, err := m.client.Get(ctx, &internal.JasperProcessID{Value: name})
	if err != nil {
		return nil, errors.Wrap(err, "problem finding process")
	}

	return &rpcProcess{client: m.client, info: info}, nil
}

func (m *rpcManager) Clear(ctx context.Context) {
	_, _ = m.client.Clear(ctx, &empty.Empty{})
}

func (m *rpcManager) Close(ctx context.Context) error {
	resp, err := m.client.Close(ctx, &empty.Empty{})
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

type rpcProcess struct {
	client internal.JasperProcessManagerClient
	info   *internal.ProcessInfo
}

func (p *rpcProcess) ID() string { return p.info.Id }

func (p *rpcProcess) Info(ctx context.Context) jasper.ProcessInfo {
	if p.info.Complete {
		return p.info.Export()
	}

	info, err := p.client.Get(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		return jasper.ProcessInfo{}
	}

	return info.Export()
}
func (p *rpcProcess) Running(ctx context.Context) bool {
	if p.info.Complete {
		return false
	}

	info, err := p.client.Get(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		return false
	}
	p.info = info

	return info.Running
}

func (p *rpcProcess) Complete(ctx context.Context) bool {
	if p.info.Complete {
		return true
	}

	info, err := p.client.Get(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
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

	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (p *rpcProcess) Wait(ctx context.Context) (int, error) {
	resp, err := p.client.Wait(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		return -1, errors.WithStack(err)
	}

	if resp.Success {
		if resp.ExitCode != 0 {
			return int(resp.ExitCode), errors.New("operation failed")
		}
		return int(resp.ExitCode), nil
	}

	return -1, errors.New(resp.Text)
}

func (p *rpcProcess) Respawn(ctx context.Context) (jasper.Process, error) {
	newProc, err := p.client.Respawn(ctx, &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &rpcProcess{client: p.client, info: newProc}, nil
}

func (p *rpcProcess) RegisterTrigger(ctx context.Context, _ jasper.ProcessTrigger) error {
	return errors.New("cannot register remote triggers")
}

func (p *rpcProcess) RegisterSignalTrigger(ctx context.Context, _ jasper.SignalTrigger) error {
	return errors.New("cannot register remote signal triggers")
}

func (p *rpcProcess) RegisterSignalTriggerID(ctx context.Context, sigID jasper.SignalTriggerID) error {
	resp, err := p.client.RegisterSignalTriggerID(ctx, &internal.SignalTriggerParams{
		ProcessID:       &internal.JasperProcessID{Value: p.info.Id},
		SignalTriggerID: internal.ConvertSignalTriggerID(sigID),
	})
	if err != nil {
		return errors.WithStack(err)
	}

	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (p *rpcProcess) Tag(tag string) {
	_, _ = p.client.TagProcess(context.TODO(), &internal.ProcessTags{
		ProcessID: p.info.Id,
		Tags:      []string{tag},
	})
}
func (p *rpcProcess) GetTags() []string {
	tags, err := p.client.GetTags(context.TODO(), &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		return nil
	}

	return tags.Tags
}
func (p *rpcProcess) ResetTags() {
	_, _ = p.client.ResetTags(context.TODO(), &internal.JasperProcessID{Value: p.info.Id})
}
