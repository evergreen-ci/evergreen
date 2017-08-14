package admin

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

var (
	testConfig = testutil.TestConfig()
)

type AdminSuite struct {
	suite.Suite
}

func TestAdminSuite(t *testing.T) {
	s := new(AdminSuite)
	testutil.ConfigureIntegrationTest(t, testConfig, "TestAdminSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	db.Clear(Collection)

	suite.Run(t, s)
}

func (s *AdminSuite) TestBanner() {
	const bannerText = "hello evergreen users!"

	_, err := SetBanner(bannerText)
	s.NoError(err)
	settings, err := GetSettingsFromDB()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(bannerText, settings.GetBanner())
}

func (s *AdminSuite) TestTaskDispatchDisabling() {
	const dispatchDisabled = true

	_, err := SetTaskDispatchDisabled(dispatchDisabled)
	s.NoError(err)
	settings, err := GetSettingsFromDB()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(dispatchDisabled, settings.GetTaskDispatchDisabled())
}

func (s *AdminSuite) TestRunnerFlags() {
	testFlags := &RunnerFlags{
		HostinitDisabled:      true,
		MonitorDisabled:       true,
		NotificationsDisabled: true,
		AlertsDisabled:        true,
		TaskrunnerDisabled:    true,
		RepotrackerDisabled:   true,
		SchedulerDisabled:     true,
	}

	_, err := SetRunnerFlags(testFlags)
	s.NoError(err)
	settings, err := GetSettingsFromDB()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(testFlags, settings.GetRunnerFlags())
}

func (s *AdminSuite) TestUpsert() {
	settings := &AdminSettings{
		Id:                   SystemSettingsID,
		Banner:               "",
		TaskDispatchDisabled: false,
		RunnerFlags: &RunnerFlags{
			HostinitDisabled:      false,
			MonitorDisabled:       false,
			NotificationsDisabled: false,
			AlertsDisabled:        false,
			TaskrunnerDisabled:    false,
			RepotrackerDisabled:   false,
			SchedulerDisabled:     false,
		},
	}
	_, err := Upsert(settings)
	s.NoError(err)

	settingsFromDB, err := GetSettingsFromDB()
	s.NoError(err)
	s.NotNil(settingsFromDB)
	s.Equal(settings, settingsFromDB)

	settings.Banner = "test"
	settings.TaskDispatchDisabled = true
	settings.RunnerFlags.HostinitDisabled = true
	_, err = Upsert(settings)
	s.NoError(err)

	settingsFromDB, err = GetSettingsFromDB()
	s.NoError(err)
	s.NotNil(settingsFromDB)
	s.Equal(settings.GetBanner(), settingsFromDB.Banner)
	s.Equal(settings.GetTaskDispatchDisabled(), settingsFromDB.TaskDispatchDisabled)
	s.Equal(settings.GetRunnerFlags().HostinitDisabled, settingsFromDB.RunnerFlags.HostinitDisabled)
}
