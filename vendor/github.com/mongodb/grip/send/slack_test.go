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

	opts.Channel = "$chat"
	s.Error(opts.Validate())
	opts.Name = "test"
	s.NoError(opts.Validate(), "%+v", opts)

	defer os.Setenv(slackClientToken, os.Getenv(slackClientToken))
	s.NoError(os.Setenv(slackClientToken, "foo"))
}

func (s *SlackSuite) TestValidateAddsOctothorpToChannelName() {
	opts := &SlackOptions{Name: "test", Channel: "chat", Hostname: "foo"}
	s.Equal("chat", opts.Channel)
	s.NoError(opts.Validate())
	s.Equal("#chat", opts.Channel)
}

func (s *SlackSuite) TestFieldSetIncludeCheck() {
	opts := &SlackOptions{}
	s.Nil(opts.FieldsSet)
	s.Error(opts.Validate())
	s.NotNil(opts.FieldsSet)

	s.False(opts.fieldSetShouldInclude("time"))
	opts.FieldsSet["time"] = struct{}{}
	s.False(opts.fieldSetShouldInclude("time"))

	s.False(opts.fieldSetShouldInclude("msg"))
	opts.FieldsSet["time"] = struct{}{}
	s.False(opts.fieldSetShouldInclude("msg"))

	for _, f := range []string{"a", "b", "c"} {
		s.False(opts.fieldSetShouldInclude(f))
		opts.FieldsSet[f] = struct{}{}
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

	params := opts.getParams(message.NewString("foo"))
	s.Equal("good", params.Attachments[0].Color)

	for _, l := range []level.Priority{level.Emergency, level.Alert, level.Critical} {
		params = opts.getParams(message.NewDefaultMessage(l, "foo"))
		s.Equal("danger", params.Attachments[0].Color)
	}

	for _, l := range []level.Priority{level.Warning, level.Notice} {
		params = opts.getParams(message.NewDefaultMessage(l, "foo"))
		s.Equal("warning", params.Attachments[0].Color)
	}

	for _, l := range []level.Priority{level.Debug, level.Info, level.Trace} {
		params = opts.getParams(message.NewDefaultMessage(l, "foo"))
		s.Equal("good", params.Attachments[0].Color)
	}
}

func (s *SlackSuite) TestGetParamsWithBasicMetaDataEnabled() {
	opts := &SlackOptions{BasicMetadata: true}
	s.False(opts.Fields)
	s.True(opts.BasicMetadata)

	params := opts.getParams(message.NewDefaultMessage(level.Alert, "foo"))
	s.Equal("danger", params.Attachments[0].Color)
	s.Len(params.Attachments[0].Fields, 1)
	s.True(strings.Contains(params.Attachments[0].Fallback, "priority=alert"), params.Attachments[0].Fallback)

	opts.Hostname = "!"
	params = opts.getParams(message.NewDefaultMessage(level.Alert, "foo"))
	s.Len(params.Attachments[0].Fields, 1)
	s.False(strings.Contains(params.Attachments[0].Fallback, "host"), params.Attachments[0].Fallback)

	opts.Hostname = "foo"
	params = opts.getParams(message.NewDefaultMessage(level.Alert, "foo"))
	s.Len(params.Attachments[0].Fields, 2)
	s.True(strings.Contains(params.Attachments[0].Fallback, "host=foo"), params.Attachments[0].Fallback)

	opts.Name = "foo"
	params = opts.getParams(message.NewDefaultMessage(level.Alert, "foo"))
	s.Len(params.Attachments[0].Fields, 3)
	s.True(strings.Contains(params.Attachments[0].Fallback, "journal=foo"), params.Attachments[0].Fallback)
}

func (s *SlackSuite) TestFieldsMessageTypeIntegration() {
	opts := &SlackOptions{Fields: true}
	s.True(opts.Fields)
	s.False(opts.BasicMetadata)
	opts.FieldsSet = map[string]struct{}{
		"message": struct{}{},
		"other":   struct{}{},
		"foo":     struct{}{},
	}

	params := opts.getParams(message.NewDefaultMessage(level.Alert, "foo"))
	s.Equal("danger", params.Attachments[0].Color)
	s.Len(params.Attachments[0].Fields, 0)

	// if the fields are nil, then we end up ignoring things, except the message
	params = opts.getParams(message.NewFieldsMessage(level.Alert, "foo", message.Fields{}))
	s.Len(params.Attachments[0].Fields, 0)

	// when msg and the message match we ignore
	params = opts.getParams(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"msg": "foo"}))
	s.Len(params.Attachments[0].Fields, 0)

	params = opts.getParams(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"foo": "bar"}))
	s.Len(params.Attachments[0].Fields, 1)

	params = opts.getParams(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"other": "baz"}))
	s.Len(params.Attachments[0].Fields, 1)

	params = opts.getParams(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"untracked": "false", "other": "bar"}))
	s.Len(params.Attachments[0].Fields, 1)

	params = opts.getParams(message.NewFieldsMessage(level.Alert, "foo", message.Fields{"foo": "false", "other": "bass"}))
	s.Len(params.Attachments[0].Fields, 2)
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
}

func (s *SlackSuite) TestCreateMethodChangesClientState() {
	base := &slackClientImpl{}
	new := &slackClientImpl{}

	s.Equal(base, new)
	new.Create("foo")
	s.NotEqual(base, new)
}
