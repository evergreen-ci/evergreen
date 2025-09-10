package agent

import (
	"context"
	"fmt"
	"regexp"
	"runtime"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"go.opentelemetry.io/contrib/detectors/aws/ec2"
	"go.opentelemetry.io/contrib/detectors/aws/ecs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.20.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	exportInterval = 15 * time.Second
	exportTimeout  = exportInterval * 2
	packageName    = "github.com/evergreen-ci/evergreen/agent"

	cpuTimeInstrumentPrefix = "system.cpu.time"
	cpuUtilInstrument       = "system.cpu.utilization"

	memoryUsageInstrumentPrefix = "system.memory.usage"
	memoryUtilizationInstrument = "system.memory.utilization"

	diskIOInstrumentPrefix         = "system.disk.io"
	diskOperationsInstrumentPrefix = "system.disk.operations"
	diskIOTimeInstrumentPrefix     = "system.disk.io_time"
	diskWeightedIOInstrumentPrefix = "system.disk.weighted_io"

	diskUsageUsedInstrumentPrefix  = "system.disk.usage.used"
	diskUsageAvailableInstrument   = "system.disk.usage.available"
	diskUsageUtilizationInstrument = "system.disk.usage.utilization"

	networkIOInstrumentPrefix = "system.network.io"

	processCountPrefix = "system.process.count"
)

var instrumentNameDisallowedCharacters = regexp.MustCompile(`[^A-Za-z0-9_.-/]`)

func (a *Agent) initOtel(ctx context.Context) error {
	if a.opts.TraceCollectorEndpoint == "" {
		a.tracer = otel.GetTracerProvider().Tracer(packageName)
		return nil
	}

	var err error
	a.otelGrpcConn, err = grpc.DialContext(ctx,
		a.opts.TraceCollectorEndpoint,
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
	)
	if err != nil {
		return errors.Wrapf(err, "opening gRPC connection to '%s'", a.opts.TraceCollectorEndpoint)
	}

	client := otlptracegrpc.NewClient(otlptracegrpc.WithGRPCConn(a.otelGrpcConn))
	traceExporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return errors.Wrap(err, "initializing otel exporter")
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(hostResource(ctx)),
	)
	tp.RegisterSpanProcessor(utility.NewAttributeSpanProcessor())
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		grip.Error(errors.Wrap(err, "otel error"))
	}))

	a.tracer = tp.Tracer(packageName)

	a.closers = append(a.closers, closerOp{
		name: "tracer provider shutdown",
		closerFn: func(ctx context.Context) error {
			catcher := grip.NewBasicCatcher()
			catcher.Wrap(tp.Shutdown(ctx), "trace provider shutdown")
			catcher.Wrap(traceExporter.Shutdown(ctx), "trace exporter shutdown")
			catcher.Wrap(a.otelGrpcConn.Close(), "closing gRPC connection")

			return catcher.Resolve()
		},
	})

	return nil
}

func (a *Agent) startMetrics(ctx context.Context, tc *internal.TaskConfig) (func(context.Context), error) {
	if a.otelGrpcConn == nil {
		return nil, nil
	}

	metricsExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(a.otelGrpcConn))
	if err != nil {
		return nil, errors.Wrap(err, "making otel metrics exporter")
	}

	r, err := resource.Merge(hostResource(ctx), resource.NewSchemaless(tc.TaskAttributes()...))
	if err != nil {
		return nil, errors.Wrap(err, "merging host resource with task attributes")
	}

	meterProvider := sdk.NewMeterProvider(
		sdk.WithResource(r),
		sdk.WithReader(sdk.NewPeriodicReader(metricsExporter, sdk.WithInterval(exportInterval), sdk.WithTimeout(exportTimeout))),
	)

	return func(ctx context.Context) {
		grip.Error(errors.Wrap(meterProvider.Shutdown(ctx), "doing meter provider"))
	}, errors.Wrap(instrumentMeter(ctx, meterProvider.Meter(packageName)), "instrumenting meter")
}

func instrumentMeter(ctx context.Context, meter metric.Meter) error {
	catcher := grip.NewBasicCatcher()

	// CPU and disk metrics are not implemented by gopsutil for macOS.
	if runtime.GOOS != "darwin" {
		catcher.Wrap(addCPUMetrics(meter), "adding CPU metrics")
		catcher.Wrap(addDiskMetrics(ctx, meter), "adding disk metrics")
	}

	catcher.Wrap(addMemoryMetrics(meter), "adding memory metrics")
	catcher.Wrap(addNetworkMetrics(ctx, meter), "adding network metrics")
	catcher.Wrap(addProcessMetrics(meter), "adding process metrics")

	return catcher.Resolve()
}

