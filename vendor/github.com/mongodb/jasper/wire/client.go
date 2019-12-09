package wire

import (
	"context"
	"net"
	"time"

	"github.com/evergreen-ci/mrpc/mongowire"
	"github.com/evergreen-ci/mrpc/shell"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
	"github.com/pkg/errors"
)

type client struct {
	conn      net.Conn
	namespace string
	timeout   time.Duration
}

const (
	namespace = "jasper.$cmd"
)

// NewClient returns a remote client for connection to a MongoDB wire protocol
// service. reqTimeout specifies the timeout for a request, or uses a default
// timeout if zero.
func NewClient(ctx context.Context, addr net.Addr, reqTimeout time.Duration) (jasper.RemoteClient, error) {
	dialer := net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr.String())
	if err != nil {
		return nil, errors.Wrapf(err, "could not establish connection to %s service at address %s", addr.Network(), addr.String())
	}
	timeout := reqTimeout
	if timeout.Seconds() == 0 {
		timeout = 30 * time.Second
	}
	return &client{conn: conn, namespace: namespace, timeout: timeout}, nil
}

func (c *client) ID() string {
	req, err := shell.RequestToMessage(&idRequest{ID: 1})
	if err != nil {
		grip.Warning(message.WrapError(err, "could not create request"))
		return ""
	}
	msg, err := c.doRequest(context.Background(), req)
	if err != nil {
		grip.Warning(message.WrapError(err, "failed during request"))
		return ""
	}
	var resp idResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapError(err, "could not read response"))
		return ""
	}
	if err := resp.SuccessOrError(); err != nil {
		grip.Warning(message.WrapError(err, "error in response"))
		return ""
	}
	return resp.ID
}

func (c *client) CreateProcess(ctx context.Context, opts *options.Create) (jasper.Process, error) {
	req, err := shell.RequestToMessage(createProcessRequest{Options: *opts})
	if err != nil {
		return nil, errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed during request")
	}
	var resp infoResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return nil, errors.Wrap(err, "could not read response")
	}
	if err := resp.SuccessOrError(); err != nil {
		return nil, errors.Wrap(err, "error in response")
	}
	return &process{info: resp.Info, doRequest: c.doRequest}, nil
}

func (c *client) CreateCommand(ctx context.Context) *jasper.Command {
	return jasper.NewCommand().ProcConstructor(c.CreateProcess)
}

func (c *client) CreateScripting(_ context.Context, _ options.ScriptingHarness) (jasper.ScriptingHarness, error) {
	return nil, errors.New("scripting environment is not supported")
}

func (c *client) GetScripting(_ context.Context, _ string) (jasper.ScriptingHarness, error) {
	return nil, errors.New("scripting environment is not supported")
}

func (c *client) Register(ctx context.Context, proc jasper.Process) error {
	return errors.New("cannot register local processes on remote process managers")
}

func (c *client) List(ctx context.Context, f options.Filter) ([]jasper.Process, error) {
	req, err := shell.RequestToMessage(listRequest{Filter: f})
	if err != nil {
		return nil, errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed during request")
	}
	var resp infosResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return nil, errors.Wrap(err, "could not read response")
	}
	if err := resp.SuccessOrError(); err != nil {
		return nil, errors.Wrap(err, "error in response")
	}
	infos := resp.Infos
	procs := make([]jasper.Process, 0, len(infos))
	for _, info := range infos {
		procs = append(procs, &process{info: info, doRequest: c.doRequest})
	}
	return procs, nil
}

func (c *client) Group(ctx context.Context, tag string) ([]jasper.Process, error) {
	req, err := shell.RequestToMessage(groupRequest{Tag: tag})
	if err != nil {
		return nil, errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed during request")
	}
	var resp infosResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return nil, errors.Wrap(err, "could not read response")
	}
	if err := resp.SuccessOrError(); err != nil {
		return nil, errors.Wrap(err, "error in response")
	}
	infos := resp.Infos
	procs := make([]jasper.Process, 0, len(infos))
	for _, info := range infos {
		procs = append(procs, &process{info: info, doRequest: c.doRequest})
	}
	return procs, nil
}

func (c *client) Get(ctx context.Context, id string) (jasper.Process, error) {
	req, err := shell.RequestToMessage(&getProcessRequest{ID: id})
	if err != nil {
		return nil, errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed during request")
	}
	var resp infoResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return nil, errors.Wrap(err, "could not read response")
	}
	if err := resp.SuccessOrError(); err != nil {
		return nil, errors.Wrap(err, "error in response")
	}
	info := resp.Info
	return &process{info: info, doRequest: c.doRequest}, nil
}

