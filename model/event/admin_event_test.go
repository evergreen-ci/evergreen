package event

import (
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

var (
	testConfig = testutil.TestConfig()
)

type AdminEventSuite struct {
	suite.Suite
	u *user.DBUser
}

func TestAdminEventSuite(t *testing.T) {
	s := new(AdminEventSuite)
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	s.u = &user.DBUser{Id: "user"}
	suite.Run(t, s)
}

func (s *AdminEventSuite) SetupTest() {
	err := db.Clear(AllLogCollection)
	if err != nil {
		panic(err)
	}
}

func (s *AdminEventSuite) TestBannerEvent() {
	const oldText = "hello evergreen users!"
	const newText = "changed text"

	// test that events log the old and new val correctly
	s.NoError(LogBannerChanged(oldText, newText, s.u))
	events, err := Find(AllLogCollection, recentAdminEvents(1))
	s.NoError(err)
	eventData, ok := events[0].Data.Data.(*AdminEventData)
	s.True(ok)
	s.True(eventData.IsValid())
	s.Equal(newText, eventData.NewVal)
	s.Equal(oldText, eventData.OldVal)

	// test that calling the logger without a change does not log
	time.Sleep(10 * time.Millisecond) // sleep between logging so that timestamps are different
	s.NoError(LogBannerChanged(newText, newText, s.u))
	newEvents, err := Find(AllLogCollection, recentAdminEvents(1))
	s.NoError(err)
	s.Equal(events[0].Timestamp, newEvents[0].Timestamp)
}

func (s *AdminEventSuite) TestFlagsEvent() {
	oldFlags := admin.ServiceFlags{
		TaskDispatchDisabled: true,
		HostinitDisabled:     true,
	}
	newFlags := admin.ServiceFlags{
		MonitorDisabled: true,
		AlertsDisabled:  true,
	}

	// test that events log the old and new val correctly
	s.NoError(LogServiceChanged(oldFlags, newFlags, s.u))
	events, err := Find(AllLogCollection, recentAdminEvents(1))
	s.NoError(err)
	eventData, ok := events[0].Data.Data.(*AdminEventData)
	s.True(ok)
	s.True(eventData.IsValid())
	s.Equal(newFlags, eventData.NewFlags)
	s.Equal(oldFlags, eventData.OldFlags)

	// test that calling the logger without a change does not log
	time.Sleep(10 * time.Millisecond)
	s.NoError(LogServiceChanged(newFlags, newFlags, s.u))
	newEvents, err := Find(AllLogCollection, recentAdminEvents(1))
	s.NoError(err)
	s.Equal(events[0].Timestamp, newEvents[0].Timestamp)
}
