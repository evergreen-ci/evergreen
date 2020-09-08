package systemmetrics

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/internal"
	"github.com/mongodb/grip"
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

// DataFormat describes how the time series data is stored.
type DataFormat int32

// Valid DataFormat values.
const (
	DataFormatText DataFormat = 0
	DataFormatFTDC DataFormat = 1
	DataFormatBSON DataFormat = 2
	DataFormatJSON DataFormat = 3
	DataFormatCSV  DataFormat = 4
)

func (f DataFormat) validate() error {
	switch f {
	case DataFormatText, DataFormatFTDC, DataFormatBSON, DataFormatJSON, DataFormatCSV:
		return nil
	default:
		return errors.New("invalid schema type specified")
	}
}

// SystemMetricsClient provides a wrapper around the grpc client for sending
// system metrics data to cedar.
type SystemMetricsClient struct {
	client     internal.CedarSystemMetricsClient
	clientConn *grpc.ClientConn
}

// ConnectionOptions contains the options needed to create a grpc connection
// with cedar.
type ConnectionOptions struct {
	DialOpts timber.DialCedarOptions
	Client   http.Client
}

func (opts ConnectionOptions) validate() error {
	hasAuth := opts.DialOpts.Username != "" && (opts.DialOpts.APIKey != "" || opts.DialOpts.Password != "")
	if (opts.DialOpts.BaseAddress == "" && opts.DialOpts.RPCPort != "") ||
		(opts.DialOpts.BaseAddress != "" && opts.DialOpts.RPCPort == "") {
		return errors.New("must provide both base address and rpc port or neither")
	}
	if !hasAuth && opts.DialOpts.BaseAddress == "" {
		return errors.New("must specify username and api key/password, or address and port for an insecure connection")
	}
	return nil
}

// NewSystemMetricsClient returns a SystemMetricsClient to send system metrics
// data to cedar. If authentication credentials (username and apikey) are not
// specified, then an insecure connection will be established with the
// specified address and port.
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

// NewSystemMetricsClientWithExistingConnection returns a SystemMetricsClient
// to send system metrics data to cedar, using the provided client connection.
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

// MetricDataOptions describes the data being streamed or sent to cedar.
type MetricDataOptions struct {
	Id         string
	MetricType string
	Format     DataFormat
}

func (s *MetricDataOptions) validate() error {
	if s.Id == "" {
		return errors.New("must specify id of system metrics object")
	}
	if s.MetricType == "" {
		return errors.New("must specify the the type of metric in data")
	}
	if err := s.Format.validate(); err != nil {
		return errors.Wrapf(err, "invalid format for id %s", s.Id)
	}
	return nil
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

	_, err := s.client.AddSystemMetrics(ctx, &internal.SystemMetricsData{
		Id:     opts.Id,
		Type:   opts.MetricType,
		Format: internal.DataFormat(opts.Format),
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

	endInfo := &internal.SystemMetricsSeriesEnd{
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
