package metrics

import (
	"context"
	"fmt"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/mem"
	"github.com/shirou/gopsutil/v3/net"
	"go.opentelemetry.io/otel/metric"
)

const (
	cpuTimeInstrumentPrefix = "system.cpu.time"
	cpuUtilInstrument       = "system.cpu.utilization"

	memoryUsageInstrumentPrefix = "system.memory.usage"
	memoryUtilizationInstrument = "system.memory.utilization"

	diskIOInstrumentPrefix         = "system.disk.io"
	diskOperationsInstrumentPrefix = "system.disk.operations"
	diskIOTimeInstrumentPrefix     = "system.disk.io_time"

	networkIOInstrumentPrefix = "system.network.io"

	processCountPrefix = "system.process.count"
)

func InstrumentMeter(ctx context.Context, meter metric.Meter) error {
	catcher := grip.NewBasicCatcher()

	catcher.Wrap(addCPUMetrics(meter), "adding CPU metrics")
	catcher.Wrap(addMemoryMetrics(meter), "adding memory metrics")
	catcher.Wrap(addDiskMetrics(ctx, meter), "adding disk metrics")
	catcher.Wrap(addNetworkMetrics(meter), "adding network metrics")
	catcher.Wrap(addProcessMetrics(meter), "adding process metrics")

	return catcher.Resolve()
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
