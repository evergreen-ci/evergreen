package agent

import (
	"context"
	"fmt"
	"sync"
	"time"

	"bytes"
	"encoding/json"

	"github.com/evergreen-ci/evergreen/rest/client"
	system_metrics "github.com/evergreen-ci/timber/system_metrics"
	"github.com/mongodb/ftdc/metrics"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/process"
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

type DiskUsageCollector struct{}

type UptimeCollector struct{}

type ProcessCollector struct{}

type DiskUsageWithTimestamp struct {
	Timestamp time.Time `json:"ts"`
	*disk.UsageStat
}

type UptimeWithTimestamp struct {
	Timestamp time.Time `json:"ts"`
	Uptime    uint64    `json:"uptime"`
	Hello     string    `json:"hello"`
}

type ProcessesWithTimestamp struct {
	Timestamp time.Time `json:"ts"`
	Processes []*process.Process
}

func (collector *DiskUsageCollector) Name() string {
	return "disk_usage"
}

func (collector *UptimeCollector) Name() string {
	return "uptime"
}

func (collector *ProcessCollector) Name() string {
	return "process"
}

func (collector *DiskUsageCollector) Metadata() interface{} {
	return nil
}

func (collector *UptimeCollector) Metadata() interface{} {
	return nil
}

func (collector *ProcessCollector) Metadata() interface{} {
	return nil
}

func (collector *DiskUsageCollector) Collect(ctx context.Context) ([]byte, error) {
	metric, err := disk.Usage("/")
	if err != nil {
		return nil, errors.Wrap(err, "problem capturing metrics with gopsutil")
	}

	diskUsageWithTimestamp := DiskUsageWithTimestamp{time.Now(), metric}
	return convertJSONToFTDC(ctx, diskUsageWithTimestamp)
}

func (collector *UptimeCollector) Collect(ctx context.Context) ([]byte, error) {
	metric, err := host.Uptime()
	if err != nil {
		return nil, errors.Wrap(err, "problem capturing metrics with gopsutil")
	}

	uptimeWithTimestamp := UptimeWithTimestamp{time.Now(), metric, "hello"}
	fmt.Println(uptimeWithTimestamp)

	return convertJSONToFTDC(ctx, uptimeWithTimestamp)
}

func (collector *ProcessCollector) Collect(ctx context.Context) ([]byte, error) {
	metric, err := process.Processes()
	if err != nil {
		return nil, errors.Wrap(err, "problem capturing metrics with gopsutil")
	}

	processesWithTimestamp := ProcessesWithTimestamp{time.Now(), metric}
	return convertJSONToFTDC(ctx, processesWithTimestamp)
}

func convertJSONToFTDC(ctx context.Context, metric interface{}) ([]byte, error) {
	jsonMetrics, err := json.Marshal(metric)
	fmt.Println("\njsonMetrics:", string(jsonMetrics))
	if err != nil {
		return nil, errors.Wrap(err, "problem converting metrics to JSON")
	}

	opts := metrics.CollectJSONOptions{
		InputSource:   bytes.NewReader(jsonMetrics),
		SampleCount:   100,
		FlushInterval: 100 * time.Millisecond,
	}

	output, err := metrics.CollectJSONStream(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "problem converting FTDC to JSON")
	}
	return output, nil
}
