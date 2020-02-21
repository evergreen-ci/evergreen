package logging

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
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
	sender, err := send.NewNativeLogger(s.grip.Name(), send.LevelInfo{Default: level.Info, Threshold: level.Trace})
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

		s.grip.GetSender().Send(message.NewLineMessage(level.Critical, "foo"))
	}()

	func() {
		// call a panic function with a recoverer set.
		defer func() {
			s.NotNil(recover())
		}()

		s.grip.sendPanic(message.NewLineMessage(level.Info, "foo"))
	}()
}

func (s *GripInternalSuite) TestSetSenderErrorsForNil() {
	s.Error(s.grip.SetSender(nil))
}

func (s *GripInternalSuite) TestPanicSenderRespectsTThreshold() {
	s.True(level.Debug > s.grip.GetSender().Level().Threshold)
	s.NoError(s.grip.GetSender().SetLevel(send.LevelInfo{Default: level.Info, Threshold: level.Notice}))
	s.True(level.Debug < s.grip.GetSender().Level().Threshold)

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
	s.grip.Log(msg.Priority(), message.When(true, msg))
	s.Equal(msg.Raw(), sink.GetMessage().Message.Raw())

	// when the conditional argument is true, it should work, and the channel is fifo
	s.grip.Log(msgTwo.Priority(), message.When(false, msgTwo))
	s.grip.Log(msg.Priority(), message.When(true, msg))
	result := sink.GetMessage().Message
	if result.Loggable() {
		s.Equal(msg.Raw(), result.Raw())
	} else {
		s.Equal(msgTwo.Raw(), result.Raw())
	}

	// change the order
	s.grip.Log(msg.Priority(), message.When(true, msg))
	s.grip.Log(msgTwo.Priority(), message.When(false, msgTwo))
	result = sink.GetMessage().Message

	if result.Loggable() {
		s.Equal(msg.Raw(), result.Raw())
	} else {
		s.Equal(msgTwo.Raw(), result.Raw())
	}
}

func (s *GripInternalSuite) TestCatchMethods() {
	sink, err := send.NewInternalLogger("sink", send.LevelInfo{Default: level.Trace, Threshold: level.Trace})
	s.NoError(err)
	s.NoError(s.grip.SetSender(sink))

	cases := []interface{}{
		s.grip.Alert,
		s.grip.Critical,
		s.grip.Debug,
		s.grip.Emergency,
		s.grip.Error,
		s.grip.Info,
		s.grip.Notice,
		s.grip.Warning,

		s.grip.Alertln,
		s.grip.Criticalln,
		s.grip.Debugln,
		s.grip.Emergencyln,
		s.grip.Errorln,
		s.grip.Infoln,
		s.grip.Noticeln,
		s.grip.Warningln,

		s.grip.Alertf,
		s.grip.Criticalf,
		s.grip.Debugf,
		s.grip.Emergencyf,
		s.grip.Errorf,
		s.grip.Infof,
		s.grip.Noticef,
		s.grip.Warningf,

		s.grip.AlertWhen,
		s.grip.CriticalWhen,
		s.grip.DebugWhen,
		s.grip.EmergencyWhen,
		s.grip.ErrorWhen,
		s.grip.InfoWhen,
		s.grip.NoticeWhen,
		s.grip.WarningWhen,

		s.grip.AlertWhenln,
		s.grip.CriticalWhenln,
		s.grip.DebugWhenln,
		s.grip.EmergencyWhenln,
		s.grip.ErrorWhenln,
		s.grip.InfoWhenln,
		s.grip.NoticeWhenln,
		s.grip.WarningWhenln,

		s.grip.AlertWhenf,
		s.grip.CriticalWhenf,
		s.grip.DebugWhenf,
		s.grip.EmergencyWhenf,
		s.grip.ErrorWhenf,
		s.grip.InfoWhenf,
		s.grip.NoticeWhenf,
		s.grip.WarningWhenf,

		func(w bool, m interface{}) { s.grip.LogWhen(w, level.Info, m) },
		func(w bool, m ...interface{}) { s.grip.LogWhenln(w, level.Info, m...) },
		func(w bool, m string, a ...interface{}) { s.grip.LogWhenf(w, level.Info, m, a...) },
		func(m interface{}) { s.grip.Log(level.Info, m) },
		func(m string, a ...interface{}) { s.grip.Logf(level.Info, m, a...) },
		func(m ...interface{}) { s.grip.Logln(level.Info, m...) },
		func(m ...message.Composer) { s.grip.Log(level.Info, m) },
		func(m []message.Composer) { s.grip.Log(level.Info, m) },
		func(w bool, m ...message.Composer) { s.grip.LogWhen(w, level.Info, m) },
		func(w bool, m []message.Composer) { s.grip.LogWhen(w, level.Info, m) },
	}

	const msg = "hello world!"
	multiMessage := []message.Composer{
		message.ConvertToComposer(0, nil),
		message.ConvertToComposer(0, msg),
	}

	for _, logger := range cases {
		s.Equal(0, sink.Len())
		s.False(sink.HasMessage())

		switch log := logger.(type) {
		case func(error):
			log(errors.New(msg))
		case func(interface{}):
			log(msg)
		case func(...interface{}):
			log(msg, "", nil)
		case func(string, ...interface{}):
			log("%s", msg)
		case func(bool, interface{}):
			log(false, msg)
			log(true, msg)
		case func(bool, ...interface{}):
			log(false, msg, "", nil)
			log(true, msg, "", nil)
		case func(bool, string, ...interface{}):
			log(false, "%s", msg)
			log(true, "%s", msg)
		case func(...message.Composer):
			log(multiMessage...)
		case func(bool, ...message.Composer):
			log(false, multiMessage...)
			log(true, multiMessage...)
		case func([]message.Composer):
			log(multiMessage)
		case func(bool, []message.Composer):
			log(false, multiMessage)
			log(true, multiMessage)
		default:
			panic(fmt.Sprintf("%T is not supported\n", log))
		}

		if sink.Len() > 1 {
			// this is the many case
			var numLogged int
			out := sink.GetMessage()
			for i := 0; i < sink.Len(); i++ {
				out = sink.GetMessage()
				if out.Logged {
					numLogged++
					s.Equal(out.Rendered, msg)
				}
			}

			s.True(numLogged == 1, fmt.Sprintf("%T: %d %s", logger, numLogged, out.Priority))

			continue
		}

		s.True(sink.Len() == 1)
		s.True(sink.HasMessage())
		out := sink.GetMessage()
		s.Equal(out.Rendered, msg)
		s.True(out.Logged, fmt.Sprintf("%T %s", logger, out.Priority))
	}
}

// This testing method uses the technique outlined in:
// http://stackoverflow.com/a/33404435 to test a function that exits
// since it's impossible to "catch" an os.Exit
func TestSendFatalExits(t *testing.T) {
	grip := NewGrip("test")
	if os.Getenv("SHOULD_CRASH") == "1" {
		grip.sendFatal(message.NewLineMessage(level.Error, "foo"))
		return
	}

	cmd := exec.Command(os.Args[0], "-test.run=TestSendFatalExits")
	cmd.Env = append(os.Environ(), "SHOULD_CRASH=1")
	err := cmd.Run()
	if err == nil {
		t.Errorf("sendFatal should have exited 0, instead: %+v", err)
	}
}
