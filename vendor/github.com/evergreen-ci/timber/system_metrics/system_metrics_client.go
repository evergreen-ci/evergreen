package systemmetrics

import (
	"context"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/internal"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

const (
	defaultMaxBufferSize int = 1e7
	defaultFlushInterval     = time.Minute
)

// CompressionType describes how the system metrics data is compressed.
type CompressionType int32

// Valid CompressionType values.
const (
	CompressionTypeNone  CompressionType = 0
	CompressionTypeTARGZ CompressionType = 1
	CompressionTypeZIP   CompressionType = 2
	CompressionTypeGZ    CompressionType = 3
	CompressionTypeXZ    CompressionType = 4
)

func (f CompressionType) validate() error {
	switch f {
	case CompressionTypeNone, CompressionTypeTARGZ, CompressionTypeZIP, CompressionTypeGZ, CompressionTypeXZ:
		return nil
	default:
		return errors.New("invalid compression type specified")
	}
}

// SchemaType describes how the time series data is stored.
type SchemaType int32

// Valid SchemaType values.
const (
	SchemaTypeRawEvents             SchemaType = 0
	SchemaTypeCollapsedEvents       SchemaType = 1
	SchemaTypeIntervalSummarization SchemaType = 2
	SchemaTypeHistogram             SchemaType = 3
)

func (f SchemaType) validate() error {
	switch f {
	case SchemaTypeRawEvents, SchemaTypeCollapsedEvents, SchemaTypeIntervalSummarization, SchemaTypeHistogram:
		return nil
	default:
		return errors.New("invalid schema type specified")
	}
}

// SystemMetricsClient provides a wrapper around the grpc client for sending system
// metrics data to cedar.
type SystemMetricsClient struct {
	client     internal.CedarSystemMetricsClient
	clientConn *grpc.ClientConn
}

// ConnectionOptions contains the options needed to create a grpc connection with cedar.
type ConnectionOptions struct {
	DialOpts timber.DialCedarOptions
	Client   http.Client
}

func (opts ConnectionOptions) validate() error {
	if (opts.DialOpts.APIKey == "" && opts.DialOpts.Username != "") ||
		(opts.DialOpts.APIKey != "" && opts.DialOpts.Username == "") {
		return errors.New("must provide both username and api key or neither")
	}
	if (opts.DialOpts.BaseAddress == "" && opts.DialOpts.RPCPort != "") ||
		(opts.DialOpts.BaseAddress != "" && opts.DialOpts.RPCPort == "") {
		return errors.New("must provide both base address and rpc port or neither")
	}
	if opts.DialOpts.APIKey == "" && opts.DialOpts.BaseAddress == "" {
		return errors.New("must specify username and api key, or address and port for an insecure connection")
	}
	return nil
}

// NewSystemMetricsClient returns a SystemMetricsClient to send system metrics data to
// cedar. If authentication credentials (username and apikey) are not specified,
// then an insecure connection will be established with the specified address
// and port.
func NewSystemMetricsClient(ctx context.Context, opts ConnectionOptions) (*SystemMetricsClient, error) {
	var conn *grpc.ClientConn
	var err error

	err = opts.validate()
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
		client:     internal.NewCedarSystemMetricsClient(conn),
		clientConn: conn,
	}
	return s, nil
}

// NewSystemMetricsClientWithExistingConnection returns a SystemMetricsClient to send
// system metrics data to cedar, using the provided client connection.
func NewSystemMetricsClientWithExistingConnection(ctx context.Context, clientConn *grpc.ClientConn) (*SystemMetricsClient, error) {
	if clientConn == nil {
		return nil, errors.New("Must provide existing client connection")
	}

	s := &SystemMetricsClient{
		client: internal.NewCedarSystemMetricsClient(clientConn),
	}
	return s, nil
}

