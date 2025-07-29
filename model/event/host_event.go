package event

import (
	"context"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeHost, func() any { return &HostEventData{} })
	registry.AllowSubscription(ResourceTypeHost, EventHostExpirationWarningSent)
	registry.AllowSubscription(ResourceTypeHost, EventHostTemporaryExemptionExpirationWarningSent)
	registry.AllowSubscription(ResourceTypeHost, EventVolumeExpirationWarningSent)
	registry.AllowSubscription(ResourceTypeHost, EventSpawnHostIdleNotification)
	registry.AllowSubscription(ResourceTypeHost, EventAlertableInstanceTypeWarningSent)
	registry.AllowSubscription(ResourceTypeHost, EventHostProvisioned)
	registry.AllowSubscription(ResourceTypeHost, EventHostProvisionFailed)
	registry.AllowSubscription(ResourceTypeHost, EventSpawnHostCreatedError)
	registry.AllowSubscription(ResourceTypeHost, EventHostStarted)
	registry.AllowSubscription(ResourceTypeHost, EventHostStopped)
	registry.AllowSubscription(ResourceTypeHost, EventHostModified)
	registry.AllowSubscription(ResourceTypeHost, EventHostScriptExecuted)
	registry.AllowSubscription(ResourceTypeHost, EventHostScriptExecuteFailed)
}

const (
	// resource type
	ResourceTypeHost = "HOST"

	// event types
	EventHostCreated                                 = "HOST_CREATED"
	EventHostCreatedError                            = "HOST_CREATED_ERROR"
	EventSpawnHostCreatedError                       = "SPAWN_HOST_CREATED_ERROR"
	EventHostStarted                                 = "HOST_STARTED"
	EventHostStopped                                 = "HOST_STOPPED"
	EventHostModified                                = "HOST_MODIFIED"
	EventHostAgentDeployed                           = "HOST_AGENT_DEPLOYED"
	EventHostAgentDeployFailed                       = "HOST_AGENT_DEPLOY_FAILED"
	EventHostAgentMonitorDeployed                    = "HOST_AGENT_MONITOR_DEPLOYED"
	EventHostAgentMonitorDeployFailed                = "HOST_AGENT_MONITOR_DEPLOY_FAILED"
	EventHostJasperRestarting                        = "HOST_JASPER_RESTARTING"
	EventHostJasperRestarted                         = "HOST_JASPER_RESTARTED"
	EventHostJasperRestartError                      = "HOST_JASPER_RESTART_ERROR"
	EventHostConvertingProvisioning                  = "HOST_CONVERTING_PROVISIONING"
	EventHostConvertedProvisioning                   = "HOST_CONVERTED_PROVISIONING"
	EventHostConvertingProvisioningError             = "HOST_CONVERTING_PROVISIONING_ERROR"
	EventHostStatusChanged                           = "HOST_STATUS_CHANGED"
	EventHostDNSNameSet                              = "HOST_DNS_NAME_SET"
	EventHostProvisionError                          = "HOST_PROVISION_ERROR"
	EventHostProvisionFailed                         = "HOST_PROVISION_FAILED"
	EventHostProvisioned                             = "HOST_PROVISIONED"
	EventHostRunningTaskSet                          = "HOST_RUNNING_TASK_SET"
	EventHostRunningTaskCleared                      = "HOST_RUNNING_TASK_CLEARED"
	EventHostTaskFinished                            = "HOST_TASK_FINISHED"
	EventHostTerminatedExternally                    = "HOST_TERMINATED_EXTERNALLY"
	EventHostExpirationWarningSent                   = "HOST_EXPIRATION_WARNING_SENT"
	EventHostTemporaryExemptionExpirationWarningSent = "HOST_TEMPORARY_EXEMPTION_EXPIRATION_WARNING_SENT"
	EventSpawnHostIdleNotification                   = "HOST_IDLE_NOTIFICATION"
	EventAlertableInstanceTypeWarningSent            = "HOST_ALERTABLE_INSTANCE_TYPE_WARNING_SENT"
	EventHostScriptExecuted                          = "HOST_SCRIPT_EXECUTED"
	EventHostScriptExecuteFailed                     = "HOST_SCRIPT_EXECUTE_FAILED"
	EventVolumeExpirationWarningSent                 = "VOLUME_EXPIRATION_WARNING_SENT"
	EventVolumeMigrationFailed                       = "VOLUME_MIGRATION_FAILED"
)

