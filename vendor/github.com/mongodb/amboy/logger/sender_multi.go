package logger

import (
	"context"
	"sync"

	"github.com/mongodb/amboy"
	"github.com/mongodb/amboy/queue"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

type multiQueueSender struct {
	mu       sync.RWMutex
	ctx      context.Context
	queue    amboy.Queue
	senders  []send.Sender
	canceler context.CancelFunc
	send.Base
}

func newMultiSender(ctx context.Context, q amboy.Queue, senders []send.Sender) *multiQueueSender {
	return &multiQueueSender{
		senders: senders,
		queue:   q,
		ctx:     ctx,
		Base:    *send.NewBase(""),
	}
}

// MakeQueueMultiSender returns a amboy.Queue-backed sender that
// distributes messages to multiple backing Sender implementations.
//
// In most respects this Sender is like any other; however, messages
// may be delivered out of order, and it pushes level-based filtering
// down to constituent senders. Additionally, the close method does
// not close the constituent senders.
//
// Internally each message maps to a single job which calls send on
// each constituent sender independently. This means that if a single
// sender is blocking, then that sender may prevent other senders from
// receiving the message.
func MakeQueueMultiSender(ctx context.Context, q amboy.Queue, senders ...send.Sender) send.Sender {
	return newMultiSender(ctx, q, senders)
}

// NewQueueMultiSender returns a queue-backed wrapper of a group of
// senders, but constructs the queue independently. When the Close
// method on this sender, the queue is canceled, which may leave some
// pending messages unsent.
func NewQueueMultiSender(ctx context.Context, workers, capacity int, senders ...send.Sender) (send.Sender, error) {
	q := queue.NewLocalLimitedSize(workers, capacity)
	s := newMultiSender(ctx, q, senders)

	s.ctx, s.canceler = context.WithCancel(s.ctx)
	if err := q.Start(s.ctx); err != nil {
		return nil, err
	}

	return s, nil
}

func (s *multiQueueSender) Send(m message.Composer) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	s.ErrorHandler()(s.queue.Put(s.ctx, NewMultiSendMessageJob(m, s.senders)), m)
}

func (s *multiQueueSender) Flush(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if !amboy.Wait(ctx, s.queue) {
		return ctx.Err()
	}

	catcher := grip.NewBasicCatcher()
	for _, sender := range s.senders {
		catcher.Add(sender.Flush(ctx))
	}

	return catcher.Resolve()
}

func (s *multiQueueSender) Close() error {
	if s.canceler != nil {
		s.canceler()
	}

	return nil
}
