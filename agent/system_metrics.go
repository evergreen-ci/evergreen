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
)

type MetricCollector interface {
	Name() string
	Metadata() interface{}
	Collect() (interface{}, error)
}

type SystemMetricsCollector struct {
	mu              sync.Mutex
	wg              sync.WaitGroup
	streamingCancel context.CancelFunc
	taskOpts        *metrics.SystemMetricsOptions
	connOpts        metrics.ConnectionOptions
	interval        time.Duration
	collectors      []MetricCollector
	id              string
	client          *metrics.SystemMetricsClient
	catcher         grip.Catcher
	closed          bool
}

func NewSystemMetricsCollector(ctx context.Context, interval time.Duration, c client.Communicator,
	t *task.Task, collectors []MetricCollector) (*SystemMetricsCollector, error) {

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

	return &SystemMetricsCollector{
		interval:   interval,
		collectors: collectors,
		taskOpts:   taskOpts,
		connOpts:   connOpts,
		catcher:    grip.NewBasicCatcher(),
	}, nil
}

func (s *SystemMetricsCollector) Start(ctx context.Context) (returnedErr error) {
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		if s.catcher.HasErrors() {
			returnedErr = s.cleanup()
		}
		return
	}()

	var err error
	s.client, err = metrics.NewSystemMetricsClient(ctx, s.connOpts)
	if s.logError(err, "problem creating new system metrics client") {
		return
	}

	s.id, err = s.client.CreateSystemMetricRecord(ctx, *s.taskOpts)
	if s.logError(err, "problem creating cedar system metrics metadata object") {
		return
	}

	var streamingCtx context.Context
	streamingCtx, s.streamingCancel = context.WithCancel(ctx)

	for _, collector := range s.collectors {
		s.wg.Add(1)
		stream, err := s.client.StreamSystemMetrics(streamingCtx, s.id, collector.Name(), metrics.StreamOpts{})
		if s.logError(err, fmt.Sprintf("problem creating system metrics stream for id %s and metricType %s", s.id, collector.Name())) {
			return
		}
		ftdcCollector := ftdc.NewStreamingCollector(1, stream)
		err = ftdcCollector.SetMetadata(collector.Metadata())
		if s.logError(err, fmt.Sprintf("problem setting metadata on stream for id %s and metricType %s", s.id, collector.Name())) {
			return
		}
		go s.timedCollect(streamingCtx, collector, stream, ftdcCollector)
	}
	go s.closeOnCancel(ctx, streamingCtx)
	return
}

func (s *SystemMetricsCollector) timedCollect(ctx context.Context, mc MetricCollector, stream *metrics.SystemMetricsWriteCloser, collector ftdc.Collector) {
	defer s.wg.Done()

	timer := time.NewTimer(0)
	defer timer.Stop()
	defer s.logError(stream.Close(), fmt.Sprintf("problem closing system metrics stream for id %s and metricType %s", s.id, mc.Name()))
	defer s.logError(ftdc.FlushCollector(collector, stream), fmt.Sprintf("problem flushing system metrics data collector for id %s and metricType %s", s.id, mc.Name()))
	defer s.logError(recovery.HandlePanicWithError(recover(), nil, fmt.Sprintf("panic in system metrics stream for id %s and metricType %s", s.id, mc.Name())), "")

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			data, err := mc.Collect()
			if s.logError(err, fmt.Sprintf("problem collecting system metrics data for id %s and metricType %s", s.id, mc.Name())) {
				return
			}
			if s.logError(collector.Add(data), fmt.Sprintf("problem adding system metrics data to collector for id %s and metricType %s", s.id, mc.Name())) {
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

func (s *SystemMetricsCollector) Close(outerCtx context.Context) error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return errors.New("system metrics collector already cancelled or closed")
	}
	s.mu.Unlock()

	return s.cleanup()
}

func (s *SystemMetricsCollector) cleanup() error {
	s.streamingCancel()
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()

	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s.logError(s.client.CloseSystemMetrics(ctx, s.id), fmt.Sprintf("error closing out system metrics object for id %s", s.id))
	s.logError(s.client.CloseClient(), fmt.Sprintf("error closing system metrics client for id %s", s.id))
	s.closed = true

	return s.catcher.Resolve()
}

func (s *SystemMetricsCollector) logError(err error, msg string) bool {
	if err != nil {
		s.mu.Lock()
		defer s.mu.Unlock()
		if msg != "" {
			s.catcher.Add(errors.Wrap(err, msg))
		} else {
			s.catcher.Add(err)
		}
		return true
	}
	return false
}

func getTaskInfo(t *task.Task) (*metrics.SystemMetricsOptions, error) {
	opts := &metrics.SystemMetricsOptions{
		Project:     t.Project,
		Version:     t.Version,
		Variant:     t.BuildVariant,
		TaskName:    t.DisplayName,
		TaskId:      t.Id,
		Execution:   int32(t.Execution),
		Mainline:    t.CommitQueueMerge,
		Compression: metrics.CompressionTypeNone,
		Schema:      metrics.SchemaTypeRawEvents,
	}
	return opts, nil
}

// TODO
// task code for startup (handle error) (pass in agent.comm.GetBuildloggerInfo)
// close code (handle nil)
// comment
