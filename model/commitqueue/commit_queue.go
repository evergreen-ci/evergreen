package commitqueue

import (
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	triggerComment    = "evergreen merge"
	SourcePullRequest = "PR"
	SourceDiff        = "diff"
)

type Module struct {
	Module string `bson:"module" json:"module"`
	Issue  string `bson:"issue" json:"issue"`
}

func (m *Module) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(m) }
func (m *Module) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, m) }

type CommitQueueItem struct {
	Issue           string    `bson:"issue"`
	Version         string    `bson:"version,omitempty"`
	EnqueueTime     time.Time `bson:"enqueue_time"`
	Modules         []Module  `bson:"modules"`
	MessageOverride string    `bson:"message_override"`
	Source          string    `bson:"source"`
}

func (i *CommitQueueItem) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(i) }
func (i *CommitQueueItem) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, i) }

type CommitQueue struct {
	ProjectID             string            `bson:"_id"`
	Processing            bool              `bson:"processing"`
	ProcessingUpdatedTime time.Time         `bson:"processing_updated_time"`
	Queue                 []CommitQueueItem `bson:"queue,omitempty"`
}

func (q *CommitQueue) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(q) }
func (q *CommitQueue) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, q) }

func InsertQueue(q *CommitQueue) error {
	return insert(q)
}

func (q *CommitQueue) Enqueue(item CommitQueueItem) (int, error) {
	position := q.FindItem(item.Issue)
	if position >= 0 {
		return position, errors.New("item already in queue")
	}

	item.EnqueueTime = time.Now()
	if err := add(q.ProjectID, q.Queue, item); err != nil {
		return 0, errors.Wrapf(err, "can't add '%s' to queue '%s'", item.Issue, q.ProjectID)
	}
	grip.Info(message.Fields{
		"source":       "commit queue",
		"item_id":      item.Issue,
		"project_id":   q.ProjectID,
		"queue_length": len(q.Queue),
		"message":      "enqueued commit queue item",
	})

	q.Queue = append(q.Queue, item)
	return len(q.Queue) - 1, nil
}

func (q *CommitQueue) EnqueueAtFront(item CommitQueueItem) (int, error) {
	position := q.FindItem(item.Issue)
	if position >= 0 {
		return position, errors.New("item already in queue")
	}

	item.EnqueueTime = time.Now()
	if err := addToFront(q.ProjectID, q.Queue, item); err != nil {
		return 0, errors.Wrapf(err, "can't force add '%s' to queue '%s'", item.Issue, q.ProjectID)
	}

	grip.Warning(message.Fields{
		"source":       "commit queue",
		"item_id":      item.Issue,
		"project_id":   q.ProjectID,
		"queue_length": len(q.Queue) + 1,
		"message":      "enqueued commit queue item at front",
		"force":        true,
	})
	if len(q.Queue) == 0 {
		q.Queue = append(q.Queue, item)
		return 0, nil
	}
	q.Queue = append([]CommitQueueItem{q.Queue[0], item}, q.Queue[1:]...)
	return 1, nil
}

func (q *CommitQueue) Next() (CommitQueueItem, bool) {
	if len(q.Queue) == 0 {
		return CommitQueueItem{}, false
	}

	return q.Queue[0], true
}

func (q *CommitQueue) Remove(issue string) (*CommitQueueItem, error) {
	itemIndex := q.FindItem(issue)
	if itemIndex < 0 {
		return nil, nil
	}
	item := q.Queue[itemIndex]

	if err := remove(q.ProjectID, issue); err != nil {
		return nil, errors.Wrap(err, "can't remove item")
	}

	q.Queue = append(q.Queue[:itemIndex], q.Queue[itemIndex+1:]...)

	// clearing the front of the queue
	if itemIndex == 0 {
		if err := q.SetProcessing(false); err != nil {
			return nil, errors.Wrap(err, "can't set processing to false")
		}
	}
	return &item, nil
}

