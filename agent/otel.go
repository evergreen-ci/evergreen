package agent

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/disk"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"go.opentelemetry.io/contrib/detectors/aws/ec2"
	"go.opentelemetry.io/contrib/detectors/aws/ecs"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/metric"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

type taskAttributeKey int

const taskAttributeContextKey taskAttributeKey = iota

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

	networkIOInstrumentPrefix = "system.network.io"
)

func (a *Agent) initOtel(ctx context.Context) error {
	if a.opts.TraceCollectorEndpoint == "" {
		a.tracer = otel.GetTracerProvider().Tracer(packageName)
		return nil
	}

	r, err := hostResource(ctx)
	if err != nil {
		return errors.Wrap(err, "making host resource")
	}

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
		sdktrace.WithResource(r),
	)
	tp.RegisterSpanProcessor(NewTaskSpanProcessor())
	otel.SetTracerProvider(tp)
	otel.SetErrorHandler(otel.ErrorHandlerFunc(func(err error) {
		grip.Error(errors.Wrap(err, "encountered otel error"))
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
	metricsExporter, err := otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(a.otelGrpcConn))
	if err != nil {
		return nil, errors.Wrap(err, "making otel metrics exporter")
	}

	r, err := hostResource(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "making resource")
	}

	r, err = resource.Merge(r, resource.NewSchemaless(tc.TaskAttributes()...))
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

	catcher.Wrap(addCPUMetrics(meter), "adding CPU metrics")
	catcher.Wrap(addMemoryMetrics(meter), "adding memory metrics")
	catcher.Wrap(addDiskMetrics(ctx, meter), "adding disk metrics")
	catcher.Wrap(addNetworkMetrics(meter), "adding network metrics")

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

func addDiskMetrics(ctx context.Context, meter metric.Meter) error {
	ioCountersMap, err := disk.IOCountersWithContext(ctx)
	if err != nil {
		return errors.Wrap(err, "getting disk stats")
	}

	type diskInstruments struct {
		diskIORead          metric.Int64ObservableCounter
		diskIOWrite         metric.Int64ObservableCounter
		diskOperationsRead  metric.Int64ObservableCounter
		diskOperationsWrite metric.Int64ObservableCounter
		diskIOTime          metric.Float64ObservableCounter
	}
	diskInstrumentMap := map[string]diskInstruments{}
	var allInstruments []metric.Observable
	for diskName := range ioCountersMap {
		diskIORead, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.read", diskIOInstrumentPrefix, diskName), metric.WithUnit("By"))
		if err != nil {
			return errors.Wrapf(err, "making disk io read counter for disk '%s'", diskName)
		}
		diskIOWrite, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.write", diskIOInstrumentPrefix, diskName), metric.WithUnit("By"))
		if err != nil {
			return errors.Wrapf(err, "making disk io write counter for disk '%s'", diskName)
		}

		diskOperationsRead, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.read", diskOperationsInstrumentPrefix, diskName), metric.WithUnit("{operation}"))
		if err != nil {
			return errors.Wrapf(err, "making disk operations read counter for disk '%s'", diskName)
		}
		diskOperationsWrite, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.%s.write", diskOperationsInstrumentPrefix, diskName), metric.WithUnit("{operation}"))
		if err != nil {
			return errors.Wrapf(err, "making disk operations write counter for disk '%s'", diskName)
		}

		diskIOTime, err := meter.Float64ObservableCounter(fmt.Sprintf("%s.%s", diskIOTimeInstrumentPrefix, diskName), metric.WithUnit("s"), metric.WithDescription("Time disk spent activated"))
		if err != nil {
			return errors.Wrapf(err, "making disk io time counter for disk '%s'", diskName)
		}

		diskInstrumentMap[diskName] = diskInstruments{
			diskIORead:          diskIORead,
			diskIOWrite:         diskIOWrite,
			diskOperationsRead:  diskOperationsRead,
			diskOperationsWrite: diskOperationsWrite,
			diskIOTime:          diskIOTime,
		}
		allInstruments = append(allInstruments, diskIORead, diskIOWrite, diskOperationsRead, diskOperationsWrite, diskIOTime)
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
		}
		return nil
	}, allInstruments...)
	if err != nil {
		return errors.Wrapf(err, "registering callbacks for disk metrics")
	}

	return nil
}

func addNetworkMetrics(meter metric.Meter) error {
	networkIOTransmit, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.transmit", networkIOInstrumentPrefix), metric.WithUnit("by"))
	if err != nil {
		return errors.Wrap(err, "making network io transmit counter")
	}

	networkIOReceive, err := meter.Int64ObservableCounter(fmt.Sprintf("%s.receive", networkIOInstrumentPrefix), metric.WithUnit("by"))
	if err != nil {
		return errors.Wrap(err, "making network io receive counter")
	}
	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		counters, err := net.IOCountersWithContext(ctx, false)
		if err != nil {
			return errors.Wrap(err, "getting network stats")
		}
		if len(counters) != 1 {
			return errors.Wrap(err, "network counters had an unexpected length")
		}

		observer.ObserveInt64(networkIOTransmit, int64(counters[0].BytesSent))
		observer.ObserveInt64(networkIOReceive, int64(counters[0].BytesRecv))

		return nil
	}, networkIOTransmit, networkIOReceive)
	return errors.Wrap(err, "registering network io callback")
}

func hostResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("evergreen-agent")),
		resource.WithAttributes(semconv.ServiceVersion(evergreen.BuildRevision)),
		resource.WithDetectors(ec2.NewResourceDetector(), ecs.NewResourceDetector()),
	)
}

type taskSpanProcessor struct{}

func NewTaskSpanProcessor() sdktrace.SpanProcessor {
	return &taskSpanProcessor{}
}

func (processor *taskSpanProcessor) OnStart(ctx context.Context, span sdktrace.ReadWriteSpan) {
	span.SetAttributes(taskAttributesFromContext(ctx)...)
}

func (processor *taskSpanProcessor) OnEnd(s sdktrace.ReadOnlySpan)    {}
func (processor *taskSpanProcessor) Shutdown(context.Context) error   { return nil }
func (processor *taskSpanProcessor) ForceFlush(context.Context) error { return nil }

func contextWithTaskAttributes(ctx context.Context, attributes []attribute.KeyValue) context.Context {
	return context.WithValue(ctx, taskAttributeContextKey, attributes)
}

func taskAttributesFromContext(ctx context.Context) []attribute.KeyValue {
	attributesIface := ctx.Value(taskAttributeContextKey)
	attributes, ok := attributesIface.([]attribute.KeyValue)
	if !ok {
		return nil
	}
	return attributes
}
