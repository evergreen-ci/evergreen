package send

import (
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
	_ = new.CreateClient("foo")
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
