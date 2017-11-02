package data

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/admin"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/pkg/errors"
)

type DBAdminConnector struct{}

// GetAdminSettings retrieves the admin settings document from the DB
func (ac *DBAdminConnector) GetAdminSettings() (*admin.AdminSettings, error) {
	return admin.GetSettings()
}

// SetAdminSettings sets the admin settings document in the DB and event logs it
func (ac *DBAdminConnector) SetAdminSettings(settings *admin.AdminSettings, u *user.DBUser) error {
	if err := ac.SetAdminBanner(settings.Banner, u); err != nil {
		return err
	}
	if err := ac.SetBannerTheme(string(settings.BannerTheme), u); err != nil {
		return err
	}
	return ac.SetServiceFlags(settings.ServiceFlags, u)
}

// SetAdminBanner sets the admin banner in the DB and event logs it
func (ac *DBAdminConnector) SetAdminBanner(text string, u *user.DBUser) error {
	oldSettings, err := admin.GetSettings()
	if err != nil {
		return err
	}

	err = admin.SetBanner(text)
	if err != nil {
		return err
	}

	return event.LogBannerChanged(oldSettings.Banner, text, u)
}

// SetBannerTheme sets the banner theme in the DB and event logs it
func (ac *DBAdminConnector) SetBannerTheme(themeString string, u *user.DBUser) error {
	valid, theme := admin.IsValidBannerTheme(themeString)
	if !valid {
		return fmt.Errorf("%s is not a valid banner theme type", themeString)
	}

	oldSettings, err := admin.GetSettings()
	if err != nil {
		return err
	}

	err = admin.SetBannerTheme(theme)
	if err != nil {
		return err
	}

	return event.LogBannerThemeChanged(oldSettings.BannerTheme, theme, u)
}

// SetServiceFlags sets the service flags in the DB and event logs it
func (ac *DBAdminConnector) SetServiceFlags(flags admin.ServiceFlags, u *user.DBUser) error {
	oldSettings, err := admin.GetSettings()
	if err != nil {
		return err
	}

	err = admin.SetServiceFlags(flags)
	if err != nil {
		return err
	}

	return event.LogServiceChanged(oldSettings.ServiceFlags, flags, u)
}

// RestartFailedTasks attempts to restart failed tasks that started between 2 times
func (ac *DBAdminConnector) RestartFailedTasks(env evergreen.Environment, startTime, endTime time.Time, user string, opts model.RestartTaskOptions) (*restModel.RestartTasksResponse, error) {
	var results model.RestartTaskResults
	var err error
	if opts.DryRun {
		results, err = model.RestartFailedTasks(startTime, endTime, user, opts)
		if err != nil {
			return nil, err
		}
	} else {
		job := units.NewTasksRestartJob(startTime, endTime, user, opts)
		if err = env.LocalQueue().Put(job); err != nil {
			return nil, errors.Wrap(err, "error starting background job for task restart")
		}
	}

	return &restModel.RestartTasksResponse{
		TasksRestarted: results.TasksRestarted,
		TasksErrored:   results.TasksErrored,
	}, nil
}

type MockAdminConnector struct {
	MockSettings *admin.AdminSettings
}

// GetAdminSettings retrieves the admin settings document from the mock connector
func (ac *MockAdminConnector) GetAdminSettings() (*admin.AdminSettings, error) {
	return ac.MockSettings, nil
}

// SetAdminSettings sets the admin settings document in the mock connector
func (ac *MockAdminConnector) SetAdminSettings(settings *admin.AdminSettings, u *user.DBUser) error {
	ac.MockSettings = settings
	return nil
}

// SetAdminBanner sets the admin banner in the mock connector
func (ac *MockAdminConnector) SetAdminBanner(text string, u *user.DBUser) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &admin.AdminSettings{}
	}
	ac.MockSettings.Banner = text
	return nil
}

func (ac *MockAdminConnector) SetBannerTheme(themeString string, u *user.DBUser) error {
	valid, theme := admin.IsValidBannerTheme(themeString)
	if !valid {
		return fmt.Errorf("%s is not a valid banner theme type", themeString)
	}
	if ac.MockSettings == nil {
		ac.MockSettings = &admin.AdminSettings{}
	}
	ac.MockSettings.BannerTheme = theme
	return nil
}

// SetServiceFlags sets the service flags in the mock connector
func (ac *MockAdminConnector) SetServiceFlags(flags admin.ServiceFlags, u *user.DBUser) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &admin.AdminSettings{}
	}
	ac.MockSettings.ServiceFlags = flags
	return nil
}

// RestartFailedTasks mocks a response to restarting failed tasks
func (ac *MockAdminConnector) RestartFailedTasks(env evergreen.Environment, startTime, endTime time.Time, user string, opts model.RestartTaskOptions) (*restModel.RestartTasksResponse, error) {
	return &restModel.RestartTasksResponse{
		TasksRestarted: []string{"task1", "task2", "task3"},
		TasksErrored:   nil,
	}, nil
}
