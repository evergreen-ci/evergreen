package notification

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/event"
	_ "github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/message"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

type notificationSuite struct {
	suite.Suite

	n   Notification
	env evergreen.Environment
}

func TestNotifications(t *testing.T) {
	suite.Run(t, &notificationSuite{})
}

func TestUnmarshalBSON(t *testing.T) {
	expectedPayload := message.GithubStatus{
		State:       message.GithubStateFailure,
		Context:     "evergreen",
		URL:         "https://example.com",
		Description: "something failed",
	}
	original := Notification{
		ID: "notification",
		Subscriber: event.Subscriber{
			Type: event.GithubPullRequestSubscriberType,
			Target: &event.GithubPullRequestSubscriber{
				Owner:    "evergreen-ci",
				Repo:     "evergreen",
				PRNumber: 9001,
			},
		},
		Payload: expectedPayload,
	}

	data, err := bson.Marshal(original)
	require.NoError(t, err)

	var result Notification
	require.NoError(t, bson.Unmarshal(data, &result))
	assert.Equal(t, original.ID, result.ID)
	assert.Equal(t, original.Subscriber, result.Subscriber)
	require.IsType(t, &message.GithubStatus{}, result.Payload)
	assert.Equal(t, expectedPayload, *result.Payload.(*message.GithubStatus))
}

func (s *notificationSuite) SetupTest() {
	s.NoError(db.Clear(Collection))
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
		Payload: message.GithubStatus{
			State:       message.GithubStateFailure,
			Context:     "evergreen",
			URL:         "https://example.com",
			Description: "something failed",
		},
	}
	s.Empty(s.n.ID)

	s.env = &mock.Environment{}
}

