package event

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeBuild, func() any { return &BuildEventData{} })

	registry.AllowSubscription(ResourceTypeBuild, BuildStateChange)
	registry.AllowSubscription(ResourceTypeBuild, BuildGithubCheckFinished)
}

const (
	ResourceTypeBuild = "BUILD"

	BuildStateChange         = "STATE_CHANGE"
	BuildGithubCheckFinished = "GITHUB_CHECK_FINISHED"
)

type BuildEventData struct {
	Status            string `bson:"status,omitempty" json:"status"`
	GithubCheckStatus string `bson:"github_check_status,omitempty" json:"github_check_status"`
}

func LogBuildStateChangeEvent(ctx context.Context, id, status string) {
	event := EventLogEntry{
		Timestamp:  time.Now(),
		ResourceId: id,
		EventType:  BuildStateChange,
		Data: &BuildEventData{
			Status: status,
		},
		ResourceType: ResourceTypeBuild,
	}

	if err := event.Log(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": event.ResourceType,
			"event_type":    BuildStateChange,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

func LogBuildGithubCheckFinishedEvent(ctx context.Context, id, status string) {
	event := EventLogEntry{
		Timestamp:  time.Now(),
		ResourceId: id,
		EventType:  BuildGithubCheckFinished,
		Data: &BuildEventData{
			GithubCheckStatus: status,
		},
		ResourceType: ResourceTypeBuild,
	}
	if err := event.Log(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": event.ResourceType,
			"event_type":    BuildGithubCheckFinished,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}
