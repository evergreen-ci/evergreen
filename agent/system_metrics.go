package agent

import (
	"context"
	"runtime"
	"sync"
	"time"

	"bytes"
	"encoding/json"

	"github.com/evergreen-ci/evergreen/rest/client"
	system_metrics "github.com/evergreen-ci/timber/system_metrics"
	"github.com/mongodb/ftdc/metrics"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/process"
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

type diskUsageCollector struct{}

// NewDiskUsageCollector creates a diskUsageCollector object.
func NewDiskUsageCollector() *diskUsageCollector {
	return new(diskUsageCollector)
}

func (collector *diskUsageCollector) Name() string { return "disk_usage" }

func (collector *diskUsageCollector) Format() DataFormat { return DataFormatFTDC }

func (collector *diskUsageCollector) Collect(ctx context.Context, dir string) ([]byte, error) {
	usage, err := disk.Usage(dir)
	if err != nil {
		return nil, errors.Wrap(err, "problem capturing metrics with gopsutil")
	}

	return convertJSONToFTDC(ctx, usage)
}

func convertJSONToFTDC(ctx context.Context, metric interface{}) ([]byte, error) {
	jsonMetrics, err := json.Marshal(metric)
	if err != nil {
		return nil, errors.Wrap(err, "problem converting metrics to JSON")
	}

	opts := metrics.CollectJSONOptions{
		InputSource:   bytes.NewReader(jsonMetrics),
		SampleCount:   1,
		FlushInterval: 1 * time.Second,
	}

	output, err := metrics.CollectJSONStream(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "problem converting FTDC to JSON")
	}
	return output, nil
}

type uptimeCollector struct{}

// NewUptimeCollector creates an uptimeCollector object.
func NewUptimeCollector() *uptimeCollector {
	return new(uptimeCollector)
}

type uptimeWrapper struct {
	Uptime uint64 `json:"uptime"`
}

func (collector *uptimeCollector) Name() string { return "uptime" }

func (collector *uptimeCollector) Format() DataFormat { return DataFormatFTDC }

func (collector *uptimeCollector) Collect(ctx context.Context) ([]byte, error) {
	uptime, err := host.Uptime()
	if err != nil {
		return nil, errors.Wrap(err, "problem capturing metrics with gopsutil")
	}

	uptimeWrap := uptimeWrapper{uptime}
	return convertJSONToFTDC(ctx, uptimeWrap)
}

type processCollector struct{}

// NewProcessCollector creates a processCollector object.
func NewProcessCollector() *processCollector {
	return new(processCollector)
}

type processesWrapper struct {
	Processes []ProcessData `json:"processes"`
}

type ProcessData struct {
	PID               int32   `json:"pid"`
	CPUPercent        float64 `json:"%cpu"`
	MemoryPercent     float32 `json:"%mem"`
	VirtualMemorySize uint64  `json:"vsz"`
	ResidentSetSize   uint64  `json:"rss"`
	Terminal          string  `json:"tt"`
	Stat              string  `json:"stat"`
	// TODO (EVG-12736): fix (*Process).CreateTime
	// Started           int64          `json:"started"`
	Time    *cpu.TimesStat `json:"time"`
	Command string         `json:"command"`
}

func (collector *processCollector) Name() string { return "process" }

func (collector *processCollector) Format() DataFormat { return DataFormatJSON }

func (collector *processCollector) Collect(ctx context.Context) ([]byte, error) {
	// TODO (EVG-12736): fix (*Process).CreateTime
	if runtime.GOOS == "darwin" {
		return []byte{}, nil
	}

	var err error
	procs, err := process.Processes()
	if err != nil {
		return nil, errors.Wrap(err, "problem capturing metrics with gopsutil.")
	}

	procMetrics := createProcMetrics(procs)
	procWrapper := processesWrapper{procMetrics}

	results, err := json.Marshal(procWrapper)
	return results, errors.Wrap(err, "problem marshaling processes into JSON")
}

func createProcMetrics(procs []*process.Process) []ProcessData {
	procMetrics := make([]ProcessData, len(procs))

	for i, process := range procs {
		cpuPercent, err := process.CPUPercent()
		if err != nil {
			cpuPercent = 0
		}
		memoryPercent, err := process.MemoryPercent()
		if err != nil {
			memoryPercent = 0
		}
		memInfo, err := process.MemoryInfo()
		var vms, rss uint64 = 0, 0
		if err == nil {
			vms = memInfo.VMS
			rss = memInfo.RSS
		}
		terminal, err := process.Terminal()
		if err != nil {
			terminal = ""
		}
		status, err := process.Status()
		if err != nil {
			status = ""
		}
		// createTime, err := process.CreateTime()
		// if err != nil {
		// 	createTime = 0
		// }
		times, err := process.Times()
		if err != nil {
			times = nil
		}
		name, err := process.Name()
		if err != nil {
			name = ""
		}

		processWrapper := ProcessData{
			PID:               process.Pid,
			CPUPercent:        cpuPercent,
			MemoryPercent:     memoryPercent,
			VirtualMemorySize: vms,
			ResidentSetSize:   rss,
			Terminal:          terminal,
			Stat:              status,
			// Started:           createTime,
			Time:    times,
			Command: name,
		}
		procMetrics[i] = processWrapper
	}
	return procMetrics
}
