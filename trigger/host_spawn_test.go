package trigger

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/stretchr/testify/suite"
)

func TestSpawnHostTriggers(t *testing.T) {
	suite.Run(t, &spawnHostTriggersSuite{})
}

type spawnHostTriggersSuite struct {
	e             event.EventLogEntry
	h             host.Host
	tProvisioning *spawnHostProvisioningTriggers
	tStateChange  *spawnHostStateChangeTriggers
	ctx           context.Context
	cancel        context.CancelFunc

	suite.Suite
}

func (s *spawnHostTriggersSuite) SetupTest() {
	s.ctx, s.cancel = context.WithCancel(context.Background())
	s.Require().NoError(db.ClearCollections(host.Collection))
	s.h = host.Host{
		Id:          "t0",
		User:        "root",
		Host:        "domain.invalid",
		Provisioned: true,
		UserHost:    true,
		Distro: distro.Distro{
			Id: "test",
		},
	}

	s.e = event.EventLogEntry{
		ResourceId:   s.h.Id,
		Data:         &event.HostEventData{},
		ResourceType: event.ResourceTypeHost,
	}

	s.Require().NotPanics(func() {
		s.tProvisioning = makeSpawnHostProvisioningTriggers().(*spawnHostProvisioningTriggers)
		s.tStateChange = makeSpawnHostStateChangeTriggers().(*spawnHostStateChangeTriggers)
	})
}

func (s *spawnHostTriggersSuite) TearDownTest() {
	s.cancel()
}

func (s *spawnHostTriggersSuite) TestSuccessfulSpawn() {
	s.e.EventType = event.EventHostProvisioned
	s.NoError(s.h.Insert(s.ctx))
	s.NoError(s.tProvisioning.Fetch(s.ctx, &s.e))

	sub := event.Subscription{
		Trigger:    event.TriggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	n, err := s.tProvisioning.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tProvisioning.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber.Type = event.JIRAIssueSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")
}

func (s *spawnHostTriggersSuite) TestFailedSpawn() {
	s.e.EventType = event.EventHostProvisioned
	s.NoError(s.h.Insert(s.ctx))
	s.e.EventType = event.EventHostProvisionError
	s.NoError(s.tProvisioning.Fetch(s.ctx, &s.e))

	sub := event.Subscription{
		Trigger:    event.TriggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	n, err := s.tProvisioning.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tProvisioning.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber.Type = event.JIRAIssueSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")
}

func (s *spawnHostTriggersSuite) TestSpawnHostStateChange() {
	s.e.EventType = event.EventHostStarted
	s.NoError(s.h.Insert(s.ctx))
	s.NoError(s.tStateChange.Fetch(s.ctx, &s.e))

	sub := event.Subscription{
		Trigger:    event.TriggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	n, err := s.tStateChange.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tStateChange.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber.Type = event.JIRAIssueSubscriberType
	n, err = s.tStateChange.Process(&sub)
	s.Nil(n)
	s.Error(err)

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.tStateChange.Process(&sub)
	s.Nil(n)
	s.Error(err)

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.tStateChange.Process(&sub)
	s.Nil(n)
	s.Error(err)

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.tStateChange.Process(&sub)
	s.Nil(n)
	s.Error(err)
}

func (s *spawnHostTriggersSuite) TestSpawnHostSuccessfulStopForSleepScheduleDoesNotCreateNotification() {
	s.e.EventType = event.EventHostStarted
	data, ok := s.e.Data.(*event.HostEventData)
	s.Require().True(ok)
	data.Successful = true
	data.Source = string(evergreen.ModifySpawnHostSleepSchedule)
	s.NoError(s.h.Insert(s.ctx))
	s.NoError(s.tStateChange.Fetch(s.ctx, &s.e))

	sub := event.Subscription{
		Trigger:    event.TriggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	n, err := s.tStateChange.Process(&sub)
	s.Nil(n)
	s.NoError(err, "should suppress notification for successfully stopping spawn host")

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tStateChange.Process(&sub)
	s.Nil(n, "should suppress notification for successfully stopping spawn host")
	s.NoError(err)
}

func (s *spawnHostTriggersSuite) TestSpawnHostUnsuccessfulStopForSleepScheduleCreatesNotification() {
	s.e.EventType = event.EventHostStopped
	data, ok := s.e.Data.(*event.HostEventData)
	s.Require().True(ok)
	data.Successful = false
	data.Source = string(evergreen.ModifySpawnHostSleepSchedule)
	s.NoError(s.h.Insert(s.ctx))
	s.NoError(s.tStateChange.Fetch(s.ctx, &s.e))

	sub := event.Subscription{
		Trigger:    event.TriggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	n, err := s.tStateChange.Process(&sub)
	s.NotNil(n, "should create notification for error trying to stop spawn host")
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tStateChange.Process(&sub)
	s.NotNil(n, "should create notification for error trying to stop spawn host")
	s.NoError(err)
}
