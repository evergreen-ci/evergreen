package admin

// AdminSettings currently holds settings related to degraded mode. It is intended
// to hold all configurable Evergreen-wide settings
type AdminSettings struct {
	Id           string       `bson:"_id"`
	Banner       string       `bson:"banner" json:"banner"`
	BannerTheme  BannerTheme  `bson:"banner_theme" json:"banner_theme"`
	ServiceFlags ServiceFlags `bson:"service_flags" json:"service_flags"`
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
	GithubPRTestingDisabled      bool `bson:"github_pr_testing_disabled" json:"github_pr_testing_disabled"`
	DisableCLIUpdates            bool `bson:"cli_updates_disabled" json:"cli_updates_disabled"`
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
