package recovery

import (
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/mongodb/grip"
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
	s.True(strings.Contains(msg.Rendered, "hit panic; recovering"))
	s.True(strings.Contains(msg.Rendered, "sorry"))
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
	s.True(strings.Contains(msg.Rendered, "this op name"))
	s.True(strings.Contains(msg.Rendered, "got grip"))
	s.True(strings.Contains(msg.Rendered, "bar"))

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
