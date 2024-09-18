//go:build windows

package agent

import (
	"context"

	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/process"
	"go.opentelemetry.io/otel/metric"
)

// addDiskMetrics is not supported for Windows.
func addDiskMetrics(_ context.Context, _ metric.Meter) error {
	return nil
}

// addProcessMetrics adds a metric for the total number of processes on Windows.
func addProcessMetrics(meter metric.Meter) error {
	processCount, err := meter.Int64ObservableUpDownCounter(processCountPrefix, metric.WithUnit("{process}"), metric.WithDescription("Total number of processes"))
	if err != nil {
		return errors.Wrap(err, "making process counter")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		processes, err := process.ProcessesWithContext(ctx)
		if err != nil {
			return errors.Wrap(err, "getting processes")
		}

		observer.ObserveInt64(processCount, int64(len(processes)))

		return nil
	}, processCount)
	return errors.Wrap(err, "registering process count callback")
}
