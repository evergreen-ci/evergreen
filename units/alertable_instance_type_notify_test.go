package units

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type alertableInstanceTypeSuite struct {
	j *alertableInstanceTypeNotifyJob
	suite.Suite
	suiteCtx context.Context
	cancel   context.CancelFunc
	ctx      context.Context
}

func TestAlertableInstanceType(t *testing.T) {
	s := new(alertableInstanceTypeSuite)
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)
	suite.Run(t, s)
}

func (s *alertableInstanceTypeSuite) SetupSuite() {
	s.j = makeAlertableInstanceTypeNotifyJob()
}

func (s *alertableInstanceTypeSuite) TearDownSuite() {
	s.cancel()
}

func (s *alertableInstanceTypeSuite) SetupTest() {
	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())

	s.NoError(db.ClearCollections(event.EventCollection, event.SubscriptionsCollection, host.Collection, alertrecord.Collection, user.Collection))

	// Set up test configuration with alertable instance types
	s.NoError(evergreen.UpdateConfig(s.ctx, &evergreen.Settings{
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				AlertableInstanceTypes: []string{"m5.large", "m5.xlarge", "c5.2xlarge"},
			},
		},
	}))

	now := time.Now()

	// Host that has been using alertable instance type for 4 days (should get notification)
	h1 := host.Host{
		Id:                   "h1",
		UserHost:             true,
		StartedBy:            "user1",
		InstanceType:         "m5.large",
		Status:               evergreen.HostRunning,
		LastInstanceEditTime: now.Add(-4 * 24 * time.Hour),
	}

	// Host that has been using alertable instance type for 2 days (should NOT get notification)
	h2 := host.Host{
		Id:                   "h2",
		UserHost:             true,
		StartedBy:            "user2",
		InstanceType:         "m5.xlarge",
		Status:               evergreen.HostRunning,
		LastInstanceEditTime: now.Add(-2 * 24 * time.Hour),
	}

	// Host using non-alertable instance type (should NOT get notification)
	h3 := host.Host{
		Id:                   "h3",
		UserHost:             true,
		StartedBy:            "user3",
		InstanceType:         "t3.micro",
		Status:               evergreen.HostRunning,
		LastInstanceEditTime: now.Add(-5 * 24 * time.Hour),
	}

	// Host with zero LastInstanceEditTime (created 4 days ago, should get notification)
	h4 := host.Host{
		Id:           "h4",
		UserHost:     true,
		StartedBy:    "user4",
		InstanceType: "c5.2xlarge",
		Status:       evergreen.HostRunning,
		CreationTime: now.Add(-4 * 24 * time.Hour),
		// LastInstanceEditTime is zero
	}

	// Non-user host (should NOT get notification)
	h5 := host.Host{
		Id:                   "h5",
		UserHost:             false,
		InstanceType:         "m5.large",
		Status:               evergreen.HostRunning,
		LastInstanceEditTime: now.Add(-5 * 24 * time.Hour),
	}

	hosts := []host.Host{h1, h2, h3, h4, h5}
	for _, h := range hosts {
		s.NoError(h.Insert(s.ctx))
	}

	// Create users that the hosts reference
	users := []user.DBUser{
		{
			Id:           "user1",
			EmailAddress: "user1@example.com",
			Settings: user.UserSettings{
				SlackUsername: "user1_slack",
				SlackMemberId: "U1234567890",
			},
		},
		{
			Id:           "user2",
			EmailAddress: "user2@example.com",
			Settings: user.UserSettings{
				SlackUsername: "user2_slack",
				SlackMemberId: "U2345678901",
			},
		},
		{
			Id:           "user3",
			EmailAddress: "user3@example.com",
			Settings: user.UserSettings{
				SlackUsername: "user3_slack",
				SlackMemberId: "U3456789012",
			},
		},
		{
			Id:           "user4",
			EmailAddress: "user4@example.com",
			Settings: user.UserSettings{
				SlackUsername: "user4_slack",
				SlackMemberId: "U4567890123",
			},
		},
	}
	for _, u := range users {
		s.NoError(u.Insert(s.ctx))
	}
}

