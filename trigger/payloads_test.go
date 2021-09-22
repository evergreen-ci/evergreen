package trigger

import (
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type payloadSuite struct {
	url    string
	status string
	t      commonTemplateData

	suite.Suite
}

func TestPayloads(t *testing.T) {
	suite.Run(t, &payloadSuite{})
}

func (s *payloadSuite) SetupSuite() {
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(s.T(), settings, "TestPayloads")
	s.Require().NoError(db.Clear(evergreen.ConfigCollection))
	s.Require().NoError(evergreen.UpdateConfig(settings))
}

func (s *payloadSuite) SetupTest() {
	s.url = "https://example.com/patch/1234"
	s.status = "failed"

	headers := http.Header{
		"X-Evergreen-test": []string{"something"},
	}

	s.t = commonTemplateData{
		ID:              "1234",
		DisplayName:     "display-1234",
		Object:          "patch",
		Project:         "test",
		URL:             s.url,
		PastTenseStatus: s.status,
		Headers:         headers,
	}
}

func (s *payloadSuite) TestEmailWithNilContent() {
	m, err := emailPayload(&s.t)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal(m.Subject, "Evergreen: patch display-1234 in 'test' has failed!")
	s.Contains(m.Body, "Your Evergreen patch in 'test' <")
	s.Contains(m.Body, "> has failed.")
	s.Contains(m.Body, `href="`+s.url+`"`)
	s.Contains(m.Body, "X-Evergreen-test:something")
}

func (s *payloadSuite) TestEmailWithTaskContent() {
	s.t.Object = "task"
	s.t.Task = &task.Task{
		Id:          "taskid",
		DisplayName: "thetask",
		Details: apimodels.TaskEndDetail{
			TimedOut: false,
		},
	}
	s.t.FailedTests = []task.TestResult{
		{
			TestFile: "test0",
		},
		{
			TestFile:        "test1",
			DisplayTestName: "display_test1",
		},
	}
	s.t.Build = &build.Build{
		DisplayName: "buildname",
	}
	s.t.ProjectRef = &model.ProjectRef{
		DisplayName: "theproject",
	}
	s.t.emailContent = emailTaskContentTemplate

	m, err := emailPayload(&s.t)
	s.NoError(err)
	s.Require().NotNil(m)
	s.Contains(m.Body, "thetask")
	s.Contains(m.Body, "TASK")
	s.Contains(m.Body, "test0")
	s.Contains(m.Body, "display_test1")
	s.Contains(m.Body, "theproject")
	s.Contains(m.Body, "buildname")

	s.t.Task.DisplayTask = &task.Task{
		DisplayName: "thedisplaytask",
	}
	m, err = emailPayload(&s.t)
	s.NoError(err)
	s.Require().NotNil(m)
	s.NotContains(m.Body, "thetask")
	s.Contains(m.Body, "thedisplaytask")
}

func (s *payloadSuite) TestEvergreenWebhook() {
	model := restModel.APIPatch{}
	model.Author = utility.ToStringPtr("somebody")

	m, err := webhookPayload(&model, s.t.Headers)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Len(m.Body, 584)
	s.Len(m.Headers, 1)
}

func (s *payloadSuite) TestJIRAComment() {
	m, err := jiraComment(&s.t)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal("Evergreen patch [display-1234|https://example.com/patch/1234] in 'test' has failed!", *m)
}

func (s *payloadSuite) TestJIRAIssue() {
	m, err := jiraIssue(&s.t)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal("Evergreen patch 'display-1234' in 'test' has failed", m.Summary)
	s.Equal("Evergreen patch [display-1234|https://example.com/patch/1234] in 'test' has failed!", m.Description)
}

func (s *payloadSuite) TestSlack() {
	m, err := slack(&s.t)
	s.NoError(err)
	s.Require().NotNil(m)

	s.Equal("The patch <https://example.com/patch/1234|display-1234> in 'test' has failed!", m.Body)
	s.Empty(m.Attachments)
}

func (s *payloadSuite) TestGetFailedTestsFromTemplate() {
	test1 := task.TestResult{
		URL:    "http://www.something.com/absolute",
		Status: evergreen.TestSucceededStatus,
	}
	test2 := task.TestResult{
		URL:    "http://www.something.com/absolute",
		Status: evergreen.TestFailedStatus,
	}
	test3 := task.TestResult{
		LogId:  "abc",
		Status: evergreen.TestFailedStatus,
	}
	t := task.Task{
		Id:          "taskid",
		DisplayName: "thetask",
		Details: apimodels.TaskEndDetail{
			TimedOut: false,
		},
		LocalTestResults: []task.TestResult{test1, test2, test3},
	}
	settings, err := evergreen.GetConfig()
	s.NoError(err)
	s.Require().NotNil(settings)

	tr, err := getFailedTestsFromTemplate(t)
	s.NoError(err)
	s.Require().Len(tr, 2)
	s.Equal(test2.GetLogURL(evergreen.LogViewerHTML), tr[0].URL)
	s.Equal(test3.GetLogURL(evergreen.LogViewerHTML), tr[1].URL)
}

func TestTruncateString(t *testing.T) {
	assert := assert.New(t)

	const sample = "12345"

	head, tail := truncateString("", 0)
	assert.Empty(head)
	assert.Empty(tail)
	head, tail = truncateString("", 255)
	assert.Empty(head)
	assert.Empty(tail)

	head, tail = truncateString(sample, 255)
	assert.Equal("12345", head)
	assert.Empty(tail)

	head, tail = truncateString(sample, 5)
	assert.Equal("12345", head)
	assert.Empty(tail)

	head, tail = truncateString(sample, 4)
	assert.Equal("1...", head)
	assert.Len(head, 4)
	assert.Equal("2345", tail)

	head, tail = truncateString(sample, 0)
	assert.Empty(head)
	assert.Len(head, 0)
	assert.Equal("12345", tail)

	head, tail = truncateString(sample, -1)
	assert.Empty(head)
	assert.Len(head, 0)
	assert.Equal("12345", tail)
}
