package model

import (
	"10gen.com/mci"
	"10gen.com/mci/db"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"labix.org/v2/mgo"
	"labix.org/v2/mgo/bson"
	"time"
)

const (
	HostsCollection = "hosts"
)

var NoRunningTask = []bson.M{
	bson.M{
		"running_task": "",
	},
	bson.M{
		"running_task": bson.M{
			"$exists": false,
		},
	},
}

type Host struct {
	Id       string `bson:"_id" json:"id"`
	Host     string `bson:"host_id" json:"host"`
	User     string `bson:"user" json:"user"`
	Tag      string `bson:"tag" json:"tag"`
	Distro   string `bson:"distro_id" json:"distro"`
	Provider string `bson:"host_type" json:"host_type"`

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
}

var (
	HostIdKey               = MustHaveBsonTag(Host{}, "Id")
	HostDNSKey              = MustHaveBsonTag(Host{}, "Host")
	HostUserKey             = MustHaveBsonTag(Host{}, "User")
	HostTagKey              = MustHaveBsonTag(Host{}, "Tag")
	HostDistroKey           = MustHaveBsonTag(Host{}, "Distro")
	HostProviderKey         = MustHaveBsonTag(Host{}, "Provider")
	HostProvisionedKey      = MustHaveBsonTag(Host{}, "Provisioned")
	HostRunningTaskKey      = MustHaveBsonTag(Host{}, "RunningTask")
	HostPidKey              = MustHaveBsonTag(Host{}, "Pid")
	HostTaskDispatchTimeKey = MustHaveBsonTag(Host{}, "TaskDispatchTime")
	HostCreateTimeKey       = MustHaveBsonTag(Host{}, "CreationTime")
	HostExpirationTimeKey   = MustHaveBsonTag(Host{}, "ExpirationTime")
	HostTerminationTimeKey  = MustHaveBsonTag(Host{}, "TerminationTime")
	HostLTCTimeKey          = MustHaveBsonTag(Host{}, "LastTaskCompletedTime")
	HostLTCKey              = MustHaveBsonTag(Host{}, "LastTaskCompleted")
	HostStatusKey           = MustHaveBsonTag(Host{}, "Status")
	HostAgentRevisionKey    = MustHaveBsonTag(Host{}, "AgentRevision")
	HostStartedByKey        = MustHaveBsonTag(Host{}, "StartedBy")
	HostInstanceTypeKey     = MustHaveBsonTag(Host{}, "InstanceType")
	HostNotificationsKey    = MustHaveBsonTag(Host{}, "Notifications")
	HostUserDataKey         = MustHaveBsonTag(Host{}, "UserData")
)

/*************************
Find
*************************/

func FindOneHost(query interface{}, projection interface{},
	sort []string) (*Host, error) {
	host := &Host{}
	err := db.FindOne(
		HostsCollection,
		query,
		projection,
		sort,
		host,
	)
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return host, err
}

func FindAllHosts(query interface{}, projection interface{},
	sort []string, skip, limit int) ([]Host, error) {
	hosts := []Host{}
	err := db.FindAll(
		HostsCollection,
		query,
		projection,
		sort,
		skip,
		limit,
		&hosts,
	)
	return hosts, err
}

