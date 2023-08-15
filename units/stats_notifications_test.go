package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
)

type notificationsStatsCollectorSuite struct {
	suite.Suite
	expectedTime time.Time
	suiteCtx     context.Context
	cancel       context.CancelFunc
	ctx          context.Context
}

func TestNotificationsStatsCollectorJob(t *testing.T) {
	s := &notificationsStatsCollectorSuite{}
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)

	suite.Run(t, s)
}

func (s *notificationsStatsCollectorSuite) SetupTest() {
	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())

	s.NoError(db.ClearCollections(event.EventCollection, notification.Collection))
	s.expectedTime = time.Time{}.Add(time.Second)

	events := []event.EventLogEntry{
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
		},
		{
			ID:           mgobson.NewObjectId().Hex(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
			ProcessedAt:  s.expectedTime,
		},
	}

	for i := range events {
		s.NoError(db.Insert(event.EventCollection, events[i]))
	}

	n := []notification.Notification{
		{
			ID: "1",
			Subscriber: event.Subscriber{
				Type: event.EmailSubscriberType,
			},
		},
		{
			ID: "2",
			Subscriber: event.Subscriber{
				Type: event.SlackSubscriberType,
			},
		},
		{
			ID: "3",
			Subscriber: event.Subscriber{
				Type: event.JIRAIssueSubscriberType,
			},
		},
		{
			ID: "4",
			Subscriber: event.Subscriber{
				Type: event.JIRACommentSubscriberType,
			},
		},
		{
			ID: "5",
			Subscriber: event.Subscriber{
				Type: event.GithubPullRequestSubscriberType,
			},
		},
		{
			ID: "6",
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
		},
		{
			ID: "7",
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
			SentAt: time.Now(),
		},
	}
	for i := range n {
		s.NoError(db.Insert(notification.Collection, n[i]))
	}
}

func (s *notificationsStatsCollectorSuite) TestStatsCollector() {
	sender := send.MakeInternalLogger()

	job := makeNotificationsStatsCollector()
	job.SetID(s.T().Name())
	job.logger = logging.MakeGrip(sender)
	job.Run(s.ctx)
	s.NoError(job.Error())

	msg, ok := sender.GetMessageSafe()
	s.Require().True(ok)
	data := msg.Message.String()

	s.Contains(data, "pending_notifications_by_type=")
}
