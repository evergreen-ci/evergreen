package send

import (
	"fmt"
	"math/rand"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

type SenderSuite struct {
	senders map[SenderType]Sender
	require *require.Assertions
	rand    *rand.Rand
	suite.Suite
}

func TestSenderSuite(t *testing.T) {
	suite.Run(t, new(SenderSuite))
}

func (s *SenderSuite) SetupSuite() {
	s.rand = rand.New(rand.NewSource(time.Now().Unix()))
	s.require = s.Require()
}
func (s *SenderSuite) SetupTest() {
	l := LevelInfo{level.Info, level.Notice}
	s.senders = map[SenderType]Sender{
		Slack:     &slackJournal{base: newBase("slack")},
		File:      &fileLogger{base: newBase("file")},
		XMPP:      &xmppLogger{base: newBase("xmpp")},
		JSON:      &jsonLogger{base: newBase("json")},
		Stream:    &streamLogger{base: newBase("stream")},
		Bootstrap: &bootstrapLogger{},
	}

	internal := new(internalSender)
	internal.name = "internal"
	internal.output = make(chan *internalMessage)
	s.senders[Internal] = internal

	native, err := NewNativeLogger("native", l)
	s.require.NoError(err)
	s.senders[Native] = native

	var sender Sender
	multiSenders := []Sender{}
	for i := 0; i < 4; i++ {
		sender, err = NewNativeLogger(fmt.Sprintf("native-%d", i), l)
		s.require.NoError(err)
		multiSenders = append(multiSenders, sender)
	}

	multi, err := NewMultiSender("multi", l, multiSenders)
	s.require.NoError(err)
	s.senders[Multi] = multi

}

func (s *SenderSuite) functionalMockSenders() map[SenderType]Sender {
	out := map[SenderType]Sender{}
	for t, sender := range s.senders {
		if t == Slack || t == File || t == Stream || t == Internal || t == XMPP {
			continue
		} else if t == JSON {
			var err error
			out[t], err = NewJSONConsoleLogger("json", LevelInfo{level.Info, level.Notice})
			s.require.NoError(err)
		} else {
			out[t] = sender
		}
	}
	return out
}

func (s *SenderSuite) TeardownSuite() {
	s.NoError(s.senders[Internal].Close())
}

func (s *SenderSuite) TestSenderImplementsInterface() {
	// this actually won't catch the error; the compiler will in
	// the fixtures, but either way we need to make sure that the
	// tests actually enforce this.
	for name, sender := range s.senders {
		s.Implements((*Sender)(nil), sender, name)
	}
}

const letters = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ1234567890!@#$%^&*()"

func randomString(n int, r *rand.Rand) string {
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[r.Int63()%int64(len(letters))]
	}
	return string(b)
}

func (s *SenderSuite) TestNameSetterRoundTrip() {
	for n, sender := range s.senders {
		s.Equal(sender.Type(), n)
		for i := 0; i < 100; i++ {
			name := randomString(12, s.rand)
			s.NotEqual(sender.Name(), name)
			sender.SetName(name)
			s.Equal(sender.Name(), name)
		}
	}
}

func (s *SenderSuite) TestLevelSetterRejectsInvalidSettings() {
	levels := []LevelInfo{
		{level.Invalid, level.Invalid},
		{level.Priority(-10), level.Priority(-1)},
		{level.Debug, level.Priority(-1)},
		{level.Priority(800), level.Priority(-2)},
	}

	for n, sender := range s.senders {
		s.NoError(sender.SetLevel(LevelInfo{level.Debug, level.Alert}))
		for _, l := range levels {
			s.True(sender.Level().Valid(), string(n))
			s.False(l.Valid(), string(n))
			s.Error(sender.SetLevel(l), string(n))
			s.True(sender.Level().Valid(), string(n))
			s.NotEqual(sender.Level(), l, string(n))
		}

	}
}

func (s *SenderSuite) TestCloserShouldUsusallyNoop() {
	for t, sender := range s.senders {
		s.NoError(sender.Close(), string(t))
	}
}

func (s *SenderSuite) TestBasicNoopSendTest() {
	for _, sender := range s.functionalMockSenders() {
		for i := -10; i <= 110; i += 5 {
			m := message.NewDefaultMessage(level.Priority(i), "hello world! "+randomString(10, s.rand))
			sender.Send(m)
		}

	}
}
