package evergreen

import (
	"context"
	"reflect"
	"strings"

	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

// ServiceFlags holds the state of each of the runner/API processes
type ServiceFlags struct {
	TaskDispatchDisabled               bool `bson:"task_dispatch_disabled" json:"task_dispatch_disabled"`
	HostInitDisabled                   bool `bson:"host_init_disabled" json:"host_init_disabled"`
	LargeParserProjectsDisabled        bool `bson:"large_parser_projects_disabled" json:"large_parser_projects_disabled"`
	MonitorDisabled                    bool `bson:"monitor_disabled" json:"monitor_disabled"`
	AlertsDisabled                     bool `bson:"alerts_disabled" json:"alerts_disabled"`
	AgentStartDisabled                 bool `bson:"agent_start_disabled" json:"agent_start_disabled"`
	RepotrackerDisabled                bool `bson:"repotracker_disabled" json:"repotracker_disabled"`
	SchedulerDisabled                  bool `bson:"scheduler_disabled" json:"scheduler_disabled"`
	CheckBlockedTasksDisabled          bool `bson:"check_blocked_tasks_disabled" json:"check_blocked_tasks_disabled"`
	GithubPRTestingDisabled            bool `bson:"github_pr_testing_disabled" json:"github_pr_testing_disabled"`
	CLIUpdatesDisabled                 bool `bson:"cli_updates_disabled" json:"cli_updates_disabled"`
	BackgroundStatsDisabled            bool `bson:"background_stats_disabled" json:"background_stats_disabled"`
	TaskLoggingDisabled                bool `bson:"task_logging_disabled" json:"task_logging_disabled"`
	CacheStatsJobDisabled              bool `bson:"cache_stats_job_disabled" json:"cache_stats_job_disabled"`
	CacheStatsEndpointDisabled         bool `bson:"cache_stats_endpoint_disabled" json:"cache_stats_endpoint_disabled"`
	TaskReliabilityDisabled            bool `bson:"task_reliability_disabled" json:"task_reliability_disabled"`
	HostAllocatorDisabled              bool `bson:"host_allocator_disabled" json:"host_allocator_disabled"`
	BackgroundReauthDisabled           bool `bson:"background_reauth_disabled" json:"background_reauth_disabled"`
	CloudCleanupDisabled               bool `bson:"cloud_cleanup_disabled" json:"cloud_cleanup_disabled"`
	SleepScheduleDisabled              bool `bson:"sleep_schedule_disabled" json:"sleep_schedule_disabled"`
	StaticAPIKeysDisabled              bool `bson:"static_api_keys_disabled" json:"static_api_keys_disabled"`
	JWTTokenForCLIDisabled             bool `bson:"jwt_token_for_cli_disabled" json:"jwt_token_for_cli_disabled"`
	SystemFailedTaskRestartDisabled    bool `bson:"system_failed_task_restart_disabled" json:"system_failed_task_restart_disabled"`
	CPUDegradedModeDisabled            bool `bson:"cpu_degraded_mode_disabled" json:"cpu_degraded_mode_disabled"`
	ElasticIPsDisabled                 bool `bson:"elastic_ips_disabled" json:"elastic_ips_disabled"`
	ReleaseModeDisabled                bool `bson:"release_mode_disabled" json:"release_mode_disabled"`
	LegacyUIAdminPageDisabled          bool `bson:"legacy_ui_admin_page_disabled" json:"legacy_ui_admin_page_disabled"`
	DebugSpawnHostDisabled             bool `bson:"debug_spawn_host_disabled" json:"debug_spawn_host_disabled"`
	S3LifecycleSyncDisabled            bool `bson:"s3_lifecycle_sync_disabled" json:"s3_lifecycle_sync_disabled"`
	UseMergeQueuePathFilteringDisabled bool `bson:"use_merge_queue_path_filtering_disabled" json:"use_merge_queue_path_filtering_disabled"`
	PSLoggingDisabled                  bool `bson:"ps_logging_disabled" json:"ps_logging_disabled"`

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
	return getConfigSection(ctx, c)
}

func (c *ServiceFlags) Set(ctx context.Context) error {
	return errors.Wrapf(setConfigSection(ctx, c.SectionId(), bson.M{
		"$set": bson.M{
			taskDispatchKey:                       c.TaskDispatchDisabled,
			hostInitKey:                           c.HostInitDisabled,
			largeParserProjectsDisabledKey:        c.LargeParserProjectsDisabled,
			monitorKey:                            c.MonitorDisabled,
			alertsKey:                             c.AlertsDisabled,
			agentStartKey:                         c.AgentStartDisabled,
			repotrackerKey:                        c.RepotrackerDisabled,
			schedulerKey:                          c.SchedulerDisabled,
			checkBlockedTasksKey:                  c.CheckBlockedTasksDisabled,
			githubPRTestingDisabledKey:            c.GithubPRTestingDisabled,
			cliUpdatesDisabledKey:                 c.CLIUpdatesDisabled,
			backgroundStatsDisabledKey:            c.BackgroundStatsDisabled,
			eventProcessingDisabledKey:            c.EventProcessingDisabled,
			jiraNotificationsDisabledKey:          c.JIRANotificationsDisabled,
			slackNotificationsDisabledKey:         c.SlackNotificationsDisabled,
			emailNotificationsDisabledKey:         c.EmailNotificationsDisabled,
			webhookNotificationsDisabledKey:       c.WebhookNotificationsDisabled,
			githubStatusAPIDisabledKey:            c.GithubStatusAPIDisabled,
			taskLoggingDisabledKey:                c.TaskLoggingDisabled,
			cacheStatsJobDisabledKey:              c.CacheStatsJobDisabled,
			cacheStatsEndpointDisabledKey:         c.CacheStatsEndpointDisabled,
			taskReliabilityDisabledKey:            c.TaskReliabilityDisabled,
			hostAllocatorDisabledKey:              c.HostAllocatorDisabled,
			backgroundReauthDisabledKey:           c.BackgroundReauthDisabled,
			cloudCleanupDisabledKey:               c.CloudCleanupDisabled,
			sleepScheduleDisabledKey:              c.SleepScheduleDisabled,
			staticAPIKeysDisabledKey:              c.StaticAPIKeysDisabled,
			JWTTokenForCLIDisabledKey:             c.JWTTokenForCLIDisabled,
			elasticIPsDisabledKey:                 c.ElasticIPsDisabled,
			systemFailedTaskRestartDisabledKey:    c.SystemFailedTaskRestartDisabled,
			cpuDegradedModeDisabledKey:            c.CPUDegradedModeDisabled,
			releaseModeDisabledKey:                c.ReleaseModeDisabled,
			legacyUIAdminPageDisabledKey:          c.LegacyUIAdminPageDisabled,
			debugSpawnHostDisabledKey:             c.DebugSpawnHostDisabled,
			s3LifecycleSyncDisabledKey:            c.S3LifecycleSyncDisabled,
			useMergeQueuePathFilteringDisabledKey: c.UseMergeQueuePathFilteringDisabled,
			psLoggingDisabledKey:                  c.PSLoggingDisabled,
		}}), "updating config section '%s'", c.SectionId(),
	)
}

func (c *ServiceFlags) ValidateAndDefault() error { return nil }

// ServiceFlagEntry holds the name and enabled state of a single service flag.
type ServiceFlagEntry struct {
	Name    string
	Enabled bool
}

// ToSlice returns all service flags as an ordered slice in struct declaration order.
// This allows new flags to be discovered dynamically without modifying any additional code.
func (c *ServiceFlags) ToSlice() []ServiceFlagEntry {
	v := reflect.ValueOf(*c)
	t := v.Type()
	result := make([]ServiceFlagEntry, 0, t.NumField())
	for i := 0; i < t.NumField(); i++ {
		field := t.Field(i)
		if field.Type.Kind() != reflect.Bool {
			continue
		}
		jsonTag := strings.Split(field.Tag.Get("json"), ",")[0]
		if jsonTag == "" || jsonTag == "-" {
			continue
		}
		result = append(result, ServiceFlagEntry{Name: jsonTag, Enabled: v.Field(i).Bool()})
	}
	return result
}

// SetByName sets the service flag with the given JSON tag name to the given value.
// Returns an error if no field with that name exists.
func (c *ServiceFlags) SetByName(name string, value bool) error {
	v := reflect.ValueOf(c).Elem()
	t := v.Type()
	for i := 0; i < t.NumField(); i++ {
		jsonTag := strings.Split(t.Field(i).Tag.Get("json"), ",")[0]
		if jsonTag == name {
			v.Field(i).SetBool(value)
			return nil
		}
	}
	return errors.Errorf("unknown service flag '%s'", name)
}