func addCPUMetrics(meter metric.Meter) error {
	cpuTimeIdle, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.idle", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time idle counter")
	}
	cpuTimeSystem, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.system", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time system counter")
	}
	cpuTimeUser, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.user", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time user counter")
	}
	cpuTimeSteal, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.steal", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time steal counter")
	}
	cpuTimeIOWait, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.iowait", cpuTimeInstrumentPrefix), metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time iowait counter")
	}

	cpuUtil, err := meter.Float64ObservableGauge(cpuUtilInstrument, metric.WithUnit("1"), metric.WithDescription("Busy CPU time since the last measurement, divided by the elapsed time"))
	if err != nil {
		return errors.Wrap(err, "making cpu util gauge")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		times, err := cpu.TimesWithContext(ctx, false)
		if err != nil {
			return errors.Wrap(err, "getting CPU times")
		}
		if len(times) != 1 {
			return errors.Wrap(err, "CPU times had an unexpected length")
		}
		observer.ObserveFloat64(cpuTimeIdle, times[0].Idle)
		observer.ObserveFloat64(cpuTimeSystem, times[0].System)
		observer.ObserveFloat64(cpuTimeUser, times[0].User)
		observer.ObserveFloat64(cpuTimeSteal, times[0].Steal)
		observer.ObserveFloat64(cpuTimeIOWait, times[0].Iowait)

		return nil
	}, cpuTimeIdle, cpuTimeSystem, cpuTimeUser, cpuTimeSteal, cpuTimeIOWait)
	if err != nil {
		return errors.Wrap(err, "registering cpu time callback")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		util, err := cpu.PercentWithContext(ctx, 0, false)
		if err != nil {
			return errors.Wrap(err, "getting CPU util")
		}
		if len(util) != 1 {
			return errors.Wrap(err, "CPU util had an unexpected length")
		}
		observer.ObserveFloat64(cpuUtil, util[0])

		return nil
	}, cpuUtil)
	return errors.Wrap(err, "registering cpu time callback")
}

func addMemoryMetrics(meter metric.Meter) error {
	memoryUsageAvailable, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.available", memoryUsageInstrumentPrefix), metric.WithUnit("By"))
	if err != nil {
		return errors.Wrap(err, "making memory usage available counter")
	}

	memoryUsageUsed, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.used", memoryUsageInstrumentPrefix), metric.WithUnit("By"))
	if err != nil {
		return errors.Wrap(err, "making memory usage used counter")
	}

	memoryUtil, err := meter.Float64ObservableGauge(memoryUtilizationInstrument, metric.WithUnit("1"))
	if err != nil {
		return errors.Wrap(err, "making memory util gauge")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		memStats, err := mem.VirtualMemoryWithContext(ctx)
		if err != nil {
			return errors.Wrap(err, "getting memory stats")
		}
		observer.ObserveInt64(memoryUsageAvailable, int64(memStats.Available))
		observer.ObserveInt64(memoryUsageUsed, int64(memStats.Used))
		observer.ObserveFloat64(memoryUtil, memStats.UsedPercent)

		return nil
	}, memoryUsageAvailable, memoryUsageUsed, memoryUtil)
	return errors.Wrap(err, "registering memory callback")
}

func isWeightedIOSupported() bool {
	switch runtime.GOOS {
	case "linux":
		return true // Linux(since 2.5.69) supports WeightedIO in /proc/diskstats
	default:
		return false // Windows and macOS typically don't provide this metric
	}
}

