package agent

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/client"
	"github.com/evergreen-ci/timber"
	system_metrics "github.com/evergreen-ci/timber/system_metrics"
	"github.com/mongodb/ftdc"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
)

type MetricCollector interface {
	Name() string
	Metadata() interface{}
	Collect() (interface{}, error)
}

type SystemMetricsCollector struct {
	mu          sync.Mutex
	wg          sync.WaitGroup
	innerCancel context.CancelFunc
	logger      client.LoggerProducer
	taskOpts    *system_metrics.SystemMetricsOptions
	connOpts    system_metrics.ConnectionOptions
	interval    time.Duration
	collectors  []MetricCollector
	id          string
	client      *system_metrics.SystemMetricsClient
	errored     bool
	closed      bool
}

func NewSystemMetricsCollector(ctx context.Context, logger client.LoggerProducer, interval time.Duration,
	c client.Communicator, t *task.Task, collectors []MetricCollector) (*SystemMetricsCollector, error) {

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
	connOpts := system_metrics.ConnectionOptions{
		DialOpts: dialOpts,
	}

	return &SystemMetricsCollector{
		logger:     logger,
		interval:   interval,
		collectors: collectors,
		taskOpts:   taskOpts,
		connOpts:   connOpts,
	}, nil
}

func (s *SystemMetricsCollector) Start(ctx context.Context) {
	catcher := grip.NewBasicCatcher()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.errored = true
		err := catcher.Resolve()
		s.logger.System().Error(message.WrapError(err, message.Fields{
			"message":  "error setting up system metrics collection",
			"id":       s.id,
			"connOpts": s.connOpts,
			"taskOpts": *s.taskOpts,
		}))
	}()

	var err error
	s.client, err = system_metrics.NewSystemMetricsClient(ctx, s.connOpts)
	if err != nil {
		catcher.Add(err)
		return
	}

	s.id, err = s.client.CreateSystemMetricRecord(ctx, *s.taskOpts)
	if err != nil {
		catcher.Add(err)
		return
	}

	innerCtx := context.Background()
	innerCtx, s.innerCancel = context.WithCancel(innerCtx)

	for _, collector := range s.collectors {
		s.wg.Add(1)
		stream, err := s.client.StreamSystemMetrics(innerCtx, s.id, collector.Name(), system_metrics.StreamOpts{})
		if err != nil {
			catcher.Add(err)
			return
		}
		ftdcCollector := ftdc.NewStreamingCollector(1, stream)
		err = ftdcCollector.SetMetadata(collector.Metadata())
		if err != nil {
			catcher.Add(err)
			return
		}
		go s.timedCollect(innerCtx, collector, stream, ftdcCollector)
	}
	go s.closeOnCancel(ctx)
}

func (s *SystemMetricsCollector) timedCollect(ctx context.Context, mc MetricCollector, stream *system_metrics.SystemMetricsWriteCloser, collector ftdc.Collector) {
	defer s.wg.Done()

	catcher := grip.NewBasicCatcher()
	defer func() {
		s.mu.Lock()
		defer s.mu.Unlock()
		s.errored = true
		err := catcher.Resolve()
		s.logger.System().Error(message.WrapError(err, message.Fields{
			"message":    "error while collecting system metrics",
			"id":         s.id,
			"metricType": mc.Name(),
		}))
	}()

	timer := time.NewTimer(0)
	defer timer.Stop()
	defer catcher.Add(stream.Close())
	defer ftdc.FlushCollector(collector, stream)
	defer catcher.Add(recovery.HandlePanicWithError(recover(), nil, "panic in system metrics collector"))

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			func() {
				data, err := mc.Collect()
				if err != nil {
					catcher.Add(err)
					return
				}
				err = collector.Add(data)
				if err != nil {
					catcher.Add(err)
					return
				}
				_ = timer.Reset(s.interval)
			}()
		}
	}
}

func (s *SystemMetricsCollector) closeOnCancel(outerCtx context.Context) {
	for {
		select {
		case <-outerCtx.Done():
			if s.cleanup() != nil {
				s.mu.Lock()
				defer s.mu.Unlock()
				s.errored = true
			}
			return
		}
	}
}

func (s *SystemMetricsCollector) Close(outerCtx context.Context) error {
	s.mu.Lock()
	if s.closed && s.errored {
		return errors.New("system metrics collector already cancelled or closed with errors, see system logs")
	}
	if s.closed {
		return errors.New("system metrics collector already cancelled or closed")
	}
	s.mu.Unlock()

	err := s.cleanup()
	if err != nil || s.errored {
		err = errors.New("system metrics collector encountered errors, see system logs.")
	}
	return err
}

func (s *SystemMetricsCollector) cleanup() error {
	s.innerCancel()
	s.wg.Wait()
	s.mu.Lock()
	defer s.mu.Unlock()

	catcher := grip.NewBasicCatcher()
	ctx := context.Background()
	ctx, cancel := context.WithTimeout(ctx, time.Minute)
	defer cancel()
	err := s.client.CloseSystemMetrics(ctx, s.id)
	catcher.Add(err)
	s.logger.System().Error(message.WrapError(err, message.Fields{
		"message":  "error closing system metrics collector",
		"id":       s.id,
		"interval": s.interval,
	}))
	err = s.client.CloseClient()
	catcher.Add(err)
	s.logger.System().Error(message.WrapError(err, message.Fields{
		"message":  "error closing system metrics collector client",
		"id":       s.id,
		"interval": s.interval,
	}))
	s.closed = true

	return catcher.Resolve()
}

func getTaskInfo(t *task.Task) (*system_metrics.SystemMetricsOptions, error) {
	// convert from task to SystemMetrics optiobn
	return nil, nil
}

// TODO
// task code for startup (handle error) (pass in agent.comm.GetBuildloggerInfo)
// close code (handle nil)
// comment
