package event

import (
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

func init() {
	registry.AddType(ResourceTypeHost, func() interface{} { return &HostEventData{} })
	registry.AllowSubscription(ResourceTypeHost, EventHostExpirationWarningSent)
	registry.AllowSubscription(ResourceTypeHost, EventVolumeExpirationWarningSent)
	registry.AllowSubscription(ResourceTypeHost, EventHostProvisioned)
	registry.AllowSubscription(ResourceTypeHost, EventHostProvisionFailed)
	registry.AllowSubscription(ResourceTypeHost, EventHostStarted)
	registry.AllowSubscription(ResourceTypeHost, EventHostStopped)
	registry.AllowSubscription(ResourceTypeHost, EventHostModified)
}

const (
	// resource type
	ResourceTypeHost = "HOST"

	// event types
	EventHostCreated                     = "HOST_CREATED"
	EventHostStarted                     = "HOST_STARTED"
	EventHostStopped                     = "HOST_STOPPED"
	EventHostModified                    = "HOST_MODIFIED"
	EventHostFallback                    = "HOST_FALLBACK"
	EventHostAgentDeployed               = "HOST_AGENT_DEPLOYED"
	EventHostAgentDeployFailed           = "HOST_AGENT_DEPLOY_FAILED"
	EventHostAgentMonitorDeployed        = "HOST_AGENT_MONITOR_DEPLOYED"
	EventHostAgentMonitorDeployFailed    = "HOST_AGENT_MONITOR_DEPLOY_FAILED"
	EventHostJasperRestarting            = "HOST_JASPER_RESTARTING"
	EventHostJasperRestarted             = "HOST_JASPER_RESTARTED"
	EventHostJasperRestartError          = "HOST_JASPER_RESTART_ERROR"
	EventHostConvertingProvisioning      = "HOST_CONVERTING_PROVISIONING"
	EventHostConvertedProvisioning       = "HOST_CONVERTED_PROVISIONING"
	EventHostConvertingProvisioningError = "HOST_CONVERTING_PROVISIONING_ERROR"
	EventHostStatusChanged               = "HOST_STATUS_CHANGED"
	EventHostDNSNameSet                  = "HOST_DNS_NAME_SET"
	EventHostProvisionError              = "HOST_PROVISION_ERROR"
	EventHostProvisionFailed             = "HOST_PROVISION_FAILED"
	EventHostProvisioned                 = "HOST_PROVISIONED"
	EventHostRunningTaskSet              = "HOST_RUNNING_TASK_SET"
	EventHostRunningTaskCleared          = "HOST_RUNNING_TASK_CLEARED"
	EventHostMonitorFlag                 = "HOST_MONITOR_FLAG"
	EventHostTaskFinished                = "HOST_TASK_FINISHED"
	EventHostTerminatedExternally        = "HOST_TERMINATED_EXTERNALLY"
	EventHostExpirationWarningSent       = "HOST_EXPIRATION_WARNING_SENT"
	EventHostScriptExecuted              = "HOST_SCRIPT_EXECUTED"
	EventHostScriptExecuteFailed         = "HOST_SCRIPT_EXECUTE_FAILED"
	EventVolumeExpirationWarningSent     = "VOLUME_EXPIRATION_WARNING_SENT"
	EventVolumeMigrationFailed           = "VOLUME_MIGRATION_FAILED"
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
}

var (
	hostDataStatusKey = bsonutil.MustHaveTag(HostEventData{}, "TaskStatus")
)

func LogHostEvent(hostId string, eventType string, eventData HostEventData) {
	event := EventLogEntry{
		Timestamp:    time.Now(),
		ResourceId:   hostId,
		EventType:    eventType,
		Data:         eventData,
		ResourceType: ResourceTypeHost,
	}

	if err := event.Log(); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeHost,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

func LogHostCreated(hostId string) {
	LogHostEvent(hostId, EventHostCreated, HostEventData{Successful: true})
}

// LogHostCreationFailed logs an event indicating that the host errored while it
// was being created.
func LogHostCreationFailed(hostID, logs string) {
	LogHostEvent(hostID, EventHostCreated, HostEventData{Successful: false, Logs: logs})
}

// LogHostStartSucceeded logs an event indicating that the host was successfully
// started.
func LogHostStartSucceeded(hostID string) {
	LogHostEvent(hostID, EventHostStarted, HostEventData{Successful: true})
}

// LogHostStartError logs an event indicating that the host errored while
// starting.
func LogHostStartError(hostID, logs string) {
	LogHostEvent(hostID, EventHostStarted, HostEventData{Successful: false, Logs: logs})
}

// LogHostStopSucceeded logs an event indicating that the host was successfully
// stopped.
func LogHostStopSucceeded(hostID string) {
	LogHostEvent(hostID, EventHostStopped, HostEventData{Successful: true})
}

// LogHostStopError logs an event indicating that the host errored while
// stopping.
func LogHostStopError(hostID, logs string) {
	LogHostEvent(hostID, EventHostStopped, HostEventData{Successful: false, Logs: logs})
}

// LogHostModifySucceeded logs an event indicating that the host was
// successfully modified.
func LogHostModifySucceeded(hostID string) {
	LogHostEvent(hostID, EventHostModified, HostEventData{Successful: true})
}

// LogHostModifyError logs an event indicating that the host errored while being
// modified.
func LogHostModifyError(hostID, logs string) {
	LogHostEvent(hostID, EventHostModified, HostEventData{Successful: false, Logs: logs})
}

func LogHostAgentDeployed(hostId string) {
	LogHostEvent(hostId, EventHostAgentDeployed, HostEventData{
		AgentRevision: evergreen.AgentVersion,
		AgentBuild:    evergreen.BuildRevision,
	})
}

func LogHostAgentDeployFailed(hostId string, err error) {
	LogHostEvent(hostId, EventHostAgentDeployFailed, HostEventData{Logs: err.Error()})
}

func LogHostAgentMonitorDeployed(hostId string) {
	LogHostEvent(hostId, EventHostAgentMonitorDeployed, HostEventData{
		AgentBuild:    evergreen.BuildRevision,
		AgentRevision: evergreen.AgentVersion,
	})
}

func LogHostAgentMonitorDeployFailed(hostId string, err error) {
	LogHostEvent(hostId, EventHostAgentMonitorDeployFailed, HostEventData{Logs: err.Error()})
}

func LogHostJasperRestarting(hostId, user string) {
	LogHostEvent(hostId, EventHostJasperRestarting, HostEventData{User: user})
}

func LogHostJasperRestarted(hostId, revision string) {
	LogHostEvent(hostId, EventHostJasperRestarted, HostEventData{JasperRevision: revision})
}

func LogHostJasperRestartError(hostId string, err error) {
	LogHostEvent(hostId, EventHostJasperRestartError, HostEventData{Logs: err.Error()})
}

func LogHostConvertingProvisioning(hostID, method, user string) {
	LogHostEvent(hostID, EventHostConvertingProvisioning, HostEventData{ProvisioningMethod: method, User: user})
}

func LogHostConvertedProvisioning(hostID, method string) {
	LogHostEvent(hostID, EventHostConvertedProvisioning, HostEventData{ProvisioningMethod: method})
}

func LogHostConvertingProvisioningError(hostId string, err error) {
	LogHostEvent(hostId, EventHostConvertingProvisioningError, HostEventData{Logs: err.Error()})
}

// LogHostProvisionError is used to log each failed provision attempt
func LogHostProvisionError(hostId string) {
	LogHostEvent(hostId, EventHostProvisionError, HostEventData{})
}

func LogHostTerminatedExternally(hostId, oldStatus string) {
	LogHostEvent(hostId, EventHostStatusChanged, HostEventData{OldStatus: oldStatus, NewStatus: EventHostTerminatedExternally, User: evergreen.HostExternalUserName})
}

func LogHostStatusChanged(hostId, oldStatus, newStatus, user string, logs string) {
	if oldStatus == newStatus {
		return
	}
	LogHostEvent(hostId, EventHostStatusChanged, HostEventData{
		OldStatus: oldStatus,
		NewStatus: newStatus,
		User:      user,
		Logs:      logs,
	})
}

func LogHostDNSNameSet(hostId string, dnsName string) {
	LogHostEvent(hostId, EventHostDNSNameSet,
		HostEventData{Hostname: dnsName})
}

func LogHostProvisioned(hostId string) {
	LogHostEvent(hostId, EventHostProvisioned, HostEventData{})
}

func LogHostRunningTaskSet(hostId string, taskId string, taskExecution int) {
	LogHostEvent(hostId, EventHostRunningTaskSet,
		HostEventData{
			TaskId:    taskId,
			Execution: strconv.Itoa(taskExecution),
		})
}

func LogHostRunningTaskCleared(hostId string, taskId string, taskExecution int) {
	LogHostEvent(hostId, EventHostRunningTaskCleared,
		HostEventData{
			TaskId:    taskId,
			Execution: strconv.Itoa(taskExecution),
		})
}

// LogHostProvisionFailed is used when Evergreen gives up on provisioning a host
// after several retries.
func LogHostProvisionFailed(hostId string, setupLogs string) {
	LogHostEvent(hostId, EventHostProvisionFailed, HostEventData{Logs: setupLogs})
}

func LogSpawnhostExpirationWarningSent(hostID string) {
	LogHostEvent(hostID, EventHostExpirationWarningSent, HostEventData{})
}

func LogVolumeExpirationWarningSent(volumeID string) {
	LogHostEvent(volumeID, EventVolumeExpirationWarningSent, HostEventData{})
}

func LogHostScriptExecuted(hostID string, logs string) {
	LogHostEvent(hostID, EventHostScriptExecuted, HostEventData{Logs: logs})
}

func LogHostScriptExecuteFailed(hostID string, err error) {
	LogHostEvent(hostID, EventHostScriptExecuteFailed, HostEventData{Logs: err.Error()})
}

// LogVolumeMigrationFailed is used when a volume is unable to migrate to a new host.
func LogVolumeMigrationFailed(hostID string, err error) {
	LogHostEvent(hostID, EventVolumeMigrationFailed, HostEventData{Logs: err.Error()})
}
