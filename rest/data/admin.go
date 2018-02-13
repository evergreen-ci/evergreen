package data

import (
	"fmt"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

type DBAdminConnector struct{}

// GetEvergreenSettings retrieves the admin settings document from the DB
func (ac *DBAdminConnector) GetEvergreenSettings() (*evergreen.Settings, error) {
	return evergreen.GetConfig()
}

// SetEvergreenSettings sets the admin settings document in the DB and event logs it
func (ac *DBAdminConnector) SetEvergreenSettings(changes *restModel.APIAdminSettings,
	oldSettings *evergreen.Settings, u *user.DBUser, persist bool) (*evergreen.Settings, error) {
	currentSettings, err := evergreen.GetConfig()
	if err != nil {
		return nil, errors.Wrap(err, "error retrieving existing settings")
	}
	newSettings := *currentSettings
	catcher := grip.NewSimpleCatcher()

	// populate each of the settings if not nil
	if changes.Alerts != nil {
		i, err := changes.Alerts.ToService()
		if err != nil {
			catcher.Add(err)
		}
		alerts, ok := i.(evergreen.AlertsConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert alerts config"))
		}
		newSettings.Alerts = alerts
	}
	if changes.Amboy != nil {
		i, err := changes.Amboy.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.AmboyConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert amboy config"))
		}
		newSettings.Amboy = config
	}
	if changes.Api != nil {
		i, err := changes.Api.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.APIConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert api config"))
		}
		newSettings.Api = config
	}
	if changes.ApiUrl != nil {
		newSettings.ApiUrl = *changes.ApiUrl
	}
	if changes.AuthConfig != nil {
		i, err := changes.AuthConfig.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.AuthConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert auth config"))
		}
		newSettings.AuthConfig = config
	}
	if changes.Banner != nil {
		newSettings.Banner = *changes.Banner
	}
	if changes.BannerTheme != nil {
		newSettings.BannerTheme = evergreen.BannerTheme(*changes.BannerTheme)
	}
	if changes.ClientBinariesDir != nil {
		newSettings.ClientBinariesDir = *changes.ClientBinariesDir
	}
	if changes.ConfigDir != nil {
		newSettings.ConfigDir = *changes.ConfigDir
	}
	if changes.Credentials != nil {
		newSettings.Credentials = changes.Credentials
	}
	if changes.Expansions != nil {
		newSettings.Expansions = changes.Expansions
	}
	if changes.GithubPRCreatorOrg != nil {
		newSettings.GithubPRCreatorOrg = *changes.GithubPRCreatorOrg
	}
	if changes.HostInit != nil {
		i, err := changes.HostInit.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.HostInitConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert hostinit config"))
		}
		newSettings.HostInit = config
	}
	if changes.IsNonProd != nil {
		newSettings.IsNonProd = *changes.IsNonProd
	}
	if changes.Jira != nil {
		i, err := changes.Jira.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.JiraConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert jira config"))
		}
		newSettings.Jira = config
	}
	if changes.Keys != nil {
		newSettings.Keys = changes.Keys
	}
	if changes.LoggerConfig != nil {
		i, err := changes.LoggerConfig.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.LoggerConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert logger config"))
		}
		newSettings.LoggerConfig = config
	}
	if changes.LogPath != nil {
		newSettings.LogPath = *changes.LogPath
	}
	if changes.NewRelic != nil {
		i, err := changes.NewRelic.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.NewRelicConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert new relic config"))
		}
		newSettings.NewRelic = config
	}
	if changes.Notify != nil {
		i, err := changes.Notify.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.NotifyConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert notify config"))
		}
		newSettings.Notify = config
	}
	if changes.Plugins != nil {
		newSettings.Plugins = changes.Plugins
	}
	if changes.PprofPort != nil {
		newSettings.PprofPort = *changes.PprofPort
	}
	if changes.Providers != nil {
		i, err := changes.Providers.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.CloudProviders)
		if !ok {
			catcher.Add(errors.New("unable to convert cloud providers config"))
		}
		newSettings.Providers = config
	}
	if changes.RepoTracker != nil {
		i, err := changes.RepoTracker.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.RepoTrackerConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert repotracker config"))
		}
		newSettings.RepoTracker = config
	}
	if changes.Scheduler != nil {
		i, err := changes.Scheduler.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.SchedulerConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert scheduler config"))
		}
		newSettings.Scheduler = config
	}
	if changes.ServiceFlags != nil {
		i, err := changes.ServiceFlags.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.ServiceFlags)
		if !ok {
			catcher.Add(errors.New("unable to convert service flags config"))
		}
		newSettings.ServiceFlags = config
	}
	if changes.Slack != nil {
		i, err := changes.Slack.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.SlackConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert slack config"))
		}
		newSettings.Slack = config
	}
	if changes.Splunk != nil {
		i, err := changes.Splunk.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(send.SplunkConnectionInfo)
		if !ok {
			catcher.Add(errors.New("unable to convert splunk config"))
		}
		newSettings.Splunk = config
	}
	if changes.SuperUsers != nil {
		newSettings.SuperUsers = changes.SuperUsers
	}
	if changes.Ui != nil {
		i, err := changes.Ui.ToService()
		if err != nil {
			catcher.Add(err)
		}
		config, ok := i.(evergreen.UIConfig)
		if !ok {
			catcher.Add(errors.New("unable to convert ui config"))
		}
		newSettings.Ui = config
	}

	if persist {
		return &newSettings, evergreen.UpdateConfig(&newSettings)
	}
	return &newSettings, nil
}

