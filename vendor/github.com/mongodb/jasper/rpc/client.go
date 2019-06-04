package rpc

import (
	"context"
	"io"
	"net"
	"syscall"

	empty "github.com/golang/protobuf/ptypes/empty"
	"github.com/mongodb/jasper"
	internal "github.com/mongodb/jasper/rpc/internal"
	"github.com/pkg/errors"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type rpcClient struct {
	client       internal.JasperProcessManagerClient
	clientCloser jasper.CloseFunc
}

// NewClient creates a connection to the RPC service specified
// in the address. If certFile is non-empty, the credentials will be read from
// the file to establish a secure TLS connection; otherwise, it will establish
// an insecure connection. The caller is responsible for closing the connection
// using the returned jasper.CloseFunc.
func NewClient(ctx context.Context, addr net.Addr, certFile string) (jasper.RemoteClient, error) {
	var credsDialOpt grpc.DialOption
	if certFile != "" {
		creds, err := credentials.NewClientTLSFromFile(certFile, "")
		if err != nil {
			return nil, errors.Wrapf(err, "could not get client credentials from cert file '%s'", certFile)
		}
		credsDialOpt = grpc.WithTransportCredentials(creds)
	} else {
		credsDialOpt = grpc.WithInsecure()
	}

	conn, err := grpc.DialContext(ctx, addr.String(), credsDialOpt, grpc.WithBlock())
	if err != nil {
		return nil, errors.Wrapf(err, "could not establish connection to service at address '%s'", addr.String())
	}

	return newRPCClient(conn), nil
}

// newRPCClient is a constructor for an RPC client.
func newRPCClient(cc *grpc.ClientConn) jasper.RemoteClient {
	return &rpcClient{
		client:       internal.NewJasperProcessManagerClient(cc),
		clientCloser: cc.Close,
	}
}

func (m *rpcClient) CloseConnection() error {
	return m.clientCloser()
}

func (m *rpcClient) CreateProcess(ctx context.Context, opts *jasper.CreateOptions) (jasper.Process, error) {
	proc, err := m.client.Create(ctx, internal.ConvertCreateOptions(opts))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &rpcProcess{client: m.client, info: proc}, nil
}

func (m *rpcClient) CreateCommand(ctx context.Context) *jasper.Command {
	return jasper.NewCommand().ProcConstructor(m.CreateProcess)
}

func (m *rpcClient) Register(ctx context.Context, proc jasper.Process) error {
	return errors.New("cannot register extant processes on remote systms")
}

func (m *rpcClient) List(ctx context.Context, f jasper.Filter) ([]jasper.Process, error) {
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

func (m *rpcClient) Group(ctx context.Context, name string) ([]jasper.Process, error) {
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

func (m *rpcClient) Get(ctx context.Context, name string) (jasper.Process, error) {
	info, err := m.client.Get(ctx, &internal.JasperProcessID{Value: name})
	if err != nil {
		return nil, errors.Wrap(err, "problem finding process")
	}

	return &rpcProcess{client: m.client, info: info}, nil
}

func (m *rpcClient) Clear(ctx context.Context) {
	_, _ = m.client.Clear(ctx, &empty.Empty{})
}

func (m *rpcClient) Close(ctx context.Context) error {
	resp, err := m.client.Close(ctx, &empty.Empty{})
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (m *rpcClient) Status(ctx context.Context) (string, bool, error) {
	resp, err := m.client.Status(ctx, &empty.Empty{})
	if err != nil {
		return "", false, errors.WithStack(err)
	}
	return resp.HostId, resp.Active, nil
}

func (m *rpcClient) ConfigureCache(ctx context.Context, opts jasper.CacheOptions) error {
	resp, err := m.client.ConfigureCache(ctx, internal.ConvertCacheOptions(opts))
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (m *rpcClient) DownloadFile(ctx context.Context, info jasper.DownloadInfo) error {
	resp, err := m.client.DownloadFile(ctx, internal.ConvertDownloadInfo(info))
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (m *rpcClient) DownloadMongoDB(ctx context.Context, opts jasper.MongoDBDownloadOptions) error {
	resp, err := m.client.DownloadMongoDB(ctx, internal.ConvertMongoDBDownloadOptions(opts))
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (m *rpcClient) GetLogStream(ctx context.Context, id string, count int) (jasper.LogStream, error) {
	stream, err := m.client.GetLogStream(ctx, &internal.LogRequest{
		Id:    &internal.JasperProcessID{Value: id},
		Count: int64(count),
	})
	if err != nil {
		return jasper.LogStream{}, errors.WithStack(err)
	}
	return stream.Export(), nil
}

func (m *rpcClient) GetBuildloggerURLs(ctx context.Context, id string) ([]string, error) {
	resp, err := m.client.GetBuildloggerURLs(ctx, &internal.JasperProcessID{Value: id})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return resp.Urls, nil
}

func (m *rpcClient) SignalEvent(ctx context.Context, name string) error {
	resp, err := m.client.SignalEvent(ctx, &internal.EventName{Value: name})
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
			return int(resp.ExitCode), errors.Wrap(errors.New(resp.Text), "operation failed")
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