// SystemMetricsOptions support the use and creation of a system metrics object.
type SystemMetricsOptions struct {
	// Unique information to identify the system metrics object.
	Project   string `bson:"project" json:"project" yaml:"project"`
	Version   string `bson:"version" json:"version" yaml:"version"`
	Variant   string `bson:"variant" json:"variant" yaml:"variant"`
	TaskName  string `bson:"task_name" json:"task_name" yaml:"task_name"`
	TaskId    string `bson:"task_id" json:"task_id" yaml:"task_id"`
	Execution int32  `bson:"execution" json:"execution" yaml:"execution"`
	Mainline  bool   `bson:"mainline" json:"mainline" yaml:"mainline"`

	// Data storage information for this object
	Compression CompressionType `bson:"compression" json:"compression" yaml:"compression"`
	Schema      SchemaType      `bson:"schema" json:"schema" yaml:"schema"`
}

// CreateSystemMetrics creates a system metrics metadata object in cedar with
// the provided info, along with setting the created_at timestamp.
func (s *SystemMetricsClient) CreateSystemMetricRecord(ctx context.Context, opts SystemMetricsOptions) (string, error) {
	if err := opts.Compression.validate(); err != nil {
		return "", err
	}
	if err := opts.Schema.validate(); err != nil {
		return "", err
	}

	resp, err := s.client.CreateSystemMetricRecord(ctx, createSystemMetrics(opts))
	if err != nil {
		return "", errors.Wrap(err, "problem creating system metrics object")
	}

	return resp.Id, nil
}

// AddSystemMetricsData sends the given byte slice to the cedar backend for the
// system metrics object with the corresponding id.
func (s *SystemMetricsClient) AddSystemMetrics(ctx context.Context, id, metricType string, data []byte) error {
	if id == "" {
		return errors.New("must specify id of system metrics object")
	}
	if metricType == "" {
		return errors.New("must specify the the type of metric in data")
	}
	if len(data) == 0 {
		return errors.New("must provide data to send")
	}

	_, err := s.client.AddSystemMetrics(ctx, &internal.SystemMetricsData{
		Id:   id,
		Type: metricType,
		Data: data,
	})
	return err
}

// SystemMetricsWriteCloser is a wrapper around a stream of system metrics data
// that implements buffering with timed flushes and the WriteCloser interface.
type SystemMetricsWriteCloser struct {
	mu            sync.Mutex
	ctx           context.Context
	cancel        context.CancelFunc
	catcher       grip.Catcher
	id            string
	metricType    string
	stream        internal.CedarSystemMetrics_StreamSystemMetricsClient
	buffer        []byte
	maxBufferSize int
	lastFlush     time.Time
	timer         *time.Timer
	flushInterval time.Duration
	closed        bool
}

// Write adds provided data to the buffer, flushing it if it exceeds its max
// size. Write will fail if the writer was previously closed or if a timed flush
// encountered an error.
func (s *SystemMetricsWriteCloser) Write(data []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.catcher.HasErrors() {
		return 0, errors.Wrapf(s.catcher.Resolve(), "writer already closed due to error")
	}
	if s.closed {
		return 0, errors.New("writer already closed")
	}
	if len(data) == 0 {
		return 0, errors.New("must provide data to write")
	}

	s.buffer = append(s.buffer, data...)
	if len(s.buffer) > s.maxBufferSize {
		if err := s.flush(); err != nil {
			s.catcher.Add(err)
			s.catcher.Add(s.close())
			return 0, errors.Wrapf(s.catcher.Resolve(), "problem writing data")
		}
	}
	return len(data), nil
}

// Close flushes any remaining data in the buffer and closes the stream. If
// the writer was previously closed, this returns an error.
func (s *SystemMetricsWriteCloser) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.catcher.HasErrors() {
		return errors.Wrapf(s.catcher.Resolve(), "writer already closed due to error")
	}
	if s.closed {
		return errors.New("writer already closed")
	}

	s.catcher.Add(s.flush())
	s.catcher.Add(s.close())
	return s.catcher.Resolve()
}

func (s *SystemMetricsWriteCloser) flush() error {
	data := &internal.SystemMetricsData{
		Id:   s.id,
		Type: s.metricType,
		Data: s.buffer,
	}

	if err := s.stream.Send(data); err != nil {
		return errors.Wrapf(err, "problem sending data for id %s", s.id)
	}

	s.buffer = []byte{}
	s.lastFlush = time.Now()
	return nil
}

