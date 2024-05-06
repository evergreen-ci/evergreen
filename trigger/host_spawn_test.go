package trigger

import (
	"context"
	"testing"

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

	suite.Suite
}

func (s *spawnHostTriggersSuite) SetupTest() {
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

func (s *spawnHostTriggersSuite) TestSuccessfulSpawn() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.e.EventType = event.EventHostProvisioned
	s.NoError(s.h.Insert(ctx))
	s.NoError(s.tProvisioning.Fetch(ctx, &s.e))

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.e.EventType = event.EventHostProvisioned
	s.NoError(s.h.Insert(ctx))
	s.e.EventType = event.EventHostProvisionError
	s.NoError(s.tProvisioning.Fetch(ctx, &s.e))

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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s.e.EventType = event.EventHostStarted
	s.NoError(s.h.Insert(ctx))
	s.NoError(s.tStateChange.Fetch(ctx, &s.e))

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

func (s *spawnHostTriggersSuite) TestSpawnHostSuccessfulStopForSleepSchedule() {
	// kim: TODO: fill in test
}

func (s *spawnHostTriggersSuite) TestSpawnHostUnsuccessfulStopForSleepSchedule() {
	// kim: TODO: fill in test
}
