package notification

import (
	"errors"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/k0kubun/pp"
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
		Subscriber: event.Subscriber{
			Type: event.GithubPullRequestSubscriberType,
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

	s.False(s.n.ID.Valid())
}

func (s *notificationSuite) TestMarkSent() {
	// MarkSent with notification that hasn't been stored
	s.EqualError(s.n.MarkSent(), "notification has no ID")
	s.False(s.n.ID.Valid())
	s.Empty(s.n.Error)
	s.Zero(s.n.SentAt)

	s.n.ID = bson.NewObjectId()
	s.NoError(InsertMany(s.n))

	// mark that notification as sent
	s.NoError(s.n.MarkSent())
	s.Empty(s.n.Error)
	s.NotZero(s.n.SentAt)

	n, err := Find(s.n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.Empty(n.Error)
	s.NotZero(n.SentAt)
}

func (s *notificationSuite) TestMarkError() {
	// MarkError, uninserted notification
	s.EqualError(s.n.MarkError(errors.New("")), "notification has no ID")
	s.False(s.n.ID.Valid())
	s.Empty(s.n.Error)
	s.Zero(s.n.SentAt)

	s.NoError(s.n.MarkError(nil))
	s.False(s.n.ID.Valid())
	s.Empty(s.n.Error)
	s.Zero(s.n.SentAt)

	// MarkError, non nil error
	s.n.ID = bson.NewObjectId()
	s.NoError(InsertMany(s.n))

	s.NoError(s.n.MarkError(errors.New("test")))
	s.True(s.n.ID.Valid())
	s.Equal("test", s.n.Error)
	s.NotZero(s.n.SentAt)
	n, err := Find(s.n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.Equal("test", n.Error)
	s.NotZero(n.SentAt)

	// nil error should have no side effect
	s.NoError(s.n.MarkError(nil))
	n, err = Find(s.n.ID)
	s.NoError(err)
	s.NotEmpty(n.Error)
	s.NotZero(n.SentAt)
}

func (s *notificationSuite) TestInsertMany() {
	s.n.ID = bson.NewObjectId()

	payload := "slack hi"
	n2 := Notification{
		ID: bson.NewObjectId(),
		Subscriber: event.Subscriber{
			Type:   event.SlackSubscriberType,
			Target: "#general",
		},
		Payload: &payload,
	}

	payload2 := "jira hi"
	n3 := Notification{
		ID: bson.NewObjectId(),
		Subscriber: event.Subscriber{
			Type:   event.JIRACommentSubscriberType,
			Target: "ABC-1234",
		},
		Payload: &payload2,
	}

	slice := []Notification{s.n, n2, n3}

	s.NoError(InsertMany(slice...))

	for i := range slice {
		s.True(slice[i].ID.Valid())
	}

	out := []Notification{}
	s.NoError(db.FindAllQ(NotificationsCollection, db.Q{}, &out))
	s.Len(out, 3)

	for _, n := range out {
		s.Zero(n.SentAt)
		s.Empty(n.Error)

		if n.ID == slice[0].ID {
			s.Require().IsType(&GithubStatusAPIPayload{}, n.Payload)
			payload := n.Payload.(*GithubStatusAPIPayload)
			s.Equal("failure", payload.Status)
			s.Equal("evergreen", payload.Context)
			s.Equal("https://example.com", payload.URL)
			s.Equal("something failed", payload.Description)

		} else if n.ID == slice[1].ID {
			s.Equal(event.SlackSubscriberType, n.Subscriber.Type)
			payload, ok := n.Payload.(*string)
			s.True(ok)
			s.Equal("slack hi", *payload)

		} else if n.ID == slice[2].ID {
			s.Equal(event.JIRACommentSubscriberType, n.Subscriber.Type)
			payload, ok := n.Payload.(*string)
			s.True(ok)
			s.Equal("jira hi", *payload)

		} else {
			s.T().Errorf("unknown notification")
		}
	}
}

func (s *notificationSuite) TestWebhookPayload() {
	jsonData := `{"iama": "potato"}`
	s.n.ID = bson.NewObjectId()
	s.n.Subscriber.Type = event.EvergreenWebhookSubscriberType
	s.n.Subscriber.Target = event.WebhookSubscriber{}
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
	s.n.Subscriber.Type = event.JIRACommentSubscriberType
	target := "BF-1234"
	s.n.Subscriber.Target = target
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
	s.n.Subscriber.Type = event.JIRAIssueSubscriberType
	issue := "1234"
	s.n.Subscriber.Target = &issue
	s.n.Payload = &message.JiraIssue{
		Summary:     "1",
		Description: "2",
		Reporter:    "3",
		Assignee:    "4",
		Type:        "5",
		Components:  []string{"6"},
		Labels:      []string{"7"},
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
	s.n.Subscriber.Type = event.EmailSubscriberType
	email := "a@a.a"
	s.n.Subscriber.Target = &email
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
	s.n.Subscriber.Type = event.SlackSubscriberType
	slack := "#general"
	s.n.Subscriber.Target = &slack
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
	s.n.Subscriber.Type = event.GithubPullRequestSubscriberType
	s.n.Subscriber.Target = &event.GithubPullRequestSubscriber{}
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

func (s *notificationSuite) TestCollectUnsentNotificationStats() {
	types := []string{event.GithubPullRequestSubscriberType, event.EmailSubscriberType,
		event.SlackSubscriberType, event.EvergreenWebhookSubscriberType,
		event.JIRACommentSubscriberType, event.JIRAIssueSubscriberType}

	n := []Notification{}
	for i, type_ := range types {
		n = append(n, s.n)
		n[i].ID = bson.NewObjectId()
		n[i].Subscriber.Type = type_
		s.NoError(db.Insert(NotificationsCollection, n[i]))
	}

	s.n.ID = bson.NewObjectId()
	s.n.SentAt = time.Now()
	s.NoError(db.Insert(NotificationsCollection, s.n))

	stats, err := CollectUnsentNotificationStats()
	s.NoError(err)
	pp.Println(stats)

	for k, _ := range stats {
		s.Equal(1, stats[k])
	}
}
