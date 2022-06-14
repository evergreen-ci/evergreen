package event

import (
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func buildEventDataFactory() interface{} {
	return &BuildEventData{}
}

const (
	ResourceTypeBuild = "BUILD"

	BuildStateChange         = "STATE_CHANGE"
	BuildGithubCheckFinished = "GITHUB_CHECK_FINISHED"
)

type BuildEventData struct {
	Status            string `bson:"status,omitempty" json:"status,omitempty"`
	GithubCheckStatus string `bson:"github_check_status,omitempty" json:"github_check_status,omitempty"`
}

func LogBuildStateChangeEvent(id, status string) {
	event := EventLogEntry{
		Timestamp:  time.Now(),
		ResourceId: id,
		EventType:  BuildStateChange,
		Data: &BuildEventData{
			Status: status,
		},
		ResourceType: ResourceTypeBuild,
	}

	if err := event.Log(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": event.ResourceType,
			"event_type":    BuildStateChange,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

func LogBuildGithubCheckFinishedEvent(id, status string) {
	event := EventLogEntry{
		Timestamp:  time.Now(),
		ResourceId: id,
		EventType:  BuildGithubCheckFinished,
		Data: &BuildEventData{
			GithubCheckStatus: status,
		},
		ResourceType: ResourceTypeBuild,
	}
	if err := event.Log(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": event.ResourceType,
			"event_type":    BuildGithubCheckFinished,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}
