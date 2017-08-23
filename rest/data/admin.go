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
func (ac *DBAdminConnector) SetAdminSettings(settings admin.AdminSettings) error {
	return UpdateSettings(settings)
}

// SetAdminBanner sets the admin banner in the DB and event logs it
func (ac *DBAdminConnector) SetAdminBanner(text string) error {
	return UpdateBanner(text)
}

// SetServiceFlags sets the service flags in the DB and event logs it
func (ac *DBAdminConnector) SetServiceFlags(flags admin.ServiceFlags) error {
	return UpdateServiceFlags(flags)
}

type MockAdminConnector struct {
	MockSettings admin.AdminSettings
}

// GetAdminSettings retrieves the admin settings document from the mock connector
func (ac *MockAdminConnector) GetAdminSettings() (*admin.AdminSettings, error) {
	return &ac.MockSettings, nil
}

// SetAdminSettings sets the admin settings document in the mock connector
func (ac *MockAdminConnector) SetAdminSettings(settings admin.AdminSettings) error {
	ac.MockSettings = settings
	return nil
}

// SetAdminBanner sets the admin banner in the mock connector
func (ac *MockAdminConnector) SetAdminBanner(text string) error {
	ac.MockSettings.Banner = text
	return nil
}

// SetServiceFlags sets the service flags in the mock connector
func (ac *MockAdminConnector) SetServiceFlags(flags admin.ServiceFlags) error {
	ac.MockSettings.ServiceFlags = flags
	return nil
}

// UpdateSettings updates the entire admin settings document and logs events
func UpdateSettings(settings admin.AdminSettings) error {
	err := UpdateBanner(settings.Banner)
	if err != nil {
		return err
	}
	return UpdateServiceFlags(settings.ServiceFlags)
}

// UpdateBanner updates the admin banner and logs an event
func UpdateBanner(bannerText string) error {
	oldSettings, err := admin.GetSettings()
	if err != nil {
		return err
	}

	err = admin.SetBanner(bannerText)
	if err != nil {
		return err
	}

	err = event.LogBannerChanged(oldSettings.Banner, bannerText)
	if err != nil {
		return err
	}

	return nil
}

// UpdateServiceFlags updates the admin service flags and logs events
func UpdateServiceFlags(flags admin.ServiceFlags) error {
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
