package trigger

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

func TestSpawnHostTriggers(t *testing.T) {
	suite.Run(t, &spawnHostTriggersSuite{})
}

type spawnHostTriggersSuite struct {
	e event.EventLogEntry
	h host.Host
	t *spawnHostTriggers

	suite.Suite
}

func (s *spawnHostTriggersSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
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
		EventType:    event.EventHostProvisioned,
	}

	s.Require().NotPanics(func() {
		s.t = makeSpawnHostTriggers().(*spawnHostTriggers)
	})
}

func (s *spawnHostTriggersSuite) TestSuccessfulSpawn() {
	s.NoError(s.h.Insert())
	s.NoError(s.t.Fetch(&s.e))

	sub := event.Subscription{
		Trigger:    triggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	n, err := s.t.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.t.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber.Type = event.JIRAIssueSubscriberType
	n, err = s.t.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.t.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.t.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.t.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")
}

func (s *spawnHostTriggersSuite) TestFailedSpawn() {
	s.NoError(s.h.Insert())
	s.e.EventType = event.EventHostProvisionError
	s.NoError(s.t.Fetch(&s.e))

	sub := event.Subscription{
		Trigger:    triggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	n, err := s.t.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.t.Process(&sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber.Type = event.JIRAIssueSubscriberType
	n, err = s.t.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.t.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.t.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.t.Process(&sub)
	s.Nil(n)
	s.EqualError(err, "unsupported subscriber type: ")
}
