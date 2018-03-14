package notification

import (
	"errors"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/testutil"
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
		Type: "slack",
		Target: event.Subscriber{
			Type:   "slack",
			Target: "#general",
		},
		Payload: &SlackPayload{
			URL:  "https://example.com",
			Text: "Hi",
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
	s.EqualError(s.n.MarkSent(nil), "notification has no ID")
	s.False(s.n.ID.Valid())
	s.Empty(s.n.Error)
	s.Zero(s.n.SentAt)

	// with non-nil err
	s.EqualError(s.n.MarkSent(errors.New("")), "notification has no ID")
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
	s.NotNil(n.Payload)
	s.IsType(&SlackPayload{}, n.Payload)
	payload := n.Payload.(*SlackPayload)
	s.Equal("https://example.com", payload.URL)
	s.Equal("Hi", payload.Text)

	// mark that notification as sent, with nil err
	s.NoError(s.n.MarkSent(nil))
	s.True(s.n.ID.Valid())
	s.Empty(s.n.Error)
	s.NotZero(s.n.SentAt)
	n, err = Find(s.n.ID)
	s.NoError(err)
	s.Require().NotNil(n)
	s.Empty(n.Error)
	s.NotZero(n.SentAt)

	// mark that notification as sent, with non-nil err
	s.NoError(s.n.MarkSent(errors.New("test")))
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
		ID:   bson.NewObjectId(),
		Type: "slack",
		Target: event.Subscriber{
			Type:   "slack",
			Target: "#general",
		},
	}

	payload := "Hi"
	n3 := Notification{
		ID:   bson.NewObjectId(),
		Type: "jira-comment",
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
		if out[i].Type == "jira-comment" {
			payload, ok := out[i].Payload.(*string)
			s.Require().True(ok)
			s.Equal("Hi", *payload)
		}
	}
}