func (s *notificationSuite) TestMarkSent() {
	// MarkSent with notification that hasn't been stored
	s.EqualError(s.n.MarkSent(s.T().Context()), "notification has no ID")
	s.Empty(s.n.ID)
	s.Empty(s.n.Error)
	s.Zero(s.n.SentAt)

	s.n.ID = "1"
	s.NoError(InsertMany(s.T().Context(), s.n))

	// mark that notification as sent
	s.NoError(s.n.MarkSent(s.T().Context()))
	s.Empty(s.n.Error)
	s.NotZero(s.n.SentAt)

	n, err := Find(s.T().Context(), s.n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.Empty(n.Error)
	s.NotZero(n.SentAt)
}

func (s *notificationSuite) TestMarkError() {
	// MarkError, uninserted notification
	s.EqualError(s.n.MarkError(s.T().Context(), errors.New("")), "notification has no ID")
	s.Empty(s.n.ID)
	s.Empty(s.n.Error)
	s.Zero(s.n.SentAt)

	s.NoError(s.n.MarkError(s.T().Context(), nil))
	s.Empty(s.n.ID)
	s.Empty(s.n.Error)
	s.Zero(s.n.SentAt)

	// MarkError, non nil error
	s.n.ID = "1"
	s.NoError(InsertMany(s.T().Context(), s.n))

	s.NoError(s.n.MarkError(s.T().Context(), errors.New("test")))
	s.NotEmpty(s.n.ID)
	s.Equal("test", s.n.Error)
	s.NotZero(s.n.SentAt)
	n, err := Find(s.T().Context(), s.n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.Equal("test", n.Error)
	s.NotZero(n.SentAt)
}

func (s *notificationSuite) TestMarkErrorWithNilErrorHasNoSideEffect() {
	s.n.ID = "1"
	s.NoError(InsertMany(s.T().Context(), s.n))

	// nil error should have no side effect
	s.NoError(s.n.MarkError(s.T().Context(), nil))
	n, err := Find(s.T().Context(), s.n.ID)
	s.NoError(err)
	s.Empty(n.Error)
	s.Zero(n.SentAt)
}

func (s *notificationSuite) TestInsertMany() {
	s.n.ID = "1-outcome-12345-32434"

	n2 := Notification{
		ID: "1-outcome-67890-32434",
		Subscriber: event.Subscriber{
			Type:   event.SlackSubscriberType,
			Target: "#general",
		},
		Payload: &SlackPayload{
			Body: "slack hi",
		},
	}

	payload2 := "jira hi"
	n3 := Notification{
		ID: "2-success-45566-32433",
		Subscriber: event.Subscriber{
			Type:   event.JIRACommentSubscriberType,
			Target: "ABC-1234",
		},
		Payload: &payload2,
	}

	slice := []Notification{s.n, n2, n3}

	s.NoError(InsertMany(s.T().Context(), slice...))

	for i := range slice {
		s.NotEmpty(slice[i].ID)
	}

	out := []Notification{}
	s.NoError(db.FindAllQ(s.T().Context(), Collection, db.Q{}, &out))
	s.Len(out, 3)

	for _, n := range out {
		s.Zero(n.SentAt)
		s.Empty(n.Error)

		if n.ID == slice[0].ID {
			payload, ok := n.Payload.(*message.GithubStatus)
			s.Require().True(ok)
			s.Equal("failure", string(payload.State))
			s.Equal("evergreen", payload.Context)
			s.Equal("https://example.com", payload.URL)
			s.Equal("something failed", payload.Description)

		} else if n.ID == slice[1].ID {
			s.Equal(event.SlackSubscriberType, n.Subscriber.Type)
			payload, ok := n.Payload.(*SlackPayload)
			s.True(ok)
			s.Equal("slack hi", payload.Body)

		} else if n.ID == slice[2].ID {
			s.Equal(event.JIRACommentSubscriberType, n.Subscriber.Type)
			payload, ok := n.Payload.(*string)
			s.True(ok)
			s.Equal("jira hi", *payload)

		} else {
			s.T().Errorf("unknown notification")
		}
	}

	out2, err := FindByEventID(s.T().Context(), "1")
	s.NoError(err)
	s.Len(out2, 2)
}

func (s *notificationSuite) TestInsertManyUnordered() {
	s.n.ID = "duplicate"

	n2 := Notification{
		ID: "duplicate",
		Subscriber: event.Subscriber{
			Type:   event.SlackSubscriberType,
			Target: "#general",
		},
		Payload: &SlackPayload{
			Body: "slack hi",
		},
	}

	payload2 := "jira hi"
	n3 := Notification{
		ID: "not_a_duplicate",
		Subscriber: event.Subscriber{
			Type:   event.JIRACommentSubscriberType,
			Target: "ABC-1234",
		},
		Payload: &payload2,
	}

	slice := []Notification{s.n, n2, n3}

	s.Error(InsertMany(s.T().Context(), slice...))
	out := []Notification{}
	s.NoError(db.FindAllQ(s.T().Context(), Collection, db.Q{}, &out))
	s.Len(out, 2)
}

func (s *notificationSuite) TestWebhookPayload() {
	jsonData := `{"iama": "potato"}`
	s.n.ID = "1"
	s.n.Subscriber.Type = event.EvergreenWebhookSubscriberType
	s.n.Subscriber.Target = event.WebhookSubscriber{
		URL:    "https://example.com",
		Secret: []byte("it's dangerous to go alone. take this!"),
	}
	s.n.Payload = &util.EvergreenWebhook{
		Body: []byte(jsonData),
	}

	s.NoError(InsertMany(s.T().Context(), s.n))

	n, err := Find(s.T().Context(), s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.JSONEq(jsonData, string(n.Payload.(*util.EvergreenWebhook).Body))

	c, err := n.Composer(s.T().Context())
	s.NoError(err)
	s.Require().NotNil(c)
	s.True(c.Loggable())
}

func (s *notificationSuite) TestJIRACommentPayload() {
	s.n.ID = "1"
	s.n.Subscriber.Type = event.JIRACommentSubscriberType
	target := "BF-1234"
	s.n.Subscriber.Target = target
	s.n.Payload = "hi"

	s.NoError(InsertMany(s.T().Context(), s.n))

	n, err := Find(s.T().Context(), s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal("hi", *n.Payload.(*string))

	c, err := n.Composer(s.T().Context())
	s.NoError(err)
	s.Require().NotNil(c)
	s.Equal("hi", c.String())
	s.True(c.Loggable())
}

func (s *notificationSuite) TestJIRAIssuePayload() {
	s.n.ID = "1"
	s.n.Subscriber.Type = event.JIRAIssueSubscriberType
	issue := event.JIRAIssueSubscriber{
		Project:   "ABC",
		IssueType: "Magic",
	}
	s.n.Subscriber.Target = &issue
	s.n.Payload = &message.JiraIssue{
		Summary:     "1",
		Description: "2",
		Reporter:    "3",
		Assignee:    "4",
		Components:  []string{"6"},
		Labels:      []string{"7"},
		Fields: map[string]any{
			"8":  "9",
			"10": "11",
		},
		FixVersions: []string{},
	}

	s.NoError(InsertMany(s.T().Context(), s.n))

	n, err := Find(s.T().Context(), s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal(s.n, *n)

	c, err := n.Composer(s.T().Context())
	s.NoError(err)
	s.Require().NotNil(c)
	s.True(c.Loggable())
}

func TestJIRAIssueCallbackLogsEventAfterCtxCancel(t *testing.T) {
	require.NoError(t, db.ClearCollections(Collection, event.EventCollection))
	t.Cleanup(func() {
		assert.NoError(t, db.ClearCollections(Collection, event.EventCollection))
	})

	taskID := "task_jira_callback_ctx_cancel"
	issueKey := "EVG-99999"
	n := Notification{
		ID: "jira-callback-ctx-cancel",
		Subscriber: event.Subscriber{
			Type: event.JIRAIssueSubscriberType,
			Target: &event.JIRAIssueSubscriber{
				Project:   "EVG",
				IssueType: "Build Failure",
			},
		},
		Payload: &message.JiraIssue{
			Summary:     "title",
			Description: "body",
		},
	}
	n.SetTaskMetadata(taskID, 0, "admin.user")
	require.NoError(t, InsertMany(t.Context(), n))

	found, err := Find(t.Context(), n.ID)
	require.NoError(t, err)
	require.NotNil(t, found)

	jobCtx, cancel := context.WithCancel(t.Context())
	c, err := found.Composer(jobCtx)
	require.NoError(t, err)
	require.NotNil(t, c)

	cancel()

	raw, ok := c.Raw().(*message.JiraIssue)
	require.True(t, ok)
	require.NotNil(t, raw.Callback)
	raw.Callback(issueKey)

	events, err := event.Find(t.Context(), event.TaskEventsForId(taskID))
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, event.TaskJiraAlertCreated, events[0].EventType)
	assert.Equal(t, taskID, events[0].ResourceId)
	data, ok := events[0].Data.(*event.TaskEventData)
	require.True(t, ok)
	assert.Equal(t, issueKey, data.JiraIssue)
	assert.Equal(t, "admin.user", data.UserId)
}

func (s *notificationSuite) TestEmailPayload() {
	s.n.ID = "1"
	s.n.Subscriber.Type = event.EmailSubscriberType
	email := "a@a.a"
	s.n.Subscriber.Target = &email
	s.n.Payload = &message.Email{
		Headers: map[string][]string{
			"8":  {"9"},
			"10": {"11"},
		},
		Subject:    "subject",
		Body:       "body",
		Recipients: []string{},
	}

	s.NoError(InsertMany(s.T().Context(), s.n))

	n, err := Find(s.T().Context(), s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal(s.n, *n)

	c, err := n.Composer(s.T().Context())
	s.NoError(err)
	s.Require().NotNil(c)

	_, ok := c.Raw().(*message.Email)
	s.True(ok)
	s.True(c.Loggable())
}

func (s *notificationSuite) TestSlackPayload() {
	s.n.ID = "1"
	s.n.Subscriber.Type = event.SlackSubscriberType
	slack := "#general"
	s.n.Subscriber.Target = &slack
	s.n.Payload = &SlackPayload{
		Body:        "Hi",
		Attachments: []message.SlackAttachment{},
	}

	s.NoError(InsertMany(s.T().Context(), s.n))

	n, err := Find(s.T().Context(), s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal(s.n, *n)

	c, err := n.Composer(s.T().Context())
	s.NoError(err)
	s.Require().NotNil(c)
	s.True(c.Loggable())
}

func (s *notificationSuite) TestGithubPayload() {
	s.n.ID = "1"
	s.n.Subscriber.Type = event.GithubPullRequestSubscriberType
	s.n.Subscriber.Target = &event.GithubPullRequestSubscriber{}
	s.n.Payload = &message.GithubStatus{
		State:       message.GithubStateFailure,
		Context:     "evergreen",
		URL:         "https://example.com",
		Description: "hi",
	}

	s.NoError(InsertMany(s.T().Context(), s.n))

	n, err := Find(s.T().Context(), s.n.ID)
	s.NoError(err)
	s.NotNil(n)

	s.Equal(s.n, *n)

	c, err := n.Composer(s.T().Context())
	s.NoError(err)
	s.Require().NotNil(c)
	s.True(c.Loggable())
}

func (s *notificationSuite) TestCollectUnsentNotificationStats() {
	types := []string{event.GithubPullRequestSubscriberType, event.EmailSubscriberType,
		event.SlackSubscriberType, event.EvergreenWebhookSubscriberType,
		event.JIRACommentSubscriberType, event.JIRAIssueSubscriberType,
		event.GithubCheckSubscriberType, event.GithubMergeSubscriberType}

	n := []Notification{}
	// add one of every notification, unsent
	for i, type_ := range types {
		n = append(n, s.n)
		n[i].ID = primitive.NewObjectID().Hex()
		n[i].Subscriber.Type = type_
		s.NoError(db.Insert(s.T().Context(), Collection, n[i]))
	}

	// add one more, mark it sent
	s.n.ID = primitive.NewObjectID().Hex()
	s.n.SentAt = time.Now()
	s.NoError(db.Insert(s.T().Context(), Collection, s.n))

	stats, err := CollectUnsentNotificationStats(s.T().Context())
	s.NoError(err)
	s.Require().NotNil(stats)

	// we should count 1 of each type
	v := reflect.ValueOf(stats).Elem()
	for i := 0; i < v.NumField(); i++ {
		f := v.Field(i)
		s.Equal(1, int(f.Int()))
	}
}

func (s *notificationSuite) TestFindUnprocessed() {
	s.n.ID = "unsent"
	s.NoError(db.Insert(s.T().Context(), Collection, s.n))
	s.n.ID = "sent"
	s.n.SentAt = time.Now()
	s.NoError(db.Insert(s.T().Context(), Collection, s.n))

	unprocessedNotifications, err := FindUnprocessed(s.T().Context())
	s.NoError(err)
	s.Len(unprocessedNotifications, 1)
	s.Equal("unsent", unprocessedNotifications[0].ID)
}
