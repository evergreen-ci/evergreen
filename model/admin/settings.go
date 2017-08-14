package admin

type AdminSettings struct {
	Id                   string       `bson:"_id" json:"id"`
	Banner               string       `bson:"banner" json:"banner"`
	TaskDispatchDisabled bool         `bson:"task_dispatch_disabled" json:"task_dispatch_disabled"`
	RunnerFlags          *RunnerFlags `bson:"runner_flags" json:"runner_flags"`
}

type RunnerFlags struct {
	HostinitDisabled      bool `bson:"hostinit_disabled" json:"hostinit_disabled"`
	MonitorDisabled       bool `bson:"monitor_disabled" json:"monitor_disabled"`
	NotificationsDisabled bool `bson:"notifications_disabled" json:"notifications_disabled"`
	AlertsDisabled        bool `bson:"alerts_disabled" json:"alerts_disabled"`
	TaskrunnerDisabled    bool `bson:"taskrunner_disabled" json:"taskrunner_disabled"`
	RepotrackerDisabled   bool `bson:"repotracker_disabled" json:"repotracker_disabled"`
	SchedulerDisabled     bool `bson:"scheduler_disabled" json:"scheduler_disabled"`
}

func (s *AdminSettings) GetBanner() string {
	return s.Banner
}

func (s *AdminSettings) GetTaskDispatchDisabled() bool {
	return s.TaskDispatchDisabled
}

func (s *AdminSettings) GetRunnerFlags() *RunnerFlags {
	return s.RunnerFlags
}
