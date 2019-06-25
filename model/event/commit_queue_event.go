package event

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func commitQueueEventDataFactory() interface{} {
	return &CommitQueueEventData{}
}

const (
	ResourceTypeCommitQueue = "COMMIT_QUEUE"
	CommitQueueStartTest    = "START_TEST"
	CommitQueueConcludeTest = "CONCLUDE_TEST"
)

type PRInfo struct {
	Owner       string `bson:"owner"`
	Repo        string `bson:"repo"`
	Ref         string `bson:"ref"`
	PRNum       int    `bson:"pr_number"`
	CommitTitle string `bson:"commit_title"`
}

func logCommitQueueEvent(patchID, eventType string, data *CommitQueueEventData) {
	event := EventLogEntry{
		Timestamp:    time.Now().Truncate(0).Round(time.Millisecond),
		ResourceId:   patchID,
		ResourceType: ResourceTypeCommitQueue,
		EventType:    eventType,
		Data:         data,
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeCommitQueue,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

type CommitQueueEventData struct {
	Status string `bson:"status,omitempty" json:"status,omitempty"`
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
