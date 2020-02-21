package send

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
)

type JiraSuite struct {
	opts *JiraOptions
	suite.Suite
}

func TestJiraSuite(t *testing.T) {
	suite.Run(t, new(JiraSuite))
}

func (j *JiraSuite) SetupSuite() {}

func (j *JiraSuite) SetupTest() {
	j.opts = &JiraOptions{
		Name:     "bot",
		BaseURL:  "url",
		Username: "username",
		Password: "password",
		client:   &jiraClientMock{},
	}
}

func (j *JiraSuite) TestMockSenderWithNewConstructor() {
	sender, err := NewJiraLogger(j.opts, LevelInfo{level.Trace, level.Info})
	j.NotNil(sender)
	j.NoError(err)
}

func (j *JiraSuite) TestConstructorMustCreate() {
	j.opts.client = &jiraClientMock{failCreate: true}
	sender, err := NewJiraLogger(j.opts, LevelInfo{level.Trace, level.Info})

	j.Nil(sender)
	j.Error(err)
}

func (j *JiraSuite) TestConstructorMustPassAuthTest() {
	j.opts.client = &jiraClientMock{failAuth: true}
	sender, err := NewJiraLogger(j.opts, LevelInfo{level.Trace, level.Info})

	j.Nil(sender)
	j.Error(err)
}

func (j *JiraSuite) TestConstructorErrorsWithInvalidConfigs() {
	sender, err := NewJiraLogger(nil, LevelInfo{level.Trace, level.Info})

	j.Nil(sender)
	j.Error(err)

	sender, err = NewJiraLogger(&JiraOptions{}, LevelInfo{level.Trace, level.Info})
	j.Nil(sender)
	j.Error(err)
}

func (j *JiraSuite) TestSendMethod() {
	sender, err := NewJiraLogger(j.opts, LevelInfo{level.Trace, level.Info})
	j.NotNil(sender)
	j.NoError(err)

	mock, ok := j.opts.client.(*jiraClientMock)
	j.True(ok)
	j.Equal(mock.numSent, 0)

	m := message.NewDefaultMessage(level.Debug, "hello")
	sender.Send(m)
	j.Equal(mock.numSent, 0)

	m = message.NewDefaultMessage(level.Alert, "")
	sender.Send(m)
	j.Equal(mock.numSent, 0)

	m = message.NewDefaultMessage(level.Alert, "world")
	sender.Send(m)
	j.Equal(mock.numSent, 1)
}

func (j *JiraSuite) TestSendMethodWithError() {
	sender, err := NewJiraLogger(j.opts, LevelInfo{level.Trace, level.Info})
	j.NotNil(sender)
	j.NoError(err)

	mock, ok := j.opts.client.(*jiraClientMock)
	j.True(ok)
	j.Equal(mock.numSent, 0)
	j.False(mock.failSend)

	m := message.NewDefaultMessage(level.Alert, "world")
	sender.Send(m)
	j.Equal(mock.numSent, 1)

	mock.failSend = true
	sender.Send(m)
	j.Equal(mock.numSent, 1)
}

func (j *JiraSuite) TestCreateMethodChangesClientState() {
	base := &jiraClientImpl{}
	new := &jiraClientImpl{}

	j.Equal(base, new)
	_ = new.CreateClient(nil, "foo")
	j.NotEqual(base, new)
}

// Test get fields
func (j *JiraSuite) TestGetFieldsWithJiraIssue() {
	project := "Hello"
	summary := "it's me"

	// Test fields
	reporterField := message.JiraField{Key: "Reporter", Value: "Annie"}
	assigneeField := message.JiraField{Key: "Assignee", Value: "Sejin"}
	typeField := message.JiraField{Key: "Type", Value: "Bug"}
	labelsField := message.JiraField{Key: "Labels", Value: []string{"Soul", "Pop"}}
	unknownField := message.JiraField{Key: "Artist", Value: "Adele"}

	// Test One: Only Summary and Project
	m1 := message.NewJiraMessage(project, summary)
	fields := getFields(m1)

	j.Equal(fields.Project.Key, project)
	j.Equal(fields.Summary, summary)
	j.Nil(fields.Reporter)
	j.Nil(fields.Assignee)
	j.Equal(fields.Type.Name, "Task")
	j.Nil(fields.Labels)
	j.Nil(fields.Unknowns)

	// Test Two: with reporter, assignee and type
	m2 := message.NewJiraMessage(project, summary, reporterField, assigneeField,
		typeField, labelsField)
	fields = getFields(m2)

	j.Equal(fields.Reporter.Name, "Annie")
	j.Equal(fields.Assignee.Name, "Sejin")
	j.Equal(fields.Type.Name, "Bug")
	j.Equal(fields.Labels, []string{"Soul", "Pop"})
	j.Nil(fields.Unknowns)

	// Test Three: everything plus Unknown fields
	m3 := message.NewJiraMessage(project, summary, reporterField, assigneeField,
		typeField, unknownField)
	fields = getFields(m3)
	j.Equal(fields.Unknowns["Artist"], "Adele")
}

