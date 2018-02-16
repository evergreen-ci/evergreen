package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
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
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	s.u = &user.DBUser{Id: "user"}
	suite.Run(t, s)
}

func (s *AdminEventSuite) SetupTest() {
	err := db.Clear(AllLogCollection)
	s.Require().NoError(err)
}

func (s *AdminEventSuite) TestEventLogging() {
	before := evergreen.ServiceFlags{}
	after := evergreen.ServiceFlags{
		MonitorDisabled:     true,
		RepotrackerDisabled: true,
	}
	s.NoError(LogAdminEvent(before.SectionId(), &before, &after, s.u.Username()))
	dbEvents, err := FindAndScrub(RecentAdminEvents(1))
	s.NoError(err)
	eventData := dbEvents[0].Data.Data.(*AdminEventData)
	s.True(eventData.IsValid())
	beforeVal := eventData.Changes.Before.(*evergreen.ServiceFlags)
	afterVal := eventData.Changes.After.(*evergreen.ServiceFlags)
	s.Equal(before, *beforeVal)
	s.Equal(false, afterVal.AlertsDisabled)
	s.Equal(true, afterVal.MonitorDisabled)
	s.Equal(true, afterVal.RepotrackerDisabled)
}

func (s *AdminEventSuite) TestEventScrubbing() {
	before := evergreen.AuthConfig{}
	after := evergreen.AuthConfig{
		Naive: &evergreen.NaiveAuthConfig{
			Users: []*evergreen.AuthUser{&evergreen.AuthUser{Username: "user", Password: "pwd"}},
		},
		Crowd: &evergreen.CrowdConfig{
			Username: "crowd",
			Password: "crowdpw",
		},
	}
	s.NoError(LogAdminEvent(before.SectionId(), &before, &after, s.u.Username()))
	dbEvents, err := FindAndScrub(RecentAdminEvents(1))
	s.NoError(err)
	eventData := dbEvents[0].Data.Data.(*AdminEventData)
	s.True(eventData.IsValid())
	beforeVal := eventData.Changes.Before.(*evergreen.AuthConfig)
	afterVal := eventData.Changes.After.(*evergreen.AuthConfig)
	s.Equal(before, *beforeVal)
	s.Equal("user", afterVal.Naive.Users[0].Username)
	s.Equal("***", afterVal.Naive.Users[0].Password)
	s.Equal("crowd", afterVal.Crowd.Username)
	s.Equal("***", afterVal.Crowd.Password)
}
