package commitqueue

import (
	"github.com/pkg/errors"
)

const (
	triggerComment = "evergreen merge"
)

type CommitQueue struct {
	ProjectID string   `bson:"_id"`
	Queue     []string `bson:"queue"`
}

func InsertQueue(q *CommitQueue) error {
	return insert(q)
}

func (q *CommitQueue) Enqueue(item string) error {
	if !(q.findItem(item) < 0) {
		return errors.New("item already in queue")
	}

	if err := add(q.ProjectID, q.Queue, item); err != nil {
		return err
	}

	q.Queue = append(q.Queue, item)
	return nil
}

func (q *CommitQueue) IsEmpty() bool {
	return len(q.Queue) == 0
}

func (q *CommitQueue) Next() string {
	if q.IsEmpty() {
		return ""
	}
	return q.Queue[0]
}

func (q *CommitQueue) All() []string {
	return q.Queue
}

func (q *CommitQueue) Remove(item string) (bool, error) {
	itemIndex := q.findItem(item)
	if itemIndex < 0 {
		return false, nil
	}

	if err := remove(q.ProjectID, item); err != nil {
		return false, errors.Wrap(err, "can't remove item")
	}

	q.Queue = append(q.Queue[:itemIndex], q.Queue[itemIndex+1:]...)
	return true, nil
}

func (q *CommitQueue) RemoveAll() error {
	if err := removeAll(q.ProjectID); err != nil {
		return errors.Wrap(err, "can't clear queue")
	}
	q.Queue = []string{}
	return nil
}

func (q *CommitQueue) findItem(item string) int {
	for i, queued := range q.Queue {
		if queued == item {
			return i
		}
	}
	return -1
}

func TriggersCommitQueue(commentAction string, comment string) bool {
	if commentAction == "delete" {
		return false
	}
	return comment == triggerComment
}
