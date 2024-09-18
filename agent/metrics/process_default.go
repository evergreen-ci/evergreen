//go:build !windows

package metrics

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/process"
	"go.opentelemetry.io/otel/metric"
)

func addProcessMetrics(meter metric.Meter) error {
	processCountRunning, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.running", processCountPrefix), metric.WithUnit("{process}"), metric.WithDescription("Total number of running processes"))
	if err != nil {
		return errors.Wrap(err, "making running process counter")
	}
	processCountSleeping, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.sleeping", processCountPrefix), metric.WithUnit("{process}"), metric.WithDescription("Total number of sleeping processes"))
	if err != nil {
		return errors.Wrap(err, "making sleeping process counter")
	}
	processCountZombie, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.zombie", processCountPrefix), metric.WithUnit("{process}"), metric.WithDescription("Total number of zombie processes"))
	if err != nil {
		return errors.Wrap(err, "making zombie process counter")
	}
	processCountStopped, err := meter.Int64ObservableUpDownCounter(fmt.Sprintf("%s.stopped", processCountPrefix), metric.WithUnit("{process}"), metric.WithDescription("Total number of stopped processes"))
	if err != nil {
		return errors.Wrap(err, "making stopped process counter")
	}

	_, err = meter.RegisterCallback(func(ctx context.Context, observer metric.Observer) error {
		processes, err := process.ProcessesWithContext(ctx)
		if err != nil {
			return errors.Wrap(err, "getting processes")
		}
		var running, sleeping, zombie, stopped int
		for _, p := range processes {
			statuses, err := p.StatusWithContext(ctx)
			if err != nil {
				continue
			}
			switch {
			case utility.StringSliceContains(statuses, process.Running):
				running++
			case utility.StringSliceContains(statuses, process.Sleep):
				sleeping++
			case utility.StringSliceContains(statuses, process.Zombie):
				zombie++
			case utility.StringSliceContains(statuses, process.Stop):
				stopped++
			}
		}

		observer.ObserveInt64(processCountRunning, int64(running))
		observer.ObserveInt64(processCountSleeping, int64(sleeping))
		observer.ObserveInt64(processCountZombie, int64(zombie))
		observer.ObserveInt64(processCountStopped, int64(stopped))

		return nil
	}, processCountRunning, processCountSleeping, processCountZombie, processCountStopped)
	return errors.Wrap(err, "registering process count callback")
}