func (s *SystemMetricsWriteCloser) close() error {
	s.cancel()
	s.closed = true
	_, err := s.stream.CloseAndRecv()
	return err
}

func (s *SystemMetricsWriteCloser) timedFlush() {
	defer func() {
		message := "panic in systemMetrics timedFlush"
		grip.Error(message)
		s.catcher.Add(recovery.HandlePanicWithError(recover(), nil, message))
	}()
	s.mu.Lock()
	s.timer = time.NewTimer(s.flushInterval)
	s.mu.Unlock()
	defer s.timer.Stop()

	for {
		select {
		case <-s.ctx.Done():
			return
		case <-s.timer.C:
			func() {
				s.mu.Lock()
				defer s.mu.Unlock()
				if len(s.buffer) > 0 && time.Since(s.lastFlush) >= s.flushInterval {
					if err := s.flush(); err != nil {
						s.catcher.Add(errors.Wrapf(err, "problem with timed flush for id %s", s.id))
						s.catcher.Add(s.close())
					}
				}
				_ = s.timer.Reset(s.flushInterval)
			}()
		}
	}
}

// StreamOpts allow maximum buffer size and the maximum time between flushes to
// be specified. If NoTimedFlush is true, then the timed flush will not occur.
type StreamOpts struct {
	FlushInterval time.Duration
	NoTimedFlush  bool
	MaxBufferSize int
}

func (s *StreamOpts) validate() error {
	if s.FlushInterval < 0 {
		return errors.New("flush interval must not be negative")
	}
	if s.FlushInterval == 0 {
		s.FlushInterval = defaultFlushInterval
	}

	if s.MaxBufferSize < 0 {
		return errors.New("buffer size must not be negative")
	}
	if s.MaxBufferSize == 0 {
		s.MaxBufferSize = defaultMaxBufferSize
	}

	return nil
}

// StreamSystemMetrics returns a buffered WriteCloser that can be used to stream system
// metrics data to cedar.
func (s *SystemMetricsClient) StreamSystemMetrics(ctx context.Context, id, metricType string, opts StreamOpts) (*SystemMetricsWriteCloser, error) {
	if id == "" {
		return nil, errors.New("must specify id of system metrics object")
	}
	if metricType == "" {
		return nil, errors.New("must specify type of metric data")
	}

	if err := opts.validate(); err != nil {
		return nil, errors.Wrapf(err, "invalid stream options for id %s", id)
	}

	stream, err := s.client.StreamSystemMetrics(ctx)
	if err != nil {
		return nil, errors.Wrapf(err, "problem starting stream for %s", id)
	}

	ctx, cancel := context.WithCancel(ctx)
	writer := &SystemMetricsWriteCloser{
		ctx:           ctx,
		cancel:        cancel,
		catcher:       grip.NewBasicCatcher(),
		id:            id,
		metricType:    metricType,
		stream:        stream,
		buffer:        []byte{},
		maxBufferSize: opts.MaxBufferSize,
	}

	if !opts.NoTimedFlush {
		writer.lastFlush = time.Now()
		writer.flushInterval = opts.FlushInterval
		go writer.timedFlush()
	}
	return writer, nil
}

// CloseMetrics will add the completed_at timestamp to the system metrics object
// in cedar with the corresponding id.
func (s *SystemMetricsClient) CloseSystemMetrics(ctx context.Context, id string) error {
	if id == "" {
		return errors.New("must specify id of system metrics object")
	}

	endInfo := &internal.SystemMetricsSeriesEnd{
		Id: id,
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

func createSystemMetrics(opts SystemMetricsOptions) *internal.SystemMetrics {
	return &internal.SystemMetrics{
		Info: &internal.SystemMetricsInfo{
			Project:   opts.Project,
			Version:   opts.Version,
			Variant:   opts.Variant,
			TaskName:  opts.TaskName,
			TaskId:    opts.TaskId,
			Execution: opts.Execution,
			Mainline:  opts.Mainline,
		},
		Artifact: &internal.SystemMetricsArtifactInfo{
			Compression: internal.CompressionType(opts.Compression),
			Schema:      internal.SchemaType(opts.Schema),
		},
	}
}
