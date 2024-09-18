//go:build !darwin

package metrics

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/cpu"
	"go.opentelemetry.io/otel/metric"
)

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
