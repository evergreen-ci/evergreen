package event

import (
	"time"

	"github.com/mongodb/grip"
)

func init() {
	registry.AddType(ResourceTypeScheduler, schedulerEventFactory)
}

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
	TaskQueueInfo TaskQueueInfo `bson:"tq_info" json:"task_queue_info"`
	DistroId      string        `bson:"d_id" json:"distro_id"`
}

// LogSchedulerEvent takes care of logging the statistics about the scheduler at a given time.
// The ResourceId is the time that the scheduler runs.
func LogSchedulerEvent(eventData SchedulerEventData) {
	event := EventLogEntry{
		Timestamp:    time.Now(),
		ResourceId:   eventData.DistroId,
		EventType:    EventSchedulerRun,
		Data:         eventData,
		ResourceType: ResourceTypeScheduler,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Errorf("Error logging host event: %+v", err)
	}
}
