package data

import (
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
)

type DBAdminConnector struct{}

// GetAdminSettings retrieves the admin settings document from the DB
func (ac *DBAdminConnector) GetAdminSettings() (*admin.AdminSettings, error) {
	return admin.GetSettings()
}

// SetAdminSettings sets the admin settings document in the DB and event logs it
func (ac *DBAdminConnector) SetAdminSettings(settings *admin.AdminSettings, u user.DBUser) error {
	err := ac.SetAdminBanner(settings.Banner, u)
	if err != nil {
		return err
	}
	return ac.SetServiceFlags(settings.ServiceFlags, u)
}

// SetAdminBanner sets the admin banner in the DB and event logs it
func (ac *DBAdminConnector) SetAdminBanner(text string, u user.DBUser) error {
	oldSettings, err := admin.GetSettings()
	if err != nil {
		return err
	}

	err = admin.SetBanner(text)
	if err != nil {
		return err
	}

	err = event.LogBannerChanged(oldSettings.Banner, text, u)
	if err != nil {
		return err
	}

	return nil
}

// SetServiceFlags sets the service flags in the DB and event logs it
func (ac *DBAdminConnector) SetServiceFlags(flags admin.ServiceFlags, u user.DBUser) error {
	oldSettings, err := admin.GetSettings()
	if err != nil {
		return err
	}

	err = admin.SetServiceFlags(flags)
	if err != nil {
		return err
	}

	err = event.LogServiceChanged(oldSettings.ServiceFlags, flags, u)
	if err != nil {
		return err
	}

	return nil
}

type MockAdminConnector struct {
	MockSettings *admin.AdminSettings
}

// GetAdminSettings retrieves the admin settings document from the mock connector
func (ac *MockAdminConnector) GetAdminSettings() (*admin.AdminSettings, error) {
	return ac.MockSettings, nil
}

// SetAdminSettings sets the admin settings document in the mock connector
func (ac *MockAdminConnector) SetAdminSettings(settings *admin.AdminSettings, u user.DBUser) error {
	ac.MockSettings = settings
	return nil
}

// SetAdminBanner sets the admin banner in the mock connector
func (ac *MockAdminConnector) SetAdminBanner(text string, u user.DBUser) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &admin.AdminSettings{}
	}
	ac.MockSettings.Banner = text
	return nil
}

// SetServiceFlags sets the service flags in the mock connector
func (ac *MockAdminConnector) SetServiceFlags(flags admin.ServiceFlags, u user.DBUser) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &admin.AdminSettings{}
	}
	ac.MockSettings.ServiceFlags = flags
	return nil
}