func addDiskMetrics(ctx context.Context, meter metric.Meter) error {
	ioCountersMap, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return errors.Wrap(err, "getting disk stats")
	}

	type diskInstruments struct {
		diskIORead           metric.Int64ObservableCounter
		diskIOWrite          metric.Int64ObservableCounter
		diskOperationsRead   metric.Int64ObservableCounter
		diskOperationsWrite  metric.Int64ObservableCounter
		diskIOTime           metric.Float64ObservableCounter
		diskWeightedIO       metric.Float64ObservableCounter // will be nil if not supported
		diskUsageAvailable   metric.Int64ObservableUpDownCounter
		diskUsageUsed        metric.Int64ObservableUpDownCounter
		diskUsageUtilization metric.Float64ObservableGauge
	}
	diskInstrumentMap := map[string]diskInstruments{}
	var allInstruments []metric.Observable
	var diskWeightedIO metric.Float64ObservableCounter
	for diskName := range ioCountersMap {
		// Instrument names may only contain characters in the allowed set. Characters such as : (such as in C: on Windows) are disallowed.
		sanitizedDiskName := instrumentNameDisallowedCharacters.ReplaceAllString(diskName, "")

		diskIORead, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.read", diskIOInstrumentPrefix, sanitizedDiskName), metric.WithUnit("By"))
		if err != nil {
			return errors.Wrapf(err, "making disk io read counter for disk '%s'", diskName)
		}
		diskIOWrite, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.write", diskIOInstrumentPrefix, sanitizedDiskName), metric.WithUnit("By"))
		if err != nil {
			return errors.Wrapf(err, "making disk io write counter for disk '%s'", diskName)
		}

		diskOperationsRead, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.read", diskOperationsInstrumentPrefix, sanitizedDiskName), metric.WithUnit("{operation}"))
		if err != nil {
			return errors.Wrapf(err, "making disk operations read counter for disk '%s'", diskName)
		}
		diskOperationsWrite, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.write", diskOperationsInstrumentPrefix, sanitizedDiskName), metric.WithUnit("{operation}"))
		if err != nil {
			return errors.Wrapf(err, "making disk operations write counter for disk '%s'", diskName)
		}

		diskIOTime, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.%s", diskIOTimeInstrumentPrefix, sanitizedDiskName), metric.WithUnit("s"), metric.WithDescription("Time disk spent activated"))
		if err != nil {
			return errors.Wrapf(err, "making disk io time counter for disk '%s'", diskName)
		}

		if isWeightedIOSupported() {
			diskWeightedIO, err = meter.Float64ObservableCounter(fmt.Sprintf("%s.%s", diskWeightedIOInstrumentPrefix, sanitizedDiskName), metric.WithUnit("s"), metric.WithDescription("Weighted time spent doing I/Os"))
			if err != nil {
				return errors.Wrapf(err, "making disk weighted io time counter for disk '%s'", diskName)
			}
		}

		// Disk usage metrics:
		diskUsageAvailable, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.%s", diskUsageAvailableInstrument, sanitizedDiskName), metric.WithUnit("By"))
		if err != nil {
			return errors.Wrapf(err, "making disk usage available counter for disk '%s'", diskName)
		}

		diskUsageUsed, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.%s", diskUsageUsedInstrumentPrefix, sanitizedDiskName), metric.WithUnit("By"))
		if err != nil {
			return errors.Wrapf(err, "making disk usage used counter for disk '%s'", diskName)
		}

		diskUsageUtilization, err := meter.Float64ObservableGauge(fmt.Sprintf("%s.%s", diskUsageUtilizationInstrument, sanitizedDiskName), metric.WithUnit("%"))
		if err != nil {
			return errors.Wrapf(err, "making disk utilization gauge for disk '%s'", diskName)
		}

		diskInstrumentMap[diskName] = diskInstruments{
			diskIORead:           diskIORead,
			diskIOWrite:          diskIOWrite,
			diskOperationsRead:   diskOperationsRead,
			diskOperationsWrite:  diskOperationsWrite,
			diskIOTime:           diskIOTime,
			diskWeightedIO:       diskWeightedIO,
			diskUsageAvailable:   diskUsageAvailable,
			diskUsageUsed:        diskUsageUsed,
			diskUsageUtilization: diskUsageUtilization,
		}
		allInstruments = append(allInstruments, diskIORead, diskIOWrite, diskOperationsRead, diskOperationsWrite, diskIOTime, diskUsageAvailable, diskUsageUsed, diskUsageUtilization)

		if isWeightedIOSupported() {
			allInstruments = append(allInstruments, diskWeightedIO)
		}
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		ioCountersMap, err := disk.IOCountersWithContext(ctx)
		if err != nil {
			return errors.Wrap(err, "getting disk stats")
		}
		for diskName, instruments := range diskInstrumentMap {
			counter, ok := ioCountersMap[diskName]
			if !ok {
				// If the disk is no longer present there are no readings for it.
				return nil
			}
			observer.ObserveInt64(instruments.diskIORead, int64(counter.ReadBytes))
			observer.ObserveInt64(instruments.diskIOWrite, int64(counter.WriteBytes))

			observer.ObserveInt64(instruments.diskOperationsRead, int64(counter.ReadCount))
			observer.ObserveInt64(instruments.diskOperationsWrite, int64(counter.WriteCount))

			observer.ObserveFloat64(instruments.diskIOTime, float64(counter.IoTime))

			// Only observe WeightedIO if the instrument was created (supported platform)
			if instruments.diskWeightedIO != nil {
				observer.ObserveFloat64(instruments.diskWeightedIO, float64(counter.WeightedIO))
			}

			// Disk usage stats:
			usageStats, err := disk.UsageWithContext(ctx, "/")
			if err != nil {
				// If there was a problem getting disk usage stats,
				// just log it rather thanerroring.
				grip.Error(errors.Wrap(err, "getting disk usage stats"))
				continue
			}
			observer.ObserveInt64(instruments.diskUsageAvailable, int64(usageStats.Free))
			observer.ObserveInt64(instruments.diskUsageUsed, int64(usageStats.Used))
			observer.ObserveFloat64(instruments.diskUsageUtilization, usageStats.UsedPercent)
		}
		return nil
	}, allInstruments...)
	if err != nil {
		return errors.Wrapf(err, "registering callbacks for disk metrics")
	}

	return nil
}

