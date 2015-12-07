package event

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"time"
)

const (
	// resource type
	ResourceTypeHost = "HOST"

	// event types
	EventHostCreated            = "HOST_CREATED"
	EventHostStatusChanged      = "HOST_STATUS_CHANGED"
	EventHostDNSNameSet         = "HOST_DNS_NAME_SET"
	EventHostProvisionFailed    = "HOST_PROVISION_FAILED"
	EventHostProvisioned        = "HOST_PROVISIONED"
	EventHostRunningTaskSet     = "HOST_RUNNING_TASK_SET"
	EventHostRunningTaskCleared = "HOST_RUNNING_TASK_CLEARED"
	EventHostTaskPidSet         = "HOST_TASK_PID_SET"
	EventHostMonitorFlag        = "HOST_MONITOR_FLAG"
)

// implements EventData
type HostEventData struct {
	// necessary for IsValid
	ResourceType string `bson:"r_type" json:"resource_type"`

	OldStatus string `bson:"o_s,omitempty" json:"old_status,omitempty"`
	NewStatus string `bson:"n_s,omitempty" json:"new_status,omitempty"`
	SetupLog  string `bson:"log,omitempty" json:"setup_log,omitempty"`
	Hostname  string `bson:"hn,omitempty" json:"hostname,omitempty"`
	TaskId    string `bson:"t_id,omitempty" json:"task_id,omitempty"`
	TaskPid   string `bson:"t_pid,omitempty" json:"task_pid,omitempty"`
	MonitorOp string `bson:"monitor_op,omitempty" json:"monitor,omitempty"`
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

	logger := NewDBEventLogger(Collection)
	if err := logger.LogEvent(event); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error logging host event: %v", err)
	}
}

func LogHostCreated(hostId string) {
	LogHostEvent(hostId, EventHostCreated, HostEventData{})
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

func LogProvisionFailed(hostId string, setupLog string) {
	LogHostEvent(hostId, EventHostProvisionFailed, HostEventData{SetupLog: setupLog})
}

func LogMonitorOperation(hostId string, op string) {
	LogHostEvent(hostId, EventHostMonitorFlag, HostEventData{MonitorOp: op})
}
