package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func init() {
	testutil.Setup()
}

type AdminEventSuite struct {
	suite.Suite
	username string
}

func TestAdminEventSuite(t *testing.T) {
	s := new(AdminEventSuite)
	s.username = "user"
	suite.Run(t, s)
}

func (s *AdminEventSuite) SetupTest() {
	s.Require().NoError(db.ClearCollections(EventCollection, evergreen.ConfigCollection))
}

func (s *AdminEventSuite) TestEventLogging() {
	before := evergreen.ServiceFlags{}
	after := evergreen.ServiceFlags{
		MonitorDisabled:     true,
		RepotrackerDisabled: true,
	}
	s.NoError(LogAdminEvent(s.T().Context(), before.SectionId(), &before, &after, s.username))
	dbEvents, err := FindAdmin(s.T().Context(), RecentAdminEvents(1))
	s.NoError(err)
	s.Require().Len(dbEvents, 1)
	eventData := dbEvents[0].Data.(*AdminEventData)
	s.NotEmpty(eventData.GUID)
	beforeVal := eventData.Changes.Before.(*evergreen.ServiceFlags)
	afterVal := eventData.Changes.After.(*evergreen.ServiceFlags)
	s.Equal(before, *beforeVal)
	s.False(afterVal.AlertsDisabled)
	s.True(afterVal.MonitorDisabled)
	s.True(afterVal.RepotrackerDisabled)
}

func (s *AdminEventSuite) TestEventLogging2() {
	before := evergreen.Settings{
		Banner: "testing",
	}
	after := evergreen.Settings{}
	s.NoError(LogAdminEvent(s.T().Context(), before.SectionId(), &before, &after, s.username))
	dbEvents, err := FindAdmin(s.T().Context(), RecentAdminEvents(1))
	s.NoError(err)
	s.Require().Len(dbEvents, 1)
	eventData := dbEvents[0].Data.(*AdminEventData)
	s.NotEmpty(eventData.GUID)
	beforeVal := eventData.Changes.Before.(*evergreen.Settings)
	afterVal := eventData.Changes.After.(*evergreen.Settings)
	s.Equal(before.Banner, beforeVal.Banner)
	s.Equal("", afterVal.Banner)
}

func (s *AdminEventSuite) TestEventLogging3() {
	before := evergreen.NotifyConfig{
		SES: evergreen.SESConfig{
			SenderAddress: "evergreen@mongodb.com",
		},
	}
	after := evergreen.NotifyConfig{
		SES: evergreen.SESConfig{
			SenderAddress: "evergreen2@mongodb.com",
		},
	}
	s.NoError(LogAdminEvent(s.T().Context(), before.SectionId(), &before, &after, s.username))
	dbEvents, err := FindAdmin(s.T().Context(), RecentAdminEvents(1))
	s.NoError(err)
	s.Require().Len(dbEvents, 1)
	eventData := dbEvents[0].Data.(*AdminEventData)
	s.NotEmpty(eventData.GUID)
	beforeVal := eventData.Changes.Before.(*evergreen.NotifyConfig)
	afterVal := eventData.Changes.After.(*evergreen.NotifyConfig)
	s.Equal(before.SES.SenderAddress, beforeVal.SES.SenderAddress)
	s.Equal(after.SES.SenderAddress, afterVal.SES.SenderAddress)
}

func (s *AdminEventSuite) TestNoSpuriousLogging() {
	before := evergreen.Settings{
		Banner: "testing",
		HostInit: evergreen.HostInitConfig{
			HostThrottle: 64,
		},
	}
	after := evergreen.Settings{
		Banner: "testing",
		HostInit: evergreen.HostInitConfig{
			HostThrottle: 128,
		},
	}
	s.NoError(LogAdminEvent(s.T().Context(), before.SectionId(), &before, &after, s.username))
	dbEvents, err := FindAdmin(s.T().Context(), RecentAdminEvents(5))
	s.NoError(err)
	s.Empty(dbEvents)
}

func (s *AdminEventSuite) TestNoChanges() {
	before := evergreen.SchedulerConfig{
		TaskFinder: "legacy",
	}
	after := evergreen.SchedulerConfig{
		TaskFinder: "legacy",
	}
	s.NoError(LogAdminEvent(s.T().Context(), before.SectionId(), &before, &after, s.username))
	dbEvents, err := FindAdmin(s.T().Context(), RecentAdminEvents(1))
	s.NoError(err)
	s.Empty(dbEvents)
}

func TestAdminEventsBeforeQuery(t *testing.T) {
	require.NoError(t, db.Clear(EventCollection))
	assert := assert.New(t)
	before := &evergreen.ServiceFlags{}
	after := &evergreen.ServiceFlags{HostInitDisabled: true}
	assert.NoError(LogAdminEvent(t.Context(), "service_flags", before, after, "beforeNow"))
	time.Sleep(10 * time.Millisecond)
	now := time.Now()
	time.Sleep(10 * time.Millisecond)
	assert.NoError(LogAdminEvent(t.Context(), "service_flags", before, after, "afterNow"))
	events, err := FindAdmin(t.Context(), AdminEventsBefore(now, 5))
	assert.NoError(err)
	require.Len(t, events, 1)
	assert.True(events[0].Timestamp.Before(now))
}
