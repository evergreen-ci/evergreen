package send

import (
	"os"
	"strings"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type SlackSuite struct {
	opts *SlackOptions

	suite.Suite
}

func TestSlackSuite(t *testing.T) {
	suite.Run(t, new(SlackSuite))
}

func (s *SlackSuite) SetupSuite() {}
func (s *SlackSuite) SetupTest() {
	s.opts = &SlackOptions{
		Channel:  "#test",
		Hostname: "testhost",
		Name:     "bot",
		client:   &slackClientMock{},
	}
}

func (s *SlackSuite) TestMakeSlackConstructorErrorsWithUnsetEnvVar() {
	sender, err := MakeSlackLogger(nil)
	s.Error(err)
	s.Nil(sender)

	sender, err = MakeSlackLogger(&SlackOptions{})
	s.Error(err)
	s.Nil(sender)

	sender, err = MakeSlackLogger(&SlackOptions{Channel: "#meta"})
	s.Error(err)
	s.Nil(sender)
}

func (s *SlackSuite) TestMakeSlackConstructorErrorsWithInvalidConfigs() {
	defer os.Setenv(slackClientToken, os.Getenv(slackClientToken))
	s.NoError(os.Setenv(slackClientToken, "foo"))

	sender, err := MakeSlackLogger(nil)
	s.Error(err)
	s.Nil(sender)

	sender, err = MakeSlackLogger(&SlackOptions{})
	s.Error(err)
	s.Nil(sender)
}

func (s *SlackSuite) TestValidateAndConstructoRequiresValidate() {
	opts := &SlackOptions{}
	s.Error(opts.Validate())

	opts.Hostname = "testsystem.com"
	s.Error(opts.Validate())

	opts.Name = "test"
	opts.Channel = "$chat"
	s.Error(opts.Validate())
	opts.Channel = "@test"
	s.NoError(opts.Validate(), "%+v", opts)
	opts.Channel = "#test"
	s.NoError(opts.Validate(), "%+v", opts)

	defer os.Setenv(slackClientToken, os.Getenv(slackClientToken))
	s.NoError(os.Setenv(slackClientToken, "foo"))
}

func (s *SlackSuite) TestValidateRequiresOctothorpOrArobase() {
	opts := &SlackOptions{Name: "test", Channel: "#chat", Hostname: "foo"}
	s.Equal("#chat", opts.Channel)
	s.NoError(opts.Validate())
	opts = &SlackOptions{Name: "test", Channel: "@chat", Hostname: "foo"}
	s.Equal("@chat", opts.Channel)
	s.NoError(opts.Validate())
}

func (s *SlackSuite) TestFieldSetIncludeCheck() {
	opts := &SlackOptions{}
	s.Nil(opts.FieldsSet)
	s.Error(opts.Validate())
	s.NotNil(opts.FieldsSet)

	s.False(opts.fieldSetShouldInclude("time"))
	opts.FieldsSet["time"] = true
	s.False(opts.fieldSetShouldInclude("time"))

	s.False(opts.fieldSetShouldInclude("msg"))
	opts.FieldsSet["time"] = true
	s.False(opts.fieldSetShouldInclude("msg"))

	for _, f := range []string{"a", "b", "c"} {
		s.False(opts.fieldSetShouldInclude(f))
		opts.FieldsSet[f] = true
		s.True(opts.fieldSetShouldInclude(f))
	}
}

func (s *SlackSuite) TestFieldShouldIncludIsAlwaysTrueWhenFieldSetIsNile() {
	opts := &SlackOptions{}

	s.Nil(opts.FieldsSet)
	s.False(opts.fieldSetShouldInclude("time"))
	for _, f := range []string{"a", "b", "c"} {
		s.True(opts.fieldSetShouldInclude(f))
	}
}

func (s *SlackSuite) TestGetParamsWithAttachementOptsDisabledLevelImpact() {
	opts := &SlackOptions{}
	s.False(opts.Fields)
	s.False(opts.BasicMetadata)

	msg, params := opts.produceMessage(message.NewString("foo"))
	s.Equal("good", params.Attachments[0].Color)
	s.Equal("foo", msg)

	for _, l := range []level.Priority{level.Emergency, level.Alert, level.Critical} {
		msg, params = opts.produceMessage(message.NewDefaultMessage(l, "foo"))
		s.Equal("danger", params.Attachments[0].Color)
		s.Equal("foo", msg)
	}

	for _, l := range []level.Priority{level.Warning, level.Notice} {
		msg, params = opts.produceMessage(message.NewDefaultMessage(l, "foo"))
		s.Equal("warning", params.Attachments[0].Color)
		s.Equal("foo", msg)
	}

	for _, l := range []level.Priority{level.Debug, level.Info, level.Trace} {
		msg, params = opts.produceMessage(message.NewDefaultMessage(l, "foo"))
		s.Equal("good", params.Attachments[0].Color)
		s.Equal("foo", msg)
	}
}

func (s *SlackSuite) TestProduceMessageWithBasicMetaDataEnabled() {
	opts := &SlackOptions{BasicMetadata: true}
	s.False(opts.Fields)
	s.True(opts.BasicMetadata)

	msg, params := opts.produceMessage(message.NewDefaultMessage(level.Alert, "foo"))
	s.Equal("danger", params.Attachments[0].Color)
	s.Equal("foo", msg)
	s.Len(params.Attachments[0].Fields, 1)
	s.True(strings.Contains(params.Attachments[0].Fallback, "priority=alert"), params.Attachments[0].Fallback)

	opts.Hostname = "!"
	msg, params = opts.produceMessage(message.NewDefaultMessage(level.Alert, "foo"))
	s.Equal("foo", msg)
	s.Len(params.Attachments[0].Fields, 1)
	s.False(strings.Contains(params.Attachments[0].Fallback, "host"), params.Attachments[0].Fallback)

	opts.Hostname = "foo"
	msg, params = opts.produceMessage(message.NewDefaultMessage(level.Alert, "foo"))
	s.Equal("foo", msg)
	s.Len(params.Attachments[0].Fields, 2)
	s.True(strings.Contains(params.Attachments[0].Fallback, "host=foo"), params.Attachments[0].Fallback)

	opts.Name = "foo"
	msg, params = opts.produceMessage(message.NewDefaultMessage(level.Alert, "foo"))
	s.Equal("foo", msg)
	s.Len(params.Attachments[0].Fields, 3)
	s.True(strings.Contains(params.Attachments[0].Fallback, "journal=foo"), params.Attachments[0].Fallback)
}

func (s *SlackSuite) TestFieldsMessageTypeIntegration() {
	opts := &SlackOptions{Fields: true}
	s.True(opts.Fields)
	s.False(opts.BasicMetadata)
	opts.FieldsSet = map[string]bool{
		"message": true,
		"other":   true,
		"foo":     true,
	}

	msg, params := opts.produceMessage(message.NewDefaultMessage(level.Alert, "foo"))
	s.Equal("foo", msg)
	s.Equal("danger", params.Attachments[0].Color)
	s.Len(params.Attachments[0].Fields, 0)

	// if the fields are nil, then we end up ignoring things, except the message
	msg, params = opts.produceMessage(message.NewFieldsMessage(level.Alert, "foo", message.Fields{}))
	s.Equal("", msg)
	s.Len(params.Attachments[0].Fields, 1)

	// when msg and the message match we ignore
	msg, params = opts.produceMessage(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"msg": "foo"}))
	s.Equal("", msg)
	s.Len(params.Attachments[0].Fields, 1)

	msg, params = opts.produceMessage(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"foo": "bar"}))
	s.Equal("", msg)
	s.Len(params.Attachments[0].Fields, 2)

	msg, params = opts.produceMessage(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"other": "baz"}))
	s.Equal("", msg)
	s.Len(params.Attachments[0].Fields, 2)

	msg, params = opts.produceMessage(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"untracked": "false", "other": "bar"}))
	s.Equal("", msg)
	s.Len(params.Attachments[0].Fields, 2)

	msg, params = opts.produceMessage(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"foo": "false", "other": "bass"}))
	s.Equal("", msg)
	s.Len(params.Attachments[0].Fields, 3)
}

