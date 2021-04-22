// +build linux windows darwin,!arm64

package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/timber/systemmetrics"
	"github.com/mongodb/ftdc/metrics"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/disk"
	"github.com/shirou/gopsutil/host"
	"github.com/shirou/gopsutil/process"
	"google.golang.org/grpc"
)

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
type systemMetricsCollector struct {
	mu              sync.Mutex
	stream          sync.WaitGroup
	close           sync.WaitGroup
	streamingCancel context.CancelFunc
	taskOpts        systemmetrics.SystemMetricsOptions
	writeOpts       systemmetrics.WriteCloserOptions
	interval        time.Duration
	collectors      []metricCollector
	client          *systemmetrics.SystemMetricsClient
	catcher         grip.Catcher
	id              string
	closed          bool
}

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

// newSystemMetricsCollector returns a systemMetricsCollector ready to start
// collecting from the provided slice of metric collector objects at the
// provided interval and streaming to cedar.
func newSystemMetricsCollector(ctx context.Context, opts *systemMetricsCollectorOptions) (*systemMetricsCollector, error) {
	err := opts.validate()
	if err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}
	s := &systemMetricsCollector{
		interval:   opts.interval,
		collectors: opts.collectors,
		taskOpts:   getSystemMetricsInfo(opts.task),
		writeOpts: systemmetrics.WriteCloserOptions{
			FlushInterval: opts.bufferTimedFlushInterval,
			NoTimedFlush:  opts.noBufferTimedFlush,
			MaxBufferSize: opts.maxBufferSize,
		},
		catcher: grip.NewBasicCatcher(),
	}
	s.client, err = systemmetrics.NewSystemMetricsClientWithExistingConnection(ctx, opts.conn)
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
	s.id, err = s.client.CreateSystemMetricsRecord(ctx, s.taskOpts)
	if err != nil {
		return errors.Wrap(err, "problem creating cedar system metrics metadata object")
	}

	var streamingCtx context.Context
	streamingCtx, s.streamingCancel = context.WithCancel(ctx)

	for _, collector := range s.collectors {
		stream, err := s.client.NewSystemMetricsWriteCloser(ctx, systemmetrics.MetricDataOptions{
			Id:         s.id,
			MetricType: collector.name(),
			Format:     collector.format(),
		}, s.writeOpts)
		if err != nil {
			s.streamingCancel()
			s.streamingCancel = nil
			return errors.Wrap(err, fmt.Sprintf("problem creating system metrics stream for id %s and metricType %s", s.id, collector.name()))
		}

		s.stream.Add(1)
		go s.timedCollect(streamingCtx, collector, stream)
	}
	s.close.Add(1)
	go s.closeOnCancel(ctx, streamingCtx)

	return nil
}