// implements EventData
type HostEventData struct {
	AgentRevision      string        `bson:"a_rev,omitempty" json:"agent_revision,omitempty"`
	AgentBuild         string        `bson:"a_build,omitempty" json:"agent_build,omitempty"`
	JasperRevision     string        `bson:"j_rev,omitempty" json:"jasper_revision,omitempty"`
	OldStatus          string        `bson:"o_s,omitempty" json:"old_status,omitempty"`
	NewStatus          string        `bson:"n_s,omitempty" json:"new_status,omitempty"`
	Logs               string        `bson:"log,omitempty" json:"logs,omitempty"`
	Hostname           string        `bson:"hn,omitempty" json:"hostname,omitempty"`
	ProvisioningMethod string        `bson:"prov_method" json:"provisioning_method,omitempty"`
	TaskId             string        `bson:"t_id,omitempty" json:"task_id,omitempty"`
	TaskPid            string        `bson:"t_pid,omitempty" json:"task_pid,omitempty"`
	TaskStatus         string        `bson:"t_st,omitempty" json:"task_status,omitempty"`
	Execution          string        `bson:"execution,omitempty" json:"execution,omitempty"`
	MonitorOp          string        `bson:"monitor_op,omitempty" json:"monitor,omitempty"`
	User               string        `bson:"usr" json:"user,omitempty"`
	Successful         bool          `bson:"successful,omitempty" json:"successful"`
	Duration           time.Duration `bson:"duration,omitempty" json:"duration"`
	// Source is the source of a host modification. Only set in specific
	// conditions where a notification may need to know the cause of a host
	// being modified.
	Source string `bson:"source,omitempty" json:"source,omitempty"`
}

var (
	hostDataStatusKey = bsonutil.MustHaveTag(HostEventData{}, "TaskStatus")
)

func LogHostEvent(ctx context.Context, hostId string, eventType string, eventData HostEventData) {
	event := EventLogEntry{
		Timestamp:    time.Now(),
		ResourceId:   hostId,
		EventType:    eventType,
		Data:         eventData,
		ResourceType: ResourceTypeHost,
	}

	if err := event.Log(ctx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeHost,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

// LogHostCreated logs an event indicating that the host was created.
func LogHostCreated(ctx context.Context, hostId string) {
	LogHostEvent(ctx, hostId, EventHostCreated, HostEventData{Successful: true})
}

// LogManyHostsCreated is the same as LogHostCreated but for multiple hosts.
func LogManyHostsCreated(ctx context.Context, hostIDs []string) {
	events := make([]EventLogEntry, 0, len(hostIDs))
	for _, hostID := range hostIDs {
		e := EventLogEntry{
			Timestamp:    time.Now(),
			ResourceId:   hostID,
			EventType:    EventHostCreated,
			Data:         HostEventData{Successful: true},
			ResourceType: ResourceTypeHost,
		}
		events = append(events, e)
	}
	if err := LogManyEvents(ctx, events); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeHost,
			"message":       "error logging events",
			"source":        "event-log-fail",
		}))
	}
}

// LogHostCreatedError logs an event indicating that the host errored while it
// was being created.
func LogHostCreatedError(ctx context.Context, hostID, logs string) {
	LogHostEvent(ctx, hostID, EventHostCreatedError, HostEventData{Successful: false, Logs: logs})
}

// LogSpawnHostCreatedError is the same as LogHostCreatedError but specifically
// for spawn hosts. The spawn host event is separate from the more general host
// creation errors to make notifications on spawn host creation errors more
// efficient.
func LogSpawnHostCreatedError(ctx context.Context, hostID, logs string) {
	LogHostEvent(ctx, hostID, EventSpawnHostCreatedError, HostEventData{Successful: false, Logs: logs})
}

