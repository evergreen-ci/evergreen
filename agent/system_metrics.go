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
	"github.com/mongodb/ftdc"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// MetricCollector is an interface representing an object that can collect
// a single system metric at a series of time steps. Name returns a string
// indicating the type of metric collected, such as "uptime". Metadata returns
// a document representing the metadata for a time series, but can optionally
// be set to nil. Collect returns a document of the metric value at a given
// time. The document format for Metadata and Collect should be usable by
// the methods of https://godoc.org/github.com/mongodb/ftdc#Collector.
type MetricCollector interface {
	Name() string
	Metadata() interface{}
	Collect() (interface{}, error)
}

// SystemMetricsCollector handles collecting an arbitrary set of system
// metrics at a fixed interval and streaming them to cedar.
type SystemMetricsCollector struct {
	mu              sync.Mutex
	wg              sync.WaitGroup
	streamingCancel context.CancelFunc
	taskOpts        *metrics.SystemMetricsOptions
	interval        time.Duration
	collectors      []MetricCollector
	id              string
	client          *metrics.SystemMetricsClient
	catcher         grip.Catcher
	closed          bool
}

// NewSystemMetricsCollector returns a SystemMetricsCollector ready to start
// collecting from the provided set of MetricCollectors at the provided interval
// and streaming to cedar using the connection defaults from the provided communicator.
// The task is used to set the cedar metadata of for the collected metrics.
func NewSystemMetricsCollector(ctx context.Context, interval time.Duration, t *task.Task,
	collectors []MetricCollector, c client.Communicator) (*SystemMetricsCollector, error) {
	s, err := systemMetricsCollectorSetup(ctx, interval, t, collectors)
	if err != nil {
		return nil, err
	}

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
	connOpts := metrics.ConnectionOptions{
		DialOpts: dialOpts,
	}
	mc, err := metrics.NewSystemMetricsClient(ctx, connOpts)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating new system metrics client")
	}
	s.client = mc
	return s, nil
}

// NewSystemMetricsCollectorWithClientConn returns a SystemMetricsCollector ready
// to start collecting from the provided set of MetricCollectors at the provided
// interval and streaming to cedar using the provided client connection. The task
// is used to set the cedar metadata of for the collected metrics.
func NewSystemMetricsCollectorWithClientConn(ctx context.Context, interval time.Duration, t *task.Task,
	collectors []MetricCollector, conn *grpc.ClientConn) (*SystemMetricsCollector, error) {
	s, err := systemMetricsCollectorSetup(ctx, interval, t, collectors)
	if err != nil {
		return nil, err
	}

	mc, err := metrics.NewSystemMetricsClientWithExistingConnection(ctx, conn)
	if err != nil {
		return nil, errors.Wrap(err, "problem creating new system metrics client")
	}
	s.client = mc
	return s, nil
}

func systemMetricsCollectorSetup(ctx context.Context, interval time.Duration, t *task.Task,
	collectors []MetricCollector) (*SystemMetricsCollector, error) {
	if interval < 0 {
		return nil, errors.New("interval cannot be negative")
	}
	if interval == 0 {
		interval = time.Minute
	}

	taskOpts, err := getTaskInfo(t)
	if err != nil {
		return nil, errors.Wrap(err, "incomplete task metadata")
	}

	if len(collectors) == 0 {
		return nil, errors.New("must provide at least one MetricCollector")
	}

	return &SystemMetricsCollector{
		interval:   interval,
		collectors: collectors,
		taskOpts:   taskOpts,
		catcher:    grip.NewBasicCatcher(),
	}, nil
}

