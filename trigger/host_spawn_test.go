package trigger

import (
	"context"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/grip/message"
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
	tSetupScript  *spawnHostSetupScriptTriggers
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
		s.tSetupScript = makeSpawnHostSetupScriptTriggers().(*spawnHostSetupScriptTriggers)
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

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	n, err := s.tProvisioning.Process(ctx, &sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tProvisioning.Process(ctx, &sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber.Type = event.JIRAIssueSubscriberType
	n, err = s.tProvisioning.Process(ctx, &sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.tProvisioning.Process(ctx, &sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.tProvisioning.Process(ctx, &sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.tProvisioning.Process(ctx, &sub)
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

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	n, err := s.tProvisioning.Process(ctx, &sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tProvisioning.Process(ctx, &sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber.Type = event.JIRAIssueSubscriberType
	n, err = s.tProvisioning.Process(ctx, &sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.tProvisioning.Process(ctx, &sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.tProvisioning.Process(ctx, &sub)
	s.Nil(n)
	s.Require().Error(err)
	s.Contains(err.Error(), "unsupported subscriber type")

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.tProvisioning.Process(ctx, &sub)
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

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	n, err := s.tStateChange.Process(ctx, &sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tStateChange.Process(ctx, &sub)
	s.NotNil(n)
	s.NoError(err)

	sub.Subscriber.Type = event.JIRAIssueSubscriberType
	n, err = s.tStateChange.Process(ctx, &sub)
	s.Nil(n)
	s.Error(err)

	sub.Subscriber.Type = event.JIRACommentSubscriberType
	n, err = s.tStateChange.Process(ctx, &sub)
	s.Nil(n)
	s.Error(err)

	sub.Subscriber.Type = event.GithubPullRequestSubscriberType
	n, err = s.tStateChange.Process(ctx, &sub)
	s.Nil(n)
	s.Error(err)

	sub.Subscriber.Type = event.EvergreenWebhookSubscriberType
	n, err = s.tStateChange.Process(ctx, &sub)
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

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	n, err := s.tStateChange.Process(ctx, &sub)
	s.Nil(n)
	s.NoError(err, "should suppress notification for successfully stopping spawn host")

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tStateChange.Process(ctx, &sub)
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

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	n, err := s.tStateChange.Process(ctx, &sub)
	s.NotNil(n, "should create notification for error trying to stop spawn host")
	s.NoError(err)

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tStateChange.Process(ctx, &sub)
	s.NotNil(n, "should create notification for error trying to stop spawn host")
	s.NoError(err)
}

func (s *spawnHostTriggersSuite) TestSpawnHostSetupScriptCompletion() {
	// Host script succeeded
	s.e.EventType = event.EventHostScriptExecuted
	_, ok := s.e.Data.(*event.HostEventData)
	s.Require().True(ok)
	s.NoError(s.h.Insert(s.ctx))
	s.NoError(s.tSetupScript.Fetch(s.ctx, &s.e))

	sub := event.Subscription{
		Trigger:    event.TriggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	n, err := s.tSetupScript.Process(ctx, &sub)
	s.Require().NotNil(n, "should create notification for setup script completion")
	s.NoError(err)
	slackResponse, ok := n.Payload.(*notification.SlackPayload)
	s.Require().True(ok)
	s.Require().NotNil(slackResponse)
	s.Contains(slackResponse.Body, "The setup script for spawn host")
	s.Contains(slackResponse.Body, "has succeeded")

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tSetupScript.Process(ctx, &sub)
	s.Require().NotNil(n, "should create notification for setup script completion")
	s.NoError(err)
	emailResponse, ok := n.Payload.(*message.Email)
	s.Require().True(ok)
	s.Require().NotNil(emailResponse)
	s.Equal(emailResponse.Subject, "The setup script for spawn host has succeeded")

	// Host script failed
	s.e.EventType = event.EventHostScriptExecuteFailed
	_, ok = s.e.Data.(*event.HostEventData)
	s.Require().True(ok)
	s.NoError(s.tSetupScript.Fetch(s.ctx, &s.e))

	sub = event.Subscription{
		Trigger:    event.TriggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	n, err = s.tSetupScript.Process(ctx, &sub)
	s.Require().NotNil(n, "should create notification for setup script completion")
	s.NoError(err)
	slackResponse, ok = n.Payload.(*notification.SlackPayload)
	s.Require().True(ok)
	s.Require().NotNil(slackResponse)
	s.Contains(slackResponse.Body, "The setup script for spawn host")
	s.Contains(slackResponse.Body, "has failed")

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tSetupScript.Process(ctx, &sub)
	s.Require().NotNil(n, "should create notification for setup script completion")
	s.NoError(err)
	emailResponse, ok = n.Payload.(*message.Email)
	s.Require().True(ok)
	s.Require().NotNil(emailResponse)
	s.Equal(emailResponse.Subject, "The setup script for spawn host has failed")

	// Host script failed to start
	s.e.Data = &event.HostEventData{
		Logs: evergreen.FetchingTaskDataUnfinishedError,
	}
	_, ok = s.e.Data.(*event.HostEventData)
	s.Require().True(ok)
	s.NoError(s.tSetupScript.Fetch(s.ctx, &s.e))

	sub = event.Subscription{
		Trigger:    event.TriggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	n, err = s.tSetupScript.Process(ctx, &sub)
	s.Require().NotNil(n, "should create notification for setup script completion")
	s.NoError(err)
	slackResponse, ok = n.Payload.(*notification.SlackPayload)
	s.Require().True(ok)
	s.Require().NotNil(slackResponse)
	s.Contains(slackResponse.Body, "The setup script for spawn host")
	s.Contains(slackResponse.Body, "has failed to start")

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tSetupScript.Process(ctx, &sub)
	s.Require().NotNil(n, "should create notification for setup script completion")
	s.NoError(err)
	emailResponse, ok = n.Payload.(*message.Email)
	s.Require().True(ok)
	s.Require().NotNil(emailResponse)
	s.Equal(emailResponse.Subject, "The setup script for spawn host has failed to start")
}

func (s *spawnHostTriggersSuite) TestSpawnHostCreationErrorCreatesNotification() {
	s.e.EventType = event.EventSpawnHostCreatedError
	s.NoError(s.h.Insert(s.ctx))
	s.NoError(s.tStateChange.Fetch(s.ctx, &s.e))

	sub := event.Subscription{
		Trigger:    event.TriggerOutcome,
		Subscriber: event.NewSlackSubscriber("@test.user"),
	}

	ctx, cancel := context.WithCancel(s.ctx)
	defer cancel()

	n, err := s.tStateChange.Process(ctx, &sub)
	s.NoError(err)
	s.NotZero(n, "should create Slack notification for spawn host creation error and user subscribed to spawn host outcomes")

	sub.Subscriber = event.NewEmailSubscriber("example@domain.invalid")
	n, err = s.tStateChange.Process(ctx, &sub)
	s.NoError(err)
	s.NotZero(n, "should create email notification for spawn host creation error and user subscribed to spawn host outcomes")
}
