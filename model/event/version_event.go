package event

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeVersion, versionEventDataFactory)
	registry.AllowSubscription(ResourceTypeVersion, VersionStateChange)
	registry.AllowSubscription(ResourceTypeVersion, VersionGithubCheckFinished)
	registry.AllowSubscription(ResourceTypeVersion, VersionChildrenCompletion)
}

func versionEventDataFactory() any {
	return &VersionEventData{}
}

const (
	ResourceTypeVersion        = "VERSION"
	VersionStateChange         = "STATE_CHANGE"
	VersionGithubCheckFinished = "GITHUB_CHECK_FINISHED"
	VersionChildrenCompletion  = "CHILDREN_FINISHED"
)

type VersionEventData struct {
	Status            string `bson:"status,omitempty" json:"status,omitempty"`
	GithubCheckStatus string `bson:"github_check_status,omitempty" json:"github_check_status,omitempty"`
	Author            string `bson:"author,omitempty" json:"author,omitempty"`
}

// logEventWithRetry attempts to log an event with a detached context and retries on failure.
// This ensures that event logging is not affected by parent context cancellation.
// The logFields parameter contains contextual information (e.g. version_id, status) that will be included in the error log if all retry attempts fail.
func logEventWithRetry(event EventLogEntry, logFields message.Fields) {
	const (
		maxRetries     = 3
		contextTimeout = 10 * time.Second
		initialBackoff = 100 * time.Millisecond
	)

	var lastErr error
	for attempt := 0; attempt < maxRetries; attempt++ {
		eventCtx, cancel := context.WithTimeout(context.Background(), contextTimeout)
		err := event.Log(eventCtx)
		cancel()

		if err == nil {
			return
		}

		lastErr = err
		if attempt < maxRetries-1 {
			time.Sleep(initialBackoff * time.Duration(1<<attempt))
		}
	}

	fields := message.Fields{
		"message": "error logging event after all retries",
		"source":  "event-log-fail",
		"retries": maxRetries,
	}
	for k, v := range logFields {
		fields[k] = v
	}
	grip.Error(message.WrapError(lastErr, fields))
}

func LogVersionStateChangeEvent(ctx context.Context, id, newStatus string) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   id,
		ResourceType: ResourceTypeVersion,
		EventType:    VersionStateChange,
		Data: &VersionEventData{
			Status: newStatus,
		},
	}

	logEventWithRetry(event, message.Fields{
		"resource_type": ResourceTypeVersion,
		"version_id":    id,
		"status":        newStatus,
	})
}

func LogVersionGithubCheckFinishedEvent(ctx context.Context, id, newStatus string) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   id,
		ResourceType: ResourceTypeVersion,
		EventType:    VersionGithubCheckFinished,
		Data: &VersionEventData{
			GithubCheckStatus: newStatus,
		},
	}

	logEventWithRetry(event, message.Fields{
		"resource_type":       ResourceTypeVersion,
		"version_id":          id,
		"github_check_status": newStatus,
	})
}

func LogVersionChildrenCompletionEvent(ctx context.Context, id, status, author string) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   id,
		ResourceType: ResourceTypeVersion,
		EventType:    VersionChildrenCompletion,
		Data: &VersionEventData{
			Status: status,
			Author: author,
		},
	}

	logEventWithRetry(event, message.Fields{
		"resource_type": ResourceTypeVersion,
		"version_id":    id,
		"status":        status,
		"author":        author,
	})
}