func (s *SlackSuite) TestMockSenderWithMakeConstructor() {
	defer os.Setenv(slackClientToken, os.Getenv(slackClientToken))
	s.NoError(os.Setenv(slackClientToken, "foo"))

	sender, err := MakeSlackLogger(s.opts)
	s.NotNil(sender)
	s.NoError(err)
}

func (s *SlackSuite) TestMockSenderWithNewConstructor() {
	sender, err := NewSlackLogger(s.opts, "foo", LevelInfo{level.Trace, level.Info})
	s.NotNil(sender)
	s.NoError(err)

}

func (s *SlackSuite) TestInvaldLevelCausesConstructionErrors() {
	sender, err := NewSlackLogger(s.opts, "foo", LevelInfo{level.Trace, level.Invalid})
	s.Nil(sender)
	s.Error(err)
}

func (s *SlackSuite) TestConstructorMustPassAuthTest() {
	s.opts.client = &slackClientMock{failAuthTest: true}
	sender, err := NewSlackLogger(s.opts, "foo", LevelInfo{level.Trace, level.Info})

	s.Nil(sender)
	s.Error(err)
}

func (s *SlackSuite) TestSendMethod() {
	sender, err := NewSlackLogger(s.opts, "foo", LevelInfo{level.Trace, level.Info})
	s.NotNil(sender)
	s.NoError(err)

	mock, ok := s.opts.client.(*slackClientMock)
	s.True(ok)
	s.Equal(mock.numSent, 0)

	m := message.NewDefaultMessage(level.Debug, "hello")
	sender.Send(m)
	s.Equal(mock.numSent, 0)

	m = message.NewDefaultMessage(level.Alert, "")
	sender.Send(m)
	s.Equal(mock.numSent, 0)

	m = message.NewDefaultMessage(level.Alert, "world")
	sender.Send(m)
	s.Equal(mock.numSent, 1)
	s.Equal("#test", mock.lastTarget)

	m = message.NewSlackMessage(level.Alert, "#somewhere", "Hi", nil)
	sender.Send(m)
	s.Equal(mock.numSent, 2)
	s.Equal("#somewhere", mock.lastTarget)
}

