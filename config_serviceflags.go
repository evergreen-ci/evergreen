package evergreen

import (
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServiceFlags holds the state of each of the runner/API processes
type ServiceFlags struct {
	TaskDispatchDisabled       bool `bson:"task_dispatch_disabled" json:"task_dispatch_disabled"`
	HostInitDisabled           bool `bson:"host_init_disabled" json:"host_init_disabled"`
	MonitorDisabled            bool `bson:"monitor_disabled" json:"monitor_disabled"`
	AlertsDisabled             bool `bson:"alerts_disabled" json:"alerts_disabled"`
	AgentStartDisabled         bool `bson:"agent_start_disabled" json:"agent_start_disabled"`
	RepotrackerDisabled        bool `bson:"repotracker_disabled" json:"repotracker_disabled"`
	SchedulerDisabled          bool `bson:"scheduler_disabled" json:"scheduler_disabled"`
	GithubPRTestingDisabled    bool `bson:"github_pr_testing_disabled" json:"github_pr_testing_disabled"`
	CLIUpdatesDisabled         bool `bson:"cli_updates_disabled" json:"cli_updates_disabled"`
	BackgroundStatsDisabled    bool `bson:"background_stats_disabled" json:"background_stats_disabled"`
	TaskLoggingDisabled        bool `bson:"task_logging_disabled" json:"task_logging_disabled"`
	CacheStatsJobDisabled      bool `bson:"cache_stats_job_disabled" json:"cache_stats_job_disabled"`
	CacheStatsEndpointDisabled bool `bson:"cache_stats_endpoint_disabled" json:"cache_stats_endpoint_disabled"`
	CacheStatsOldTasksDisabled bool `bson:"cache_stats_old_tasks_disabled" json:"cache_stats_old_tasks_disabled"`
	TaskReliabilityDisabled    bool `bson:"task_reliability_disabled" json:"task_reliability_disabled"`
	CommitQueueDisabled        bool `bson:"commit_queue_disabled" json:"commit_queue_disabled"`
	PlannerDisabled            bool `bson:"planner_disabled" json:"planner_disabled"`
	HostAllocatorDisabled      bool `bson:"host_allocator_disabled" json:"host_allocator_disabled"`
	DRBackupDisabled           bool `bson:"dr_backup_disabled" json:"dr_backup_disabled"`

	// Notification Flags
	EventProcessingDisabled      bool `bson:"event_processing_disabled" json:"event_processing_disabled"`
	JIRANotificationsDisabled    bool `bson:"jira_notifications_disabled" json:"jira_notifications_disabled"`
	SlackNotificationsDisabled   bool `bson:"slack_notifications_disabled" json:"slack_notifications_disabled"`
	EmailNotificationsDisabled   bool `bson:"email_notifications_disabled" json:"email_notifications_disabled"`
	WebhookNotificationsDisabled bool `bson:"webhook_notifications_disabled" json:"webhook_notifications_disabled"`
	GithubStatusAPIDisabled      bool `bson:"github_status_api_disabled" json:"github_status_api_disabled"`
}

func (c *ServiceFlags) SectionId() string { return "service_flags" }

func (c *ServiceFlags) Get(env Environment) error {
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	res := coll.FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}

	if err := res.Decode(c); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = ServiceFlags{}
			return nil
		}
		return errors.Wrapf(err, "error retrieving section %s", c.SectionId())
	}
	return nil
}

func (c *ServiceFlags) Set() error {
	env := GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()
	coll := env.DB().Collection(ConfigCollection)

	_, err := coll.UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			taskDispatchKey:                 c.TaskDispatchDisabled,
			hostInitKey:                     c.HostInitDisabled,
			monitorKey:                      c.MonitorDisabled,
			alertsKey:                       c.AlertsDisabled,
			agentStartKey:                   c.AgentStartDisabled,
			repotrackerKey:                  c.RepotrackerDisabled,
			schedulerKey:                    c.SchedulerDisabled,
			githubPRTestingDisabledKey:      c.GithubPRTestingDisabled,
			cliUpdatesDisabledKey:           c.CLIUpdatesDisabled,
			backgroundStatsDisabledKey:      c.BackgroundStatsDisabled,
			eventProcessingDisabledKey:      c.EventProcessingDisabled,
			jiraNotificationsDisabledKey:    c.JIRANotificationsDisabled,
			slackNotificationsDisabledKey:   c.SlackNotificationsDisabled,
			emailNotificationsDisabledKey:   c.EmailNotificationsDisabled,
			webhookNotificationsDisabledKey: c.WebhookNotificationsDisabled,
			githubStatusAPIDisabledKey:      c.GithubStatusAPIDisabled,
			taskLoggingDisabledKey:          c.TaskLoggingDisabled,
			cacheStatsJobDisabledKey:        c.CacheStatsJobDisabled,
			cacheStatsEndpointDisabledKey:   c.CacheStatsEndpointDisabled,
			cacheStatsOldTasksDisabledKey:   c.CacheStatsOldTasksDisabled,
			taskReliabilityDisabledKey:      c.TaskReliabilityDisabled,
			commitQueueDisabledKey:          c.CommitQueueDisabled,
			plannerDisabledKey:              c.PlannerDisabled,
			hostAllocatorDisabledKey:        c.HostAllocatorDisabled,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "error updating section %s", c.SectionId())
}

func (c *ServiceFlags) ValidateAndDefault() error { return nil }
