package internal

import (
	"github.com/golang/protobuf/ptypes"
	"github.com/mongodb/ftdc/events"
)

func (m *EventMetrics) Export() *events.Performance {
	dur, _ := ptypes.Duration(m.Timers.Duration)
	total, _ := ptypes.Duration(m.Timers.Total)
	time, _ := ptypes.Timestamp(m.Time)

	out := &events.Performance{
		Timestamp: time,
		ID:        m.Id,
		Timers: events.PerformanceTimers{
			Duration: dur,
			Total:    total,
		},
	}

	if m.Counters != nil {
		out.Counters.Number = m.Counters.Number
		out.Counters.Operations = m.Counters.Ops
		out.Counters.Size = m.Counters.Size
		out.Counters.Errors = m.Counters.Errors
	}

	if m.Gauges != nil {
		out.Gauges.State = m.Gauges.State
		out.Gauges.Workers = m.Gauges.Workers
		out.Gauges.Failed = m.Gauges.Failed
	}

	return out
}