func FindAllHostsNoFilter() ([]Host, error) {
	return FindAllHosts(
		bson.M{},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindRunningHostsForUser(user string) ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostStartedByKey: user,
			HostStatusKey: bson.M{
				"$ne": mci.HostTerminated,
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindRunningHosts() ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostStatusKey: bson.M{
				"$ne": mci.HostTerminated,
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindLiveHosts() ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostStartedByKey: mci.MCIUser,
			HostStatusKey: bson.M{
				"$in": mci.UphostStatus,
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindUnterminatedHostsForUser(user string) ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostStartedByKey: user,
			HostStatusKey: bson.M{
				"$ne": mci.HostTerminated,
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindAvailableFreeHosts() ([]Host, error) {
	return FindAllHosts(
		bson.M{
			"$or":            NoRunningTask,
			HostStatusKey:    mci.HostRunning,
			HostStartedByKey: mci.MCIUser,
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindFreeHosts() ([]Host, error) {
	return FindAllHosts(
		bson.M{
			"$or":            NoRunningTask,
			HostStartedByKey: mci.MCIUser,
			HostStatusKey: bson.M{
				"$ne": mci.HostTerminated,
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindUnprovisionedHosts(threshold time.Time) ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostProvisionedKey: false,
			HostCreateTimeKey: bson.M{
				"$lte": threshold,
			},
			HostStatusKey: bson.M{
				"$ne": mci.HostTerminated,
			},
			HostStartedByKey: mci.MCIUser,
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindUninitializedHosts() ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostStatusKey:    mci.HostUninitialized,
			HostStartedByKey: mci.MCIUser,
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindUnproductiveHosts(threshold time.Time) ([]Host, error) {
	return FindAllHosts(
		bson.M{
			"$or":      NoRunningTask,
			HostLTCKey: "",
			HostCreateTimeKey: bson.M{
				"$lte": threshold,
			},
			HostStatusKey: bson.M{
				"$ne": mci.HostTerminated,
			},
			HostStartedByKey: mci.MCIUser,
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindHungHosts(threshold time.Time) ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostRunningTaskKey: bson.M{
				"$ne": "",
			},
			HostTaskDispatchTimeKey: bson.M{
				"$lte": threshold,
			},
			HostStatusKey: bson.M{
				"$ne": mci.HostTerminated,
			},
			HostStartedByKey: mci.MCIUser,
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindRunningSpawnedHosts() ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostStartedByKey: bson.M{
				"$ne": mci.MCIUser,
			},
			HostStatusKey: bson.M{
				"$ne": mci.HostTerminated,
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindDecommissionedHosts() ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostRunningTaskKey: "",
			HostStatusKey:      mci.HostDecommissioned,
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindHostsForDistro(distroId string) ([]Host, error) {
	return FindAllHosts(
		bson.M{
			HostDistroKey:    distroId,
			HostStartedByKey: mci.MCIUser,
			HostStatusKey: bson.M{
				"$in": mci.UphostStatus,
			},
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

func FindHost(id string) (*Host, error) {
	return FindOneHost(
		bson.M{
			HostIdKey: id,
		},
		db.NoProjection,
		db.NoSort,
	)
}

func FindHostByRunningTask(taskId string) (*Host, error) {
	return FindOneHost(
		bson.M{
			HostRunningTaskKey: taskId,
		},
		db.NoProjection,
		db.NoSort,
	)
}

// Gets the next task that should be run on the host.
func (self *Host) FindNextTask() (*Task, error) {
	taskQueue, err := FindTaskQueueForDistro(self.Distro)
	if err != nil {
		return nil, err
	}

	if taskQueue == nil || taskQueue.IsEmpty() {
		return nil, nil
	}

	nextTaskId := taskQueue.Queue[0].Id
	fullTask, err := FindTask(nextTaskId)
	if err != nil {
		return nil, err
	}

	return fullTask, nil
}

// CountIdleHosts returns total hosts that have been provisioned but
// aren't running any tasks. Helpful for getting an idea of wasted
// computing power.
func FindIdleHosts() ([]Host, error) {
	return FindAllHosts(
		bson.M{
			"$or":            NoRunningTask,
			HostStatusKey:    mci.HostRunning,
			HostStartedByKey: mci.MCIUser,
		},
		db.NoProjection,
		db.NoSort,
		db.NoSkip,
		db.NoLimit,
	)
}

/*************************
Update
*************************/

func UpdateOneHost(query interface{}, update interface{}) error {
	return db.Update(
		HostsCollection,
		query,
		update,
	)
}

func UpdateAllHosts(query interface{}, update interface{}) error {
	_, err := db.UpdateAll(
		HostsCollection,
		query,
		update,
	)
	return err
}

func UpsertOneHost(query interface{}, update interface{}) (*mgo.ChangeInfo,
	error) {
	return db.Upsert(
		HostsCollection,
		query,
		update,
	)
}

func (self *Host) SetStatus(status string) error {
	if self.Status == mci.HostTerminated {
		msg := fmt.Sprintf("Refusing to mark host %v as"+
			" %v because it is already terminated", self.Id, status)
		mci.Logger.Logf(slogger.WARN, msg)
		return fmt.Errorf(msg)
	}

	LogHostStatusChangedEvent(self.Id, self.Status, status)
	self.Status = status
	return UpdateOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostStatusKey: status,
			},
		},
	)
}

// SetInitializing marks the host as initializing. Only allow this
// if the host is uninitialized.
func (self *Host) SetInitializing() error {
	return UpdateOneHost(
		bson.M{
			HostIdKey:     self.Id,
			HostStatusKey: mci.HostUninitialized,
		},
		bson.M{
			"$set": bson.M{
				HostStatusKey: mci.HostInitializing,
			},
		},
	)
}

func (self *Host) SetDecommissioned() error {
	return self.SetStatus(mci.HostDecommissioned)
}

func (self *Host) SetUninitialized() error {
	return self.SetStatus(mci.HostUninitialized)
}

func (self *Host) SetRunning() error {
	return self.SetStatus(mci.HostRunning)
}

func (self *Host) SetTerminated() error {
	return self.SetStatus(mci.HostTerminated)
}

func (self *Host) SetUnreachable() error {
	return self.SetStatus(mci.HostUnreachable)
}

func (self *Host) SetUnprovisioned() error {
	return UpdateOneHost(
		bson.M{
			HostIdKey:     self.Id,
			HostStatusKey: mci.HostInitializing,
		},
		bson.M{
			"$set": bson.M{
				HostStatusKey: mci.HostProvisionFailed,
			},
		},
	)
}

func (self *Host) SetQuarantined(status string) error {
	return self.SetStatus(mci.HostQuarantined)
}

func (self *Host) Terminate() error {
	err := self.SetTerminated()
	if err != nil {
		return err
	}
	self.TerminationTime = time.Now()
	return UpdateOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostTerminationTimeKey: self.TerminationTime,
			},
		},
	)
}

// SetDNSName updates the DNS name for a given host once
func (self *Host) SetDNSName(dnsName string) error {
	err := UpdateOneHost(
		bson.M{
			HostIdKey:  self.Id,
			HostDNSKey: "",
		},
		bson.M{
			"$set": bson.M{
				HostDNSKey: dnsName,
			},
		},
	)
	if err == nil {
		self.Host = dnsName
		LogHostDNSNameSetEvent(self.Id, dnsName)
	}
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

func (self *Host) MarkAsProvisioned() error {
	LogHostProvisionedEvent(self.Id)

	self.Status = mci.HostRunning
	self.Provisioned = true
	return UpdateOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostStatusKey:      mci.HostRunning,
				HostProvisionedKey: true,
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
		HostIdKey: host.Id,
	}
	update := bson.M{
		"$set": bson.M{
			HostLTCKey:         prevTaskId,
			HostRunningTaskKey: newTaskId,
			HostLTCTimeKey:     finishTime,
			HostPidKey:         "",
		},
	}

	if err = UpdateOneHost(selector, update); err == nil {
		LogHostRunningTaskSetEvent(host.Id, newTaskId)
	}
	return err
}

// Marks that the specified task was started on the host at the specified time.
func (self *Host) SetRunningTask(taskId, agentRevision string,
	taskDispatchTime time.Time) error {

	// log the event
	LogHostRunningTaskSetEvent(self.Id, taskId)

	// update the in-memory host, then the database
	self.RunningTask = taskId
	self.AgentRevision = agentRevision
	self.TaskDispatchTime = taskDispatchTime
	return UpdateOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostRunningTaskKey:      taskId,
				HostAgentRevisionKey:    agentRevision,
				HostTaskDispatchTimeKey: taskDispatchTime,
			},
		},
	)
}

// SetExpirationTime updates the expiration time of a spawn host
func (self *Host) SetExpirationTime(expirationTime time.Time) error {
	// update the in-memory host, then the database
	self.ExpirationTime = expirationTime
	self.Notifications = make(map[string]bool)
	return UpdateOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostExpirationTimeKey: expirationTime,
			},
			"$unset": bson.M{
				HostNotificationsKey: 1,
			},
		},
	)
}

// SetUserData updates the userdata field of a spawn host
func (self *Host) SetUserData(userData string) error {
	// update the in-memory host, then the database
	self.UserData = userData
	return UpdateOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostUserDataKey: userData,
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
	return UpdateOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostNotificationsKey: self.Notifications,
			},
		},
	)
}

func (self *Host) ClearRunningTask() error {
	LogHostRunningTaskClearedEvent(self.Id, self.RunningTask)
	self.RunningTask = ""
	self.TaskDispatchTime = time.Unix(0, 0)
	self.Pid = ""
	return UpdateOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostRunningTaskKey:      "",
				HostTaskDispatchTimeKey: time.Unix(0, 0),
				HostPidKey:              "",
			},
		},
	)
}

func (self *Host) SetTaskPid(pid string) error {
	LogHostTaskPidSetEvent(self.Id, pid)
	return UpdateOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostPidKey: pid,
			},
		},
	)
}