func (s *SlackSuite) TestSendMethodWithError() {
	sender, err := NewSlackLogger(s.opts, "foo", LevelInfo{level.Trace, level.Info})
	s.NotNil(sender)
	s.NoError(err)

	mock, ok := s.opts.client.(*slackClientMock)
	s.True(ok)
	s.Equal(mock.numSent, 0)
	s.False(mock.failSendingMessage)

	m := message.NewDefaultMessage(level.Alert, "world")
	sender.Send(m)
	s.Equal(mock.numSent, 1)

	mock.failSendingMessage = true
	sender.Send(m)
	s.Equal(mock.numSent, 1)

	// sender should not panic with empty attachments
	s.NotPanics(func() {
		m = message.NewSlackMessage(level.Alert, "#general", "I am a formatted slack message", nil)
		sender.Send(m)
		s.Equal(mock.numSent, 1)
	})
}

func (s *SlackSuite) TestCreateMethodChangesClientState() {
	base := &slackClientImpl{}
	new := &slackClientImpl{}

	s.Equal(base, new)
	new.Create("foo")
	s.NotEqual(base, new)
}

func (s *SlackSuite) TestSendMethodDoesIncorrectlyAllowTooLowMessages() {
	sender, err := NewSlackLogger(s.opts, "foo", LevelInfo{level.Trace, level.Info})
	s.NotNil(sender)
	s.NoError(err)

	mock, ok := s.opts.client.(*slackClientMock)
	s.True(ok)
	s.Equal(mock.numSent, 0)

	s.NoError(sender.SetLevel(LevelInfo{Default: level.Critical, Threshold: level.Alert}))
	s.Equal(mock.numSent, 0)
	sender.Send(message.NewDefaultMessage(level.Info, "hello"))
	s.Equal(mock.numSent, 0)
	sender.Send(message.NewDefaultMessage(level.Alert, "hello"))
	s.Equal(mock.numSent, 1)
	sender.Send(message.NewDefaultMessage(level.Alert, "hello"))
	s.Equal(mock.numSent, 2)
}

func (s *SlackSuite) TestSettingBotIdentity() {
	sender, err := NewSlackLogger(s.opts, "foo", LevelInfo{level.Trace, level.Info})
	s.NoError(err)
	s.NotNil(sender)

	mock, ok := s.opts.client.(*slackClientMock)
	s.True(ok)
	s.Equal(mock.numSent, 0)
	s.False(mock.failSendingMessage)

	m := message.NewDefaultMessage(level.Alert, "world")
	sender.Send(m)
	s.Equal(1, mock.numSent)
	s.Empty(mock.lastMsg.Username)
	s.Empty(mock.lastMsg.IconUrl)

	s.opts.Username = "Grip"
	s.opts.IconURL = "https://example.com/icon.ico"
	sender, err = NewSlackLogger(s.opts, "foo", LevelInfo{level.Trace, level.Info})
	s.NoError(err)
	sender.Send(m)
	s.Equal(2, mock.numSent)
	s.Equal("Grip", mock.lastMsg.Username)
	s.Equal("https://example.com/icon.ico", mock.lastMsg.IconUrl)
	s.False(mock.lastMsg.AsUser)
}
