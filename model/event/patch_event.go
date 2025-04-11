package event

import (
	"context"
	"time"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypePatch, patchEventDataFactory)
	registry.AllowSubscription(ResourceTypePatch, PatchStateChange)
	registry.AllowSubscription(ResourceTypePatch, PatchChildrenCompletion)
}

func patchEventDataFactory() any {
	return &PatchEventData{}
}

const (
	ResourceTypePatch = "PATCH"

	PatchStateChange        = "STATE_CHANGE"
	PatchChildrenCompletion = "CHILDREN_FINISHED"
)

type PatchEventData struct {
	Status string `bson:"status,omitempty" json:"status,omitempty"`
	Author string `bson:"author,omitempty" json:"author,omitempty"`
}

func LogPatchStateChangeEvent(ctx context.Context, id, newStatus string) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   id,
		ResourceType: ResourceTypePatch,
		EventType:    PatchStateChange,
		Data: &PatchEventData{
			Status: newStatus,
		},
	}

	if err := event.Log(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypePatch,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

func LogPatchChildrenCompletionEvent(ctx context.Context, id, status, author string) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   id,
		ResourceType: ResourceTypePatch,
		EventType:    PatchChildrenCompletion,
		Data: &PatchEventData{
			Status: status,
			Author: author,
		},
	}

	if err := event.Log(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypePatch,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}
