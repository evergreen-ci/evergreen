package logging

import (
	"os"
	"os/exec"
	"testing"

	"github.com/stretchr/testify/suite"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

type GripInternalSuite struct {
	grip *Grip
	name string
	suite.Suite
}

func TestGripSuite(t *testing.T) {
	suite.Run(t, new(GripInternalSuite))
}

func (s *GripInternalSuite) SetupSuite() {
	s.name = "test"
	s.grip = NewGrip(s.name)
	s.Equal(s.grip.Name(), s.name)
}

func (s *GripInternalSuite) SetupTest() {
	s.grip.SetName(s.name)
	sender, err := send.NewNativeLogger(s.grip.Name(), s.grip.GetSender().Level())
	s.NoError(err)
	s.NoError(s.grip.SetSender(sender))
}

func (s *GripInternalSuite) TestPanicSenderActuallyPanics() {
	// both of these are in anonymous functions so that the defers
	// cover the correct area.

	func() {
		// first make sure that the default send method doesn't panic
		defer func() {
			s.Nil(recover())
		}()

		s.grip.GetSender().Send(message.NewLineMessage(s.grip.DefaultLevel(), "foo"))
	}()

	func() {
		// call a panic function with a recoverer set.
		defer func() {
			s.NotNil(recover())
		}()

		s.grip.sendPanic(message.NewLineMessage(s.grip.DefaultLevel(), "foo"))
	}()
}

func (s *GripInternalSuite) TestPanicSenderRespectsTThreshold() {
	s.True(level.Debug < s.grip.DefaultLevel())

	// test that there is a no panic if the message isn't "logabble"
	defer func() {
		s.Nil(recover())
	}()

	s.grip.sendPanic(message.NewLineMessage(level.Debug, "foo"))
}

func (s *GripInternalSuite) TestConditionalSend() {
	// because sink is an internal type (implementation of
	// sender,) and "GetMessage" isn't in the interface, though it
	// is exported, we can't pass the sink between functions.
	sink, err := send.NewInternalLogger("sink", s.grip.GetSender().Level())
	s.NoError(err)
	s.NoError(s.grip.SetSender(sink))

	msg := message.NewLineMessage(level.Info, "foo")
	msgTwo := message.NewLineMessage(level.Notice, "bar")

	// when the conditional argument is true, it should work
	s.grip.conditionalSend(true, msg)
	s.Equal(sink.GetMessage().Message, msg)

	// when the conditional argument is true, it should work, and the channel is fifo
	s.grip.conditionalSend(false, msgTwo)
	s.grip.conditionalSend(true, msg)
	s.Equal(sink.GetMessage().Message, msg)

	// change the order
	s.grip.conditionalSend(true, msg)
	s.grip.conditionalSend(false, msgTwo)
	s.Equal(sink.GetMessage().Message, msg)
}

// This testing method uses the technique outlined in:
// http://stackoverflow.com/a/33404435 to test a function that exits
// since it's impossible to "catch" an os.Exit
func TestSendFatalExits(t *testing.T) {
	grip := NewGrip("test")
	if os.Getenv("SHOULD_CRASH") == "1" {
		grip.sendFatal(message.NewLineMessage(grip.DefaultLevel(), "foo"))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestSendFatalExits")
	cmd.Env = append(os.Environ(), "SHOULD_CRASH=1")
	err := cmd.Run()
	if err == nil {
		t.Errorf("sendFatal should have exited 0, instead: %+v", err)
	}
}
