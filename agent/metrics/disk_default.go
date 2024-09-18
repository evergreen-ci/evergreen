//go:build !windows

package metrics

import (
	"context"
	"fmt"

	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/disk"
	"go.opentelemetry.io/otel/metric"
)

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
