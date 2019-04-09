package event

import (
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"go.mongodb.org/mongo-driver/bson"
)

func init() {
	registry.AddType(ResourceTypeHost, hostEventDataFactory)
	registry.AllowSubscription(ResourceTypeHost, EventHostExpirationWarningSent)
	registry.AllowSubscription(ResourceTypeHost, EventHostProvisioned)
	registry.AllowSubscription(ResourceTypeHost, EventHostProvisionFailed)
}

const (
	// resource type
	ResourceTypeHost = "HOST"

	// event types
	EventHostCreated               = "HOST_CREATED"
	EventHostStarted               = "HOST_STARTED"
	EventHostAgentDeployed         = "HOST_AGENT_DEPLOYED"
	EventHostAgentDeployFailed     = "HOST_AGENT_DEPLOY_FAILED"
	EventHostStatusChanged         = "HOST_STATUS_CHANGED"
	EventHostDNSNameSet            = "HOST_DNS_NAME_SET"
	EventHostProvisionError        = "HOST_PROVISION_ERROR"
	EventHostProvisionFailed       = "HOST_PROVISION_FAILED"
	EventHostProvisioned           = "HOST_PROVISIONED"
	EventHostRunningTaskSet        = "HOST_RUNNING_TASK_SET"
	EventHostRunningTaskCleared    = "HOST_RUNNING_TASK_CLEARED"
	EventHostTaskPidSet            = "HOST_TASK_PID_SET"
	EventHostMonitorFlag           = "HOST_MONITOR_FLAG"
	EventTaskFinished              = "HOST_TASK_FINISHED"
	EventHostTeardown              = "HOST_TEARDOWN"
	EventHostTerminatedExternally  = "HOST_TERMINATED_EXTERNALLY"
	EventHostExpirationWarningSent = "HOST_EXPIRATION_WARNING_SENT"
)

// implements EventData
type HostEventData struct {
	AgentRevision string        `bson:"a_rev,omitempty" json:"agent_revision,omitempty"`
	OldStatus     string        `bson:"o_s,omitempty" json:"old_status,omitempty"`
	NewStatus     string        `bson:"n_s,omitempty" json:"new_status,omitempty"`
	Logs          string        `bson:"log,omitempty" json:"logs,omitempty"`
	Hostname      string        `bson:"hn,omitempty" json:"hostname,omitempty"`
	TaskId        string        `bson:"t_id,omitempty" json:"task_id,omitempty"`
	TaskExecution int           `bson:"t_execution,omitempty" json:"task_execution,omitempty"`
	TaskPid       string        `bson:"t_pid,omitempty" json:"task_pid,omitempty"`
	TaskStatus    string        `bson:"t_st,omitempty" json:"task_status,omitempty"`
	Execution     string        `bson:"execution,omitempty" json:"execution,omitempty"`
	MonitorOp     string        `bson:"monitor_op,omitempty" json:"monitor,omitempty"`
	User          string        `bson:"usr" json:"user,omitempty"`
	Successful    bool          `bson:"successful,omitempty" json:"successful"`
	Duration      time.Duration `bson:"duration,omitempty" json:"duration"`
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

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(&event); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"resource_type": ResourceTypeHost,
			"message":       "error logging event",
			"source":        "event-log-fail",
		}))
	}
}

func LogHostStarted(hostId string) {
	LogHostEvent(hostId, EventHostStarted, HostEventData{})
}

func LogHostCreated(hostId string) {
	LogHostEvent(hostId, EventHostCreated, HostEventData{})
}

func LogHostAgentDeployed(hostId string) {
	LogHostEvent(hostId, EventHostAgentDeployed, HostEventData{AgentRevision: evergreen.BuildRevision})
}

func LogHostAgentDeployFailed(hostId string, err error) {
	LogHostEvent(hostId, EventHostAgentDeployFailed, HostEventData{Logs: err.Error()})
}

// LogHostProvisionError is used to log each failed provision attempt
func LogHostProvisionError(hostId string) {
	LogHostEvent(hostId, EventHostProvisionError, HostEventData{})
}

func LogHostTerminatedExternally(hostId string) {
	LogHostEvent(hostId, EventHostStatusChanged, HostEventData{NewStatus: EventHostTerminatedExternally})
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

func LogHostRunningTaskSet(hostId string, taskId string) {
	LogHostEvent(hostId, EventHostRunningTaskSet,
		HostEventData{TaskId: taskId})
}

func LogHostRunningTaskCleared(hostId string, taskId string) {
	LogHostEvent(hostId, EventHostRunningTaskCleared,
		HostEventData{TaskId: taskId})
}

func LogHostTaskPidSet(hostId string, taskPid string) {
	LogHostEvent(hostId, EventHostTaskPidSet, HostEventData{TaskPid: taskPid})
}

// LogProvisionFailed is used when Evergreen gives up on spawning a host after
// several retries
func LogProvisionFailed(hostId string, setupLogs string) {
	LogHostEvent(hostId, EventHostProvisionFailed, HostEventData{Logs: setupLogs})
}

func LogHostTeardown(hostId, teardownLogs string, success bool, duration time.Duration) {
	LogHostEvent(hostId, EventHostTeardown,
		HostEventData{Logs: teardownLogs, Successful: success, Duration: duration})
}

func LogMonitorOperation(hostId string, op string) {
	LogHostEvent(hostId, EventHostMonitorFlag, HostEventData{MonitorOp: op})
}

func LogExpirationWarningSent(hostID string) {
	LogHostEvent(hostID, EventHostExpirationWarningSent, HostEventData{})
}

// UpdateExecutions updates host events to track multiple executions of the same task
func UpdateExecutions(hostId, taskId string, execution int) error {
	taskIdKey := bsonutil.MustHaveTag(HostEventData{}, "TaskId")
	executionKey := bsonutil.MustHaveTag(HostEventData{}, "Execution")
	query := bson.M{
		"r_id":                    hostId,
		DataKey + "." + taskIdKey: taskId,
	}
	update := bson.M{
		"$set": bson.M{
			DataKey + "." + executionKey: strconv.Itoa(execution),
		},
	}
	_, err := db.UpdateAll(AllLogCollection, query, update)
	return err
}