func addNetworkMetrics(ctx context.Context, meter metric.Meter) error {
	transmit, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.transmit", networkIOInstrumentPrefix), metric.WithUnit("By"))
	if err != nil {
		return errors.Wrap(err, "making transmit counter")
	}
	receive, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.receive", networkIOInstrumentPrefix), metric.WithUnit("By"))
	if err != nil {
		return errors.Wrap(err, "making receive counter")
	}
	transmitBps, err := meter.Float64ObservableGauge(fmt.Sprintf("%s.transmit_bps", networkIOInstrumentPrefix), metric.WithUnit("By/s"))
	if err != nil {
		return errors.Wrap(err, "making transmit gauge")
	}
	receiveBps, err := meter.Float64ObservableGauge(fmt.Sprintf("%s.receive_bps", networkIOInstrumentPrefix), metric.WithUnit("By/s"))
	if err != nil {
		return errors.Wrap(err, "making receive gauge")
	}
	maxTransmitBps, err := meter.Float64ObservableGauge(fmt.Sprintf("%s.max_transmit_bps", networkIOInstrumentPrefix), metric.WithUnit("By/s"))
	if err != nil {
		return errors.Wrap(err, "making max transmit gauge")
	}
	maxReceiveBps, err := meter.Float64ObservableGauge(fmt.Sprintf("%s.max_receive_bps", networkIOInstrumentPrefix), metric.WithUnit("By/s"))
	if err != nil {
		return errors.Wrap(err, "making max receive gauge")
	}

	var lastTransmit, lastReceive uint64
	var lastTime time.Time
	var maxTransmit, maxReceive float64

	if cs, err := net.IOCountersWithContext(ctx, false); err == nil && len(cs) == 1 {
		lastTransmit = cs[0].BytesSent
		lastReceive = cs[0].BytesRecv
		lastTime = time.Now()
	} else if err != nil {
		return errors.Wrap(err, "getting initial network stats")
	} else {
		return errors.New("network counters had an unexpected length")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		now := time.Now()

		counters, err := net.IOCountersWithContext(ctx, false)
		if err != nil {
			return errors.Wrap(err, "getting network stats")
		}
		if len(counters) != 1 {
			return errors.New("network counters had an unexpected length")
		}
		ac := counters[0]
		observer.ObserveInt64(transmit, int64(ac.BytesSent))
		observer.ObserveInt64(receive, int64(ac.BytesRecv))

		if !lastTime.IsZero() {
			dt := now.Sub(lastTime).Seconds()
			if dt > 0 {
				txBps := float64(ac.BytesSent-lastTransmit) / dt
				rxBps := float64(ac.BytesRecv-lastReceive) / dt
				if txBps < 0 {
					txBps = 0
				}
				if rxBps < 0 {
					rxBps = 0
				}
				observer.ObserveFloat64(transmitBps, txBps)
				observer.ObserveFloat64(receiveBps, rxBps)
				if txBps > maxTransmit {
					maxTransmit = txBps
				}
				if rxBps > maxReceive {
					maxReceive = rxBps
				}
				observer.ObserveFloat64(maxTransmitBps, maxTransmit)
				observer.ObserveFloat64(maxReceiveBps, maxReceive)
			}
		}
		lastTransmit = ac.BytesSent
		lastReceive = ac.BytesRecv
		lastTime = now
		return nil
	}, transmit, receive, transmitBps, receiveBps, maxTransmitBps, maxReceiveBps)
	return errors.Wrap(err, "registering network io callback")
}

func hostResource(ctx context.Context) *resource.Resource {
	r := resource.NewSchemaless(
		semconv.ServiceName("evergreen-agent"),
		semconv.ServiceVersion(evergreen.BuildRevision),
	)

	mergedResource, err := addEnvironmentAttributes(ctx, r)
	grip.Error(errors.Wrap(err, "adding environment attributes"))
	if err == nil {
		r = mergedResource
	}

	return r
}

// addEnvironmentAttributes adds attributes to the resource about the environment itself. When running in EC2
// this includes information like the instance id and when running in ECS this includes information like the
// container name. This will noop if not running in EC2/ECS.
func addEnvironmentAttributes(ctx context.Context, r *resource.Resource) (*resource.Resource, error) {
	for name, detector := range map[string]resource.Detector{
		"ec2": ec2.NewResourceDetector(),
		"ecs": ecs.NewResourceDetector(),
	} {
		detectedResource, err := detector.Detect(ctx)
		if err != nil {
			return nil, errors.Wrapf(err, "detecting resource '%s'", name)
		}
		mergedResource, err := resource.Merge(r, detectedResource)
		if err != nil {
			return nil, errors.Wrapf(err, "merging resource for detector '%s'", name)
		}
		r = mergedResource
	}

	return r, nil
}
