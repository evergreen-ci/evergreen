package agent

import (
	"context"
	"sync"
	"time"

	"bytes"
	"encoding/json"
	"time"

	"github.com/evergreen-ci/evergreen/rest/client"
	system_metrics "github.com/evergreen-ci/timber/system_metrics"

	"github.com/k0kubun/pp"
	"github.com/mongodb/ftdc/metrics"
	"github.com/shirou/gopsutil/disk"
)

type MetricCollector interface {
	Name() string
	Metadata() interface{}
	Collect() (interface{}, error)
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

func (collector MetricCollector) Collect(ctx context.Context, metricType string) {
	diskUsage, _ := disk.Usage("/")
	diskJson, _ := json.Marshal(diskUsage)

	opts := metrics.CollectJSONOptions{
		OutputFilePrefix: "test",
		InputSource:      bytes.NewReader(diskJson),
		SampleCount:      100,
		FlushInterval:    100 * time.Millisecond,
	}

	err := metrics.CollectJSONStream(ctx, opts)
	if err != nil {
		pp.Println("error in CollectJSONStream")
	}
}
