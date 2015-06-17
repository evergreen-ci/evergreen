package host

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
	"time"
)

type Host struct {
	Id       string        `bson:"_id" json:"id"`
	Host     string        `bson:"host_id" json:"host"`
	User     string        `bson:"user" json:"user"`
	Tag      string        `bson:"tag" json:"tag"`
	Distro   distro.Distro `bson:"distro" json:"distro"`
	Provider string        `bson:"host_type" json:"host_type"`

	// true if the host has been set up properly
	Provisioned bool `bson:"provisioned" json:"provisioned"`

	// the task that is currently running on the host
	RunningTask string `bson:"running_task" json:"running_task"`

	// the pid of the task that is currently running on the host
	Pid string `bson:"pid" json:"pid"`

	// duplicate of the DispatchTime field in the above task
	TaskDispatchTime time.Time `bson:"task_dispatch_time" json:"task_dispatch_time"`
	ExpirationTime   time.Time `bson:"expiration_time,omitempty" json:"expiration_time"`
	CreationTime     time.Time `bson:"creation_time" json:"creation_time"`
	TerminationTime  time.Time `bson:"termination_time" json:"termination_time"`

	LastTaskCompletedTime time.Time `bson:"last_task_completed_time" json:"last_task_completed_time"`
	LastTaskCompleted     string    `bson:"last_task" json:"last_task"`
	Status                string    `bson:"status" json:"status"`
	StartedBy             string    `bson:"started_by" json:"started_by"`
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
}

// IdleTime returns how long has this host been idle
func (self *Host) IdleTime() time.Duration {

	// if the host is currently running a task, it is not idle
	if self.RunningTask != "" {
		return time.Duration(0)
	}

	// if the host has run a task before, then the idle time is just the time
	// passed since the last task finished
	if self.LastTaskCompleted != "" {
		return time.Now().Sub(self.LastTaskCompletedTime)
	}

	// if the host has not run a task before, the idle time is just
	// how long is has been since the host was created
	return time.Now().Sub(self.CreationTime)
}

func (self *Host) SetStatus(status string) error {
	if self.Status == evergreen.HostTerminated {
		msg := fmt.Sprintf("Refusing to mark host %v as"+
			" %v because it is already terminated", self.Id, status)
		evergreen.Logger.Logf(slogger.WARN, msg)
		return fmt.Errorf(msg)
	}

	event.LogHostStatusChanged(self.Id, self.Status, status)
	self.Status = status
	return UpdateOne(
		bson.M{
			IdKey: self.Id,
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
func (self *Host) SetInitializing() error {
	return UpdateOne(
		bson.M{
			IdKey:     self.Id,
			StatusKey: evergreen.HostUninitialized,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostInitializing,
			},
		},
	)
}

func (self *Host) SetDecommissioned() error {
	return self.SetStatus(evergreen.HostDecommissioned)
}

func (self *Host) SetUninitialized() error {
	return self.SetStatus(evergreen.HostUninitialized)
}

func (self *Host) SetRunning() error {
	return self.SetStatus(evergreen.HostRunning)
}

func (self *Host) SetTerminated() error {
	return self.SetStatus(evergreen.HostTerminated)
}

func (self *Host) SetUnreachable() error {
	return self.SetStatus(evergreen.HostUnreachable)
}

func (self *Host) SetUnprovisioned() error {
	return UpdateOne(
		bson.M{
			IdKey:     self.Id,
			StatusKey: evergreen.HostInitializing,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostProvisionFailed,
			},
		},
	)
}

func (self *Host) SetQuarantined(status string) error {
	return self.SetStatus(evergreen.HostQuarantined)
}

func (self *Host) Terminate() error {
	err := self.SetTerminated()
	if err != nil {
		return err
	}
	self.TerminationTime = time.Now()
	return UpdateOne(
		bson.M{
			IdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				TerminationTimeKey: self.TerminationTime,
			},
		},
	)
}

