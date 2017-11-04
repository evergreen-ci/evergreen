package send

import (
	"errors"
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
	errs     chan error
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
		errs:     make(chan error),
	}

	go s.backgroundDispatcher()

	return s
}

func (s *bufferedSender) backgroundDispatcher() {
	buffer := []message.Composer{}
	timer := time.NewTimer(s.duration)
	work := make(chan []message.Composer)
	complete := make(chan struct{})

	go s.backgroundWorker(work, complete)

daemon:
	for {
		select {
		case msg := <-s.pipe:
			buffer = append(buffer, msg)

			if len(buffer) < s.number {
				continue daemon
			}

			work <- buffer
			buffer = []message.Composer{}
			timer.Reset(s.duration)
		case <-timer.C:
			work <- buffer
			buffer = []message.Composer{}
			timer.Reset(s.duration)
		case <-s.signal:
			close(s.signal)
			close(s.pipe)

			for m := range s.pipe {
				buffer = append(buffer, m)
			}

			if len(buffer) != 0 {
				work <- buffer
			}

			close(work)

			break daemon
		}

	}

	<-complete
	s.errs <- s.Sender.Close()
	close(s.errs)
}

func (s *bufferedSender) backgroundWorker(work <-chan []message.Composer, complete chan<- struct{}) {
	for msgs := range work {
		if len(msgs) == 1 {
			s.Sender.Send(msgs[0])
		} else if len(msgs) > 1 {
			s.Sender.Send(message.NewGroupComposer(msgs))
		}
	}

	complete <- struct{}{}
}

func (s *bufferedSender) Send(msg message.Composer) {
	if !s.Level().ShouldLog(msg) {
		return
	}

	switch msg := msg.(type) {
	case *message.GroupComposer:
		for _, m := range msg.Messages() {
			s.pipe <- m
		}
	default:
		s.pipe <- msg
	}
}

func (s *bufferedSender) Close() (err error) {
	defer func() {
		if r := recover(); r != nil {
			err = errors.New("cannot close alreday closed buffered sender")
		}
	}()

	s.signal <- struct{}{}

	if err != nil {
		return
	}

	err = <-s.errs

	return
}
