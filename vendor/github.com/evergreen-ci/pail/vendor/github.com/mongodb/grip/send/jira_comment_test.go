package send

import (
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type JiraCommentSuite struct {
	opts *JiraOptions
	suite.Suite
}

func TestJiraCommentSuite(t *testing.T) {
	suite.Run(t, new(JiraCommentSuite))
}

func (j *JiraCommentSuite) SetupTest() {
	j.opts = &JiraOptions{
		BaseURL:  "url",
		Username: "username",
		Password: "password",
		client:   &jiraClientMock{},
		Name:     "1234",
	}
}

func (j *JiraCommentSuite) TestMockSenderWithNewConstructor() {
	sender, err := NewJiraCommentLogger("1234", j.opts, LevelInfo{level.Trace, level.Info})
	j.NotNil(sender)
	j.NoError(err)
}

func (j *JiraCommentSuite) TestConstructorMustCreate() {
	j.opts.client = &jiraClientMock{failCreate: true}
	sender, err := NewJiraCommentLogger("1234", j.opts, LevelInfo{level.Trace, level.Info})
	j.Nil(sender)
	j.Error(err)
}

func (j *JiraCommentSuite) TestConstructorMustPassAuthTest() {
	j.opts.client = &jiraClientMock{failAuth: true}
	sender, err := NewJiraCommentLogger("1234", j.opts, LevelInfo{level.Trace, level.Info})
	j.Nil(sender)
	j.Error(err)
}

func (j *JiraCommentSuite) TestConstructorErrorsWithInvalidConfigs() {
	sender, err := NewJiraCommentLogger("1234", nil, LevelInfo{level.Trace, level.Info})
	j.Nil(sender)
	j.Error(err)

	sender, err = NewJiraLogger(&JiraOptions{}, LevelInfo{level.Trace, level.Info})
	j.Nil(sender)
	j.Error(err)
}

func (j *JiraCommentSuite) TestSendMethod() {
	numShouldHaveSent := 0
	sender, err := NewJiraCommentLogger("1234", j.opts, LevelInfo{level.Trace, level.Info})
	j.NoError(err)
	j.Require().NotNil(sender)

	mock, ok := j.opts.client.(*jiraClientMock)
	j.True(ok)
	j.Equal(mock.numSent, 0)

	m := message.NewDefaultMessage(level.Debug, "sending debug level comment")
	sender.Send(m)
	j.Equal(mock.numSent, numShouldHaveSent)

	m = message.NewDefaultMessage(level.Alert, "sending alert level comment")
	sender.Send(m)
	numShouldHaveSent++
	j.Equal(mock.numSent, numShouldHaveSent)

	m = message.NewDefaultMessage(level.Emergency, "sending emergency level comment")
	sender.Send(m)
	numShouldHaveSent++
	j.Equal(mock.numSent, numShouldHaveSent)
}

func (j *JiraCommentSuite) TestSendMethodWithError() {
	sender, err := NewJiraCommentLogger("1234", j.opts, LevelInfo{level.Trace, level.Info})
	j.NotNil(sender)
	j.NoError(err)

	mock, ok := j.opts.client.(*jiraClientMock)
	j.True(ok)
	j.Equal(mock.numSent, 0)
	j.False(mock.failSend)

	m := message.NewDefaultMessage(level.Alert, "test")
	sender.Send(m)
	j.Equal(mock.numSent, 1)

	mock.failSend = true
	sender.Send(m)
	j.Equal(mock.numSent, 1)
}

func (j *JiraCommentSuite) TestCreateMethodChangesClientState() {
	base := &jiraClientImpl{}
	new := &jiraClientImpl{}

	j.Equal(base, new)
	j.NoError(new.CreateClient(nil, "foo"))
	j.NotEqual(base, new)
}

func (j *JiraCommentSuite) TestSendWithJiraIssueComposer() {
	c := message.NewJIRACommentMessage(level.Notice, "ABC-123", "Hi")

	sender, err := NewJiraCommentLogger("XYZ-123", j.opts, LevelInfo{level.Trace, level.Info})
	j.NoError(err)
	j.Require().NotNil(sender)

	sender.Send(c)

	mock, ok := j.opts.client.(*jiraClientMock)
	j.True(ok)
	j.Equal(1, mock.numSent)
	j.Equal("ABC-123", mock.lastIssue)
}
