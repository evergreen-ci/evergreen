package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip/logging"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type notificationsStatsCollectorSuite struct {
	suite.Suite
	expectedTime time.Time
}

func TestNotificationsStatsCollectorJob(t *testing.T) {
	suite.Run(t, &notificationsStatsCollectorSuite{})
}

func (s *notificationsStatsCollectorSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
}

func (s *notificationsStatsCollectorSuite) SetupTest() {
	s.NoError(db.ClearCollections(event.AllLogCollection, notification.NotificationsCollection))
	s.expectedTime = time.Time{}.Add(time.Second)

	events := []event.EventLogEntry{
		{
			ID:           bson.NewObjectId(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
		},
		{
			ID:           bson.NewObjectId(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
		},
		{
			ID:           bson.NewObjectId(),
			ResourceType: event.ResourceTypeHost,
			Data:         event.HostEventData{},
			ProcessedAt:  s.expectedTime,
		},
	}

	for i := range events {
		s.NoError(db.Insert(event.AllLogCollection, events[i]))
	}

	n := []notification.Notification{
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.EmailSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.SlackSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.JIRAIssueSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.JIRACommentSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.GithubPullRequestSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
		},
		{
			ID: bson.NewObjectId(),
			Subscriber: event.Subscriber{
				Type: event.EvergreenWebhookSubscriberType,
			},
			SentAt: time.Now(),
		},
	}
	for i := range n {
		s.NoError(db.Insert(notification.NotificationsCollection, n[i]))
	}
}

func (s *notificationsStatsCollectorSuite) TestStatsCollector() {
	sender := send.MakeInternalLogger()

	job := makeNotificationsStatsCollector()
	job.SetID("TestStatsCollector")
	job.logger = logging.MakeGrip(sender)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	job.Run(ctx)
	s.NoError(job.Error())

	msg, ok := sender.GetMessageSafe()
	s.Require().True(ok)
	data := msg.Message.String()

	s.Contains(data, "pending_notifications_by_type=")
}

func (s *notificationsStatsCollectorSuite) TestStatsCollectorWithCancelledContext() {
	sender := send.MakeInternalLogger()
	startTime := time.Now().Round(time.Millisecond).Truncate(0)

	job := makeNotificationsStatsCollector()
	job.SetID("TestStatsCollectorWithCancelledContext")
	job.logger = logging.MakeGrip(sender)
	job.UpdateTimeInfo(amboy.JobTimeInfo{
		Start: startTime,
	})

	// expires almost immediately
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(100*time.Millisecond))
	defer cancel()
	_ = <-ctx.Done()

	job.Run(ctx)
	s.Error(job.Error(), context.Canceled.Error())

	msg, ok := sender.GetMessageSafe()
	s.Require().True(ok)
	data := msg.Message.String()
	s.Contains(data, "context deadline exceeded")

	_, ok = sender.GetMessageSafe()
	s.False(ok)
}
