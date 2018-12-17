package data

import (
	"github.com/evergreen-ci/evergreen/model/commitq"
	"github.com/pkg/errors"
)

type DBCommitQConnector struct{}

func (pc *DBCommitQConnector) EnqueueItem(queueId string, item string) error {
	q, err := commitq.FindOneId(queueId)
	if err != nil {
		return errors.Wrapf(err, "can't query for queue id %s", queueId)
	}
	if q == nil {
		return errors.Errorf("commit queue not found for id %s", queueId)
	}

	if err := q.Enqueue(item); err != nil {
		return errors.Wrap(err, "can't enqueue item")
	}

	return nil
}

type MockCommitQConnector struct {
	Queue map[string][]string
}

func (pc *MockCommitQConnector) EnqueueItem(queueId string, item string) error {
	if pc.Queue == nil {
		pc.Queue = make(map[string][]string)
	}
	pc.Queue[queueId] = append(pc.Queue[queueId], item)

	return nil
}
