package notification

import (
	"errors"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type notificationSuite struct {
	suite.Suite

	n Notification
}

func TestNotifications(t *testing.T) {
	suite.Run(t, &notificationSuite{})
}

func (s *notificationSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *notificationSuite) SetupTest() {
	s.NoError(db.Clear(NotificationsCollection))
	s.n = Notification{
		Target: event.Subscriber{
			Type: "github_pull_request",
			Target: event.GithubPullRequestSubscriber{
				Owner:    "evergreen-ci",
				Repo:     "evergreen",
				PRNumber: 9001,
				Ref:      "sadasdkjsad",
			},
		},
		Payload: GithubStatusAPIPayload{
			Status:      "failure",
			Context:     "evergreen",
			URL:         "https://example.com",
			Description: "something failed",
		},
	}
}

func (s *notificationSuite) TestDBInteractions() {
	s.False(s.n.ID.Valid())

	// find something that doesn't exist
	n, err := Find(bson.NewObjectId())
	s.NoError(err)
	s.Nil(n)

	// mark notification that hasn't been stored
	// with nil err
	s.EqualError(s.n.MarkSent(), "notification has no ID")
	s.False(s.n.ID.Valid())
	s.Empty(s.n.Error)
	s.Zero(s.n.SentAt)

	// with non-nil err
	s.EqualError(s.n.MarkError(errors.New("")), "notification has no ID")
	s.False(s.n.ID.Valid())
	s.Empty(s.n.Error)
	s.Zero(s.n.SentAt)

	// try insert; verify local id is changed
	s.n.ID = bson.NewObjectId()
	s.NoError(InsertMany(s.n))
	s.True(s.n.ID.Valid())

	// try fetching it back, and ensuring attributes have been set correctly
	n, err = Find(s.n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.True(n.ID.Valid())
	s.Empty(n.Error)
	s.Zero(n.SentAt)
	s.Require().IsType(&GithubStatusAPIPayload{}, n.Payload)
	payload := n.Payload.(*GithubStatusAPIPayload)
	s.Equal("failure", payload.Status)
	s.Equal("evergreen", payload.Context)
	s.Equal("https://example.com", payload.URL)
	s.Equal("something failed", payload.Description)

	// mark that notification as sent, with nil err
	s.NoError(s.n.MarkSent())
	s.True(s.n.ID.Valid())
	s.Empty(s.n.Error)
	s.NotZero(s.n.SentAt)
	n, err = Find(s.n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.Empty(n.Error)
	s.NotZero(n.SentAt)

	// mark that notification as sent, with non-nil err
	s.NoError(s.n.MarkError(errors.New("test")))
	s.True(s.n.ID.Valid())
	s.Equal("test", s.n.Error)
	s.NotZero(s.n.SentAt)
	n, err = Find(s.n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.Equal("test", n.Error)
	s.NotZero(n.SentAt)

}

func (s *notificationSuite) TestInsertMany() {
	s.n.ID = bson.NewObjectId()

	n2 := Notification{
		ID: bson.NewObjectId(),
		Target: event.Subscriber{
			Type:   "slack",
			Target: "#general",
		},
	}

	payload := "Hi"
	n3 := Notification{
		ID: bson.NewObjectId(),
		Target: event.Subscriber{
			Type:   "jira-comment",
			Target: "ABC-1234",
		},
		Payload: &payload,
	}

	slice := []Notification{s.n, n2, n3}

	s.NoError(InsertMany(slice...))

	out := []Notification{}
	s.NoError(db.FindAllQ(NotificationsCollection, db.Q{}, &out))
	s.Len(out, 3)

	for i := range out {
		if out[i].Target.Type == "jira-comment" {
			payload, ok := out[i].Payload.(*string)
			s.Require().True(ok)
			s.Equal("Hi", *payload)
		}
	}
}

func (s *notificationSuite) TestWebhookPayload() {
	jsonData := `{"iama": "potato"}`
	s.n.ID = bson.NewObjectId()
	s.n.Target.Type = "evergreen-webhook"
	s.n.Target.Target = event.WebhookSubscriber{}
	s.n.Payload = jsonData

	s.NoError(InsertMany(s.n))

	n, err := Find(s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal(jsonData, *n.Payload.(*string))

	c, err := n.Composer()
	s.NoError(err)
	s.Require().NotNil(c)
	s.Equal(jsonData, c.String())
}

func (s *notificationSuite) TestJIRACommentPayload() {
	s.n.ID = bson.NewObjectId()
	s.n.Target.Type = "jira-comment"
	target := "BF-1234"
	s.n.Target.Target = target
	s.n.Payload = "hi"

	s.NoError(InsertMany(s.n))

	n, err := Find(s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal("hi", *n.Payload.(*string))

	c, err := n.Composer()
	s.NoError(err)
	s.Require().NotNil(c)
	s.Equal("hi", c.String())
}

func (s *notificationSuite) TestJIRAIssuePayload() {
	s.n.ID = bson.NewObjectId()
	s.n.Target.Type = "jira-issue"
	issue := "1234"
	s.n.Target.Target = &issue
	s.n.Payload = &jiraIssuePayload{
		Summary:     "1",
		Description: "2",
		Reporter:    "3",
		Assignee:    "4",
		Type:        "5",
		Components:  []string{"6"},
		Labels:      []string{"7"},
		// ... other fields
		Fields: map[string]string{
			"8":  "9",
			"10": "11",
		},
	}

	s.NoError(InsertMany(s.n))

	n, err := Find(s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal(s.n, *n)

	c, err := n.Composer()
	s.NoError(err)
	s.Require().NotNil(c)

	fields, ok := c.Raw().(message.JiraIssue)
	s.True(ok)

	s.Equal("1234", fields.Project)
	s.Equal("1", fields.Summary)
	s.Equal("2", fields.Description)
	s.Equal("3", fields.Reporter)
	s.Equal("4", fields.Assignee)
	s.Equal("5", fields.Type)
	s.Equal([]string{"6"}, fields.Components)
	s.Equal([]string{"7"}, fields.Labels)
	s.Len(fields.Fields, 2)
}

func (s *notificationSuite) TestEmailPayload() {
	s.n.ID = bson.NewObjectId()
	s.n.Target.Type = "email"
	email := "a@a.a"
	s.n.Target.Target = &email
	s.n.Payload = &EmailPayload{
		Headers: map[string]string{
			"8":  "9",
			"10": "11",
		},
		Subject: "subject",
		Body:    []byte("body"),
	}

	s.NoError(InsertMany(s.n))

	n, err := Find(s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal(s.n, *n)

	c, err := n.Composer()
	s.NoError(err)
	s.Require().NotNil(c)

	_, ok := c.Raw().(message.Fields)
	s.True(ok)
}

func (s *notificationSuite) TestSlackPayload() {
	s.n.ID = bson.NewObjectId()
	s.n.Target.Type = "slack"
	slack := "#general"
	s.n.Target.Target = &slack
	data := "text"
	s.n.Payload = &data

	s.NoError(InsertMany(s.n))

	n, err := Find(s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal(s.n, *n)

	c, err := n.Composer()
	s.NoError(err)
	s.Require().NotNil(c)

	s.Equal("text", c.String())
}

func (s *notificationSuite) TestGithubPayload() {
	s.n.ID = bson.NewObjectId()
	s.n.Target.Type = "github_pull_request"
	s.n.Target.Target = &event.GithubPullRequestSubscriber{}
	s.n.Payload = &GithubStatusAPIPayload{
		Status:      "failure",
		Context:     "evergreen",
		URL:         "https://example.com",
		Description: "hi",
	}

	s.NoError(InsertMany(s.n))

	n, err := Find(s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal(s.n, *n)

	c, err := n.Composer()
	s.NoError(err)
	s.Require().NotNil(c)

	_, ok := c.Raw().(message.Fields)
	s.True(ok)
}
