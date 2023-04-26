package agent

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/cpu"
	"github.com/shirou/gopsutil/v3/mem"
	"go.opentelemetry.io/contrib/detectors/aws/ec2"
	"go.opentelemetry.io/contrib/detectors/aws/ecs"
	"go.opentelemetry.io/otel/metric"
	sdk "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.17.0"
)

const (
	exportInterval = 15 * time.Second
	exportTimeout  = exportInterval * 2
)

func (a *Agent) initMetrics(ctx context.Context, tc *internal.TaskConfig) (func(context.Context) error, error) {
	if a.metricsExporter == nil {
		return nil, nil
	}

	r, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName("evergreen-agent")),
		resource.WithAttributes(semconv.ServiceVersion(evergreen.BuildRevision)),
		resource.WithDetectors(ec2.NewResourceDetector(), ecs.NewResourceDetector()),
	)
	if err != nil {
		return nil, errors.Wrap(err, "making resource")
	}

	meterProvider := sdk.NewMeterProvider(
		sdk.WithResource(r),
		sdk.WithReader(sdk.NewPeriodicReader(a.metricsExporter, sdk.WithInterval(exportInterval), sdk.WithTimeout(exportTimeout))),
	)
	meter := meterProvider.Meter(
		"github.com/evergreen-ci/evergreen/agent",
	)

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

	return func(ctx context.Context) error {
		catcher := grip.NewBasicCatcher()
		catcher.Add(meterProvider.Shutdown(ctx))

		return nil
	}, catcher.Resolve()
}
