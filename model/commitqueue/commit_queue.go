package commitqueue

import (
	"strings"

	"github.com/pkg/errors"
)

const (
	triggerComment = "evergreen merge"
	PRPatchType    = "PR"
	CLIPatchType   = "CLI"
)

type Module struct {
	Module string `bson:"module" json:"module"`
	Issue  string `bson:"issue" json:"issue"`
}
type CommitQueueItem struct {
	Issue   string   `bson:"issue"`
	Modules []Module `bson:"modules"`
}
type CommitQueue struct {
	ProjectID  string            `bson:"_id"`
	Processing bool              `bson:"processing"`
	Queue      []CommitQueueItem `bson:"queue"`
}

func InsertQueue(q *CommitQueue) error {
	return insert(q)
}

func (q *CommitQueue) Enqueue(item CommitQueueItem) (int, error) {
	position := q.findItem(item.Issue)
	if !(position < 0) {
		return position + 1, errors.New("item already in queue")
	}

	if err := add(q.ProjectID, q.Queue, item); err != nil {
		return 0, err
	}

	q.Queue = append(q.Queue, item)
	return len(q.Queue), nil
}

func (q *CommitQueue) Next() *CommitQueueItem {
	if len(q.Queue) == 0 || q.Processing {
		return nil
	}

	return &q.Queue[0]
}

func (q *CommitQueue) Remove(issue string) (bool, error) {
	itemIndex := q.findItem(issue)
	if itemIndex < 0 {
		return false, nil
	}

	if err := remove(q.ProjectID, issue); err != nil {
		return false, errors.Wrap(err, "can't remove item")
	}

	q.Queue = append(q.Queue[:itemIndex], q.Queue[itemIndex+1:]...)

	// clearing the front of the queue
	if itemIndex == 0 {
		if err := q.SetProcessing(false); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (q *CommitQueue) findItem(issue string) int {
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
