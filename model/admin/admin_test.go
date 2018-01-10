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
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
	db.Clear(Collection)

	suite.Run(t, s)
}

func (s *AdminSuite) TestBanner() {
	const bannerText = "hello evergreen users!"

	err := SetBanner(bannerText)
	s.NoError(err)
	settings, err := GetSettings()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(bannerText, settings.Banner)

	err = SetBannerTheme(Important)
	s.NoError(err)
	settings, err = GetSettings()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(Important, string(settings.BannerTheme))
}

func (s *AdminSuite) TestServiceFlags() {
	testFlags := ServiceFlags{
		TaskDispatchDisabled:    true,
		HostinitDisabled:        true,
		MonitorDisabled:         true,
		NotificationsDisabled:   true,
		AlertsDisabled:          true,
		TaskrunnerDisabled:      true,
		RepotrackerDisabled:     true,
		SchedulerDisabled:       true,
		GithubWebhookPushEvent:  true,
		GithubPushEventDisabled: true,
	}

	err := SetServiceFlags(testFlags)
	s.NoError(err)
	settings, err := GetSettings()
	s.NoError(err)
	s.NotNil(settings)
	s.Equal(testFlags, settings.ServiceFlags)
}

func (s *AdminSuite) TestUpsert() {
	db.Clear(Collection)
	settings := &AdminSettings{
		Id:     systemSettingsDocID,
		Banner: "",

		ServiceFlags: ServiceFlags{
			TaskDispatchDisabled:    false,
			HostinitDisabled:        false,
			MonitorDisabled:         false,
			NotificationsDisabled:   false,
			AlertsDisabled:          false,
			TaskrunnerDisabled:      false,
			RepotrackerDisabled:     false,
			SchedulerDisabled:       false,
			GithubWebhookPushEvent:  false,
			GithubPushEventDisabled: false,
		},
	}
	err := Upsert(settings)
	s.NoError(err)

	settingsFromDB, err := GetSettings()
	s.NoError(err)
	s.NotNil(settingsFromDB)
	s.Equal(settings, settingsFromDB)

	settings.Banner = "test"
	settings.ServiceFlags.HostinitDisabled = true
	err = Upsert(settings)
	s.NoError(err)

	settingsFromDB, err = GetSettings()
	s.NoError(err)
	s.NotNil(settingsFromDB)
	s.Equal(settings.Banner, settingsFromDB.Banner)
	s.Equal(settings.ServiceFlags.HostinitDisabled, settingsFromDB.ServiceFlags.HostinitDisabled)
}