// SetAdminBanner sets the admin banner in the DB and event logs it
func (ac *DBAdminConnector) SetAdminBanner(text string, u *user.DBUser) error {
	oldSettings, err := evergreen.GetConfig()
	if err != nil {
		return err
	}

	err = evergreen.SetBanner(text)
	if err != nil {
		return err
	}

	return event.LogBannerChanged(oldSettings.Banner, text, u)
}

// SetBannerTheme sets the banner theme in the DB and event logs it
func (ac *DBAdminConnector) SetBannerTheme(themeString string, u *user.DBUser) error {
	valid, theme := evergreen.IsValidBannerTheme(themeString)
	if !valid {
		return fmt.Errorf("%s is not a valid banner theme type", themeString)
	}

	oldSettings, err := evergreen.GetConfig()
	if err != nil {
		return err
	}

	err = evergreen.SetBannerTheme(theme)
	if err != nil {
		return err
	}

	return event.LogBannerThemeChanged(oldSettings.BannerTheme, theme, u)
}

// SetServiceFlags sets the service flags in the DB and event logs it
func (ac *DBAdminConnector) SetServiceFlags(flags evergreen.ServiceFlags, u *user.DBUser) error {
	oldSettings, err := evergreen.GetConfig()
	if err != nil {
		return err
	}

	err = evergreen.SetServiceFlags(flags)
	if err != nil {
		return err
	}

	return event.LogServiceChanged(oldSettings.ServiceFlags, flags, u)
}

// RestartFailedTasks attempts to restart failed tasks that started between 2 times
func (ac *DBAdminConnector) RestartFailedTasks(queue amboy.Queue, opts model.RestartTaskOptions) (*restModel.RestartTasksResponse, error) {
	var results model.RestartTaskResults
	var err error

	if opts.DryRun {
		results, err = model.RestartFailedTasks(opts)
		if err != nil {
			return nil, err
		}
	} else {
		if err = queue.Put(units.NewTasksRestartJob(opts)); err != nil {
			return nil, errors.Wrap(err, "error starting background job for task restart")
		}
	}

	return &restModel.RestartTasksResponse{
		TasksRestarted: results.TasksRestarted,
		TasksErrored:   results.TasksErrored,
	}, nil
}

type MockAdminConnector struct {
	mu           sync.RWMutex
	MockSettings *evergreen.Settings
}

// GetEvergreenSettings retrieves the admin settings document from the mock connector
func (ac *MockAdminConnector) GetEvergreenSettings() (*evergreen.Settings, error) {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.MockSettings, nil
}

// SetEvergreenSettings sets the admin settings document in the mock connector
func (ac *MockAdminConnector) SetEvergreenSettings(changes *restModel.APIAdminSettings,
	oldSettings *evergreen.Settings, u *user.DBUser, persist bool) (*evergreen.Settings, error) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	i, err := changes.ToService()
	if err != nil {
		return nil, err
	}
	settingsModel := i.(evergreen.Settings)
	ac.MockSettings = &settingsModel
	return ac.MockSettings, nil
}

// SetAdminBanner sets the admin banner in the mock connector
func (ac *MockAdminConnector) SetAdminBanner(text string, u *user.DBUser) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &evergreen.Settings{}
	}
	ac.MockSettings.Banner = text
	return nil
}

func (ac *MockAdminConnector) SetBannerTheme(themeString string, u *user.DBUser) error {
	valid, theme := evergreen.IsValidBannerTheme(themeString)
	if !valid {
		return fmt.Errorf("%s is not a valid banner theme type", themeString)
	}
	if ac.MockSettings == nil {
		ac.MockSettings = &evergreen.Settings{}
	}
	ac.MockSettings.BannerTheme = theme
	return nil
}

// SetServiceFlags sets the service flags in the mock connector
func (ac *MockAdminConnector) SetServiceFlags(flags evergreen.ServiceFlags, u *user.DBUser) error {
	if ac.MockSettings == nil {
		ac.MockSettings = &evergreen.Settings{}
	}
	ac.MockSettings.ServiceFlags = flags
	return nil
}

// RestartFailedTasks mocks a response to restarting failed tasks
func (ac *MockAdminConnector) RestartFailedTasks(queue amboy.Queue, opts model.RestartTaskOptions) (*restModel.RestartTasksResponse, error) {
	return &restModel.RestartTasksResponse{
		TasksRestarted: []string{"task1", "task2", "task3"},
		TasksErrored:   nil,
	}, nil
}
