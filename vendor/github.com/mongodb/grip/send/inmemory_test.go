package send

import (
	"fmt"
	"io"
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
		str, err := s.sender.Formatter()(msg)
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
	s.Require().Empty(s.sender.buffer)
	s.Require().Equal(readHeadNone, s.sender.readHead)
	s.Require().False(s.sender.readHeadCaughtUp)
	s.Require().Equal(0, s.sender.writeHead)
	s.Require().Zero(s.sender.totalBytesSent)

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

func (s *InMemorySuite) TestGetCountInvalidCount() {
	msgs, n, err := s.sender.GetCount(-1)
	s.Error(err)
	s.Zero(n)
	s.Nil(msgs)

	msgs, n, err = s.sender.GetCount(0)
	s.Error(err)
	s.Zero(n)
	s.Nil(msgs)
}

func (s *InMemorySuite) TestGetCountOne() {
	for i := 0; i < s.maxCap-1; i++ {
		s.sender.Send(s.msgs[i])
	}
	for i := 0; i < s.maxCap-1; i++ {
		msgs, n, err := s.sender.GetCount(1)
		s.Require().NoError(err)
		s.Require().Equal(1, n)
		s.Equal(s.msgs[i], msgs[0])
	}

	s.sender.Send(s.msgs[s.maxCap])

	msgs, n, err := s.sender.GetCount(1)
	s.Require().NoError(err)
	s.Require().Equal(1, n)
	s.Equal(s.msgs[s.maxCap], msgs[0])

	msgs, n, err = s.sender.GetCount(1)
	s.Require().Error(io.EOF, err)
	s.Require().Equal(0, n)
	s.Empty(msgs)
}

func (s *InMemorySuite) TestGetCountMultiple() {
	for i := 0; i < s.maxCap; i++ {
		s.sender.Send(s.msgs[i])
	}

	for count := 1; count <= s.maxCap; count++ {
		s.sender.ResetRead()
		for i := 0; i < s.maxCap; i += count {
			msgs, n, err := s.sender.GetCount(count)
			s.Require().NoError(err)
			remaining := count
			start := i
			end := start + count
			if end > s.maxCap {
				end = s.maxCap
				remaining = end - start
			}
			s.Equal(remaining, n)
			s.Equal(s.msgs[start:end], msgs)
		}
		s.True(s.sender.readHeadCaughtUp)

		_, _, err := s.sender.GetCount(count)
		s.Require().Equal(io.EOF, err)
	}
}

func (s *InMemorySuite) TestGetCountMultipleWithOverflow() {
	for _, msg := range s.msgs {
		s.sender.Send(msg)
	}

	for count := 1; count <= s.maxCap; count++ {
		s.sender.ResetRead()
		for i := 0; i < s.maxCap; i += count {
			msgs, n, err := s.sender.GetCount(count)
			s.Require().NoError(err)
			remaining := count
			start := len(s.msgs) - s.maxCap + i
			end := start + count
			if end > len(s.msgs) {
				end = len(s.msgs)
				remaining = end - start
			}
			s.Equal(remaining, n)
			s.Equal(s.msgs[start:end], msgs)
		}
		s.True(s.sender.readHeadCaughtUp)

		_, _, err := s.sender.GetCount(count)
		s.Require().Equal(io.EOF, err)
	}
}

func (s *InMemorySuite) TestGetCountTruncated() {
	s.sender.Send(s.msgs[0])
	s.sender.Send(s.msgs[1])

	msgs, n, err := s.sender.GetCount(1)
	s.Require().NoError(err)
	s.Equal(1, n)
	s.Equal(s.msgs[0], msgs[0])
	s.Require().False(s.sender.readHeadCaughtUp)

	for i := 0; i < s.maxCap; i++ {
		s.Require().NotEqual(readHeadTruncated, s.sender.readHead)
		s.sender.Send(s.msgs[i])
	}
	s.Require().Equal(readHeadTruncated, s.sender.readHead)
	_, _, err = s.sender.GetCount(1)
	s.Require().Equal(ErrorTruncated, err)
}

func (s *InMemorySuite) TestGetCountWithCatchupTruncated() {
	s.sender.Send(s.msgs[0])
	msgs, n, err := s.sender.GetCount(1)
	s.Require().NoError(err)
	s.Equal(1, n)
	s.Equal(s.msgs[0], msgs[0])
	s.True(s.sender.readHeadCaughtUp)

	for i := 0; i < s.maxCap; i++ {
		s.Require().NotEqual(readHeadTruncated, s.sender.readHead)
		s.sender.Send(s.msgs[i])
		s.Require().False(s.sender.readHeadCaughtUp)
	}
	s.Require().False(s.sender.readHeadCaughtUp)
	s.Require().NotEqual(readHeadTruncated, s.sender.readHead)

	s.sender.Send(s.msgs[0])
	s.Require().False(s.sender.readHeadCaughtUp)
	s.Require().Equal(readHeadTruncated, s.sender.readHead)

	_, _, err = s.sender.GetCount(1)
	s.Equal(ErrorTruncated, err)
}

func (s *InMemorySuite) TestGetCountWithCatchupWithOverflowTruncated() {
	for i := 0; i < s.maxCap; i++ {
		s.sender.Send(s.msgs[i])
	}
	for i := 0; i < s.maxCap; i++ {
		msgs, n, err := s.sender.GetCount(1)
		s.Require().NoError(err)
		s.Equal(1, n)
		s.Equal(s.msgs[i], msgs[0])
	}
	s.Require().True(s.sender.readHeadCaughtUp)

	for i := 0; i < s.maxCap+1; i++ {
		s.Require().NotEqual(readHeadTruncated, s.sender.readHead)
		s.sender.Send(s.msgs[i])
		s.Require().False(s.sender.readHeadCaughtUp)
	}
	s.Require().Equal(readHeadTruncated, s.sender.readHead)

	_, _, err := s.sender.GetCount(1)
	s.Equal(ErrorTruncated, err)
}

func (s *InMemorySuite) TestGetCountWithOverflowTruncated() {
	for i := 0; i < s.maxCap; i++ {
		s.sender.Send(s.msgs[i])
	}
	for i := 0; i < s.maxCap; i++ {
		msgs, n, err := s.sender.GetCount(1)
		s.Require().NoError(err)
		s.Equal(1, n)
		s.Equal(s.msgs[i], msgs[0])
	}
	s.Require().True(s.sender.readHeadCaughtUp)

	for i := 0; i < s.maxCap+1; i++ {
		s.Require().NotEqual(readHeadTruncated, s.sender.readHead)
		s.sender.Send(s.msgs[i])
		s.Require().False(s.sender.readHeadCaughtUp)
	}
	s.Require().Equal(readHeadTruncated, s.sender.readHead)

	_, _, err := s.sender.GetCount(1)
	s.Equal(ErrorTruncated, err)
}

func (s *InMemorySuite) TestGetCountWithWritesAfterEOF() {
	s.sender.Send(s.msgs[0])
	msgs, n, err := s.sender.GetCount(1)
	s.Require().NoError(err)
	s.Equal(1, n)
	s.Equal(s.msgs[0], msgs[0])
	s.True(s.sender.readHeadCaughtUp)
	_, _, err = s.sender.GetCount(1)
	s.Equal(io.EOF, err)

	s.sender.Send(s.msgs[1])
	s.False(s.sender.readHeadCaughtUp)
	msgs, n, err = s.sender.GetCount(1)
	s.Require().NoError(err)
	s.Equal(1, n)
	s.Equal(s.msgs[1], msgs[0])
	s.True(s.sender.readHeadCaughtUp)
	_, _, err = s.sender.GetCount(1)
	s.Equal(io.EOF, err)
}

func (s *InMemorySuite) TestResetRead() {
	for i := 0; i < s.maxCap-1; i++ {
		s.sender.Send(s.msgs[i])
	}

	var err error
	var n int
	var msgs []message.Composer
	for i := 0; i < s.maxCap-1; i++ {
		msgs, n, err = s.sender.GetCount(1)
		s.Require().NoError(err)
		s.Require().Equal(1, n)
		s.Equal(s.msgs[i], msgs[0])
	}
	s.True(s.sender.readHeadCaughtUp)

	_, _, err = s.sender.GetCount(1)
	s.Equal(io.EOF, err)

	s.sender.ResetRead()
	s.Equal(readHeadNone, s.sender.readHead)
	s.False(s.sender.readHeadCaughtUp)

	for i := 0; i < s.maxCap-1; i++ {
		msgs, n, err = s.sender.GetCount(1)
		s.Require().NoError(err)
		s.Require().Equal(1, n)
		s.Equal(s.msgs[i], msgs[0])
	}

	_, _, err = s.sender.GetCount(1)
	s.Require().Error(io.EOF, err)
}

func (s *InMemorySuite) TestGetCountEmptyBuffer() {
	msgs, n, err := s.sender.GetCount(1)
	s.Require().Equal(io.EOF, err)
	s.Zero(n)
	s.Empty(msgs)
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
