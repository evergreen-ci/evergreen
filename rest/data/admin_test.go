package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
)

type AdminDataSuite struct {
	ctx Connector
	suite.Suite
}

func TestDataConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &DBConnector{}
	testutil.ConfigureIntegrationTest(t, testConfig, "TestDataConnectorSuite")
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	db.Clear(admin.Collection)
	suite.Run(t, s)
}

func TestMockConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &MockConnector{}
	suite.Run(t, s)
}

func (s *AdminDataSuite) TestSetAndGetSettings() {
	settings := &admin.AdminSettings{
		Banner: "test banner",
		ServiceFlags: admin.ServiceFlags{
			NotificationsDisabled: true,
			TaskrunnerDisabled:    true,
		},
	}

	err := s.ctx.SetAdminSettings(settings)
	s.NoError(err)

	settingsFromConnector, err := s.ctx.GetAdminSettings()
	s.NoError(err)
	s.Equal(settings.Banner, settingsFromConnector.Banner)
	s.Equal(settings.ServiceFlags, settingsFromConnector.ServiceFlags)
}
