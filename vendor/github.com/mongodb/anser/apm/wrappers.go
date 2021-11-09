package apm

import (
	"context"
	"time"

	"github.com/mongodb/ftdc"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/recovery"
)

type loggingMonitor struct {
	interval time.Duration
	Monitor
}

// NewLoggingMonitor wraps an existing logging monitor that
// automatically logs the output the default grip logger on the
// specified interval.
func NewLoggingMonitor(ctx context.Context, dur time.Duration, m Monitor) Monitor {
	impl := &loggingMonitor{
		interval: dur,
		Monitor:  m,
	}
	go impl.flusher(ctx)
	return impl
}

func (m *loggingMonitor) flusher(ctx context.Context) {
	defer recovery.LogStackTraceAndContinue("logging driver apm collector")
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			grip.Info(m.Monitor.Rotate().Message())
		}
	}
}

type ftdcCollector struct {
	collector ftdc.Collector
	interval  time.Duration
	Monitor
}

// NewFTDCMonitor produces a monitor implementation that automatically
// flushes the event data to an FTDC data collector.
func NewFTDCMonitor(ctx context.Context, dur time.Duration, collector ftdc.Collector, m Monitor) Monitor {
	impl := &ftdcCollector{
		interval:  dur,
		collector: collector,
		Monitor:   m,
	}
	go impl.flusher(ctx)
	return impl
}

func (m *ftdcCollector) flusher(ctx context.Context) {
	defer recovery.LogStackTraceAndContinue("timeseries driver apm collector")
	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			grip.Warning(m.collector.Add(m.Monitor.Rotate().Document()))
		}
	}
}