// LogHostStartSucceeded logs an event indicating that the host was successfully
// started.
func LogHostStartSucceeded(ctx context.Context, hostID string, source string) {
	LogHostEvent(ctx, hostID, EventHostStarted, HostEventData{Successful: true, Source: source})
}

// LogHostStartError logs an event indicating that the host errored while
// starting.
func LogHostStartError(ctx context.Context, hostID, source, logs string) {
	LogHostEvent(ctx, hostID, EventHostStarted, HostEventData{Successful: false, Source: source, Logs: logs})
}

// LogHostStopSucceeded logs an event indicating that the host was successfully
// stopped.
func LogHostStopSucceeded(ctx context.Context, hostID, source string) {
	LogHostEvent(ctx, hostID, EventHostStopped, HostEventData{Successful: true, Source: source})
}

// LogHostStopError logs an event indicating that the host errored while
// stopping.
func LogHostStopError(ctx context.Context, hostID, source, logs string) {
	LogHostEvent(ctx, hostID, EventHostStopped, HostEventData{Successful: false, Source: source, Logs: logs})
}

// LogHostModifySucceeded logs an event indicating that the host was
// successfully modified.
func LogHostModifySucceeded(ctx context.Context, hostID, source string) {
	LogHostEvent(ctx, hostID, EventHostModified, HostEventData{Successful: true, Source: source})
}

// LogHostModifyError logs an event indicating that the host errored while being
// modified.
func LogHostModifyError(ctx context.Context, hostID, logs string) {
	LogHostEvent(ctx, hostID, EventHostModified, HostEventData{Successful: false, Logs: logs})
}

func LogHostAgentDeployed(ctx context.Context, hostID string) {
	LogHostEvent(ctx, hostID, EventHostAgentDeployed, HostEventData{
		AgentRevision: evergreen.AgentVersion,
		AgentBuild:    evergreen.BuildRevision,
	})
}

func LogHostAgentDeployFailed(ctx context.Context, hostID string, err error) {
	LogHostEvent(ctx, hostID, EventHostAgentDeployFailed, HostEventData{Logs: err.Error()})
}

func LogHostAgentMonitorDeployed(ctx context.Context, hostID string) {
	LogHostEvent(ctx, hostID, EventHostAgentMonitorDeployed, HostEventData{
		AgentBuild:    evergreen.BuildRevision,
		AgentRevision: evergreen.AgentVersion,
	})
}

func LogHostAgentMonitorDeployFailed(ctx context.Context, hostID string, err error) {
	LogHostEvent(ctx, hostID, EventHostAgentMonitorDeployFailed, HostEventData{Logs: err.Error()})
}

func LogHostJasperRestarting(ctx context.Context, hostID, user string) {
	LogHostEvent(ctx, hostID, EventHostJasperRestarting, HostEventData{User: user})
}

func LogHostJasperRestarted(ctx context.Context, hostID, revision string) {
	LogHostEvent(ctx, hostID, EventHostJasperRestarted, HostEventData{JasperRevision: revision})
}

func LogHostJasperRestartError(ctx context.Context, hostID string, err error) {
	LogHostEvent(ctx, hostID, EventHostJasperRestartError, HostEventData{Logs: err.Error()})
}

func LogHostConvertingProvisioning(ctx context.Context, hostID, method, user string) {
	LogHostEvent(ctx, hostID, EventHostConvertingProvisioning, HostEventData{ProvisioningMethod: method, User: user})
}

func LogHostConvertedProvisioning(ctx context.Context, hostID, method string) {
	LogHostEvent(ctx, hostID, EventHostConvertedProvisioning, HostEventData{ProvisioningMethod: method})
}

func LogHostConvertingProvisioningError(ctx context.Context, hostID string, err error) {
	LogHostEvent(ctx, hostID, EventHostConvertingProvisioningError, HostEventData{Logs: err.Error()})
}

// LogHostProvisionError is used to log each failed provision attempt
func LogHostProvisionError(ctx context.Context, hostID string) {
	LogHostEvent(ctx, hostID, EventHostProvisionError, HostEventData{})
}

