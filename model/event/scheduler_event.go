package event

import (
	"time"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
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
	ResourceType  string                   `bson:"r_type" json:"resource_type"`
	TaskQueueInfo map[string]TaskQueueInfo `bson:"tq_info" json:"task_queue_info"`
}

func (sed SchedulerEventData) IsValid() bool {
	return sed.ResourceType == ResourceTypeScheduler
}

// LogSchedulerEvent takes care of logging the statistics about the scheduler at a given time.
// The ResourceId is the time that the scheduler runs.
func LogSchedulerEvent(eventData SchedulerEventData) {
	eventData.ResourceType = ResourceTypeScheduler
	event := Event{
		Timestamp:  time.Now(),
		ResourceId: time.Now().String(),
		EventType:  EventSchedulerRun,
		Data:       DataWrapper{eventData},
	}

	logger := NewDBEventLogger(Collection)
	if err := logger.LogEvent(event); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error logging host event: %v", err)
	}
}