// SetDNSName updates the DNS name for a given host once
func (self *Host) SetDNSName(dnsName string) error {
	err := UpdateOne(
		bson.M{
			IdKey:  self.Id,
			DNSKey: "",
		},
		bson.M{
			"$set": bson.M{
				DNSKey: dnsName,
			},
		},
	)
	if err == nil {
		self.Host = dnsName
		event.LogHostDNSNameSet(self.Id, dnsName)
	}
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

func (self *Host) MarkAsProvisioned() error {
	event.LogHostProvisioned(self.Id)

	self.Status = evergreen.HostRunning
	self.Provisioned = true
	return UpdateOne(
		bson.M{
			IdKey: self.Id,
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
func (host *Host) UpdateRunningTask(prevTaskId, newTaskId string,
	finishTime time.Time) (err error) {
	selector := bson.M{
		IdKey: host.Id,
	}
	update := bson.M{
		"$set": bson.M{
			LTCKey:         prevTaskId,
			RunningTaskKey: newTaskId,
			LTCTimeKey:     finishTime,
			PidKey:         "",
		},
	}

	if err = UpdateOne(selector, update); err == nil {
		event.LogHostRunningTaskSet(host.Id, newTaskId)
	}
	return err
}

// Marks that the specified task was started on the host at the specified time.
func (self *Host) SetRunningTask(taskId, agentRevision string,
	taskDispatchTime time.Time) error {

	// log the event
	event.LogHostRunningTaskSet(self.Id, taskId)

	// update the in-memory host, then the database
	self.RunningTask = taskId
	self.AgentRevision = agentRevision
	self.TaskDispatchTime = taskDispatchTime
	return UpdateOne(
		bson.M{
			IdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				RunningTaskKey:      taskId,
				AgentRevisionKey:    agentRevision,
				TaskDispatchTimeKey: taskDispatchTime,
			},
		},
	)
}

// SetExpirationTime updates the expiration time of a spawn host
func (self *Host) SetExpirationTime(expirationTime time.Time) error {
	// update the in-memory host, then the database
	self.ExpirationTime = expirationTime
	self.Notifications = make(map[string]bool)
	return UpdateOne(
		bson.M{
			IdKey: self.Id,
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
func (self *Host) SetUserData(userData string) error {
	// update the in-memory host, then the database
	self.UserData = userData
	return UpdateOne(
		bson.M{
			IdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				UserDataKey: userData,
			},
		},
	)
}

// SetExpirationNotification updates the notification time for a spawn host
func (self *Host) SetExpirationNotification(thresholdKey string) error {
	// update the in-memory host, then the database
	if self.Notifications == nil {
		self.Notifications = make(map[string]bool)
	}
	self.Notifications[thresholdKey] = true
	return UpdateOne(
		bson.M{
			IdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				NotificationsKey: self.Notifications,
			},
		},
	)
}

func (self *Host) ClearRunningTask() error {
	event.LogHostRunningTaskCleared(self.Id, self.RunningTask)
	self.RunningTask = ""
	self.TaskDispatchTime = time.Unix(0, 0)
	self.Pid = ""
	return UpdateOne(
		bson.M{
			IdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				RunningTaskKey:      "",
				TaskDispatchTimeKey: time.Unix(0, 0),
				PidKey:              "",
			},
		},
	)
}

func (self *Host) SetTaskPid(pid string) error {
	event.LogHostTaskPidSet(self.Id, pid)
	return UpdateOne(
		bson.M{
			IdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				PidKey: pid,
			},
		},
	)
}

// UpdateReachability sets a host as either running or unreachable, depending on the bool passed
// in. also update the last reachability check for the host
func (self *Host) UpdateReachability(reachable bool) error {
	status := evergreen.HostRunning
	if !reachable {
		status = evergreen.HostUnreachable
	}

	event.LogHostStatusChanged(self.Id, self.Status, status)
	self.Status = status

	return UpdateOne(
		bson.M{
			IdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				StatusKey:                status,
				LastReachabilityCheckKey: time.Now(),
			},
		},
	)
}

func (self *Host) Upsert() (*mgo.ChangeInfo, error) {
	return UpsertOne(
		bson.M{
			IdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				DNSKey:         self.Host,
				UserKey:        self.User,
				DistroKey:      self.Distro,
				ProvisionedKey: self.Provisioned,
				StartedByKey:   self.StartedBy,
				ProviderKey:    self.Provider,
			},
			"$setOnInsert": bson.M{
				StatusKey: self.Status,
			},
		},
	)
}

func (self *Host) Insert() error {
	event.LogHostCreated(self.Id)
	return db.Insert(Collection, self)
}

func (self *Host) Remove() error {
	return db.Remove(
		Collection,
		bson.M{
			IdKey: self.Id,
		},
	)
}
