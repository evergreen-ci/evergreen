package commitqueue

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
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

func (cq *commitQueueDequeueLogger) Send(m message.Composer) {
	if !cq.Level().ShouldLog(m) {
		return
	}

	if err := cq.doSend(m); err != nil {
		cq.ErrorHandler()(errors.Wrap(err, "can't process dequeue message"), m)
	}
}

func (cq *commitQueueDequeueLogger) doSend(m message.Composer) error {
	dequeue, ok := m.Raw().(*DequeueItem)
	if !ok {
		return errors.Errorf("message of type *DequeueItem is really of type %T", m)
	}

	status := evergreen.MergeTestSucceeded
	if dequeue.Status == evergreen.PatchFailed {
		status = evergreen.MergeTestFailed
	}
	event.LogCommitQueueConcludeTest(dequeue.Item, status)

	queue, err := FindOneId(dequeue.ProjectID)
	if err != nil {
		return errors.Wrapf(err, "no matching commit queue for project %s", dequeue.ProjectID)
	}

	found, err := queue.Remove(dequeue.Item)
	if err != nil {
		return errors.Wrap(err, "can't remove item")
	}
	if !found {
		return errors.Errorf("item %s not found on queue %s", dequeue.Item, dequeue.ProjectID)
	}

	if err = queue.SetProcessing(false); err != nil {
		return errors.Wrap(err, "can't set processing to false")
	}

	return nil
}
