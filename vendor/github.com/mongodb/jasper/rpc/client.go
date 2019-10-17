package rpc

import (
	"context"
	"io"
	"net"
	"syscall"

	"github.com/evergreen-ci/certdepot"
	empty "github.com/golang/protobuf/ptypes/empty"
	"github.com/mongodb/grip"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	internal "github.com/mongodb/jasper/rpc/internal"
	"github.com/pkg/errors"
	grpc "google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type rpcClient struct {
	client       internal.JasperProcessManagerClient
	clientCloser jasper.CloseFunc
}

// NewClient creates a connection to the RPC service with the specified address
// addr. If creds is non-nil, the credentials will be used to establish a secure
// TLS connection with the service; otherwise, it will establish an insecure
// connection. The caller is responsible for closing the connection using the
// returned jasper.CloseFunc.
func NewClient(ctx context.Context, addr net.Addr, creds *certdepot.Credentials) (jasper.RemoteClient, error) {
	opts := []grpc.DialOption{
		grpc.WithBlock(),
		grpc.WithDefaultCallOptions(grpc.WaitForReady(true)),
	}
	if creds != nil {
		tlsConf, err := creds.Resolve()
		if err != nil {
			return nil, errors.Wrap(err, "could not resolve credentials into TLS config")
		}
		opts = append(opts, grpc.WithTransportCredentials(credentials.NewTLS(tlsConf)))
	} else {
		opts = append(opts, grpc.WithInsecure())
	}

	conn, err := grpc.DialContext(ctx, addr.String(), opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "could not establish connection to service at address '%s'", addr.String())
	}

	return newRPCClient(conn), nil
}

// NewClientWithFile is the same as NewClient but the credentials will
// be read from the file given by filePath if the filePath is non-empty. The
// credentials file should contain the JSON-encoded bytes from
// (*Credentials).Export().
func NewClientWithFile(ctx context.Context, addr net.Addr, filePath string) (jasper.RemoteClient, error) {
	var creds *certdepot.Credentials
	if filePath != "" {
		var err error
		creds, err = certdepot.NewCredentialsFromFile(filePath)
		if err != nil {
			return nil, errors.Wrap(err, "error getting credentials from file")
		}
	}

	return NewClient(ctx, addr, creds)
}

// newRPCClient is a constructor for an RPC client.
func newRPCClient(cc *grpc.ClientConn) jasper.RemoteClient {
	return &rpcClient{
		client:       internal.NewJasperProcessManagerClient(cc),
		clientCloser: cc.Close,
	}
}

func (c *rpcClient) ID() string {
	resp, err := c.client.ID(context.Background(), &empty.Empty{})
	if err != nil {
		return ""
	}
	return resp.Value
}

func (c *rpcClient) CreateProcess(ctx context.Context, opts *options.Create) (jasper.Process, error) {
	proc, err := c.client.Create(ctx, internal.ConvertCreateOptions(opts))
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return &rpcProcess{client: c.client, info: proc}, nil
}

func (c *rpcClient) CreateCommand(ctx context.Context) *jasper.Command {
	return jasper.NewCommand().ProcConstructor(c.CreateProcess)
}

func (c *rpcClient) Register(ctx context.Context, proc jasper.Process) error {
	return errors.New("cannot register extant processes on remote process managers")
}

func (c *rpcClient) List(ctx context.Context, f options.Filter) ([]jasper.Process, error) {
	procs, err := c.client.List(ctx, internal.ConvertFilter(f))
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
			client: c.client,
			info:   info,
		})
	}

	return out, nil
}

func (c *rpcClient) Group(ctx context.Context, name string) ([]jasper.Process, error) {
	procs, err := c.client.Group(ctx, &internal.TagName{Value: name})
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
			client: c.client,
			info:   info,
		})
	}

	return out, nil
}

