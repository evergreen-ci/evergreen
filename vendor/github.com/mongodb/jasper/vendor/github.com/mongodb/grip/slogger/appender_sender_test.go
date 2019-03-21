package slogger

import (
	"bytes"
	"math/rand"
	"testing"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

type AppenderSenderSuite struct {
	buffer   *bytes.Buffer
	rand     *rand.Rand
	appender send.Sender
	sender   send.Sender
	require  *require.Assertions
	suite.Suite
}

func TestAppenderSenderSuite(t *testing.T) {
	suite.Run(t, new(AppenderSenderSuite))
}

func (s *AppenderSenderSuite) SetupSuite() {
	s.rand = rand.New(rand.NewSource(time.Now().Unix()))
	s.require = s.Require()
}

func (s *AppenderSenderSuite) SetupTest() {
	s.buffer = bytes.NewBuffer([]byte{})
	s.appender = NewStringAppender(s.buffer)
	s.sender = NewAppenderSender("gripTest", SenderAppender{s.appender})
}

func (s *AppenderSenderSuite) TearDownSuite() {
	s.NoError(s.sender.Close())
	s.NoError(s.appender.Close())
}

func (s *AppenderSenderSuite) TestSenderImplementsInterface() {
	// this actually won't catch the error; the compiler will in
	// the fixtures, but either way we need to make sure that the
	// tests actually enforce this.
	s.Implements((*send.Sender)(nil), s.sender)
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890!@#$%^&*()"

func randomString(n int, r *rand.Rand) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[r.Int63()%int64(len(letters))]
	}
	return string(b)
}

func (s *AppenderSenderSuite) TestNameSetterRoundTrip() {
	for i := 0; i < 100; i++ {
		name := randomString(12, s.rand)
		s.NotEqual(s.sender.Name(), name)
		s.sender.SetName(name)
		s.Equal(s.sender.Name(), name)
	}
}

func (s *AppenderSenderSuite) TestLevelSetterRejectsInvalidSettings() {
	levels := []send.LevelInfo{
		{Default: level.Invalid, Threshold: level.Invalid},
		{Default: level.Priority(-10), Threshold: level.Priority(-1)},
		{Default: level.Debug, Threshold: level.Priority(-1)},
		{Default: level.Priority(800), Threshold: level.Priority(-2)},
	}

	s.NoError(s.sender.SetLevel(send.LevelInfo{Default: level.Debug, Threshold: level.Alert}))
	for _, l := range levels {
		s.True(s.sender.Level().Valid())
		s.False(l.Valid())
		s.Error(s.sender.SetLevel(l))
		s.True(s.sender.Level().Valid())
		s.NotEqual(s.sender.Level(), l)
	}
}

func (s *AppenderSenderSuite) TestCloserShouldNoop() {
	s.NoError(s.sender.Close())
}

func (s *AppenderSenderSuite) TestBasicNoopSendTest() {
	size := s.buffer.Len()
	s.True(size == 0)
	for i := -10; i <= 110; i += 5 {
		m := message.NewDefaultMessage(level.Priority(i), "hello world! "+randomString(10, s.rand))
		s.sender.Send(m)
		size = s.buffer.Len()
	}
	s.True(size > 0)
}
