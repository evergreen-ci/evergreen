package host

import (
	"errors"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/tychoish/grip"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

type Host struct {
	Id       string        `bson:"_id" json:"id"`
	Host     string        `bson:"host_id" json:"host"`
	User     string        `bson:"user" json:"user"`
	Secret   string        `bson:"secret" json:"secret"`
	Tag      string        `bson:"tag" json:"tag"`
	Distro   distro.Distro `bson:"distro" json:"distro"`
	Provider string        `bson:"host_type" json:"host_type"`

	// true if the host has been set up properly
	Provisioned bool `bson:"provisioned" json:"provisioned"`

	ProvisionOptions *ProvisionOptions `bson:"provision_options,omitempty" json:"provision_options,omitempty"`

	// the task that is currently running on the host
	RunningTask string `bson:"running_task,omitempty" json:"running_task,omitempty"`

	// the pid of the task that is currently running on the host
	Pid string `bson:"pid" json:"pid"`

	// duplicate of the DispatchTime field in the above task
	TaskDispatchTime time.Time `bson:"task_dispatch_time" json:"task_dispatch_time"`
	ExpirationTime   time.Time `bson:"expiration_time,omitempty" json:"expiration_time"`
	CreationTime     time.Time `bson:"creation_time" json:"creation_time"`
	TerminationTime  time.Time `bson:"termination_time" json:"termination_time"`

	LastTaskCompletedTime time.Time `bson:"last_task_completed_time" json:"last_task_completed_time"`
	LastTaskCompleted     string    `bson:"last_task" json:"last_task"`
	LastCommunicationTime time.Time `bson:"last_communication" json:"last_communication"`

	Status    string `bson:"status" json:"status"`
	StartedBy string `bson:"started_by" json:"started_by"`
	// True if this host was created manually by a user (i.e. with spawnhost)
	UserHost      bool   `bson:"user_host" json:"user_host"`
	AgentRevision string `bson:"agent_revision" json:"agent_revision"`
	// for ec2 dynamic hosts, the instance type requested
	InstanceType string `bson:"instance_type" json:"instance_type,omitempty"`
	// stores information on expiration notifications for spawn hosts
	Notifications map[string]bool `bson:"notifications,omitempty" json:"notifications,omitempty"`

	// stores userdata that was placed on the host at spawn time
	UserData string `bson:"userdata" json:"userdata,omitempty"`

	// the last time that the host's reachability was checked
	LastReachabilityCheck time.Time `bson:"last_reachability_check" json:"last_reachability_check"`

	// if set, the time at which the host first became unreachable
	UnreachableSince time.Time `bson:"unreachable_since,omitempty" json:"unreachable_since"`
}

// ProvisionOptions is struct containing options about how a new host should be set up.
type ProvisionOptions struct {
	// LoadCLI indicates (if set) that while provisioning the host, the CLI binary should
	// be placed onto the host after startup.
	LoadCLI bool `bson:"load_cli" json:"load_cli"`

	// TaskId if non-empty will trigger the CLI tool to fetch source and artifacts for the given task.
	// Ignored if LoadCLI is false.
	TaskId string `bson:"task_id" json:"task_id"`

	// Owner is the user associated with the host used to populate any necessary metadata.
	OwnerId string `bson:"owner_id" json:"owner_id"`
}

// IdleTime returns how long has this host been idle
func (h *Host) IdleTime() time.Duration {

	// if the host is currently running a task, it is not idle
	if h.RunningTask != "" {
		return time.Duration(0)
	}

	// if the host has run a task before, then the idle time is just the time
	// passed since the last task finished
	if h.LastTaskCompleted != "" {
		return time.Now().Sub(h.LastTaskCompletedTime)
	}

	// if the host has not run a task before, the idle time is just
	// how long is has been since the host was created
	return time.Now().Sub(h.CreationTime)
}

func (h *Host) SetStatus(status string) error {
	if h.Status == evergreen.HostTerminated {
		msg := fmt.Sprintf("Refusing to mark host %v as"+
			" %v because it is already terminated", h.Id, status)
		grip.Warning(msg)
		return errors.New(msg)
	}

	event.LogHostStatusChanged(h.Id, h.Status, status)

	h.Status = status
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: status,
			},
		},
	)
}

