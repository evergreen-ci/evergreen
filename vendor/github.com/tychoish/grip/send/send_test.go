package send

import (
	"fmt"
	"log"
	"math/rand"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"github.com/tychoish/grip/level"
	"github.com/tychoish/grip/message"
)

type SenderSuite struct {
	senders map[string]Sender
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
	s.senders = map[string]Sender{
		"slack":  &slackJournal{base: newBase("slack")},
		"xmpp":   &xmppLogger{base: newBase("xmpp")},
		"json":   &jsonLogger{base: newBase("json")},
		"stream": &streamLogger{base: newBase("stream")},
	}

	internal := new(internalSender)
	internal.name = "internal"
	internal.output = make(chan *internalMessage)
	s.senders["internal"] = internal

	callsite := &callSiteLogger{base: newBase("callsite"), depth: 1}
	callsite.logger = log.New(os.Stdout, "callsite", log.LstdFlags)
	s.senders["callsite"] = callsite

	native, err := NewNativeLogger("native", l)
	s.require.NoError(err)
	s.senders["native"] = native

	var sender Sender
	multiSenders := []Sender{}
	for i := 0; i < 4; i++ {
		sender, err = NewNativeLogger(fmt.Sprintf("native-%d", i), l)
		s.require.NoError(err)
		multiSenders = append(multiSenders, sender)
	}

	multi, err := NewMultiSender("multi", l, multiSenders)
	s.require.NoError(err)
	s.senders["multi"] = multi
}

func (s *SenderSuite) functionalMockSenders() map[string]Sender {
	out := map[string]Sender{}
	for t, sender := range s.senders {
		if t == "slack" || t == "stream" || t == "internal" || t == "xmpp" {
			continue
		} else if t == "json" {
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
	s.NoError(s.senders["internal"].Close())
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
		for i := 0; i < 100; i++ {
			name := randomString(12, s.rand)
			s.NotEqual(sender.Name(), name, n)
			sender.SetName(name)
			s.Equal(sender.Name(), name, n)
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
