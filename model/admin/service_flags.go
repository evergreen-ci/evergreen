package admin

import (
	"github.com/evergreen-ci/evergreen/db"
	"gopkg.in/mgo.v2/bson"
)

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

func (c *ServiceFlags) id() string { return "service_flags" }
func (c *ServiceFlags) get() error {
	err := db.FindOneQ(Collection, db.Query(byId(c.id())), c)
	if err != nil && err.Error() == "not found" {
		return nil
	}
	return err
}
func (c *ServiceFlags) set() error {
	_, err := db.Upsert(Collection, byId(c.id()), bson.M{
		"$set": bson.M{
			taskDispatchKey:                 c.TaskDispatchDisabled,
			hostinitKey:                     c.HostinitDisabled,
			monitorKey:                      c.MonitorDisabled,
			notificationsKey:                c.NotificationsDisabled,
			alertsKey:                       c.AlertsDisabled,
			taskrunnerKey:                   c.TaskrunnerDisabled,
			repotrackerKey:                  c.RepotrackerDisabled,
			schedulerKey:                    c.SchedulerDisabled,
			githubPRTestingDisabledKey:      c.GithubPRTestingDisabled,
			repotrackerPushEventDisabledKey: c.RepotrackerPushEventDisabled,
			cliUpdatesDisabledKey:           c.CLIUpdatesDisabled,
			githubStatusAPIDisabled:         c.GithubStatusAPIDisabled,
		},
	})
	return err
}