func (c *rpcClient) Get(ctx context.Context, name string) (jasper.Process, error) {
	info, err := c.client.Get(ctx, &internal.JasperProcessID{Value: name})
	if err != nil {
		return nil, errors.Wrap(err, "problem finding process")
	}

	return &rpcProcess{client: c.client, info: info}, nil
}

func (c *rpcClient) Clear(ctx context.Context) {
	_, _ = c.client.Clear(ctx, &empty.Empty{})
}

func (c *rpcClient) Close(ctx context.Context) error {
	resp, err := c.client.Close(ctx, &empty.Empty{})
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (c *rpcClient) Status(ctx context.Context) (string, bool, error) {
	resp, err := c.client.Status(ctx, &empty.Empty{})
	if err != nil {
		return "", false, errors.WithStack(err)
	}
	return resp.HostId, resp.Active, nil
}

func (c *rpcClient) CloseConnection() error {
	return c.clientCloser()
}

func (c *rpcClient) ConfigureCache(ctx context.Context, opts options.Cache) error {
	resp, err := c.client.ConfigureCache(ctx, internal.ConvertCacheOptions(opts))
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (c *rpcClient) DownloadFile(ctx context.Context, info options.Download) error {
	resp, err := c.client.DownloadFile(ctx, internal.ConvertDownloadInfo(info))
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (c *rpcClient) DownloadMongoDB(ctx context.Context, opts options.MongoDBDownload) error {
	resp, err := c.client.DownloadMongoDB(ctx, internal.ConvertMongoDBDownloadOptions(opts))
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (c *rpcClient) GetLogStream(ctx context.Context, id string, count int) (jasper.LogStream, error) {
	stream, err := c.client.GetLogStream(ctx, &internal.LogRequest{
		Id:    &internal.JasperProcessID{Value: id},
		Count: int64(count),
	})
	if err != nil {
		return jasper.LogStream{}, errors.WithStack(err)
	}
	return stream.Export(), nil
}

func (c *rpcClient) GetBuildloggerURLs(ctx context.Context, id string) ([]string, error) {
	resp, err := c.client.GetBuildloggerURLs(ctx, &internal.JasperProcessID{Value: id})
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return resp.Urls, nil
}

func (c *rpcClient) SignalEvent(ctx context.Context, name string) error {
	resp, err := c.client.SignalEvent(ctx, &internal.EventName{Value: name})
	if err != nil {
		return errors.WithStack(err)
	}
	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (c *rpcClient) WriteFile(ctx context.Context, jinfo options.WriteFile) error {
	stream, err := c.client.WriteFile(ctx)
	if err != nil {
		return errors.Wrap(err, "error getting client stream to write file")
	}

	sendInfo := func(jinfo options.WriteFile) error {
		info := internal.ConvertWriteFileInfo(jinfo)
		return stream.Send(info)
	}

	if err := jinfo.WriteBufferedContent(sendInfo); err != nil {
		catcher := grip.NewBasicCatcher()
		catcher.Wrapf(err, "error reading from content source")
		catcher.Wrapf(stream.CloseSend(), "error closing send stream after error during read: %s", err.Error())
		return catcher.Resolve()
	}

	resp, err := stream.CloseAndRecv()
	if err != nil {
		return errors.WithStack(err)
	}

	if !resp.Success {
		return errors.New(resp.Text)
	}

	return nil
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
	p.info = info

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

	if resp.Success {
		return nil
	}

	return errors.New(resp.Text)
}

func (p *rpcProcess) Tag(tag string) {
	_, _ = p.client.TagProcess(context.Background(), &internal.ProcessTags{
		ProcessID: p.info.Id,
		Tags:      []string{tag},
	})
}

func (p *rpcProcess) GetTags() []string {
	tags, err := p.client.GetTags(context.Background(), &internal.JasperProcessID{Value: p.info.Id})
	if err != nil {
		return nil
	}

	return tags.Tags
}

func (p *rpcProcess) ResetTags() {
	_, _ = p.client.ResetTags(context.Background(), &internal.JasperProcessID{Value: p.info.Id})
}