func (self *Host) Upsert() (*mgo.ChangeInfo, error) {
	return UpsertOneHost(
		bson.M{
			HostIdKey: self.Id,
		},
		bson.M{
			"$set": bson.M{
				HostDNSKey:         self.Host,
				HostUserKey:        self.User,
				HostDistroKey:      self.Distro,
				HostProvisionedKey: self.Provisioned,
				HostStartedByKey:   self.StartedBy,
				HostProviderKey:    self.Provider,
			},
			"$setOnInsert": bson.M{
				HostStatusKey: self.Status,
			},
		},
	)
}

// DecommissionInactiveStaticHosts decommissions static hosts
// in the database provided their ids aren't contained in the
// passed in activeStaticHosts slice
func DecommissionInactiveStaticHosts(activeStaticHosts []string) error {
	if activeStaticHosts == nil {
		return nil
	}
	err := UpdateAllHosts(
		bson.M{
			HostIdKey: bson.M{
				"$nin": activeStaticHosts,
			},
			HostProviderKey: mci.HostTypeStatic,
		},
		bson.M{
			"$set": bson.M{
				HostStatusKey: mci.HostDecommissioned,
			},
		},
	)
	if err == mgo.ErrNotFound {
		return nil
	}
	return err
}

// CountIdleHosts returns total hosts that have been provisioned but
// aren't running any tasks. Helpful for getting an idea of wasted
// computing power.
func CountIdleHosts() (int, error) {
	return db.Count(HostsCollection,
		bson.M{
			"$or":            NoRunningTask,
			HostStatusKey:    mci.HostRunning,
			HostStartedByKey: mci.MCIUser,
		})
}

// CountActiveHosts returns a count of existing hosts.
// Useful in reference to a count of idle hosts.
func CountActiveHosts() (int, error) {
	return db.Count(HostsCollection,
		bson.M{
			HostStartedByKey: mci.MCIUser,
			HostStatusKey: bson.M{
				"$nin": []string{
					mci.HostTerminated,
					mci.HostDecommissioned,
					mci.HostInitializing,
				},
			},
		})
}

/*************************
Create
*************************/

func (self *Host) Insert() error {
	LogHostCreatedEvent(self.Id)
	return db.Insert(HostsCollection, self)
}

/*************************
Remove
*************************/

func (self *Host) Remove() error {
	return db.Remove(
		HostsCollection,
		bson.M{
			HostIdKey: self.Id,
		},
	)
}
