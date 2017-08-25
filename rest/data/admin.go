package data

import (
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/event"
)

type DBAdminConnector struct{}

// GetAdminSettings retrieves the admin settings document from the DB
func (ac *DBAdminConnector) GetAdminSettings() (*admin.AdminSettings, error) {
	return admin.GetSettings()
}

// SetAdminSettings sets the admin settings document in the DB and event logs it
func (ac *DBAdminConnector) SetAdminSettings(settings *admin.AdminSettings) error {
	err := ac.SetAdminBanner(settings.Banner)
	if err != nil {
		return err
	}
	return ac.SetServiceFlags(settings.ServiceFlags)
}

// SetAdminBanner sets the admin banner in the DB and event logs it
func (ac *DBAdminConnector) SetAdminBanner(text string) error {
	oldSettings, err := admin.GetSettings()
	if err != nil {
		return err
	}

	err = admin.SetBanner(text)
	if err != nil {
		return err
	}

	err = event.LogBannerChanged(oldSettings.Banner, text)
	if err != nil {
		return err
	}

	return nil
}

// SetServiceFlags sets the service flags in the DB and event logs it
func (ac *DBAdminConnector) SetServiceFlags(flags admin.ServiceFlags) error {
	oldSettings, err := admin.GetSettings()
	if err != nil {
		return err
	}

	err = admin.SetServiceFlags(flags)
	if err != nil {
		return err
	}

	err = event.LogServiceChanged(oldSettings.ServiceFlags, flags)
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
func (ac *MockAdminConnector) SetAdminSettings(settings *admin.AdminSettings) error {
	ac.MockSettings = settings
	return nil
}

// SetAdminBanner sets the admin banner in the mock connector
func (ac *MockAdminConnector) SetAdminBanner(text string) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &admin.AdminSettings{}
	}
	ac.MockSettings.Banner = text
	return nil
}

// SetServiceFlags sets the service flags in the mock connector
func (ac *MockAdminConnector) SetServiceFlags(flags admin.ServiceFlags) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &admin.AdminSettings{}
	}
	ac.MockSettings.ServiceFlags = flags
	return nil
}
