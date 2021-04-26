package agent

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/timber/systemmetrics"
	"github.com/pkg/errors"
	"google.golang.org/grpc"
)

// TODO (EVG-14497): remove this hack.
// This is a no-op file hack to deal with the fact that gopsutil does not
// compile with MacOS ARM64 (i.e. GOOS=darwin, GOARCH=arm64) until v3. However,
// we cannot use v3 until go modules are supported, so it's easier to simply
// no-op all system metrics code until this can be fixed.

// metricCollector is an interface representing an object that can collect
// a single system metric over a series of time steps.
type metricCollector interface {
	// name returns a string indicating the type of metric collected, such as "uptime".
	name() string
	// format returns the format of the collected data.
	format() systemmetrics.DataFormat
	// collect returns the value of the collected metric when the function is called.
	collect(context.Context) ([]byte, error)
}

// systemMetricsCollector handles collecting an arbitrary set of system
// metrics at a fixed interval and streaming them to cedar.
type systemMetricsCollector struct{}

// systemMetricsCollectorOptions are the required values for creating a new
// systemMetricsCollector.
type systemMetricsCollectorOptions struct {
	task       *task.Task
	interval   time.Duration
	collectors []metricCollector
	conn       *grpc.ClientConn

	// These options configure the timber stream buffer, and can be left empty
	// to use the default settings.
	maxBufferSize            int
	noBufferTimedFlush       bool
	bufferTimedFlushInterval time.Duration
}

func (s *systemMetricsCollectorOptions) validate() error {
	if s.task == nil {
		return errors.New("must provide a valid task")
	}

	if s.interval < 0 {
		return errors.New("interval cannot be negative")
	}
	if s.interval == 0 {
		s.interval = time.Minute
	}

	if len(s.collectors) == 0 {
		return errors.New("must provide at least one metric collector")
	}

	if s.conn == nil {
		return errors.New("must provide an existing client connection to cedar")
	}

	if s.bufferTimedFlushInterval < 0 {
		return errors.New("flush interval must not be negative")
	}

	if s.maxBufferSize < 0 {
		return errors.New("buffer size must not be negative")
	}

	return nil
}

func newSystemMetricsCollector(ctx context.Context, opts *systemMetricsCollectorOptions) (*systemMetricsCollector, error) {
	return &systemMetricsCollector{}, nil
}

func (s *systemMetricsCollector) Start(ctx context.Context) error { return nil }

func (s *systemMetricsCollector) Close() error { return nil }

type uptimeCollector struct{}

func newUptimeCollector() *uptimeCollector {
	return new(uptimeCollector)
}

func (c *uptimeCollector) name() string { return "uptime" }

func (c *uptimeCollector) format() systemmetrics.DataFormat { return systemmetrics.DataFormatFTDC }

func (c *uptimeCollector) collect(ctx context.Context) ([]byte, error) {
	return []byte{}, nil
}

type processCollector struct{}

func newProcessCollector() *processCollector {
	return new(processCollector)
}

func (c *processCollector) name() string { return "process" }

func (c *processCollector) format() systemmetrics.DataFormat { return systemmetrics.DataFormatJSON }

func (c *processCollector) collect(ctx context.Context) ([]byte, error) {
	return []byte{}, nil
}

type diskUsageCollector struct{}

func newDiskUsageCollector(dir string) *diskUsageCollector {
	return new(diskUsageCollector)
}

func (c *diskUsageCollector) name() string { return "disk_usage" }

func (c *diskUsageCollector) format() systemmetrics.DataFormat { return systemmetrics.DataFormatFTDC }

func (c *diskUsageCollector) collect(ctx context.Context) ([]byte, error) {
	return []byte{}, nil
}
