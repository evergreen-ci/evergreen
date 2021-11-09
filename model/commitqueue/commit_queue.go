package commitqueue

import (
	"strings"
	"time"

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
	GithubContext     = "evergreen/commitqueue"
)

type Module struct {
	Module string `bson:"module" json:"module"`
	Issue  string `bson:"issue" json:"issue"`
}

func (m *Module) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(m) }
func (m *Module) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, m) }

type CommitQueueItem struct {
	Issue   string `bson:"issue"`
	PatchId string `bson:"patch_id,omitempty"`
	// Version is the ID of the version that is running the patch. It's also used to determine what entries are processing
	Version             string    `bson:"version,omitempty"`
	EnqueueTime         time.Time `bson:"enqueue_time"`
	ProcessingStartTime time.Time `bson:"processing_start_time"`
	Modules             []Module  `bson:"modules"`
	MessageOverride     string    `bson:"message_override"`
	Source              string    `bson:"source"`
}

func (i *CommitQueueItem) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(i) }
func (i *CommitQueueItem) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, i) }

type CommitQueue struct {
	ProjectID string            `bson:"_id"`
	Queue     []CommitQueueItem `bson:"queue,omitempty"`
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

// EnqueueAtFront adds a given item to the front of the _unprocessed_ items in the queue
func (q *CommitQueue) EnqueueAtFront(item CommitQueueItem) (int, error) {
	position := q.FindItem(item.Issue)
	if position >= 0 {
		return position, errors.New("item already in queue")
	}

	newPos := 0
	for i, item := range q.Queue {
		if item.Version != "" {
			newPos = i + 1
		} else {
			break
		}
	}
	item.EnqueueTime = time.Now()
	if err := addAtPosition(q.ProjectID, q.Queue, item, newPos); err != nil {
		return 0, errors.Wrapf(err, "can't force add '%s' to queue '%s'", item.Issue, q.ProjectID)
	}

	grip.Warning(message.Fields{
		"source":       "commit queue",
		"item_id":      item.Issue,
		"project_id":   q.ProjectID,
		"queue_length": len(q.Queue) + 1,
		"position":     newPos,
		"message":      "enqueued commit queue item at front",
	})
	if len(q.Queue) == 0 {
		q.Queue = append(q.Queue, item)
		return newPos, nil
	}
	q.Queue = append(q.Queue[:newPos], append([]CommitQueueItem{item}, q.Queue[newPos:]...)...)
	return newPos, nil
}

func (q *CommitQueue) Next() (CommitQueueItem, bool) {
	if len(q.Queue) == 0 {
		return CommitQueueItem{}, false
	}

	return q.Queue[0], true
}

func (q *CommitQueue) NextUnprocessed(n int) []CommitQueueItem {
	items := []CommitQueueItem{}
	for i, item := range q.Queue {
		if i+1 > n {
			return items
		}
		if item.Version != "" {
			continue
		}
		items = append(items, item)
	}

	return items
}

func (q *CommitQueue) Processing() bool {
	for _, item := range q.Queue {
		if item.Version != "" {
			return true
		}
	}

	return false
}

func (q *CommitQueue) Remove(issue string) (*CommitQueueItem, error) {
	itemIndex := q.FindItem(issue)
	if itemIndex < 0 {
		return nil, nil
	}
	item := q.Queue[itemIndex]

	if err := remove(q.ProjectID, item.Issue); err != nil {
		return nil, errors.Wrap(err, "can't remove item")
	}

	q.Queue = append(q.Queue[:itemIndex], q.Queue[itemIndex+1:]...)

	return &item, nil
}

func (q *CommitQueue) UpdateVersion(item CommitQueueItem) error {
	for i, currentEntry := range q.Queue {
		if currentEntry.Issue == item.Issue {
			q.Queue[i].Version = item.Version
		}
	}
	return errors.Wrapf(addVersionID(q.ProjectID, item), "error updating version")
}

func (q *CommitQueue) FindItem(issue string) int {
	for i, queued := range q.Queue {
		if queued.Issue == issue || queued.Version == issue || queued.PatchId == issue {
			return i
		}
	}
	return -1
}

// EnsureCommitQueueExistsForProject inserts a skeleton commit queue if project doesn't have one
func EnsureCommitQueueExistsForProject(id string) error {
	cq, err := FindOneId(id)
	if err != nil {
		return errors.Wrapf(err, "database error finding commit queue")
	}
	if cq == nil {
		cq = &CommitQueue{ProjectID: id}
		if err = InsertQueue(cq); err != nil {
			return errors.Wrapf(err, "problem inserting new commit queue")
		}
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
	removed, err := cq.Remove(issue)
	if err != nil {
		return removed, errors.Wrapf(err, "can't remove item '%s' from queue '%s'", issue, cq.ProjectID)
	}

	if removed == nil {
		return nil, nil
	}
	if versionExists {
		err = preventMergeForItem(*removed, user)
	}

	return removed, errors.Wrapf(err, "can't prevent merge for item '%s' on queue '%s'", issue, cq.ProjectID)
}

func preventMergeForItem(item CommitQueueItem, user string) error {
	// Disable the merge task
	mergeTask, err := task.FindMergeTaskForVersion(item.Version)
	if err != nil {
		return errors.Wrapf(err, "can't find merge task for '%s'", item.Issue)
	}
	if mergeTask == nil {
		return errors.New("merge task doesn't exist")
	}
	event.LogMergeTaskUnscheduled(mergeTask.Id, mergeTask.Execution, user)
	if err = mergeTask.SetDisabledPriority(user); err != nil {
		return errors.Wrap(err, "can't disable merge task")
	}

	return nil
}
