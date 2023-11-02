package evergreen

import (
	"context"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ServiceFlags holds the state of each of the runner/API processes
type ServiceFlags struct {
	TaskDispatchDisabled           bool `bson:"task_dispatch_disabled" json:"task_dispatch_disabled"`
	HostInitDisabled               bool `bson:"host_init_disabled" json:"host_init_disabled"`
	PodInitDisabled                bool `bson:"pod_init_disabled" json:"pod_init_disabled"`
	S3BinaryDownloadsDisabled      bool `bson:"s3_binary_downloads_disabled" json:"s3_binary_downloads_disabled"`
	MonitorDisabled                bool `bson:"monitor_disabled" json:"monitor_disabled"`
	AlertsDisabled                 bool `bson:"alerts_disabled" json:"alerts_disabled"`
	AgentStartDisabled             bool `bson:"agent_start_disabled" json:"agent_start_disabled"`
	RepotrackerDisabled            bool `bson:"repotracker_disabled" json:"repotracker_disabled"`
	SchedulerDisabled              bool `bson:"scheduler_disabled" json:"scheduler_disabled"`
	CheckBlockedTasksDisabled      bool `bson:"check_blocked_tasks_disabled" json:"check_blocked_tasks_disabled"`
	GithubPRTestingDisabled        bool `bson:"github_pr_testing_disabled" json:"github_pr_testing_disabled"`
	CLIUpdatesDisabled             bool `bson:"cli_updates_disabled" json:"cli_updates_disabled"`
	BackgroundStatsDisabled        bool `bson:"background_stats_disabled" json:"background_stats_disabled"`
	TaskLoggingDisabled            bool `bson:"task_logging_disabled" json:"task_logging_disabled"`
	CacheStatsJobDisabled          bool `bson:"cache_stats_job_disabled" json:"cache_stats_job_disabled"`
	CacheStatsEndpointDisabled     bool `bson:"cache_stats_endpoint_disabled" json:"cache_stats_endpoint_disabled"`
	TaskReliabilityDisabled        bool `bson:"task_reliability_disabled" json:"task_reliability_disabled"`
	CommitQueueDisabled            bool `bson:"commit_queue_disabled" json:"commit_queue_disabled"`
	HostAllocatorDisabled          bool `bson:"host_allocator_disabled" json:"host_allocator_disabled"`
	PodAllocatorDisabled           bool `bson:"pod_allocator_disabled" json:"pod_allocator_disabled"`
	UnrecognizedPodCleanupDisabled bool `bson:"unrecognized_pod_cleanup_disabled" json:"unrecognized_pod_cleanup_disabled"`
	BackgroundReauthDisabled       bool `bson:"background_reauth_disabled" json:"background_reauth_disabled"`
	BackgroundCleanupDisabled      bool `bson:"background_cleanup_disabled" json:"background_cleanup_disabled"`
	CloudCleanupDisabled           bool `bson:"cloud_cleanup_disabled" json:"cloud_cleanup_disabled"`
	LegacyUIPublicAccessDisabled   bool `bson:"legacy_ui_public_access_disabled" json:"legacy_ui_public_access_disabled"`
	GlobalGitHubTokenDisabled      bool `bson:"global_github_token_disabled" json:"global_github_token_disabled"`

	// Notification Flags
	EventProcessingDisabled      bool `bson:"event_processing_disabled" json:"event_processing_disabled"`
	JIRANotificationsDisabled    bool `bson:"jira_notifications_disabled" json:"jira_notifications_disabled"`
	SlackNotificationsDisabled   bool `bson:"slack_notifications_disabled" json:"slack_notifications_disabled"`
	EmailNotificationsDisabled   bool `bson:"email_notifications_disabled" json:"email_notifications_disabled"`
	WebhookNotificationsDisabled bool `bson:"webhook_notifications_disabled" json:"webhook_notifications_disabled"`
	GithubStatusAPIDisabled      bool `bson:"github_status_api_disabled" json:"github_status_api_disabled"`
}

func (c *ServiceFlags) SectionId() string { return "service_flags" }

func (c *ServiceFlags) Get(ctx context.Context) error {
	res := GetEnvironment().DB().Collection(ConfigCollection).FindOne(ctx, byId(c.SectionId()))
	if err := res.Err(); err != nil {
		if err == mongo.ErrNoDocuments {
			*c = ServiceFlags{}
			return nil
		}
		return errors.Wrapf(err, "getting config section '%s'", c.SectionId())
	}

	if err := res.Decode(&c); err != nil {
		return errors.Wrapf(err, "decoding config section '%s'", c.SectionId())
	}
	return nil
}

func (c *ServiceFlags) Set(ctx context.Context) error {
	_, err := GetEnvironment().DB().Collection(ConfigCollection).UpdateOne(ctx, byId(c.SectionId()), bson.M{
		"$set": bson.M{
			taskDispatchKey:                   c.TaskDispatchDisabled,
			hostInitKey:                       c.HostInitDisabled,
			podInitDisabledKey:                c.PodInitDisabled,
			s3BinaryDownloadsDisabledKey:      c.S3BinaryDownloadsDisabled,
			monitorKey:                        c.MonitorDisabled,
			alertsKey:                         c.AlertsDisabled,
			agentStartKey:                     c.AgentStartDisabled,
			repotrackerKey:                    c.RepotrackerDisabled,
			schedulerKey:                      c.SchedulerDisabled,
			checkBlockedTasksKey:              c.CheckBlockedTasksDisabled,
			githubPRTestingDisabledKey:        c.GithubPRTestingDisabled,
			cliUpdatesDisabledKey:             c.CLIUpdatesDisabled,
			backgroundStatsDisabledKey:        c.BackgroundStatsDisabled,
			eventProcessingDisabledKey:        c.EventProcessingDisabled,
			jiraNotificationsDisabledKey:      c.JIRANotificationsDisabled,
			slackNotificationsDisabledKey:     c.SlackNotificationsDisabled,
			emailNotificationsDisabledKey:     c.EmailNotificationsDisabled,
			webhookNotificationsDisabledKey:   c.WebhookNotificationsDisabled,
			githubStatusAPIDisabledKey:        c.GithubStatusAPIDisabled,
			taskLoggingDisabledKey:            c.TaskLoggingDisabled,
			cacheStatsJobDisabledKey:          c.CacheStatsJobDisabled,
			cacheStatsEndpointDisabledKey:     c.CacheStatsEndpointDisabled,
			taskReliabilityDisabledKey:        c.TaskReliabilityDisabled,
			commitQueueDisabledKey:            c.CommitQueueDisabled,
			hostAllocatorDisabledKey:          c.HostAllocatorDisabled,
			podAllocatorDisabledKey:           c.PodAllocatorDisabled,
			backgroundCleanupDisabledKey:      c.BackgroundCleanupDisabled,
			backgroundReauthDisabledKey:       c.BackgroundReauthDisabled,
			cloudCleanupDisabledKey:           c.CloudCleanupDisabled,
			legacyUIPublicAccessDisabledKey:   c.LegacyUIPublicAccessDisabled,
			globalGitHubTokenDisabledKey:      c.GlobalGitHubTokenDisabled,
			unrecognizedPodCleanupDisabledKey: c.UnrecognizedPodCleanupDisabled,
		},
	}, options.Update().SetUpsert(true))

	return errors.Wrapf(err, "updating config section '%s'", c.SectionId())
}

func (c *ServiceFlags) ValidateAndDefault() error { return nil }