// SetInitializing marks the host as initializing. Only allow this
// if the host is uninitialized.
func (h *Host) SetInitializing() error {
	return UpdateOne(
		bson.M{
			IdKey:     h.Id,
			StatusKey: evergreen.HostUninitialized,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostInitializing,
			},
		},
	)
}

func (h *Host) SetDecommissioned() error {
	return h.SetStatus(evergreen.HostDecommissioned)
}

func (h *Host) SetUninitialized() error {
	return h.SetStatus(evergreen.HostUninitialized)
}

func (h *Host) SetRunning() error {
	return h.SetStatus(evergreen.HostRunning)
}

func (h *Host) SetTerminated() error {
	return h.SetStatus(evergreen.HostTerminated)
}

func (h *Host) SetUnreachable() error {
	return h.SetStatus(evergreen.HostUnreachable)
}

func (h *Host) SetUnprovisioned() error {
	return UpdateOne(
		bson.M{
			IdKey:     h.Id,
			StatusKey: evergreen.HostInitializing,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostProvisionFailed,
			},
		},
	)
}

func (h *Host) SetQuarantined(status string) error {
	return h.SetStatus(evergreen.HostQuarantined)
}

// CreateSecret generates a host secret and updates the host both locally
// and in the database.
func (h *Host) CreateSecret() error {
	secret := util.RandomString()
	err := UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{SecretKey: secret}},
	)
	if err != nil {
		return err
	}
	h.Secret = secret
	return nil
}

// UpdateLastCommunicated sets the host's last communication time to the current time.
func (h *Host) UpdateLastCommunicated() error {
	now := time.Now()
	err := UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{LastCommunicationTimeKey: now}},
	)
	if err != nil {
		return err
	}
	h.LastCommunicationTime = now
	return nil
}

func (h *Host) Terminate() error {
	err := h.SetTerminated()
	if err != nil {
		return err
	}
	h.TerminationTime = time.Now()
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				TerminationTimeKey: h.TerminationTime,
			},
		},
	)
}

// SetDNSName updates the DNS name for a given host once
func (h *Host) SetDNSName(dnsName string) error {
	err := UpdateOne(
		bson.M{
			IdKey:  h.Id,
			DNSKey: "",
		},
		bson.M{
			"$set": bson.M{
				DNSKey: dnsName,
			},
		},
	)
	if err == nil {
		h.Host = dnsName
		event.LogHostDNSNameSet(h.Id, dnsName)
	}
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

func (h *Host) MarkAsProvisioned() error {
	event.LogHostProvisioned(h.Id)
	h.Status = evergreen.HostRunning
	h.Provisioned = true
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey:      evergreen.HostRunning,
				ProvisionedKey: true,
			},
		},
	)
}

// UpdateRunningTask takes two id strings - an old task and a new one - finds
// the host running the task with Id, 'prevTaskId' and updates its running task
// to 'newTaskId'; also setting the completion time of 'prevTaskId'
// Returns true for success and error if it exists
func (host *Host) UpdateRunningTask(prevTaskId, newTaskId string,
	finishTime time.Time) (bool, error) {
	selector := bson.M{
		IdKey: host.Id,
	}
	update := bson.M{
		"$set": bson.M{
			RunningTaskKey: newTaskId,
			LTCKey:         prevTaskId,
			LTCTimeKey:     finishTime,
			PidKey:         "",
		},
	}
	// if the new task id is empty, unset the running task key
	if newTaskId == "" {
		update = bson.M{
			"$set": bson.M{
				LTCKey:     prevTaskId,
				LTCTimeKey: finishTime,
				PidKey:     "",
			},
			"$unset": bson.M{
				RunningTaskKey: 1,
			},
		}
	}

	err := UpdateOne(selector, update)
	if err != nil {
		// if its a duplicate key error, don't log the error.
		if mgo.IsDup(err) {
			return false, nil
		}
		return false, err
	}
	event.LogHostRunningTaskSet(host.Id, newTaskId)

	return true, nil
}

