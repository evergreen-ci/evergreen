package commitqueue

import (
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

type commitQueueDequeueLogger struct {
	*send.Base
}

// NewCommitQueueDequeueLogger constructs a Sender that dequeues an item from
// a commit queue
func NewCommitQueueDequeueLogger(name string, l send.LevelInfo) (send.Sender, error) {
	cq := &commitQueueDequeueLogger{
		Base: send.NewBase(name),
	}

	if err := cq.SetLevel(l); err != nil {
		return nil, err
	}

	cq.SetName(name)

	if err := cq.SetErrorHandler(send.ErrorHandlerFromSender(grip.GetSender())); err != nil {
		return nil, errors.Wrap(err, "can't set commit queue dequeue error handler")
	}

	return cq, nil
}

// Send post issues via jiraCommentJournal with information in the message.Composer
func (cq *commitQueueDequeueLogger) Send(m message.Composer) {
	if !cq.Level().ShouldLog(m) {
		return
	}

	dequeue, ok := m.Raw().(*DequeueItem)
	if !ok {
		cq.ErrorHandler(errors.Errorf("message of type *DequeueItem is really of type %T", m), m)
		return
	}

	queue, err := FindOneId(dequeue.ProjectID)
	if err != nil {
		cq.ErrorHandler(errors.Wrapf(err, "no matching commit queue for project %s", dequeue.ProjectID), m)
		return
	}

	found, err := queue.Remove(dequeue.Item)
	if err != nil {
		cq.ErrorHandler(errors.Wrap(err, "can't remove item"), m)
		return
	}
	if !found {
		cq.ErrorHandler(errors.Errorf("item %s not found on queue %s", dequeue.Item, dequeue.ProjectID), m)
		return
	}

	if err = queue.SetProcessing(false); err != nil {
		cq.ErrorHandler(errors.Wrap(err, "can't set processing to false"), m)
		return
	}
}
