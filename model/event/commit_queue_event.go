package event

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeCommitQueue, func() interface{} { return &CommitQueueEventData{} })

	registry.AllowSubscription(ResourceTypeCommitQueue, CommitQueueStartTest)
	registry.AllowSubscription(ResourceTypeCommitQueue, CommitQueueConcludeTest)
	registry.AllowSubscription(ResourceTypeCommitQueue, CommitQueueEnqueueFailed)
}

const (
	ResourceTypeCommitQueue  = "COMMIT_QUEUE"
	CommitQueueEnqueueFailed = "ENQUEUE_FAILED"
	CommitQueueStartTest     = "START_TEST"
	CommitQueueConcludeTest  = "CONCLUDE_TEST"
)

type PRInfo struct {
	Owner           string `bson:"owner"`
	Repo            string `bson:"repo"`
	Ref             string `bson:"ref"`
	PRNum           int    `bson:"pr_number"`
	CommitTitle     string `bson:"commit_title"`
	MessageOverride string `bson:"message_override"`
}

func logCommitQueueEvent(patchID, eventType string, data *CommitQueueEventData) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   patchID,
		ResourceType: ResourceTypeCommitQueue,
		EventType:    eventType,
		Data:         data,
	}

	if err := event.Log(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeCommitQueue,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

type CommitQueueEventData struct {
	Status string `bson:"status,omitempty" json:"status,omitempty"`
	Error  string `bson:"error,omitempty" json:"error,omitempty"`
}

func LogCommitQueueStartTestEvent(patchID string) {
	data := &CommitQueueEventData{
		Status: evergreen.MergeTestStarted,
	}
	logCommitQueueEvent(patchID, CommitQueueStartTest, data)
}

func LogCommitQueueConcludeTest(patchID, status string) {
	data := &CommitQueueEventData{
		Status: status,
	}
	logCommitQueueEvent(patchID, CommitQueueConcludeTest, data)
}

func LogCommitQueueConcludeWithError(patchID, status string, err error) {
	data := &CommitQueueEventData{
		Status: status,
	}
	if err != nil {
		data.Error = err.Error()
	}
	logCommitQueueEvent(patchID, CommitQueueConcludeTest, data)
}

func LogCommitQueueEnqueueFailed(patchID string, err error) {
	data := &CommitQueueEventData{
		Status: evergreen.EnqueueFailed,
		Error:  err.Error(),
	}
	logCommitQueueEvent(patchID, CommitQueueEnqueueFailed, data)

}
