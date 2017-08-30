package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/stretchr/testify/suite"
)

type AdminDataSuite struct {
	ctx Connector
	suite.Suite
}

func TestDataConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &DBConnector{}
	db.SetGlobalSessionProvider(db.SessionFactoryFromConfig(testConfig))
	err := db.Clear(admin.Collection)
	if err != nil {
		panic(err)
	}
	suite.Run(t, s)
}

func TestMockConnectorSuite(t *testing.T) {
	s := new(AdminDataSuite)
	s.ctx = &MockConnector{}
	suite.Run(t, s)
}

func (s *AdminDataSuite) TestSetAndGetSettings() {
	u := &user.DBUser{Id: "user"}
	settings := &admin.AdminSettings{
		Banner: "test banner",
		ServiceFlags: admin.ServiceFlags{
			NotificationsDisabled: true,
			TaskrunnerDisabled:    true,
		},
	}

	err := s.ctx.SetAdminSettings(settings, u)
	s.NoError(err)

	settingsFromConnector, err := s.ctx.GetAdminSettings()
	s.NoError(err)
	s.Equal(settings.Banner, settingsFromConnector.Banner)
	s.Equal(settings.ServiceFlags, settingsFromConnector.ServiceFlags)
}
