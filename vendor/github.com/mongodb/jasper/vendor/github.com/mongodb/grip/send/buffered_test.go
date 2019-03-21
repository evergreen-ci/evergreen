package send

import (
	"strings"
	"testing"
	"time"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type BufferedSenderSuite struct {
	sender *InternalSender
	buffs  *bufferedSender
	dur    time.Duration
	num    int
	suite.Suite
}

func TestBufferedSenderSuite(t *testing.T) {
	suite.Run(t, new(BufferedSenderSuite))
}

func (s *BufferedSenderSuite) SetupSuite() {
	s.dur = minBufferLength
	s.num = 10
}

func (s *BufferedSenderSuite) SetupTest() {
	var err error
	s.sender, err = NewInternalLogger("buffs", LevelInfo{level.Trace, level.Trace})
	s.Require().NoError(err)
	s.buffs = NewBufferedSender(s.sender, s.dur, s.num).(*bufferedSender)
	s.Require().NotNil(s.buffs)
}

func (s *BufferedSenderSuite) TearDownTest() {
	//	_ = s.buffs.Close()
}

func (s *BufferedSenderSuite) TestConstructor() {
	s.NoError(s.buffs.Close())

	// check override for very small numbers
	s.buffs = NewBufferedSender(s.sender, 0, 0).(*bufferedSender)
	s.Equal(100, s.buffs.number)
	s.Equal(24*time.Hour, s.buffs.duration)
	//	s.buffs.Close()

	s.buffs = NewBufferedSender(s.sender, time.Second, 0).(*bufferedSender)
	s.Equal(100, s.buffs.number)
	s.Equal(5*time.Second, s.buffs.duration)
}

func (s *BufferedSenderSuite) TestSendingSingleMessage() {
	m, ok := s.sender.GetMessageSafe()
	s.False(ok)
	s.Nil(m)

	s.buffs.Send(message.NewDefaultMessage(level.Warning, "hello"))
	m, ok = s.sender.GetMessageSafe()
	s.False(ok)
	s.Nil(m)

	time.Sleep(s.dur)
	m = s.sender.GetMessage()
	s.Equal(m.Rendered, "hello")
}

func (s *BufferedSenderSuite) TestSendingBigGroup() {
	m, ok := s.sender.GetMessageSafe()
	s.False(ok)
	s.Nil(m)

	for i := 0; i < 80; i++ {
		s.buffs.Send(message.NewLineMessage(level.Notice, "hi", i))
	}

	time.Sleep(100 * time.Millisecond)
	for i := 0; i < 8; i++ {
		m, ok = s.sender.GetMessageSafe()
		if s.True(ok, "%+v", m) {
			s.True(strings.HasPrefix(m.Rendered, "hi"))
		}
	}

	m, ok = s.sender.GetMessageSafe()
	s.False(ok)
	s.Nil(m)
}

func (s *BufferedSenderSuite) TestSendingGroup() {
	msgs := []message.Composer{}
	for i := 0; i < 100; i++ {
		msgs = append(msgs, message.NewLineMessage(level.Notice, "hi", i))
	}

	m := message.NewGroupComposer(msgs)

	out, ok := s.sender.GetMessageSafe()
	s.False(ok)
	s.Nil(out)

	s.buffs.Send(m)
	time.Sleep(s.dur)
	for i := 0; i < 10; i++ {
		out, ok = s.sender.GetMessageSafe()
		s.True(ok)
		s.Len(strings.Split(out.Rendered, "\n"), 10)
	}

	out, ok = s.sender.GetMessageSafe()
	s.False(ok)
	s.Nil(out)
}

func (s *BufferedSenderSuite) TestCloseTwiceReturnsAnError() {
	s.NoError(s.buffs.Close())
	s.Error(s.buffs.Close())
}
