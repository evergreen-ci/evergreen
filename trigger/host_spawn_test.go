package trigger

import (
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
	s.e.EventType = event.EventHostProvisioned
	s.NoError(s.h.Insert())
	s.NoError(s.tProvisioning.Fetch(&s.e))

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
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")
}

func (s *spawnHostTriggersSuite) TestFailedSpawn() {
	s.e.EventType = event.EventHostProvisioned
	s.NoError(s.h.Insert())
	s.e.EventType = event.EventHostProvisionError
	s.NoError(s.tProvisioning.Fetch(&s.e))

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
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.tProvisioning.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")
}

func (s *spawnHostTriggersSuite) TestSpawnHostStateChange() {
	s.e.EventType = event.EventHostStarted
	s.NoError(s.h.Insert())
	s.NoError(s.tStateChange.Fetch(&s.e))

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