func (j *JiraSuite) TestGetFieldsWithFields() {
	testFields := message.Fields{"key0": 12, "key1": 42}
	msg := "Get the message"
	m := message.MakeFieldsMessage(msg, testFields)

	fields := getFields(m)
	j.Equal(fields.Summary, msg)
	j.NotNil(fields.Description)
}

func (j *JiraSuite) TestTruncate() {
	sender, err := NewJiraLogger(j.opts, LevelInfo{level.Trace, level.Info})
	j.NotNil(sender)
	j.NoError(err)

	mock, ok := j.opts.client.(*jiraClientMock)
	j.True(ok)
	j.Equal(mock.numSent, 0)

	m := message.NewDefaultMessage(level.Info, "aaa")
	j.True(m.Loggable())
	sender.Send(m)
	j.Len(mock.lastSummary, 3)

	var longString bytes.Buffer
	for i := 0; i < 1000; i++ {
		longString.WriteString("a")
	}
	m = message.NewDefaultMessage(level.Info, longString.String())
	j.True(m.Loggable())
	sender.Send(m)
	j.Len(mock.lastSummary, 254)

	buffer := bytes.NewBufferString("")
	buffer.Grow(40000)
	for i := 0; i < 40000; i++ {
		buffer.WriteString("a")
	}

	m = message.NewDefaultMessage(level.Info, buffer.String())
	j.True(m.Loggable())
	sender.Send(m)
	j.Len(mock.lastDescription, 32767)
}

func (j *JiraSuite) TestCustomFields() {
	sender, err := NewJiraLogger(j.opts, LevelInfo{level.Trace, level.Info})
	j.NotNil(sender)
	j.NoError(err)

	mock, ok := j.opts.client.(*jiraClientMock)
	j.True(ok)
	j.Equal(mock.numSent, 0)

	jiraIssue := &message.JiraIssue{
		Summary: "test",
		Type:    "type",
		Fields: map[string]interface{}{
			"customfield_12345": []string{"hi", "bye"},
		},
	}

	m := message.MakeJiraMessage(jiraIssue)
	j.NoError(m.SetPriority(level.Warning))
	j.True(m.Loggable())
	sender.Send(m)

	j.Equal([]string{"hi", "bye"}, mock.lastFields.Unknowns["customfield_12345"])
	j.Equal("test", mock.lastFields.Summary)

	bytes, err := json.Marshal(&mock.lastFields)
	j.NoError(err)
	j.Len(bytes, 79)
	j.Equal(`{"customfield_12345":["hi","bye"],"issuetype":{"name":"type"},"summary":"test"}`, string(bytes))
}

func (j *JiraSuite) TestPopulateKey() {
	sender, err := NewJiraLogger(j.opts, LevelInfo{level.Trace, level.Info})
	j.NotNil(sender)
	j.NoError(err)
	mock, ok := j.opts.client.(*jiraClientMock)
	j.True(ok)
	j.Equal(mock.numSent, 0)

	count := 0
	jiraIssue := &message.JiraIssue{
		Summary: "foo",
		Type:    "bug",
		Callback: func(_ string) {
			count++
		},
	}

	j.Equal(0, count)
	m := message.MakeJiraMessage(jiraIssue)
	j.NoError(m.SetPriority(level.Alert))
	j.True(m.Loggable())
	sender.Send(m)
	j.Equal(1, count)
	issue := m.Raw().(*message.JiraIssue)
	j.Equal(mock.issueKey, issue.IssueKey)

	messageFields := message.NewFieldsMessage(level.Info, "something", message.Fields{
		"message": "foo",
	})
	j.True(messageFields.Loggable())
	sender.Send(messageFields)
	messageIssue := messageFields.Raw().(message.Fields)
	j.Equal(mock.issueKey, messageIssue[jiraIssueKey])
}

func (j *JiraSuite) TestWhenCallbackNil() {
	sender, err := NewJiraLogger(j.opts, LevelInfo{level.Trace, level.Info})
	j.NotNil(sender)
	j.NoError(err)
	mock, ok := j.opts.client.(*jiraClientMock)
	j.True(ok)
	j.Equal(mock.numSent, 0)

	jiraIssue := &message.JiraIssue{
		Summary: "foo",
		Type:    "bug",
	}

	m := message.MakeJiraMessage(jiraIssue)
	j.NoError(m.SetPriority(level.Alert))
	j.True(m.Loggable())
	j.NotPanics(func() {
		sender.Send(m)
	})
}