func LogHostTerminatedExternally(ctx context.Context, hostID, oldStatus string) {
	LogHostEvent(ctx, hostID, EventHostStatusChanged, HostEventData{OldStatus: oldStatus, NewStatus: EventHostTerminatedExternally, User: evergreen.HostExternalUserName})
}

func LogHostStatusChanged(ctx context.Context, hostId, oldStatus, newStatus, user string, logs string) {
	if oldStatus == newStatus {
		return
	}
	LogHostEvent(ctx, hostId, EventHostStatusChanged, HostEventData{
		OldStatus: oldStatus,
		NewStatus: newStatus,
		User:      user,
		Logs:      logs,
	})
}

func LogHostDNSNameSet(ctx context.Context, hostID string, dnsName string) {
	LogHostEvent(ctx, hostID, EventHostDNSNameSet,
		HostEventData{Hostname: dnsName})
}

func LogHostProvisioned(ctx context.Context, hostID string) {
	LogHostEvent(ctx, hostID, EventHostProvisioned, HostEventData{})
}

func LogHostRunningTaskSet(ctx context.Context, hostID string, taskId string, taskExecution int) {
	LogHostEvent(ctx, hostID, EventHostRunningTaskSet,
		HostEventData{
			TaskId:    taskId,
			Execution: strconv.Itoa(taskExecution),
		})
}

func LogHostRunningTaskCleared(ctx context.Context, hostID string, taskId string, taskExecution int) {
	LogHostEvent(ctx, hostID, EventHostRunningTaskCleared,
		HostEventData{
			TaskId:    taskId,
			Execution: strconv.Itoa(taskExecution),
		})
}

// LogHostProvisionFailed is used when Evergreen gives up on provisioning a host
// after several retries.
func LogHostProvisionFailed(ctx context.Context, hostID string, setupLogs string) {
	LogHostEvent(ctx, hostID, EventHostProvisionFailed, HostEventData{Logs: setupLogs})
}

func LogSpawnhostExpirationWarningSent(ctx context.Context, hostID string) {
	LogHostEvent(ctx, hostID, EventHostExpirationWarningSent, HostEventData{})
}

// LogHostTemporaryExemptionExpirationWarningSent logs an event warning about
// the host's temporary exemption, which is about to expire.
func LogHostTemporaryExemptionExpirationWarningSent(ctx context.Context, hostID string) {
	LogHostEvent(ctx, hostID, EventHostTemporaryExemptionExpirationWarningSent, HostEventData{})
}

func LogVolumeExpirationWarningSent(ctx context.Context, volumeID string) {
	LogHostEvent(ctx, volumeID, EventVolumeExpirationWarningSent, HostEventData{})
}

// LogSpawnHostIdleNotification logs an event for the spawn host being idle.
func LogSpawnHostIdleNotification(ctx context.Context, hostID string) {
	LogHostEvent(ctx, hostID, EventSpawnHostIdleNotification, HostEventData{})
}

// LogAlertableInstanceTypeWarningSent logs an event warning about
// the host using an alertable instance type.
func LogAlertableInstanceTypeWarningSent(ctx context.Context, hostID string) {
	LogHostEvent(ctx, hostID, EventAlertableInstanceTypeWarningSent, HostEventData{})
}

func LogHostScriptExecuted(ctx context.Context, hostID string, logs string) {
	LogHostEvent(ctx, hostID, EventHostScriptExecuted, HostEventData{Logs: logs})
}

func LogHostScriptExecuteFailed(ctx context.Context, hostID, logs string, err error) {
	if logs == "" {
		logs = err.Error()
	}
	LogHostEvent(ctx, hostID, EventHostScriptExecuteFailed, HostEventData{Logs: logs})
}

// LogVolumeMigrationFailed is used when a volume is unable to migrate to a new host.
func LogVolumeMigrationFailed(ctx context.Context, hostID string, err error) {
	LogHostEvent(ctx, hostID, EventVolumeMigrationFailed, HostEventData{Logs: err.Error()})
}
