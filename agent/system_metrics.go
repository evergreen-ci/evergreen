package agent

import (
	"context"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/rest/client"
	system_metrics "github.com/evergreen-ci/timber/system_metrics"
)

// MetricCollector is an interface representing an object that can collect
// a single system metric over a series of time steps.
type MetricCollector interface {
	// Name returns a string indicating the type of metric collected, such as "uptime".
	Name() string
	// Format returns the format of the collected data.
	Format() DataFormat
	// Collect returns the value of the collected metric when the function is called.
	Collect() ([]byte, error)
}

type SystemMetricCollector struct {
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
