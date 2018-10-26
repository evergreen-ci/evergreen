package send

import (
	"fmt"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type InMemorySuite struct {
	maxCap int
	msgs   []message.Composer
	sender *InMemorySender
	suite.Suite
}

func (s *InMemorySuite) msgsToString(msgs []message.Composer) []string {
	strs := make([]string, 0, len(msgs))
	for _, msg := range msgs {
		str, err := s.sender.Formatter(msg)
		s.Require().NoError(err)
		strs = append(strs, str)
	}
	return strs
}

func (s *InMemorySuite) msgsToRaw(msgs []message.Composer) []interface{} {
	raw := make([]interface{}, 0, len(msgs))
	for _, msg := range msgs {
		raw = append(raw, msg.Raw())
	}
	return raw
}

func TestInMemorySuite(t *testing.T) {
	suite.Run(t, new(InMemorySuite))
}

func (s *InMemorySuite) SetupTest() {
	s.maxCap = 10
	info := LevelInfo{Default: level.Debug, Threshold: level.Debug}
	sender, err := NewInMemorySender("inmemory", info, s.maxCap)
	s.Require().NoError(err)
	s.Require().NotNil(sender)
	s.sender = sender.(*InMemorySender)
	s.msgs = make([]message.Composer, 2*s.maxCap)
	for i := range s.msgs {
		s.msgs[i] = message.NewDefaultMessage(info.Default, fmt.Sprint(i))
	}
}

func (s *InMemorySuite) TestInvalidCapacityErrors() {
	badCap := -1
	sender, err := NewInMemorySender("inmemory", LevelInfo{Default: level.Debug, Threshold: level.Debug}, badCap)
	s.Require().Error(err)
	s.Require().Nil(sender)
}

func (s *InMemorySuite) TestSendIgnoresMessagesWithPrioritiesBelowThreshold() {
	msg := message.NewDefaultMessage(level.Trace, "foo")
	s.sender.Send(msg)
	s.Assert().Equal(0, len(s.sender.buffer))
}

func (s *InMemorySuite) TestGetEmptyBuffer() {
	s.Assert().Empty(s.sender.Get())
}

func (s *InMemorySuite) TestGetWithOverflow() {
	for i, msg := range s.msgs {
		s.sender.Send(msg)
		found := s.sender.Get()

		if i < s.maxCap {
			for j := 0; j < i+1; j++ {
				s.Assert().Equal(s.msgs[j], found[j])
			}
		} else {
			for j := 0; j < s.maxCap; j++ {
				s.Assert().Equal(s.msgs[i+1-s.maxCap+j], found[j])
			}
		}
	}
}

func (s *InMemorySuite) TestGetStringEmptyBuffer() {
	str, err := s.sender.GetString()
	s.Assert().NoError(err)
	s.Assert().Empty(str)
}

func (s *InMemorySuite) TestGetStringWithOverflow() {
	for i, msg := range s.msgs {
		s.sender.Send(msg)
		found, err := s.sender.GetString()
		s.Require().NoError(err)

		var expected []string
		if i+1 < s.maxCap {
			s.Require().Equal(i+1, len(found))
			expected = s.msgsToString(s.msgs[:i+1])
		} else {
			s.Require().Equal(s.maxCap, len(found))
			expected = s.msgsToString(s.msgs[i+1-s.maxCap : i+1])
		}
		s.Require().Equal(len(expected), len(found))

		for j := 0; j < len(found); j++ {
			s.Assert().Equal(expected[j], found[j])
		}
	}
}

func (s *InMemorySuite) TestGetRawEmptyBuffer() {
	s.Assert().Empty(s.sender.GetRaw())
}

func (s *InMemorySuite) TestGetRawWithOverflow() {
	for i, msg := range s.msgs {
		s.sender.Send(msg)
		found := s.sender.GetRaw()
		var expected []interface{}

		if i+1 < s.maxCap {
			s.Require().Equal(i+1, len(found))
			expected = s.msgsToRaw(s.msgs[:i+1])
		} else {
			s.Require().Equal(s.maxCap, len(found))
			expected = s.msgsToRaw(s.msgs[i+1-s.maxCap : i+1])
		}

		s.Assert().Equal(len(expected), len(found))
		for j := 0; j < len(found); j++ {
			s.Assert().Equal(expected[j], found[j])
		}
	}
}

func (s *InMemorySuite) TestTotalBytes() {
	var totalBytes int64
	for _, msg := range s.msgs {
		s.sender.Send(msg)
		totalBytes += int64(len(msg.String()))
		s.Assert().Equal(totalBytes, s.sender.TotalBytesSent())
	}
}