func (q *CommitQueue) UpdateVersion(item CommitQueueItem) error {
	return errors.Wrapf(addVersionID(q.ProjectID, item), "error updating version")
}

func (q *CommitQueue) FindItem(issue string) int {
	for i, queued := range q.Queue {
		if queued.Issue == issue {
			return i
		}
	}
	return -1
}

func (q *CommitQueue) SetProcessing(status bool) error {
	q.Processing = status
	if err := setProcessing(q.ProjectID, status); err != nil {
		return errors.Wrapf(err, "can't set processing on queue id '%s'", q.ProjectID)
	}

	return nil
}

func TriggersCommitQueue(commentAction string, comment string) bool {
	if commentAction == "deleted" {
		return false
	}
	return strings.HasPrefix(comment, triggerComment)
}

func ClearAllCommitQueues() (int, error) {
	clearedCount, err := clearAll()
	if err != nil {
		return 0, errors.Wrap(err, "can't clear queue")
	}

	return clearedCount, nil
}

func RemoveCommitQueueItemForVersion(projectId, version string, user string) (*CommitQueueItem, error) {
	cq, err := FindOneId(projectId)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get commit queue for id '%s'", projectId)
	}
	if cq == nil {
		return nil, errors.Errorf("no commit queue found for '%s'", projectId)
	}

	issue := ""
	for _, item := range cq.Queue {
		if item.Version == version {
			issue = item.Issue
		}
	}
	if issue == "" {
		return nil, nil
	}

	return cq.RemoveItemAndPreventMerge(issue, true, user)
}

func (cq *CommitQueue) RemoveItemAndPreventMerge(issue string, versionExists bool, user string) (*CommitQueueItem, error) {

	head, valid := cq.Next()
	if !valid {
		return nil, nil
	}
	removed, err := cq.Remove(issue)
	if err != nil {
		return removed, errors.Wrapf(err, "can't remove item '%s' from queue '%s'", issue, cq.ProjectID)
	}

	if removed == nil || head.Issue != issue {
		return nil, nil
	}
	if versionExists {
		err = preventMergeForItem(head, user)
	}

	return removed, errors.Wrapf(err, "can't prevent merge for item '%s' on queue '%s'", issue, cq.ProjectID)
}

func preventMergeForItem(item CommitQueueItem, user string) error {
	if err := clearVersionPatchSubscriber(item.Version, event.CommitQueueDequeueSubscriberType); err != nil {
		return errors.Wrap(err, "can't clear subscriptions")
	}

	// Disable the merge task
	mergeTask, err := task.FindMergeTaskForVersion(item.Version)
	if err != nil {
		return errors.Wrapf(err, "can't find merge task for '%s'", item.Issue)
	}
	if mergeTask == nil {
		return errors.New("merge task doesn't exist")
	}
	event.LogMergeTaskUnscheduled(mergeTask.Id, mergeTask.Execution, user)
	if _, err = mergeTask.SetDisabledPriority(user); err != nil {
		return errors.Wrap(err, "can't disable merge task")
	}
	if err = build.SetCachedTaskActivated(mergeTask.BuildId, mergeTask.Id, false); err != nil {
		return errors.Wrapf(err, "error updating task cache for build %s", mergeTask.BuildId)
	}

	return nil
}

func clearVersionPatchSubscriber(versionID, subscriberType string) error {
	subscriptions, err := event.FindSubscriptions(event.ResourceTypePatch, []event.Selector{{Type: event.SelectorID, Data: versionID}})
	if err != nil {
		return errors.Wrapf(err, "can't find subscription to patch '%s'", versionID)
	}
	for _, subscription := range subscriptions {
		if subscription.Subscriber.Type == subscriberType {
			err = event.RemoveSubscription(subscription.ID)
			if err != nil {
				return errors.Wrapf(err, "can't remove subscription for '%s', type '%s'", versionID, subscriberType)
			}
		}
	}

	return nil
}
