package agent

import (
	"context"
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

	cpuTimeInstrument = "system.cpu.time"
	cpuUtilInstrument = "system.cpu.utilization"

	memoryUsageInstrument       = "system.memory.usage"
	memoryUtilizationInstrument = "system.memory.utilization"

	diskIOInstrument         = "system.disk.io"
	diskOperationsInstrument = "system.disk.operations"
	diskIOTimeInstrument     = "system.disk.io_time"

	networkIOInstrument = "system.network.io"
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
	}, errors.Wrap(instrumentMeter(meterProvider.Meter(packageName)), "instrumenting meter")
}

func instrumentMeter(meter metric.Meter) error {
	catcher := grip.NewBasicCatcher()

	catcher.Wrap(addCPUMetrics(meter), "adding CPU metrics")
	catcher.Wrap(addMemoryMetrics(meter), "adding memory metrics")
	catcher.Wrap(addDiskMetrics(meter), "adding disk metrics")
	catcher.Wrap(addNetworkMetrics(meter), "adding network metrics")

	return catcher.Resolve()
}

func addCPUMetrics(meter metric.Meter) error {
	cpuTime, err := meter.Float64ObservableCounter(cpuTimeInstrument, metric.WithUnit("s"))
	if err != nil {
		return errors.Wrap(err, "making cpu time counter")
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
		observer.ObserveFloat64(cpuTime, times[0].Idle, metric.WithAttributes(attribute.String("state", "idle")))
		observer.ObserveFloat64(cpuTime, times[0].System, metric.WithAttributes(attribute.String("state", "system")))
		observer.ObserveFloat64(cpuTime, times[0].User, metric.WithAttributes(attribute.String("state", "user")))
		observer.ObserveFloat64(cpuTime, times[0].Steal, metric.WithAttributes(attribute.String("state", "steal")))
		observer.ObserveFloat64(cpuTime, times[0].Iowait, metric.WithAttributes(attribute.String("state", "iowait")))

		return nil
	}, cpuTime)
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
	memoryUsage, err := meter.Int64ObservableUpDownCounter(memoryUsageInstrument, metric.WithUnit("By"))
	if err != nil {
		return errors.Wrap(err, "making memory usage counter")
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
		observer.ObserveInt64(memoryUsage, int64(memStats.Available), metric.WithAttributes(attribute.String("state", "available")))
		observer.ObserveInt64(memoryUsage, int64(memStats.Used), metric.WithAttributes(attribute.String("state", "used")))

		observer.ObserveFloat64(memoryUtil, memStats.UsedPercent)

		return nil
	}, memoryUsage, memoryUtil)
	return errors.Wrap(err, "registering memory callback")
}

func addDiskMetrics(meter metric.Meter) error {
	diskIO, err := meter.Int64ObservableCounter(diskIOInstrument, metric.WithUnit("By"))
	if err != nil {
		return errors.Wrap(err, "making disk io counter")
	}

	diskOperations, err := meter.Int64ObservableCounter(diskOperationsInstrument, metric.WithUnit("{operation}"))
	if err != nil {
		return errors.Wrap(err, "making disk operations counter")
	}

	diskIOTime, err := meter.Float64ObservableCounter(diskIOTimeInstrument, metric.WithUnit("s"), metric.WithDescription("Time disk spent activated"))
	if err != nil {
		return errors.Wrap(err, "making disk io time counter")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		ioCountersMap, err := disk.IOCountersWithContext(ctx)
		if err != nil {
			return errors.Wrap(err, "getting disk stats")
		}
		for disk, counter := range ioCountersMap {
			observer.ObserveInt64(diskIO, int64(counter.ReadBytes), metric.WithAttributes(attribute.String("device", disk), attribute.String("direction", "read")))
			observer.ObserveInt64(diskIO, int64(counter.WriteBytes), metric.WithAttributes(attribute.String("device", disk), attribute.String("direction", "write")))

			observer.ObserveInt64(diskOperations, int64(counter.ReadCount), metric.WithAttributes(attribute.String("device", disk), attribute.String("direction", "read")))
			observer.ObserveInt64(diskOperations, int64(counter.WriteCount), metric.WithAttributes(attribute.String("device", disk), attribute.String("direction", "write")))

			observer.ObserveFloat64(diskIOTime, float64(counter.IoTime), metric.WithAttributes(attribute.String("device", disk)))
		}

		return nil
	}, diskIO, diskOperations, diskIOTime)
	return errors.Wrap(err, "registering disk callback")
}

func addNetworkMetrics(meter metric.Meter) error {
	networkIO, err := meter.Int64ObservableCounter(networkIOInstrument, metric.WithUnit("by"))
	if err != nil {
		return errors.Wrap(err, "making network io counter")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		counters, err := net.IOCountersWithContext(ctx, false)
		if err != nil {
			return errors.Wrap(err, "getting network stats")
		}
		if len(counters) != 1 {
			return errors.Wrap(err, "Network counters had an unexpected length")
		}

		for _, counter := range counters {
			observer.ObserveInt64(networkIO, int64(counter.BytesSent), metric.WithAttributes(attribute.String("direction", "transmit")))
			observer.ObserveInt64(networkIO, int64(counter.BytesRecv), metric.WithAttributes(attribute.String("direction", "receive")))
		}

		return nil
	}, networkIO)
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
