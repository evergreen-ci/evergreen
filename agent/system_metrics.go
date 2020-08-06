package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/timber"
	metrics "github.com/evergreen-ci/timber/system_metrics"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// dataFormat describes how the time series data is stored.
type dataFormat int32

// Valid dataFormat values.
const (
	dataFormatText dataFormat = 0
	dataFormatFTDC dataFormat = 1
	dataFormatBSON dataFormat = 2
	dataFormatJSON dataFormat = 3
	dataFormatCSV  dataFormat = 4
)

func (f dataFormat) validate() error {
	switch f {
	case dataFormatText, dataFormatFTDC, dataFormatBSON, dataFormatJSON, dataFormatCSV:
		return nil
	default:
		return errors.New("invalid schema type specified")
	}
}

// metricCollector is an interface representing an object that can collect
// a single system metric over a series of time steps.
type metricCollector interface {
	// Name returns a string indicating the type of metric collected, such as "uptime".
	Name() string
	// Format returns the format of the collected data.
	Format() dataFormat
	// Collect returns the value of the collected metric when the function is called.
	Collect() ([]byte, error)
}

// systemMetricsCollector handles collecting an arbitrary set of system
// metrics at a fixed interval and streaming them to cedar.
type systemMetricsCollector struct {
	mu              sync.Mutex
	stream          sync.WaitGroup
	close           sync.WaitGroup
	streamingCancel context.CancelFunc
	taskOpts        *metrics.SystemMetricsOptions
	streamOpts      *metrics.StreamOpts
	interval        time.Duration
	collectors      []metricCollector
	client          *metrics.SystemMetricsClient
	catcher         grip.Catcher
	id              string
	closed          bool
}

// systemMetricsCollectorOptions are the required values for creating a new
// systemMetricsCollector. At least one of DialOpts or Conn should be set.
type systemMetricsCollectorOptions struct {
	Task       *task.Task
	Interval   time.Duration
	Collectors []metricCollector
	DialOpts   *timber.DialCedarOptions
	Conn       *grpc.ClientConn

	// These options configure the timber stream buffer, and can be left empty
	// to use the default settings.
	MaxBufferSize            int
	NoBufferTimedFlush       bool
	BufferTimedFlushInterval time.Duration
}

func (s *systemMetricsCollectorOptions) validate() error {
	if s.Task == nil {
		return errors.New("must provide a valid task")
	}

	if s.Interval < 0 {
		return errors.New("interval cannot be negative")
	}
	if s.Interval == 0 {
		s.Interval = time.Minute
	}

	if len(s.Collectors) == 0 {
		return errors.New("must provide at least one metric collector")
	}

	if s.DialOpts == nil && s.Conn == nil {
		return errors.New("must provide either options to dial cedar or an existing client connection")
	}

	if s.BufferTimedFlushInterval < 0 {
		return errors.New("flush interval must not be negative")
	}

	if s.MaxBufferSize < 0 {
		return errors.New("buffer size must not be negative")
	}

	return nil
}

// newSystemMetricsCollector returns a systemMetricsCollector ready to start
// collecting from the provided slice of metric collector objects at the
// provided interval and streaming to cedar.
func newSystemMetricsCollector(ctx context.Context, opts *systemMetricsCollectorOptions) (*systemMetricsCollector, error) {
	err := opts.validate()
	if err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}
	s := &systemMetricsCollector{
		interval:   opts.Interval,
		collectors: opts.Collectors,
		taskOpts:   getSystemMetricsInfo(opts.Task),
		streamOpts: &metrics.StreamOpts{
			FlushInterval: opts.BufferTimedFlushInterval,
			NoTimedFlush:  opts.NoBufferTimedFlush,
			MaxBufferSize: opts.MaxBufferSize,
		},
		catcher: grip.NewBasicCatcher(),
	}
	if opts.Conn != nil {
		s.client, err = metrics.NewSystemMetricsClientWithExistingConnection(ctx, opts.Conn)
	} else {
		s.client, err = dialCedar(ctx, opts.DialOpts)
	}
	if err != nil {
		return nil, errors.Wrap(err, "problem creating new system metrics client")
	}
	return s, nil
}

