package admin

// AdminSettings currently holds settings related to degraded mode. It is intended
// to hold all configurable Evergreen-wide settings
type AdminSettings struct {
	Id           string       `bson:"_id"`
	Banner       string       `bson:"banner" json:"banner"`
	ServiceFlags ServiceFlags `bson:"service_flags" json:"service_flags"`
}

// ServiceFlags holds the state of each of the runner/API processes
type ServiceFlags struct {
	TaskDispatchDisabled  bool `bson:"task_dispatch_disabled" json:"task_dispatch_disabled"`
	HostinitDisabled      bool `bson:"hostinit_disabled" json:"hostinit_disabled"`
	MonitorDisabled       bool `bson:"monitor_disabled" json:"monitor_disabled"`
	NotificationsDisabled bool `bson:"notifications_disabled" json:"notifications_disabled"`
	AlertsDisabled        bool `bson:"alerts_disabled" json:"alerts_disabled"`
	TaskrunnerDisabled    bool `bson:"taskrunner_disabled" json:"taskrunner_disabled"`
	RepotrackerDisabled   bool `bson:"repotracker_disabled" json:"repotracker_disabled"`
	SchedulerDisabled     bool `bson:"scheduler_disabled" json:"scheduler_disabled"`
}

type TaskRestartResponse struct {
	TasksRestarted []string
	TasksErrored   []string
}
