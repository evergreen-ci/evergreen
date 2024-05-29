package units

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/alertrecord"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type spawnHostExpirationSuite struct {
	j *spawnhostExpirationWarningsJob
	suite.Suite
	suiteCtx context.Context
	cancel   context.CancelFunc
	ctx      context.Context
}

func TestSpawnHostExpiration(t *testing.T) {
	s := new(spawnHostExpirationSuite)
	s.suiteCtx, s.cancel = context.WithCancel(context.Background())
	s.suiteCtx = testutil.TestSpan(s.suiteCtx, t)
	suite.Run(t, s)
}

func (s *spawnHostExpirationSuite) SetupSuite() {
	s.j = makeSpawnhostExpirationWarningsJob()
}

func (s *spawnHostExpirationSuite) TearDownSuite() {
	s.cancel()
}

func (s *spawnHostExpirationSuite) SetupTest() {
	s.ctx = testutil.TestSpan(s.suiteCtx, s.T())

	s.NoError(db.ClearCollections(event.EventCollection, host.Collection, alertrecord.Collection))
	now := time.Now()
	h1 := host.Host{
		Id:             "h1",
		ExpirationTime: now.Add(13 * time.Hour),
	}
	h2 := host.Host{ // should get a 12 hr warning
		Id:             "h2",
		ExpirationTime: now.Add(9 * time.Hour),
	}
	h3 := host.Host{ // should get a 12 and 2 hr warning
		Id:             "h3",
		ExpirationTime: now.Add(1 * time.Hour),
	}
	h4 := host.Host{ // should get a 12 hr warning
		Id:           "h4",
		NoExpiration: true,
		Status:       evergreen.HostRunning,
		SleepSchedule: host.SleepScheduleInfo{
			WholeWeekdaysOff:       []time.Weekday{time.Saturday, time.Sunday},
			TimeZone:               "America/New_York",
			TemporarilyExemptUntil: now.Add(9 * time.Hour),
		},
	}
	h5 := host.Host{ // should get a 12 hr and 2 hr warning
		Id:           "h5",
		NoExpiration: true,
		Status:       evergreen.HostRunning,
		SleepSchedule: host.SleepScheduleInfo{
			WholeWeekdaysOff:       []time.Weekday{time.Saturday, time.Sunday},
			TimeZone:               "America/New_York",
			TemporarilyExemptUntil: now.Add(1 * time.Hour),
		},
	}
	h6 := host.Host{
		Id:           "h6",
		NoExpiration: true,
		SleepSchedule: host.SleepScheduleInfo{
			WholeWeekdaysOff:       []time.Weekday{time.Saturday, time.Sunday},
			TimeZone:               "America/New_York",
			TemporarilyExemptUntil: now.Add(utility.Day),
		},
	}
	s.NoError(h1.Insert(s.ctx))
	s.NoError(h2.Insert(s.ctx))
	s.NoError(h3.Insert(s.ctx))
	s.NoError(h4.Insert(s.ctx))
	s.NoError(h5.Insert(s.ctx))
	s.NoError(h6.Insert(s.ctx))
}

func (s *spawnHostExpirationSuite) TestEventsAreLogged() {
	s.j.Run(s.ctx)
	events, err := event.FindUnprocessedEvents(-1)
	s.NoError(err)
	s.Len(events, 6)

	type hostEvent struct {
		hostID    string
		eventType string
	}
	expectedEvents := map[hostEvent]struct {
		count       int
		actualCount int
	}{
		{hostID: "h2", eventType: event.EventHostExpirationWarningSent}: {
			count: 1,
		},
		{hostID: "h3", eventType: event.EventHostExpirationWarningSent}: {
			count: 2,
		},
		{hostID: "h4", eventType: event.EventHostTemporaryExemptionExpirationWarningSent}: {
			count: 1,
		},
		{hostID: "h5", eventType: event.EventHostTemporaryExemptionExpirationWarningSent}: {
			count: 2,
		},
	}
	for _, e := range events {
		hostEvt := hostEvent{hostID: e.ResourceId, eventType: e.EventType}
		counts, ok := expectedEvents[hostEvt]
		if ok {
			counts.actualCount++
			expectedEvents[hostEvt] = counts
		}
	}

	for hostEvt, expected := range expectedEvents {
		s.Equal(expected.count, expected.actualCount, "expected %d events of type %s for host %s, got %d", expected.count, hostEvt.eventType, hostEvt.hostID, expected.actualCount)
	}
}

func (s *spawnHostExpirationSuite) TestDuplicateEventsAreNotLoggedWithinRenotificationInterval() {
	s.j.Run(s.ctx)
	events, err := event.FindUnprocessedEvents(-1)
	s.NoError(err)
	s.Len(events, 6, "should log expected events on first run")

	s.j.Run(s.ctx)
	eventsAfterRerun, err := event.FindUnprocessedEvents(-1)
	s.NoError(err)
	s.Len(eventsAfterRerun, len(events), "should not log duplicate events on second run")
}

func (s *spawnHostExpirationSuite) TestDuplicateEventsAreLoggedAfterRenotificationIntervalElapses() {
	s.j.Run(s.ctx)
	events, err := event.FindUnprocessedEvents(-1)
	s.NoError(err)
	s.Len(events, 6, "should log expected events on first run")

	// Update the alert records to simulate a condition where the temporary
	// exemption events were logged a long time ago, meaning they're eligible to
	// log again.
	var temporaryExemptionExpirationEventIDs []string
	for _, e := range events {
		if e.EventType == event.EventHostTemporaryExemptionExpirationWarningSent {
			temporaryExemptionExpirationEventIDs = append(temporaryExemptionExpirationEventIDs, e.ResourceId)
		}
	}
	res, err := db.UpdateAll(alertrecord.Collection, bson.M{
		alertrecord.HostIdKey: bson.M{"$in": []string{"h4", "h5"}}}, bson.M{
		"$set": bson.M{
			alertrecord.AlertTimeKey: time.Now().Add(-7 * utility.Day),
		},
	})
	s.Require().NoError(err)
	s.Equal(len(temporaryExemptionExpirationEventIDs), res.Updated, "should have updated the alert records for temporary exemption expiration warnings so that they are old")

	s.j.Run(s.ctx)
	eventsAfterRerun, err := event.FindUnprocessedEvents(-1)
	s.NoError(err)
	s.Len(eventsAfterRerun, len(events)+len(temporaryExemptionExpirationEventIDs), "should log duplicate events when renotification interval has passed")
}

func (s *spawnHostExpirationSuite) TestCanceledJob() {
	ctx, cancel := context.WithCancel(s.ctx)
	cancel()

	s.j.Run(ctx)
	events, err := event.FindUnprocessedEvents(-1)
	s.NoError(err)
	s.Len(events, 0)
}