func (s *alertableInstanceTypeSuite) TestEventsAreLogged() {
	s.j.Run(s.ctx)
	events, err := event.FindUnprocessedEvents(s.T().Context(), -1)
	s.NoError(err)

	// Should have 2 events: h1 and h4 (both have been using alertable types for 3+ days)
	s.Len(events, 2)

	expectedHosts := map[string]bool{
		"h1": false,
		"h4": false,
	}

	for _, e := range events {
		s.Equal(event.EventAlertableInstanceTypeWarningSent, e.EventType)
		s.Contains(expectedHosts, e.ResourceId)
		expectedHosts[e.ResourceId] = true
	}

	// Verify all expected hosts had events logged
	for hostID, found := range expectedHosts {
		s.True(found, "Expected event for host %s", hostID)
	}
}

func (s *alertableInstanceTypeSuite) TestAlertRecordsAreCreated() {
	s.j.Run(s.ctx)

	// Check that alert records were created for the hosts that should be notified
	for _, hostID := range []string{"h1", "h4"} {
		rec, err := alertrecord.FindByMostRecentAlertableInstanceType(s.ctx, hostID)
		s.NoError(err)
		s.Require().NotNil(rec, "Expected alert record for host %s", hostID)
		s.Equal(hostID, rec.HostId)
		s.Equal("alertable_instance_type", rec.Type)
	}

	// Check that no alert records were created for hosts that shouldn't be notified
	for _, hostID := range []string{"h2", "h3", "h5"} {
		rec, err := alertrecord.FindByMostRecentAlertableInstanceType(s.ctx, hostID)
		s.NoError(err)
		s.Nil(rec, "Should not have alert record for host %s", hostID)
	}
}

func (s *alertableInstanceTypeSuite) TestSubscriptionsAreCreated() {
	s.j.Run(s.ctx)

	// Check that both email and slack subscriptions were created for the hosts that should be notified
	for _, hostID := range []string{"h1", "h4"} {
		// Check email subscription
		emailSubscription, err := event.FindSubscriptionByID(s.ctx, fmt.Sprintf("alertable-instance-type-email-%s", hostID))
		s.NoError(err)
		s.Require().NotNil(emailSubscription, "Expected email subscription for host %s", hostID)
		s.Equal(fmt.Sprintf("alertable-instance-type-email-%s", hostID), emailSubscription.ID)
		s.Equal(event.ResourceTypeHost, emailSubscription.ResourceType)
		s.Equal(event.TriggerAlertableInstanceType, emailSubscription.Trigger)
		s.Equal(hostID, emailSubscription.Filter.ID)
		s.Equal(event.EmailSubscriberType, emailSubscription.Subscriber.Type)

		// Verify the email target matches the user's email
		expectedEmail := fmt.Sprintf("user%s@example.com", hostID[1:]) // Extract number from hostID
		switch target := emailSubscription.Subscriber.Target.(type) {
		case string:
			s.Equal(expectedEmail, target)
		case *string:
			s.Equal(expectedEmail, *target)
		default:
			s.Fail("Unexpected target type for email subscription", "got %T", target)
		}

		// Check slack subscription
		slackSubscription, err := event.FindSubscriptionByID(s.ctx, fmt.Sprintf("alertable-instance-type-slack-%s", hostID))
		s.NoError(err)
		s.Require().NotNil(slackSubscription, "Expected slack subscription for host %s", hostID)
		s.Equal(fmt.Sprintf("alertable-instance-type-slack-%s", hostID), slackSubscription.ID)
		s.Equal(event.ResourceTypeHost, slackSubscription.ResourceType)
		s.Equal(event.TriggerAlertableInstanceType, slackSubscription.Trigger)
		s.Equal(hostID, slackSubscription.Filter.ID)
		s.Equal(event.SlackSubscriberType, slackSubscription.Subscriber.Type)

		// Verify the slack target matches the user's slack member ID
		var expectedSlackTarget string
		if hostID == "h1" {
			expectedSlackTarget = "U1234567890"
		} else if hostID == "h4" {
			expectedSlackTarget = "U4567890123"
		}
		switch target := slackSubscription.Subscriber.Target.(type) {
		case string:
			s.Equal(expectedSlackTarget, target)
		case *string:
			s.Equal(expectedSlackTarget, *target)
		default:
			s.Fail("Unexpected target type for slack subscription", "got %T", target)
		}
	}

	// Check that no subscriptions were created for hosts that shouldn't be notified
	for _, hostID := range []string{"h2", "h3", "h5"} {
		emailSubscription, err := event.FindSubscriptionByID(s.ctx, fmt.Sprintf("alertable-instance-type-email-%s", hostID))
		s.NoError(err)
		s.Nil(emailSubscription, "Should not have email subscription for host %s", hostID)

		slackSubscription, err := event.FindSubscriptionByID(s.ctx, fmt.Sprintf("alertable-instance-type-slack-%s", hostID))
		s.NoError(err)
		s.Nil(slackSubscription, "Should not have slack subscription for host %s", hostID)
	}
}

