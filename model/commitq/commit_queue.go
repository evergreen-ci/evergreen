package commitq

import (
	"github.com/pkg/errors"
)

type CommitQueue struct {
	ProjectID    string   `bson:"_id"`
	Queue        []string `bson:"queue"`
	MergeAction  string   `bson:"merge"`
	StatusAction string   `bson:"status"`
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

func (q CommitQueue) IsEmpty() bool {
	return len(q.Queue) == 0
}

func (q CommitQueue) All() []string {
	return q.Queue
}

func (q *CommitQueue) Remove(item string) error {
	itemIndex := q.findItem(item)
	if itemIndex < 0 {
		return errors.New("item not in queue")
	}

	if err := remove(q.ProjectID, item); err != nil {
		return errors.Wrap(err, "can't remove item")
	}

	q.Queue = append(q.Queue[:itemIndex], q.Queue[itemIndex+1:]...)
	return nil
}

func (q *CommitQueue) RemoveAll() error {
	if err := removeAll(q.ProjectID); err != nil {
		return errors.Wrap(err, "can't clear queue")
	}
	q.Queue = []string{}
	return nil
}

func (q *CommitQueue) UpdateMergeAction(mergeAction string) error {
	if err := updateMerge(q.ProjectID, mergeAction); err != nil {
		return errors.Wrap(err, "can't update merge")
	}
	q.MergeAction = mergeAction
	return nil
}

func (q *CommitQueue) UpdateStatusAction(statusAction string) error {
	if err := updateStatus(q.ProjectID, statusAction); err != nil {
		return errors.Wrap(err, "can't update merge")
	}
	q.StatusAction = statusAction
	return nil
}

func (q CommitQueue) findItem(item string) int {
	for i, queued := range q.Queue {
		if queued == item {
			return i
		}
	}
	return -1
}
