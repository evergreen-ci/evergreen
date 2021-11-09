package systemmetrics

import (
	"context"
	"fmt"
	"io"
	"time"

	"github.com/evergreen-ci/juniper/gopb"
	"github.com/evergreen-ci/timber"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

const (
	defaultMaxBufferSize int = 1e7
	defaultFlushInterval     = time.Minute
)

// SystemMetricsClient provides a wrapper around the gRPC client for sending
// system metrics data to cedar.
type SystemMetricsClient struct {
	client     gopb.CedarSystemMetricsClient
	clientConn *grpc.ClientConn
}

// NewSystemMetricsClient returns a SystemMetricsClient to send system metrics
// data to cedar. If authentication credentials (username and apikey) are not
// specified, then an insecure connection will be established with the
// specified address and port.
func NewSystemMetricsClient(ctx context.Context, opts timber.ConnectionOptions) (*SystemMetricsClient, error) {
	var conn *grpc.ClientConn
	var err error

	err = opts.Validate()
	if err != nil {
		return nil, errors.Wrap(err, "invalid connection options")
	}

	if opts.DialOpts.APIKey == "" {
		addr := fmt.Sprintf("%s:%s", opts.DialOpts.BaseAddress, opts.DialOpts.RPCPort)
		conn, err = grpc.DialContext(ctx, addr, grpc.WithInsecure())
	} else {
		conn, err = timber.DialCedar(ctx, &opts.Client, opts.DialOpts)
	}
	if err != nil {
		return nil, errors.Wrap(err, "problem dialing rpc server")
	}

	s := &SystemMetricsClient{
		client:     gopb.NewCedarSystemMetricsClient(conn),
		clientConn: conn,
	}
	return s, nil
}

// NewSystemMetricsClientWithExistingConnection returns a SystemMetricsClient
// to send system metrics data to cedar, using the provided client connection.
func NewSystemMetricsClientWithExistingConnection(ctx context.Context, clientConn *grpc.ClientConn) (*SystemMetricsClient, error) {
	if clientConn == nil {
		return nil, errors.New("must provide existing client connection")
	}

	s := &SystemMetricsClient{
		client: gopb.NewCedarSystemMetricsClient(clientConn),
	}
	return s, nil
}

// CreateSystemMetricsRecord creates a system metrics metadata object in cedar
// with the provided info, along with setting the created_at timestamp.
func (s *SystemMetricsClient) CreateSystemMetricsRecord(ctx context.Context, opts SystemMetricsOptions) (string, error) {
	if err := opts.Compression.validate(); err != nil {
		return "", err
	}
	if err := opts.Schema.validate(); err != nil {
		return "", err
	}

	resp, err := s.client.CreateSystemMetricsRecord(ctx, createSystemMetrics(opts))
	if err != nil {
		return "", errors.Wrap(err, "problem creating system metrics object")
	}

	return resp.Id, nil
}

// AddSystemMetrics sends the given byte slice to the cedar backend for the
// system metrics object with the corresponding id.
func (s *SystemMetricsClient) AddSystemMetrics(ctx context.Context, opts MetricDataOptions, data []byte) error {
	if err := opts.validate(); err != nil {
		return errors.Wrapf(err, "invalid system metrics options")
	}

	if len(data) == 0 {
		return errors.New("must provide data to send")
	}

	_, err := s.client.AddSystemMetrics(ctx, &gopb.SystemMetricsData{
		Id:     opts.Id,
		Type:   opts.MetricType,
		Format: gopb.DataFormat(opts.Format),
		Data:   data,
	})
	return err
}

// NewSystemMetricsWriteCloser returns a buffered WriteCloser that can be used
// to send system metrics data to cedar.
func (s *SystemMetricsClient) NewSystemMetricsWriteCloser(ctx context.Context, dataOpts MetricDataOptions, writerOpts WriteCloserOptions) (io.WriteCloser, error) {
	if err := dataOpts.validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid system metrics data options")
	}
	if err := writerOpts.validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid write closer options for id %s", dataOpts.Id)
	}

	ctx, cancel := context.WithCancel(ctx)
	w := &systemMetricsWriteCloser{
		ctx:           ctx,
		cancel:        cancel,
		catcher:       grip.NewBasicCatcher(),
		client:        s,
		opts:          dataOpts,
		buffer:        []byte{},
		maxBufferSize: writerOpts.MaxBufferSize,
	}

	if !writerOpts.NoTimedFlush {
		w.lastFlush = time.Now()
		w.flushInterval = writerOpts.FlushInterval
		go w.timedFlush()
	}
	return w, nil
}

// CloseMetrics will add the completed_at timestamp to the system metrics
// object in cedar with the corresponding id, as well as the success boolean.
func (s *SystemMetricsClient) CloseSystemMetrics(ctx context.Context, id string, success bool) error {
	if id == "" {
		return errors.New("must specify id of system metrics object")
	}

	endInfo := &gopb.SystemMetricsSeriesEnd{
		Id:      id,
		Success: success,
	}
	_, err := s.client.CloseMetrics(ctx, endInfo)
	return err
}

// CloseClient closes out the client connection if one was created by
// NewSystemMetricsClient. A provided client connection will not be closed.
func (s *SystemMetricsClient) CloseClient() error {
	if s.clientConn == nil {
		return nil
	}
	return s.clientConn.Close()
}

func createSystemMetrics(opts SystemMetricsOptions) *gopb.SystemMetrics {
	return &gopb.SystemMetrics{
		Info: &gopb.SystemMetricsInfo{
			Project:   opts.Project,
			Version:   opts.Version,
			Variant:   opts.Variant,
			TaskName:  opts.TaskName,
			TaskId:    opts.TaskId,
			Execution: opts.Execution,
			Mainline:  opts.Mainline,
		},
		Artifact: &gopb.SystemMetricsArtifactInfo{
			Compression: gopb.CompressionType(opts.Compression),
			Schema:      gopb.SchemaType(opts.Schema),
		},
	}
}
