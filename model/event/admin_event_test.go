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
	s.Require().NoError(db.ClearCollections(AllLogCollection, evergreen.ConfigCollection))
}

func (s *AdminEventSuite) TestEventLogging() {
	before := evergreen.ServiceFlags{}
	after := evergreen.ServiceFlags{
		MonitorDisabled:     true,
		RepotrackerDisabled: true,
	}
	s.NoError(LogAdminEvent(before.SectionId(), &before, &after, s.u.Username()))
	dbEvents, err := FindAdmin(RecentAdminEvents(1))
	s.NoError(err)
	s.Require().Len(dbEvents, 1)
	eventData := dbEvents[0].Data.(*AdminEventData)
	s.True(eventData.IsValid())
	s.NotEmpty(eventData.GUID)
	beforeVal := eventData.Changes.Before.(*evergreen.ServiceFlags)
	afterVal := eventData.Changes.After.(*evergreen.ServiceFlags)
	s.Equal(before, *beforeVal)
	s.Equal(false, afterVal.AlertsDisabled)
	s.Equal(true, afterVal.MonitorDisabled)
	s.Equal(true, afterVal.RepotrackerDisabled)
}

func (s *AdminEventSuite) TestEventLogging2() {
	before := evergreen.Settings{
		ApiUrl:     "api",
		Keys:       map[string]string{"k1": "v1"},
		SuperUsers: []string{"a", "b", "c"},
	}
	after := evergreen.Settings{}
	s.NoError(LogAdminEvent(before.SectionId(), &before, &after, s.u.Username()))
	dbEvents, err := FindAdmin(RecentAdminEvents(1))
	s.NoError(err)
	s.Require().Len(dbEvents, 1)
	eventData := dbEvents[0].Data.(*AdminEventData)
	s.True(eventData.IsValid())
	s.NotEmpty(eventData.GUID)
	beforeVal := eventData.Changes.Before.(*evergreen.Settings)
	afterVal := eventData.Changes.After.(*evergreen.Settings)
	s.Equal(before.ApiUrl, beforeVal.ApiUrl)
	s.Equal(before.Keys, beforeVal.Keys)
	s.Equal(before.SuperUsers, beforeVal.SuperUsers)
	s.Equal("", afterVal.ApiUrl)
	s.Equal(map[string]string{}, afterVal.Keys)
	s.Equal([]string{}, afterVal.SuperUsers)
}

func (s *AdminEventSuite) TestEventLogging3() {
	before := evergreen.NotifyConfig{
		SMTP: &evergreen.SMTPConfig{
			Port:     10,
			Password: "pass",
		},
	}
	after := evergreen.NotifyConfig{
		SMTP: &evergreen.SMTPConfig{
			Port:     20,
			Password: "nope",
		},
	}
	s.NoError(LogAdminEvent(before.SectionId(), &before, &after, s.u.Username()))
	dbEvents, err := FindAdmin(RecentAdminEvents(1))
	s.NoError(err)
	s.Require().Len(dbEvents, 1)
	eventData := dbEvents[0].Data.(*AdminEventData)
	s.True(eventData.IsValid())
	s.NotEmpty(eventData.GUID)
	beforeVal := eventData.Changes.Before.(*evergreen.NotifyConfig)
	afterVal := eventData.Changes.After.(*evergreen.NotifyConfig)
	s.Equal(before.SMTP.Port, beforeVal.SMTP.Port)
	s.Equal(before.SMTP.Password, beforeVal.SMTP.Password)
	s.Equal(after.SMTP.Port, afterVal.SMTP.Port)
	s.Equal(after.SMTP.Password, afterVal.SMTP.Password)
}

func (s *AdminEventSuite) TestNoChanges() {
	before := evergreen.SchedulerConfig{
		MergeToggle: 5,
		TaskFinder:  "legacy",
	}
	after := evergreen.SchedulerConfig{
		MergeToggle: 5,
		TaskFinder:  "legacy",
	}
	s.NoError(LogAdminEvent(before.SectionId(), &before, &after, s.u.Username()))
	dbEvents, err := FindAdmin(RecentAdminEvents(1))
	s.NoError(err)
	s.Len(dbEvents, 0)
}

func (s *AdminEventSuite) TestReverting() {
	before := evergreen.SchedulerConfig{
		TaskFinder:  "legacy",
		MergeToggle: 5,
	}
	after := evergreen.SchedulerConfig{
		TaskFinder:  "alternate",
		MergeToggle: 10,
	}
	s.NoError(after.Set())
	s.NoError(LogAdminEvent(before.SectionId(), &before, &after, s.u.Username()))

	dbEvents, err := FindAdmin(RecentAdminEvents(1))
	s.NoError(err)
	s.Require().Len(dbEvents, 1)
	eventData := dbEvents[0].Data.(*AdminEventData)
	s.True(eventData.IsValid())
	beforeVal := eventData.Changes.Before.(*evergreen.SchedulerConfig)
	afterVal := eventData.Changes.After.(*evergreen.SchedulerConfig)
	s.Equal(before, *beforeVal)
	s.Equal(after, *afterVal)
	guid := eventData.GUID
	s.NotEmpty(guid)

	settings, err := evergreen.GetConfig()
	s.NoError(err)
	s.Equal(after, settings.Scheduler)
	s.NoError(RevertConfig(guid, "me"))
	settings, err = evergreen.GetConfig()
	s.NoError(err)
	s.Equal(before, settings.Scheduler)

	// check that reverting a nonexistent guid errors
	s.Error(RevertConfig("abcd", "me"))
}

func (s *AdminEventSuite) TestRevertingRoot() {
	// this verifies that reverting the root document does not revert other sections
	before := evergreen.Settings{
		Banner:      "before_banner",
		Credentials: map[string]string{"k1": "v1"},
		SuperUsers:  []string{"su1", "su2"},
		Ui: evergreen.UIConfig{
			Url: "before_url",
		},
	}
	after := evergreen.Settings{
		Banner:      "after_banner",
		Credentials: map[string]string{"k2": "v2"},
		SuperUsers:  []string{"su3"},
		Ui: evergreen.UIConfig{
			Url:            "after_url",
			CacheTemplates: true,
		},
	}
	s.NoError(evergreen.UpdateConfig(&after))
	s.NoError(LogAdminEvent(before.SectionId(), &before, &after, s.u.Username()))

	dbEvents, err := FindAdmin(RecentAdminEvents(1))
	s.NoError(err)
	s.Require().Len(dbEvents, 1)
	eventData := dbEvents[0].Data.(*AdminEventData)
	guid := eventData.GUID
	s.NotEmpty(guid)

	settings, err := evergreen.GetConfig()
	s.NoError(err)
	s.Equal(after.Banner, settings.Banner)
	s.Equal(after.Credentials, settings.Credentials)
	s.Equal(after.SuperUsers, settings.SuperUsers)
	s.Equal(after.Ui, settings.Ui)
	s.NoError(RevertConfig(guid, "me"))
	settings, err = evergreen.GetConfig()
	s.NoError(err)
	s.Equal(before.Banner, settings.Banner)
	s.Equal(before.Credentials, settings.Credentials)
	s.Equal(before.SuperUsers, settings.SuperUsers)
	s.Equal(after.Ui, settings.Ui)
}