// Start commences collecting metrics using each of the provided MetricCollector
// objects. Regardless of if Start returns an error, Close should still be called
// to close any opened connections, set the exit code, and handle any returned
// errors. This can also be handled by cancelling the provided context, but
// any errors will only be logged to the global error logs in this case.
func (s *systemMetricsCollector) Start(ctx context.Context) error {
	var err error
	s.id, err = s.client.CreateSystemMetricsRecord(ctx, *s.taskOpts)
	if err != nil {
		return errors.Wrap(err, "problem creating cedar system metrics metadata object")
	}

	var streamingCtx context.Context
	streamingCtx, s.streamingCancel = context.WithCancel(ctx)

	for _, collector := range s.collectors {
		stream, err := s.client.StreamSystemMetrics(ctx, metrics.MetricDataOpts{
			Id:         s.id,
			MetricType: collector.Name(),
			Format:     metrics.DataFormat(collector.Format()),
		}, *s.streamOpts)
		if err != nil {
			s.streamingCancel()
			s.streamingCancel = nil
			return errors.Wrap(err, fmt.Sprintf("problem creating system metrics stream for id %s and metricType %s", s.id, collector.Name()))
		}

		s.stream.Add(1)
		go s.timedCollect(streamingCtx, collector, stream)
	}
	s.close.Add(1)
	go s.closeOnCancel(ctx, streamingCtx)

	return nil
}

func (s *systemMetricsCollector) timedCollect(ctx context.Context, mc metricCollector, stream *metrics.SystemMetricsWriteCloser) {
	timer := time.NewTimer(0)
	defer func() {
		s.catcher.Add(errors.Wrap(recovery.HandlePanicWithError(recover(), nil, fmt.Sprintf("panic in system metrics stream for id %s and metricType %s", s.id, mc.Name())), ""))
		s.catcher.Add(errors.Wrap(stream.Close(), fmt.Sprintf("problem closing system metrics stream for id %s and metricType %s", s.id, mc.Name())))
		timer.Stop()
		s.stream.Done()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			data, err := mc.Collect()
			if err != nil {
				s.catcher.Add(errors.Wrapf(err, "problem collecting system metrics data for id %s and metricType %s", s.id, mc.Name()))
				return
			}
			_, err = stream.Write(data)
			if err != nil {
				s.catcher.Add(errors.Wrapf(err, "problem writing system metrics data to stream for id %s and metricType %s", s.id, mc.Name()))
				return
			}
			_ = timer.Reset(s.interval)
		}
	}
}

func (s *systemMetricsCollector) closeOnCancel(outerCtx, streamingCtx context.Context) {
	defer s.close.Done()
	for {
		select {
		case <-outerCtx.Done():
			if s.streamingCancel != nil {
				s.streamingCancel()
			}
			s.cleanup()
			grip.Error(s.catcher.Resolve())
			return
		case <-streamingCtx.Done():
			s.cleanup()
			return
		}
	}
}

func (s *systemMetricsCollector) cleanup() {
	s.stream.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if s.id != "" {
			s.catcher.Add(errors.Wrap(s.client.CloseSystemMetrics(ctx, s.id, s.catcher.HasErrors()), fmt.Sprintf("error closing out system metrics object for id %s", s.id)))
		}
		s.catcher.Add(errors.Wrap(s.client.CloseClient(), fmt.Sprintf("error closing system metrics client for id %s", s.id)))
	}
	s.closed = true
}

// Close cleans up any remaining connections and closes out the cedar
// metadata if one was created with the completed_at timestamp and an
// exit code.
func (s *systemMetricsCollector) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("system metrics collector already cancelled or closed")
	}
	s.mu.Unlock()

	if s.streamingCancel != nil {
		s.streamingCancel()
	}
	s.close.Wait()
	return s.catcher.Resolve()
}

func getSystemMetricsInfo(t *task.Task) *metrics.SystemMetricsOptions {
	return &metrics.SystemMetricsOptions{
		Project:     t.Project,
		Version:     t.Version,
		Variant:     t.BuildVariant,
		TaskName:    t.DisplayName,
		TaskId:      t.Id,
		Execution:   int32(t.Execution),
		Mainline:    !t.IsPatchRequest(),
		Compression: metrics.CompressionTypeNone,
		Schema:      metrics.SchemaTypeRawEvents,
	}
}

func getDialOpts(ctx context.Context, c client.Communicator) (*timber.DialCedarOptions, error) {
	bi, err := c.GetBuildloggerInfo(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "error getting cedar dial options")
	}
	dialOpts := timber.DialCedarOptions{
		BaseAddress: bi.BaseURL,
		RPCPort:     bi.RPCPort,
		Username:    bi.Username,
		Password:    bi.Password,
		APIKey:      bi.APIKey,
		Retries:     10,
	}
	return &dialOpts, nil
}

func dialCedar(ctx context.Context, dialOpts *timber.DialCedarOptions) (*metrics.SystemMetricsClient, error) {
	connOpts := metrics.ConnectionOptions{
		DialOpts: *dialOpts,
	}
	mc, err := metrics.NewSystemMetricsClient(ctx, connOpts)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating new system metrics client")
	}
	return mc, nil
}
