package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"runtime"
	"time"

	"github.com/mongodb/ftdc/metrics"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/cpu"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/process"
)

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

func (c *diskUsageCollector) Name() string { return "disk_usage" }

func (c *diskUsageCollector) Format() DataFormat { return DataFormatFTDC }

func (c *diskUsageCollector) Collect(ctx context.Context, dir string) ([]byte, error) {
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
		FlushInterval: time.Second,
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
	Uptime uint64 `json:"uptime,omitempty"`
}

func (c *uptimeCollector) Name() string { return "uptime" }

func (c *uptimeCollector) Format() DataFormat { return DataFormatFTDC }

func (c *uptimeCollector) Collect(ctx context.Context) ([]byte, error) {
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
	Processes []processData `json:"processes,omitempty"`
}

type processData struct {
	PID               int32   `json:"pid,omitempty"`
	CPUPercent        float64 `json:"percent_cpu,omitempty"`
	MemoryPercent     float32 `json:"percent_mem,omitempty"`
	VirtualMemorySize uint64  `json:"vsz,omitempty"`
	ResidentSetSize   uint64  `json:"rss,omitempty"`
	Terminal          string  `json:"tt,omitempty"`
	Stat              string  `json:"stat,omitempty"`
	// TODO (EVG-12736): fix (*Process).CreateTime
	// Started           int64          `json:"started"`
	Time    *cpu.TimesStat `json:"time,omitempty"`
	Command string         `json:"command,omitempty"`
}

func (c *processCollector) Name() string { return "process" }

func (c *processCollector) Format() DataFormat { return DataFormatJSON }

func (c *processCollector) Collect(ctx context.Context) ([]byte, error) {
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

func createProcMetrics(procs []*process.Process) []processData {
	procMetrics := make([]processData, len(procs))

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

		processWrapper := processData{
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
