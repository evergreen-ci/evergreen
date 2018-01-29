package admin

import (
	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip/send"
)

// AdminSettings currently holds settings related to degraded mode. It is intended
// to hold all configurable Evergreen-wide settings
type AdminSettings struct {
	Id string `bson:"_id" json:"id"`

	// Degraded mode-related settings
	Banner       string       `bson:"banner" json:"banner"`
	BannerTheme  BannerTheme  `bson:"banner_theme" json:"banner_theme"`
	ServiceFlags ServiceFlags `bson:"service_flags" json:"service_flags"`

	// Evergreen config
	ConfigDir          string                      `bson:"configdir" json:"configdir"`
	ApiUrl             string                      `bson:"api_url" json:"api_url"`
	ClientBinariesDir  string                      `bson:"client_binaries_dir" json:"client_binaries_dir"`
	SuperUsers         []string                    `bson:"superusers" json:"superusers"`
	Jira               evergreen.JiraConfig        `bson:"jira" json:"jira"`
	Splunk             send.SplunkConnectionInfo   `bson:"splunk" json:"splunk"`
	Slack              evergreen.SlackConfig       `bson:"slack" json:"slack"`
	Providers          evergreen.CloudProviders    `bson:"providers" json:"providers"`
	Keys               map[string]string           `bson:"keys" json:"keys"`
	Credentials        map[string]string           `bson:"credentials" json:"credentials"`
	AuthConfig         evergreen.AuthConfig        `bson:"auth" json:"auth"`
	RepoTracker        evergreen.RepoTrackerConfig `bson:"repotracker" json:"repotracker"`
	Api                evergreen.APIConfig         `bson:"api" json:"api"`
	Alerts             evergreen.AlertsConfig      `bson:"alerts" json:"alerts"`
	Ui                 evergreen.UIConfig          `bson:"ui" json:"ui"`
	HostInit           evergreen.HostInitConfig    `bson:"hostinit" json:"hostinit"`
	Notify             evergreen.NotifyConfig      `bson:"notify" json:"notify"`
	Scheduler          evergreen.SchedulerConfig   `bson:"scheduler" json:"scheduler"`
	Amboy              evergreen.AmboyConfig       `bson:"amboy" json:"amboy"`
	Expansions         map[string]string           `bson:"expansions" json:"expansions"`
	Plugins            evergreen.PluginConfig      `bson:"plugins" json:"plugins"`
	IsNonProd          bool                        `bson:"isnonprod" json:"isnonprod"`
	LoggerConfig       evergreen.LoggerConfig      `bson:"logger_config" json:"logger_config"`
	LogPath            string                      `bson:"log_path" json:"log_path"`
	PprofPort          string                      `bson:"pprof_port" json:"pprof_port"`
	GithubPRCreatorOrg string                      `bson:"github_pr_creator_org" json:"github_pr_creator_org"`
	NewRelic           evergreen.NewRelicConfig    `bson:"new_relic" json:"new_relic"`
}

// ServiceFlags holds the state of each of the runner/API processes
type ServiceFlags struct {
	TaskDispatchDisabled         bool `bson:"task_dispatch_disabled" json:"task_dispatch_disabled"`
	HostinitDisabled             bool `bson:"hostinit_disabled" json:"hostinit_disabled"`
	MonitorDisabled              bool `bson:"monitor_disabled" json:"monitor_disabled"`
	NotificationsDisabled        bool `bson:"notifications_disabled" json:"notifications_disabled"`
	AlertsDisabled               bool `bson:"alerts_disabled" json:"alerts_disabled"`
	TaskrunnerDisabled           bool `bson:"taskrunner_disabled" json:"taskrunner_disabled"`
	RepotrackerDisabled          bool `bson:"repotracker_disabled" json:"repotracker_disabled"`
	SchedulerDisabled            bool `bson:"scheduler_disabled" json:"scheduler_disabled"`
	GithubPRTestingDisabled      bool `bson:"github_pr_testing_disabled" json:"github_pr_testing_disabled"`
	RepotrackerPushEventDisabled bool `bson:"repotracker_push_event_disabled" json:"repotracker_push_event_disabled"`
	CLIUpdatesDisabled           bool `bson:"cli_updates_disabled" json:"cli_updates_disabled"`
	GithubStatusAPIDisabled      bool `bson:"github_status_api_disabled" json:"github_status_api_disabled"`
}

// supported banner themes in Evergreen
type BannerTheme string

const (
	Announcement BannerTheme = "announcement"
	Information              = "information"
	Warning                  = "warning"
	Important                = "important"
)

func IsValidBannerTheme(input string) (bool, BannerTheme) {
	switch input {
	case "":
		return true, ""
	case "announcement":
		return true, Announcement
	case "information":
		return true, Information
	case "warning":
		return true, Warning
	case "important":
		return true, Important
	default:
		return false, ""
	}
}