func (c *client) Clear(ctx context.Context) {
	req, err := shell.RequestToMessage(&clearRequest{Clear: 1})
	if err != nil {
		grip.Warning(message.WrapError(err, "could not create request"))
		return
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		grip.Warning(message.WrapError(err, "failed during request"))
		return
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		grip.Warning(message.WrapError(shell.MessageToResponse(msg, &resp), "could not read response"))
	}
	grip.Warning(message.WrapError(resp.SuccessOrError(), "error in response"))
}

func (c *client) Close(ctx context.Context) error {
	req, err := shell.RequestToMessage(&closeRequest{Close: 1})
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed during request")
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return errors.Wrap(err, "could not read response")
	}
	return errors.Wrap(resp.SuccessOrError(), "error in response")
}

func (c *client) WriteFile(ctx context.Context, opts options.WriteFile) error {
	sendOpts := func(opts options.WriteFile) error {
		req, err := shell.RequestToMessage(writeFileRequest{Options: opts})
		if err != nil {
			return errors.Wrap(err, "could not create request")
		}
		msg, err := c.doRequest(ctx, req)
		if err != nil {
			return errors.Wrap(err, "failed during request")
		}
		var resp shell.ErrorResponse
		if err := shell.MessageToResponse(msg, &resp); err != nil {
			return errors.Wrap(err, "could not read response")
		}
		return errors.Wrap(resp.SuccessOrError(), "error in response")
	}
	return opts.WriteBufferedContent(sendOpts)
}

// CloseConnection closes the client connection. Callers are expected to call
// this when finished with the client.
func (c *client) CloseConnection() error {
	return c.conn.Close()
}

func (c *client) ConfigureCache(ctx context.Context, opts options.Cache) error {
	req, err := shell.RequestToMessage(configureCacheRequest{Options: opts})
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed during request")
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return errors.Wrap(err, "could not read response")
	}
	return errors.Wrap(resp.SuccessOrError(), "error in response")
}

func (c *client) DownloadFile(ctx context.Context, opts options.Download) error {
	req, err := shell.RequestToMessage(downloadFileRequest{Options: opts})
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed during request")
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return errors.Wrap(err, "could not read response")
	}
	return errors.Wrap(resp.SuccessOrError(), "error in response")
}

func (c *client) DownloadMongoDB(ctx context.Context, opts options.MongoDBDownload) error {
	req, err := shell.RequestToMessage(downloadMongoDBRequest{Options: opts})
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed during request")
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return errors.Wrap(err, "could not read response")
	}
	return errors.Wrap(resp.SuccessOrError(), "error in response")
}

func (c *client) GetLogStream(ctx context.Context, id string, count int) (jasper.LogStream, error) {
	r := getLogStreamRequest{}
	r.Params.ID = id
	r.Params.Count = count
	req, err := shell.RequestToMessage(r)
	if err != nil {
		return jasper.LogStream{}, errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return jasper.LogStream{}, errors.Wrap(err, "failed during request")
	}
	var resp getLogStreamResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return jasper.LogStream{}, errors.Wrap(err, "could not read response)")
	}
	if err := resp.SuccessOrError(); err != nil {
		return jasper.LogStream{}, errors.Wrap(err, "error in response")
	}
	return resp.LogStream, nil
}

func (c *client) GetBuildloggerURLs(ctx context.Context, id string) ([]string, error) {
	req, err := shell.RequestToMessage(getBuildloggerURLsRequest{ID: id})
	if err != nil {
		return nil, errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return nil, errors.Wrap(err, "failed during request")
	}
	var resp getBuildloggerURLsResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return nil, errors.Wrap(err, "could not read response)")
	}
	if err := resp.SuccessOrError(); err != nil {
		return nil, errors.Wrap(err, "error in response")
	}
	return resp.URLs, nil
}

func (c *client) SignalEvent(ctx context.Context, name string) error {
	req, err := shell.RequestToMessage(signalEventRequest{Name: name})
	if err != nil {
		return errors.Wrap(err, "could not create request")
	}
	msg, err := c.doRequest(ctx, req)
	if err != nil {
		return errors.Wrap(err, "failed during request")
	}
	var resp shell.ErrorResponse
	if err := shell.MessageToResponse(msg, &resp); err != nil {
		return errors.Wrap(err, "could not read response")
	}
	return errors.Wrap(resp.SuccessOrError(), "error in response")
}

// doRequest sends the given request and reads the response.
func (c *client) doRequest(ctx context.Context, req mongowire.Message) (mongowire.Message, error) {
	ctx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	if err := mongowire.SendMessage(ctx, req, c.conn); err != nil {
		return nil, errors.Wrap(err, "problem sending request")
	}
	msg, err := mongowire.ReadMessage(ctx, c.conn)
	if err != nil {
		return nil, errors.Wrap(err, "error in response")
	}
	return msg, nil
}
