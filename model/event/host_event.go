package event

import (
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"gopkg.in/mgo.v2/bson"
)

const (
	// resource type
	ResourceTypeHost = "HOST"

	// event types
	EventHostCreated              = "HOST_CREATED"
	EventHostStarted              = "HOST_STARTED"
	EventHostAgentDeployed        = "HOST_AGENT_DEPLOYED"
	EventHostStatusChanged        = "HOST_STATUS_CHANGED"
	EventHostDNSNameSet           = "HOST_DNS_NAME_SET"
	EventHostProvisionFailed      = "HOST_PROVISION_FAILED"
	EventHostProvisioned          = "HOST_PROVISIONED"
	EventHostRunningTaskSet       = "HOST_RUNNING_TASK_SET"
	EventHostRunningTaskCleared   = "HOST_RUNNING_TASK_CLEARED"
	EventHostTaskPidSet           = "HOST_TASK_PID_SET"
	EventHostMonitorFlag          = "HOST_MONITOR_FLAG"
	EventTaskFinished             = "HOST_TASK_FINISHED"
	EventHostTeardown             = "HOST_TEARDOWN"
	EventHostTerminatedExternally = "HOST_TERMINATED_EXTERNALLY"
)

// implements EventData
type HostEventData struct {
	// necessary for IsValid
	ResourceType string `bson:"r_type" json:"resource_type"`

	AgentRevision string        `bson:"a_rev,omitempty" json:"agent_revision,omitempty"`
	OldStatus     string        `bson:"o_s,omitempty" json:"old_status,omitempty"`
	NewStatus     string        `bson:"n_s,omitempty" json:"new_status,omitempty"`
	Logs          string        `bson:"log,omitempty" json:"logs,omitempty"`
	Hostname      string        `bson:"hn,omitempty" json:"hostname,omitempty"`
	TaskId        string        `bson:"t_id,omitempty" json:"task_id,omitempty"`
	TaskPid       string        `bson:"t_pid,omitempty" json:"task_pid,omitempty"`
	TaskStatus    string        `bson:"t_st,omitempty" json:"task_status,omitempty"`
	Execution     string        `bson:"execution,omitempty" json:"execution,omitempty"`
	MonitorOp     string        `bson:"monitor_op,omitempty" json:"monitor,omitempty"`
	Successful    bool          `bson:"successful,omitempty" json:"successful"`
	Duration      time.Duration `bson:"duration,omitempty" json:"duration"`
}

func GetHostEvent(events []Event) ([]HostEventData, error) {
	out := []HostEventData{}
	errCount := 0

	for _, e := range events {
		hostEvent, ok := e.Data.(HostEventData)
		if !ok {
			errCount++
			continue
		}
		out = append(out, hostEvent)
	}

	grip
}

func (self HostEventData) IsValid() bool {
	return self.ResourceType == ResourceTypeHost
}

func LogHostEvent(hostId string, eventType string, eventData HostEventData) {
	eventData.ResourceType = ResourceTypeHost
	event := Event{
		Timestamp:  time.Now(),
		ResourceId: hostId,
		EventType:  eventType,
		Data:       DataWrapper{eventData},
	}

	logger := NewDBEventLogger(AllLogCollection)
	if err := logger.LogEvent(event); err != nil {
		grip.Errorf("Error logging host event: %+v", err)
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

func LogHostTerminatedExternally(hostId string) {
	LogHostEvent(hostId, EventHostStatusChanged, HostEventData{NewStatus: EventHostTerminatedExternally})
}

func LogHostStatusChanged(hostId string, oldStatus string, newStatus string) {
	if oldStatus == newStatus {
		return
	}
	LogHostEvent(hostId, EventHostStatusChanged,
		HostEventData{OldStatus: oldStatus, NewStatus: newStatus})
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

// UpdateExecutions updates host events to track multiple executions of the same task
func UpdateExecutions(hostId, taskId string, execution int) error {
	taskIdKey := bsonutil.MustHaveTag(HostEventData{}, "TaskId")
	executionKey := bsonutil.MustHaveTag(HostEventData{}, "Execution")
	query := bson.M{
		"r_id": hostId,
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