func (s *systemMetricsCollector) timedCollect(ctx context.Context, mc metricCollector, stream io.WriteCloser) {
	timer := time.NewTimer(0)
	defer func() {
		s.catcher.Add(errors.Wrap(recovery.HandlePanicWithError(recover(), nil, fmt.Sprintf("panic in system metrics stream for id %s and metricType %s", s.id, mc.name())), ""))
		s.catcher.Add(errors.Wrap(stream.Close(), fmt.Sprintf("problem closing system metrics stream for id %s and metricType %s", s.id, mc.name())))
		timer.Stop()
		s.stream.Done()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-timer.C:
			data, err := mc.collect(ctx)
			if err != nil {
				// Do not accumulate errors caused by system metrics collectors
				// closing this metric collector's context.
				if ctx.Err() != nil {
					return
				}
				s.catcher.Add(errors.Wrapf(err, "problem collecting system metrics data for id %s and metricType %s", s.id, mc.name()))
				return
			}
			_, err = stream.Write(data)
			if err != nil {
				// Do not accumulate errors caused by system metrics collectors
				// closing this metric collector's context.
				if ctx.Err() != nil {
					return
				}
				s.catcher.Add(errors.Wrapf(err, "problem writing system metrics data to stream for id %s and metricType %s", s.id, mc.name()))
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
		return s.catcher.Resolve()
	}
	s.mu.Unlock()

	if s.streamingCancel != nil {
		s.streamingCancel()
	}
	s.close.Wait()
	return s.catcher.Resolve()
}

func getSystemMetricsInfo(t *task.Task) systemmetrics.SystemMetricsOptions {
	return systemmetrics.SystemMetricsOptions{
		Project:     t.Project,
		Version:     t.Version,
		Variant:     t.BuildVariant,
		TaskName:    t.DisplayName,
		TaskId:      t.Id,
		Execution:   int32(t.Execution),
		Mainline:    !t.IsPatchRequest(),
		Compression: systemmetrics.CompressionTypeNone,
		Schema:      systemmetrics.SchemaTypeRawEvents,
	}
}

type diskUsageCollector struct {
	dir string
}

// newDiskusageCollector creates a collector that gathers disk usage information
// for the given directory.
func newDiskUsageCollector(dir string) *diskUsageCollector {
	return &diskUsageCollector{
		dir: dir,
	}
}

type diskUsageWrapper struct {
	Path              string  `json:"path,omitempty"`
	Fstype            string  `json:"fstype,omitempty"`
	Total             uint64  `json:"total,omitempty"`
	Free              uint64  `json:"free,omitempty"`
	Used              uint64  `json:"used,omitempty"`
	UsedPercent       float64 `json:"used_percent,omitempty"`
	InodesTotal       uint64  `json:"inodes_total,omitempty"`
	InodesUsed        uint64  `json:"inodes_used,omitempty"`
	InodesFree        uint64  `json:"inodes_free,omitempty"`
	InodesUsedPercent float64 `json:"inodes_used_percent,omitempty"`
}

func (c *diskUsageCollector) name() string { return "disk_usage" }

func (c *diskUsageCollector) format() systemmetrics.DataFormat { return systemmetrics.DataFormatFTDC }

func (c *diskUsageCollector) collect(ctx context.Context) ([]byte, error) {
	usage, err := disk.UsageWithContext(ctx, c.dir)
	if err != nil {
		return nil, errors.Wrap(err, "problem capturing metrics with gopsutil")
	}
	wrapper := diskUsageWrapper{
		Path:              usage.Path,
		Fstype:            usage.Fstype,
		Total:             usage.Total,
		Free:              usage.Free,
		Used:              usage.Used,
		UsedPercent:       usage.UsedPercent,
		InodesTotal:       usage.InodesTotal,
		InodesUsed:        usage.InodesUsed,
		InodesFree:        usage.InodesFree,
		InodesUsedPercent: usage.InodesUsedPercent,
	}

	return convertJSONToFTDC(ctx, wrapper)
}

type uptimeCollector struct{}

// newUptimeCollector creates a collector that gathers host uptime information.
func newUptimeCollector() *uptimeCollector {
	return new(uptimeCollector)
}

type uptimeWrapper struct {
	Uptime uint64 `json:"uptime,omitempty"`
}

func (c *uptimeCollector) name() string { return "uptime" }

func (c *uptimeCollector) format() systemmetrics.DataFormat { return systemmetrics.DataFormatFTDC }

func (c *uptimeCollector) collect(ctx context.Context) ([]byte, error) {
	uptime, err := host.UptimeWithContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem capturing metrics with gopsutil")
	}

	uptimeWrap := uptimeWrapper{Uptime: uptime}
	return convertJSONToFTDC(ctx, uptimeWrap)
}

type processCollector struct{}

// newProcessCollector creates a collector that collects information on the
// processes currently running on the host.
func newProcessCollector() *processCollector {
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
	Started           int64   `json:"started"`
	Time              string  `json:"time,omitempty"`
	Command           string  `json:"command,omitempty"`
}

func (c *processCollector) name() string { return "process" }

func (c *processCollector) format() systemmetrics.DataFormat { return systemmetrics.DataFormatJSON }

func (c *processCollector) collect(ctx context.Context) ([]byte, error) {
	var err error
	procs, err := process.ProcessesWithContext(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "problem capturing metrics with gopsutil")
	}

	procMetrics := createProcMetrics(ctx, procs)
	procWrapper := processesWrapper{Processes: procMetrics}

	results, err := json.Marshal(procWrapper)
	return results, errors.Wrap(err, "problem marshaling processes into JSON")
}

func createProcMetrics(ctx context.Context, procs []*process.Process) []processData {
	procMetrics := make([]processData, len(procs))

	for i, proc := range procs {
		cpuPercent, err := proc.CPUPercentWithContext(ctx)
		if err != nil {
			cpuPercent = 0
		}
		memoryPercent, err := proc.MemoryPercentWithContext(ctx)
		if err != nil {
			memoryPercent = 0
		}
		memInfo, err := proc.MemoryInfoWithContext(ctx)
		var vms, rss uint64
		if err == nil {
			vms = memInfo.VMS
			rss = memInfo.RSS
		}
		terminal, err := proc.TerminalWithContext(ctx)
		if err != nil {
			terminal = ""
		}
		status, err := proc.StatusWithContext(ctx)
		if err != nil {
			status = ""
		}
		createTime, err := proc.CreateTimeWithContext(ctx)
		if err != nil {
			createTime = 0
		}
		var cpuTime string
		times, err := proc.TimesWithContext(ctx)
		if err != nil {
			times = nil
		} else {
			cpuTime = times.CPU
		}
		name, err := proc.NameWithContext(ctx)
		if err != nil {
			name = ""
		}

		procWrapper := processData{
			PID:               proc.Pid,
			CPUPercent:        cpuPercent,
			MemoryPercent:     memoryPercent,
			VirtualMemorySize: vms,
			ResidentSetSize:   rss,
			Terminal:          terminal,
			Stat:              status,
			Started:           createTime,
			Time:              cpuTime,
			Command:           name,
		}
		procMetrics[i] = procWrapper
	}
	return procMetrics
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
