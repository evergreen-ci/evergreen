package commitqueue

import (
	"time"

	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
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
	// QueueLengthAtEnqueue is the length of the queue when the item was enqueued. Used for tracking the speed of the
	// commit queue as this value is logged when a commit queue item is processed.
	QueueLengthAtEnqueue int `bson:"queue_length_at_enqueue"`
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
	item.QueueLengthAtEnqueue = len(q.Queue)
	if err := add(q.ProjectID, item); err != nil {
		return 0, errors.Wrapf(err, "adding '%s' to queue for project '%s'", item.Issue, q.ProjectID)
	}

	q.Queue = append(q.Queue, item)
	grip.Info(message.Fields{
		"source":       "commit queue",
		"item":         item,
		"project_id":   q.ProjectID,
		"queue_length": len(q.Queue),
		"message":      "enqueued commit queue item",
	})
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
	item.QueueLengthAtEnqueue = len(q.Queue)
	if err := addAtPosition(q.ProjectID, item, newPos); err != nil {
		return 0, errors.Wrapf(err, "force adding '%s' to queue for project '%s'", item.Issue, q.ProjectID)
	}
	if len(q.Queue) == 0 {
		q.Queue = append(q.Queue, item)
		return newPos, nil
	}
	q.Queue = append(q.Queue[:newPos], append([]CommitQueueItem{item}, q.Queue[newPos:]...)...)

	grip.Info(message.Fields{
		"source":       "commit queue",
		"item":         item,
		"project_id":   q.ProjectID,
		"queue_length": len(q.Queue),
		"position":     newPos,
		"message":      "enqueued commit queue item at front",
	})
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
		return nil, errors.Wrap(err, "removing item")
	}

	q.Queue = append(q.Queue[:itemIndex], q.Queue[itemIndex+1:]...)
	grip.Info(message.Fields{
		"source":       "commit queue",
		"item":         item,
		"project_id":   q.ProjectID,
		"queue_length": len(q.Queue),
		"message":      "removed item from commit queue",
	})
	return &item, nil
}

func (q *CommitQueue) UpdateVersion(item *CommitQueueItem) error {
	for i, currentEntry := range q.Queue {
		if currentEntry.Issue == item.Issue {
			q.Queue[i].Version = item.Version
			q.Queue[i].ProcessingStartTime = item.ProcessingStartTime
		}
	}
	return errors.Wrap(addVersionAndTime(q.ProjectID, *item), "updating version")
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
		return errors.Wrap(err, "finding commit queue")
	}
	if cq == nil {
		cq = &CommitQueue{ProjectID: id}
		if err = InsertQueue(cq); err != nil {
			return errors.Wrap(err, "inserting new commit queue")
		}
	}
	return nil
}

func ClearAllCommitQueues() (int, error) {
	clearedCount, err := clearAll()
	if err != nil {
		return 0, errors.Wrap(err, "clearing all commit queues")
	}

	return clearedCount, nil
}

// EnqueuePRInfo holds information necessary to enqueue a PR to the commit queue.
type EnqueuePRInfo struct {
	Username      string
	Owner         string
	Repo          string
	PR            int
	CommitMessage string
}
