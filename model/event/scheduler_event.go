package event

import (
	"time"

	"github.com/mongodb/grip"
)

const (
	// resource type
	ResourceTypeScheduler = "SCHEDULER"

	// event types
	EventSchedulerRun = "SCHEDULER_RUN"
)

type TaskQueueInfo struct {
	TaskQueueLength  int           `bson:"tq_l" json:"task_queue_length"`
	NumHostsRunning  int           `bson:"n_h" json:"num_hosts_running"`
	ExpectedDuration time.Duration `bson:"ex_d" json:"expected_duration,"`
}

// implements EventData
type SchedulerEventData struct {
	// necessary for IsValid
	ResourceType  string        `bson:"r_type,omitempty" json:"resource_type,omitempty"`
	TaskQueueInfo TaskQueueInfo `bson:"tq_info" json:"task_queue_info"`
	DistroId      string        `bson:"d_id" json:"distro_id"`
}

func (sed SchedulerEventData) IsValid() bool {
	return sed.ResourceType == ResourceTypeScheduler
}

// LogSchedulerEvent takes care of logging the statistics about the scheduler at a given time.
// The ResourceId is the time that the scheduler runs.
func LogSchedulerEvent(eventData SchedulerEventData) {
	eventData.ResourceType = ResourceTypeScheduler
	event := EventLogEntry{
		Timestamp:  time.Now(),
		ResourceId: eventData.DistroId,
		EventType:  EventSchedulerRun,
		Data:       eventData,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Errorf("Error logging host event: %+v", err)
	}
}
