package agent

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/honeycombio/honeycomb-opentelemetry-go"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
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
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

const (
	exportInterval = 15 * time.Second
	exportTimeout  = exportInterval * 2
	packageName    = "github.com/evergreen-ci/evergreen/agent"
)

func (a *Agent) initOtel(ctx context.Context) error {
	if a.opts.TraceCollectorEndpoint == "" {
		a.tracer = otel.GetTracerProvider().Tracer(packageName)
		return nil
	}

	r, err := hostResource(ctx)
	if err != nil {
		return errors.Wrap(err, "making resource")
	}
	if err != nil {
		return errors.Wrap(err, "making otel resource")
	}

	grpcConn, err := grpc.DialContext(ctx,
		a.opts.TraceCollectorEndpoint,
		grpc.WithTransportCredentials(credentials.NewTLS(nil)),
	)
	if err != nil {
		return errors.Wrapf(err, "opening gRPC connection to '%s'", a.opts.TraceCollectorEndpoint)
	}

	client := otlptracegrpc.NewClient(otlptracegrpc.WithGRPCConn(grpcConn))
	traceExporter, err := otlptrace.New(ctx, client)
	if err != nil {
		return errors.Wrap(err, "initializing otel exporter")
	}
	tp := sdktrace.NewTracerProvider(
		sdktrace.WithBatcher(traceExporter),
		sdktrace.WithResource(r),
	)
	tp.RegisterSpanProcessor(honeycomb.NewBaggageSpanProcessor())
	otel.SetTracerProvider(tp)
	a.tracer = tp.Tracer(packageName)

	a.metricsExporter, err = otlpmetricgrpc.New(ctx, otlpmetricgrpc.WithGRPCConn(grpcConn))
	if err != nil {
		return errors.Wrap(err, "making otel metrics exporter")
	}

	a.closers = append(a.closers, closerOp{
		name: "tracer provider shutdown",
		closerFn: func(ctx context.Context) error {
			catcher := grip.NewBasicCatcher()
			catcher.Wrap(tp.Shutdown(ctx), "trace provider shutdown")
			catcher.Wrap(traceExporter.Shutdown(ctx), "trace exporter shutdown")
			catcher.Wrap(a.metricsExporter.Shutdown(ctx), "metrics exporter shutdown")
			catcher.Wrap(grpcConn.Close(), "closing gRPC connection")

			return catcher.Resolve()
		},
	})

	return nil
}

func (a *Agent) startMetrics(ctx context.Context, tc *internal.TaskConfig) (func(context.Context) error, error) {
	if a.metricsExporter == nil {
		return nil, nil
	}

	r, err := hostResource(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "making resource")
	}

	meterProvider := sdk.NewMeterProvider(
		sdk.WithResource(r),
		sdk.WithReader(sdk.NewPeriodicReader(a.metricsExporter, sdk.WithInterval(exportInterval), sdk.WithTimeout(exportTimeout))),
	)

	return meterProvider.Shutdown, instrumentMeter(meterProvider.Meter(packageName), tc)
}

func instrumentMeter(meter metric.Meter, tc *internal.TaskConfig) error {
	catcher := grip.NewBasicCatcher()
	cpuPercent, err := meter.Float64ObservableGauge("cpu_percent")
	catcher.Add(err)
	memoryPercent, err := meter.Float64ObservableGauge("memory_percent")
	catcher.Add(err)

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		percentages, err := cpu.PercentWithContext(ctx, 0, false)
		if err != nil {
			return errors.Wrap(err, "getting CPU percentages")
		}
		if len(percentages) != 1 {
			return errors.Wrap(err, "CPU percentages had an unexpected length")
		}
		observer.ObserveFloat64(cpuPercent, percentages[0], tc.TaskAttributes()...)

		return nil
	}, cpuPercent)
	catcher.Add(err)

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		stat, err := mem.VirtualMemoryWithContext(ctx)
		if err != nil {
			return errors.Wrap(err, "getting virtual memory stats")
		}
		observer.ObserveFloat64(memoryPercent, stat.UsedPercent, tc.TaskAttributes()...)

		return nil
	}, memoryPercent)
	catcher.Add(err)

	return catcher.Resolve()
}

func hostResource(ctx context.Context) (*resource.Resource, error) {
	return resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("evergreen-agent")),
		resource.WithAttributes(semconv.ServiceVersion(evergreen.BuildRevision)),
		resource.WithDetectors(ec2.NewResourceDetector(), ecs.NewResourceDetector()),
	)
}
