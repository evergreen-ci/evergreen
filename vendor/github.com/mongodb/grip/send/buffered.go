package send

import (
	"time"

	"github.com/mongodb/grip/message"
)

const minBufferLength = 5 * time.Second

type bufferedSender struct {
	Sender

	duration time.Duration
	number   int
	pipe     chan message.Composer
	signal   chan struct{}
}

// NewBufferedSender provides a Sender implementation that wraps an
// existing Sender sending messages in batches, on a specified
// duration or after an interval has passed.
//
// Be aware that while messages are sent asynchronously, each message
// is sent individually. Furthermore, no more than 2 batches of events
// can be sent at once.
//
// If the duration is 0, the constructor sets a duration of 24 hours,
// and if the duration is not 5 seconds, the constructor sets a 5 second
// duration. If the number threshold is 0, then the constructor sets a
// threshold of 100.
func NewBufferedSender(sender Sender, duration time.Duration, number int) Sender {
	if duration == 0 {
		duration = time.Hour * 24
	} else if duration < minBufferLength {
		duration = minBufferLength
	}

	if number == 0 {
		number = 100
	}

	s := &bufferedSender{
		Sender:   sender,
		duration: duration,
		number:   number,
		pipe:     make(chan message.Composer, number),
		signal:   make(chan struct{}),
	}

	go s.backgroundWorker()

	return s
}

func (s *bufferedSender) backgroundWorker() {
	buffer := []message.Composer{}
	timer := time.NewTimer(s.duration)
	complete := make(chan struct{}, 2)

daemon:
	for {
		select {
		case msg := <-s.pipe:
			if len(buffer) < s.number {
				buffer = append(buffer, msg)
				continue daemon
			}

			go s.backgroundSender(buffer, complete)
			buffer = []message.Composer{}
			timer.Reset(s.duration)
		case <-timer.C:
			go s.backgroundSender(buffer, complete)
			buffer = []message.Composer{}
			timer.Reset(s.duration)
		case <-s.signal:
			close(s.pipe)
			close(s.signal)
			break daemon
		}

		<-complete
	}

	<-complete
	_ = s.Sender.Close()
}

func (s *bufferedSender) backgroundSender(msgs []message.Composer, complete chan struct{}) {
	if len(msgs) == 1 {
		s.Sender.Send(msgs[0])
	} else if len(msgs) > 1 {
		s.Sender.Send(message.NewGroupComposer(msgs))
	}

	complete <- struct{}{}
}

func (s *bufferedSender) Send(msg message.Composer) {
	switch msg := msg.(type) {
	case *message.GroupComposer:
		for _, m := range msg.Messages() {
			s.pipe <- m
		}
	default:
		s.pipe <- msg
	}
}

func (s *bufferedSender) Close() error {
	// do a non-blocking send on the signal thread to close the
	// background worker
	select {
	case s.signal <- struct{}{}:
	default:
	}

	return nil
}