func (s *alertableInstanceTypeSuite) TestDuplicateEventsAreNotLoggedWithinRenotificationInterval() {
	// First run - should log events
	s.j.Run(s.ctx)
	events, err := event.FindUnprocessedEvents(s.T().Context(), -1)
	s.NoError(err)
	s.Len(events, 2, "should log expected events on first run")

	// Second run immediately - should NOT log duplicate events
	s.j.Run(s.ctx)
	eventsAfterRerun, err := event.FindUnprocessedEvents(s.T().Context(), -1)
	s.NoError(err)
	s.Len(eventsAfterRerun, len(events), "should not log duplicate events on second run")
}

func (s *alertableInstanceTypeSuite) TestDuplicateEventsAreLoggedAfterRenotificationIntervalElapses() {
	// First run - should log events
	s.j.Run(s.ctx)
	events, err := event.FindUnprocessedEvents(s.T().Context(), -1)
	s.NoError(err)
	s.Len(events, 2, "should log expected events on first run")

	// Update alert records to simulate they were created more than 24 hours ago
	oldTime := time.Now().Add(-25 * time.Hour)
	filter := bson.M{
		"type": "alertable_instance_type",
	}
	update := bson.M{
		"$set": bson.M{
			"alert_time": oldTime,
		},
	}
	res, err := db.UpdateAllContext(s.ctx, alertrecord.Collection, filter, update)
	s.NoError(err)
	s.Equal(2, res.Updated, "should have updated 2 alert records")

	// Third run - should log new events since renotification interval has passed
	s.j.Run(s.ctx)
	eventsAfterRerun, err := event.FindUnprocessedEvents(s.T().Context(), -1)
	s.NoError(err)
	s.Len(eventsAfterRerun, len(events)+2, "should log new events when renotification interval has passed")

	// Fourth run immediately - should NOT log duplicate events again
	s.j.Run(s.ctx)
	eventsAfterSecondRerun, err := event.FindUnprocessedEvents(s.T().Context(), -1)
	s.NoError(err)
	s.Len(eventsAfterSecondRerun, len(events)+2, "should not log any more events when recently renotified")
}

func (s *alertableInstanceTypeSuite) TestNoEventsWhenNoAlertableTypes() {
	// Clear alertable instance types from config
	s.NoError(evergreen.UpdateConfig(s.ctx, &evergreen.Settings{
		Providers: evergreen.CloudProviders{
			AWS: evergreen.AWSConfig{
				AlertableInstanceTypes: []string{}, // Empty list
			},
		},
	}))

	s.j.Run(s.ctx)
	events, err := event.FindUnprocessedEvents(s.T().Context(), -1)
	s.NoError(err)
	s.Empty(events, "should not log any events when no alertable instance types configured")
}

func (s *alertableInstanceTypeSuite) TestCanceledJob() {
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()

	s.j.Run(ctx)
	events, err := event.FindUnprocessedEvents(s.T().Context(), -1)
	s.NoError(err)
	s.Empty(events, "should not log events when job is canceled")
}

func TestNewAlertableInstanceTypeNotifyJob(t *testing.T) {
	job := NewAlertableInstanceTypeNotifyJob("test-id")
	assert.NotNil(t, job)
	assert.Equal(t, "alertable-instance-type-notify.test-id", job.ID())
	assert.Equal(t, alertableInstanceTypeNotifyJobName, job.Type().Name)
}
