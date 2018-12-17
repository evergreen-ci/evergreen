package send

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/mongodb/grip/message"
)

type asyncGroupSender struct {
	pipes   []chan message.Composer
	senders []Sender
	cancel  context.CancelFunc
	*Base
}

// NewAsyncGroupSender produces an implementation of the Sender
// interface that, like the MultiSender, distributes a single message
// to a group of underlying sender implementations.
//
// This sender does not guarantee ordering of messages, and Send
// operations may block if the underlying senders fall behind the
// buffer size.
func NewAsyncGroupSender(ctx context.Context, bufferSize int, senders ...Sender) Sender {
	s := &asyncGroupSender{
		senders: senders,
		Base:    NewBase(""),
	}
	ctx, s.cancel = context.WithCancel(ctx)

	for i := 0; i < len(senders); i++ {
		p := make(chan message.Composer, bufferSize)
		s.pipes = append(s.pipes, p)
		go func(pipe chan message.Composer, sender Sender) {
			for {
				select {
				case <-ctx.Done():
					return
				case m := <-pipe:
					if m == nil {
						continue
					}
					sender.Send(m)
				}
			}
		}(p, senders[i])
	}

	s.closer = func() error {
		//s.cancel()

		errs := []string{}
		for _, sender := range s.senders {
			if err := sender.Close(); err != nil {
				errs = append(errs, err.Error())
			}
		}

		for idx, pipe := range s.pipes {
			if len(pipe) > 0 {
				errs = append(errs, fmt.Sprintf("buffer for sender #%d has %d items remaining",
					idx, len(pipe)))

			}
			close(pipe)
		}

		if len(errs) > 0 {
			return errors.New(strings.Join(errs, "\n"))
		}

		return nil
	}
	return s
}

func (s *asyncGroupSender) SetLevel(l LevelInfo) error {
	// if the base level isn't valid, then we shouldn't overwrite
	// constinuent senders (this is the indication that they were overridden.)
	if !s.Base.Level().Valid() {
		return nil
	}

	if err := s.Base.SetLevel(l); err != nil {
		return err
	}

	for _, sender := range s.senders {
		_ = sender.SetLevel(l)
	}

	return nil
}

func (s *asyncGroupSender) Send(m message.Composer) {
	bl := s.Base.Level()
	if bl.Valid() && !bl.ShouldLog(m) {
		return
	}

	for _, p := range s.pipes {
		p <- m
	}
}