// Start commences collecting metrics using each of the provided MetricCollectors.
// Regardless of if Start returns an error, Close should still be called
// to close any opened connections, set the exit code, and handle any returned
// errors. This can also be handled by cancelling the provided context, but
// any errors will only be logged to the global error logs in this case.
func (s *SystemMetricsCollector) Start(ctx context.Context) error {
	var err error
	s.id, err = s.client.CreateSystemMetricRecord(ctx, *s.taskOpts)
	if err != nil {
		return errors.Wrap(err, "problem creating cedar system metrics metadata object")
	}

	var streamingCtx context.Context
	streamingCtx, s.streamingCancel = context.WithCancel(ctx)

	for _, collector := range s.collectors {
		stream, err := s.client.StreamSystemMetrics(streamingCtx, s.id, collector.Name(), metrics.StreamOpts{})
		if err != nil {
			s.streamingCancel()
			s.streamingCancel = nil
			return errors.Wrap(err, fmt.Sprintf("problem creating system metrics stream for id %s and metricType %s", s.id, collector.Name()))
		}

		ftdcCollector := ftdc.NewStreamingCollector(1, stream)
		err = ftdcCollector.SetMetadata(collector.Metadata())
		if err != nil {
			s.streamingCancel()
			s.streamingCancel = nil
			return errors.Wrap(err, fmt.Sprintf("problem setting metadata on stream for id %s and metricType %s", s.id, collector.Name()))
		}

		s.wg.Add(1)
		go s.timedCollect(streamingCtx, collector, stream, ftdcCollector)
	}
	go s.closeOnCancel(ctx, streamingCtx)

	return nil
}

func (s *SystemMetricsCollector) timedCollect(ctx context.Context, mc MetricCollector, stream *metrics.SystemMetricsWriteCloser, collector ftdc.Collector) {
	defer s.wg.Done()

	timer := time.NewTimer(0)
	defer timer.Stop()
	defer s.catcher.Add(errors.Wrap(stream.Close(), fmt.Sprintf("problem closing system metrics stream for id %s and metricType %s", s.id, mc.Name())))
	defer s.catcher.Add(errors.Wrap(ftdc.FlushCollector(collector, stream), fmt.Sprintf("problem flushing system metrics data collector for id %s and metricType %s", s.id, mc.Name())))
	defer s.catcher.Add(errors.Wrap(recovery.HandlePanicWithError(recover(), nil, fmt.Sprintf("panic in system metrics stream for id %s and metricType %s", s.id, mc.Name()), "")))

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			data, err := mc.Collect()
			if err != nil {
				s.catcher.Add(errors.Wrap(err, "problem collecting system metrics data for id %s and metricType %s", s.id, mc.Name()))
				return
			}
			err = collector.Add(data)
			if err != nil {
				s.catcher.Add(errors.Wrap(err, "problem adding system metrics data to collector for id %s and metricType %s", s.id, mc.Name()))
				return
			}
			_ = timer.Reset(s.interval)
		}
	}
}

func (s *SystemMetricsCollector) closeOnCancel(outerCtx, streamingCtx context.Context) {
	for {
		select {
		case <-outerCtx.Done():
			grip.Error(s.cleanup())
			return
		case <-streamingCtx.Done():
			return
		}
	}
}

// Close cleans up any remaining connections and closes out the cedar
// metadata if one was created with the completed_at timestamp and an
// exit code. Close needs to be called before the context provided to
// Start is cancelled.
func (s *SystemMetricsCollector) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("system metrics collector already cancelled or closed")
	}
	s.mu.Unlock()

	return s.cleanup()
}

func (s *SystemMetricsCollector) cleanup() error {
	if s.streamingCancel != nil {
		s.streamingCancel()
		s.wg.Wait()
	}
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.client != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		if s.id != "" {
			s.catcher.Add(errors.Wrap(s.client.CloseSystemMetrics(ctx, s.id), fmt.Sprintf("error closing out system metrics object for id %s", s.id)))
		}
		s.catcher.Add(errors.Wrap(s.client.CloseClient(), fmt.Sprintf("error closing system metrics client for id %s", s.id)))
	}
	s.closed = true

	return s.catcher.Resolve()
}

func getTaskInfo(t *task.Task) (*metrics.SystemMetricsOptions, error) {
	opts := &metrics.SystemMetricsOptions{
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
	return opts, nil
}