// Marks that the specified task was started on the host at the specified time.
func (h *Host) SetRunningTask(taskId, agentRevision string,
	taskDispatchTime time.Time) error {

	// log the event
	event.LogHostRunningTaskSet(h.Id, taskId)

	// update the in-memory host, then the database
	h.RunningTask = taskId
	h.AgentRevision = agentRevision
	h.TaskDispatchTime = taskDispatchTime

	update := bson.M{
		"$set": bson.M{
			AgentRevisionKey:    agentRevision,
			TaskDispatchTimeKey: taskDispatchTime,
			RunningTaskKey:      taskId,
		},
	}
	// if the task id is empty unset the running task field.
	if taskId == "" {
		update = bson.M{
			"$set": bson.M{
				AgentRevisionKey:    agentRevision,
				TaskDispatchTimeKey: taskDispatchTime,
			},
			"$unset": bson.M{RunningTaskKey: 1},
		}
	}
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		update,
	)
}

// SetExpirationTime updates the expiration time of a spawn host
func (h *Host) SetExpirationTime(expirationTime time.Time) error {
	// update the in-memory host, then the database
	h.ExpirationTime = expirationTime
	h.Notifications = make(map[string]bool)
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				ExpirationTimeKey: expirationTime,
			},
			"$unset": bson.M{
				NotificationsKey: 1,
			},
		},
	)
}

// SetUserData updates the userdata field of a spawn host
func (h *Host) SetUserData(userData string) error {
	// update the in-memory host, then the database
	h.UserData = userData
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				UserDataKey: userData,
			},
		},
	)
}

// SetExpirationNotification updates the notification time for a spawn host
func (h *Host) SetExpirationNotification(thresholdKey string) error {
	// update the in-memory host, then the database
	if h.Notifications == nil {
		h.Notifications = make(map[string]bool)
	}
	h.Notifications[thresholdKey] = true
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				NotificationsKey: h.Notifications,
			},
		},
	)
}

func (h *Host) ClearRunningTask() error {
	event.LogHostRunningTaskCleared(h.Id, h.RunningTask)
	h.RunningTask = ""
	h.TaskDispatchTime = util.ZeroTime
	h.Pid = ""
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$unset": bson.M{
				RunningTaskKey: "",
			},
			"$set": bson.M{
				TaskDispatchTimeKey: util.ZeroTime,
				PidKey:              "",
			},
		},
	)
}

func (h *Host) SetTaskPid(pid string) error {
	event.LogHostTaskPidSet(h.Id, pid)
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				PidKey: pid,
			},
		},
	)
}

// UpdateReachability sets a host as either running or unreachable,
// and updates the timestamp of the host's last reachability check.
// If the host is being set to unreachable, the "unreachable since" field
// is also set to the current time if it is unset.
func (h *Host) UpdateReachability(reachable bool) error {
	status := evergreen.HostRunning
	setUpdate := bson.M{
		StatusKey:                status,
		LastReachabilityCheckKey: time.Now(),
	}

	update := bson.M{}
	if !reachable {
		status = evergreen.HostUnreachable
		setUpdate[StatusKey] = status

		// If the host is being switched to unreachable for the first time, then
		// "unreachable since" will be unset, so we set it to the current time.
		if h.UnreachableSince.Equal(util.ZeroTime) || h.UnreachableSince.Before(util.ZeroTime) {
			now := time.Now()
			setUpdate[UnreachableSinceKey] = now
			h.UnreachableSince = now
		}
	} else {
		// host is reachable, so unset the unreachable_since field
		update["$unset"] = bson.M{UnreachableSinceKey: 1}
		h.UnreachableSince = util.ZeroTime
	}
	update["$set"] = setUpdate

	event.LogHostStatusChanged(h.Id, h.Status, status)

	h.Status = status

	return UpdateOne(bson.M{IdKey: h.Id}, update)
}

func (h *Host) Upsert() (*mgo.ChangeInfo, error) {

	return UpsertOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				DNSKey:         h.Host,
				UserKey:        h.User,
				DistroKey:      h.Distro,
				ProvisionedKey: h.Provisioned,
				StartedByKey:   h.StartedBy,
				ProviderKey:    h.Provider,
			},
			"$setOnInsert": bson.M{
				StatusKey:     h.Status,
				CreateTimeKey: h.CreationTime,
			},
		},
	)
}

func (h *Host) Insert() error {
	event.LogHostCreated(h.Id)
	return db.Insert(Collection, h)
}

func (h *Host) Remove() error {
	return db.Remove(
		Collection,
		bson.M{
			IdKey: h.Id,
		},
	)
}

func DecommissionHostsWithDistroId(distroId string) error {
	err := UpdateAll(
		ByDistroId(distroId),
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostDecommissioned,
			},
		},
	)
	return err
}
