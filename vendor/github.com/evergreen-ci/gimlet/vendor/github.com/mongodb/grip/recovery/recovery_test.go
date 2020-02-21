package recovery

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mongodb/grip"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

type RecoverySuite struct {
	sender       *send.InternalSender
	globalSender send.Sender
	suite.Suite
}

func TestRecoverySuite(t *testing.T) {
	suite.Run(t, new(RecoverySuite))
}

func (s *RecoverySuite) SetupSuite() {
	s.Require().NoError(os.Setenv(killOverrideVarName, "true"))
}

func (s *RecoverySuite) TearDownSuite() {
	s.Require().NoError(os.Setenv(killOverrideVarName, ""))
}

func (s *RecoverySuite) SetupTest() {
	s.sender = send.MakeInternalLogger()
	s.globalSender = grip.GetSender()
	s.Require().NoError(grip.SetSender(s.sender))
}

func (s *RecoverySuite) logger() grip.Journaler { return logging.MakeGrip(s.sender) }

func (s *RecoverySuite) TearDownTest() {
	s.Require().NoError(grip.SetSender(s.globalSender))
}

func (s *RecoverySuite) TestWithoutPanicNoErrorsLoged() {
	s.False(s.sender.HasMessage())
	LogStackTraceAndContinue()
	s.False(s.sender.HasMessage())
	LogStackTraceAndExit()
	s.False(s.sender.HasMessage())
	s.NoError(HandlePanicWithError(nil, nil))
	s.False(s.sender.HasMessage())
}

func (s *RecoverySuite) TestPanicCausesLogsWithContinueRecoverer() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		defer LogStackTraceAndContinue()
		panic("sorry")
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.Contains(msg.Rendered, "hit panic; recovering")
	s.Contains(msg.Rendered, "sorry")
}

func (s *RecoverySuite) TestPanicsCausesLogsWithExitHandler() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		defer LogStackTraceAndExit("exit op")
		panic("sorry buddy")
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "hit panic; exiting"))
	s.True(strings.Contains(msg.Rendered, "sorry buddy"))
	s.True(strings.Contains(msg.Rendered, "exit op"))
}

func (s *RecoverySuite) TestPanicCausesLogsWithErrorHandler() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		err := func() (err error) {
			defer func() { err = HandlePanicWithError(recover(), nil) }()
			panic("get a grip")
		}()

		s.Error(err)
		s.True(strings.Contains(err.Error(), "get a grip"))
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "hit panic; adding error"))
	s.True(strings.Contains(msg.Rendered, "get a grip"))
}

func (s *RecoverySuite) TestErrorHandlerPropogatesErrorAndPanicMessage() {
	s.NotPanics(func() {
		err := func() (err error) {
			defer func() { err = HandlePanicWithError(recover(), errors.New("bar"), "this op name") }()
			panic("got grip")
		}()

		s.Error(err)
		s.True(strings.Contains(err.Error(), "got grip"))
		s.True(strings.Contains(err.Error(), "bar"))
		s.False(strings.Contains(err.Error(), "op name"))
	})

	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.Contains(msg.Rendered, "this op name")
	s.Contains(msg.Rendered, "got grip")
	s.Contains(msg.Rendered, "bar")
}

func (s *RecoverySuite) TestPanicHandlerWithErrorPropogatesErrorWithoutPanic() {
	err := HandlePanicWithError(nil, errors.New("foo"))
	s.Error(err)
	s.True(strings.Contains(err.Error(), "foo"))
}

func (s *RecoverySuite) TestPanicHandlerPropogatesOperationName() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		defer LogStackTraceAndContinue("test handler op")
		panic("sorry")
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "test handler op"))
}

func (s *RecoverySuite) TestPanicHandlerPropogatesOperationNameWithArgs() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		defer LogStackTraceAndContinue("test handler op", "for real")
		panic("sorry")
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "test handler op for real"))
}

func (s *RecoverySuite) TestPanicHandlerAnnotationPropogagaesMessage() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		defer AnnotateMessageWithStackTraceAndContinue(message.Fields{"foo": "test handler op1 for real"})
		panic("sorry")
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "test handler op1 for real"))

}

func (s *RecoverySuite) TestPanicsCausesAnnotateLogsWithExitHandler() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		defer AnnotateMessageWithStackTraceAndExit(message.Fields{"foo": "exit op1"})
		panic("sorry buddy")
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "hit panic; exiting"))
	s.True(strings.Contains(msg.Rendered, "sorry buddy"))
	s.True(strings.Contains(msg.Rendered, "exit op1"))
}

func (s *RecoverySuite) TestPanicAnnotatesLogsWithErrorHandler() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		err := func() (err error) {
			defer func() { err = AnnotateMessageWithPanicError(recover(), nil, message.Fields{"foo": "bar"}) }()
			panic("get a grip")
		}()

		s.Error(err)
		s.True(strings.Contains(err.Error(), "get a grip"))
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "hit panic; adding error"))
	s.True(strings.Contains(msg.Rendered, "get a grip"))
	s.True(strings.Contains(msg.Rendered, "foo='bar'"))
}

func (s *RecoverySuite) TestPanicHandlerSendJournalerPropogagaesMessage() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		defer SendStackTraceAndContinue(s.logger(), message.Fields{"foo": "test handler op2 for real"})
		panic("sorry")
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "test handler op2 for real"))

}

func (s *RecoverySuite) TestPanicsCausesSendJournalerLogsWithExitHandler() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		defer SendStackTraceMessageAndExit(s.logger(), message.Fields{"foo": "exit op2"})
		panic("sorry buddy")
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "hit panic; exiting"))
	s.True(strings.Contains(msg.Rendered, "sorry buddy"))
	s.True(strings.Contains(msg.Rendered, "exit op2"))
}

func (s *RecoverySuite) TestPanicSendJournalerLogsWithErrorHandler() {
	s.False(s.sender.HasMessage())
	s.NotPanics(func() {
		err := func() (err error) {
			defer func() { err = SendMessageWithPanicError(recover(), nil, s.logger(), message.Fields{"foo": "bar1"}) }()
			panic("get a grip")
		}()

		s.Error(err)
		s.True(strings.Contains(err.Error(), "get a grip"))
	})
	s.True(s.sender.HasMessage())
	msg, ok := s.sender.GetMessageSafe()
	s.True(ok)
	s.True(strings.Contains(msg.Rendered, "hit panic; adding error"))
	s.True(strings.Contains(msg.Rendered, "get a grip"))
	s.True(strings.Contains(msg.Rendered, "foo='bar1'"))
}
