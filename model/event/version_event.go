package event

import (
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeVersion, versionEventDataFactory)
	registry.AllowSubscription(ResourceTypeVersion, VersionStateChange)
	registry.AllowSubscription(ResourceTypeVersion, VersionGithubCheckFinished)
}

func versionEventDataFactory() interface{} {
	return &VersionEventData{}
}

const (
	ResourceTypeVersion        = "VERSION"
	VersionStateChange         = "STATE_CHANGE"
	VersionGithubCheckFinished = "GITHUB_CHECK_FINISHED"
)

type VersionEventData struct {
	Status            string `bson:"status,omitempty" json:"status,omitempty"`
	GithubCheckStatus string `bson:"github_check_status,omitempty" json:"github_check_status,omitempty"`
}

func LogVersionStateChangeEvent(id, newStatus string) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   id,
		ResourceType: ResourceTypeVersion,
		EventType:    VersionStateChange,
		Data: &VersionEventData{
			Status: newStatus,
		},
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeVersion,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

func LogVersionGithubCheckFinishedEvent(id, newStatus string) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   id,
		ResourceType: ResourceTypeVersion,
		EventType:    VersionGithubCheckFinished,
		Data: &VersionEventData{
			GithubCheckStatus: newStatus,
		},
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeVersion,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}
