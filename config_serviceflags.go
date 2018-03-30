package evergreen

import (
	"github.com/evergreen-ci/evergreen/db"
	"github.com/pkg/errors"
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
	BackgroundStatsDisabled      bool `bson:"background_stats_disabled" json:"background_stats_disabled"`
}

func (c *ServiceFlags) SectionId() string { return "service_flags" }

func (c *ServiceFlags) Get() error {
	err := db.FindOneQ(ConfigCollection, db.Query(byId(c.SectionId())), c)
	if err != nil && err.Error() == errNotFound {
		*c = ServiceFlags{}
		return nil
	}
	return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
}

func (c *ServiceFlags) Set() error {
	_, err := db.Upsert(ConfigCollection, byId(c.SectionId()), bson.M{
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
			githubStatusAPIDisabledKey:      c.GithubStatusAPIDisabled,
			backgroundStatsDisabledKey:      c.BackgroundStatsDisabled,
		},
	})
	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *ServiceFlags) ValidateAndDefault() error { return nil }
