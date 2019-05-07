package host

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

const (
	// Collection is the name of the MongoDB collection that stores hosts.
	Collection = "hosts"
)

var (
	IdKey                        = bsonutil.MustHaveTag(Host{}, "Id")
	DNSKey                       = bsonutil.MustHaveTag(Host{}, "Host")
	SecretKey                    = bsonutil.MustHaveTag(Host{}, "Secret")
	UserKey                      = bsonutil.MustHaveTag(Host{}, "User")
	TagKey                       = bsonutil.MustHaveTag(Host{}, "Tag")
	DistroKey                    = bsonutil.MustHaveTag(Host{}, "Distro")
	ProviderKey                  = bsonutil.MustHaveTag(Host{}, "Provider")
	IPKey                        = bsonutil.MustHaveTag(Host{}, "IP")
	ProvisionedKey               = bsonutil.MustHaveTag(Host{}, "Provisioned")
	ProvisionTimeKey             = bsonutil.MustHaveTag(Host{}, "ProvisionTime")
	ExtIdKey                     = bsonutil.MustHaveTag(Host{}, "ExternalIdentifier")
	RunningTaskKey               = bsonutil.MustHaveTag(Host{}, "RunningTask")
	RunningTaskGroupKey          = bsonutil.MustHaveTag(Host{}, "RunningTaskGroup")
	RunningTaskBuildVariantKey   = bsonutil.MustHaveTag(Host{}, "RunningTaskBuildVariant")
	RunningTaskVersionKey        = bsonutil.MustHaveTag(Host{}, "RunningTaskVersion")
	RunningTaskProjectKey        = bsonutil.MustHaveTag(Host{}, "RunningTaskProject")
	TaskDispatchTimeKey          = bsonutil.MustHaveTag(Host{}, "TaskDispatchTime")
	CreateTimeKey                = bsonutil.MustHaveTag(Host{}, "CreationTime")
	ExpirationTimeKey            = bsonutil.MustHaveTag(Host{}, "ExpirationTime")
	TerminationTimeKey           = bsonutil.MustHaveTag(Host{}, "TerminationTime")
	LTCTimeKey                   = bsonutil.MustHaveTag(Host{}, "LastTaskCompletedTime")
	LTCTaskKey                   = bsonutil.MustHaveTag(Host{}, "LastTask")
	LTCGroupKey                  = bsonutil.MustHaveTag(Host{}, "LastGroup")
	LTCBVKey                     = bsonutil.MustHaveTag(Host{}, "LastBuildVariant")
	LTCVersionKey                = bsonutil.MustHaveTag(Host{}, "LastVersion")
	LTCProjectKey                = bsonutil.MustHaveTag(Host{}, "LastProject")
	StatusKey                    = bsonutil.MustHaveTag(Host{}, "Status")
	AgentRevisionKey             = bsonutil.MustHaveTag(Host{}, "AgentRevision")
	NeedsNewAgentKey             = bsonutil.MustHaveTag(Host{}, "NeedsNewAgent")
	StartedByKey                 = bsonutil.MustHaveTag(Host{}, "StartedBy")
	InstanceTypeKey              = bsonutil.MustHaveTag(Host{}, "InstanceType")
	VolumeSizeKey                = bsonutil.MustHaveTag(Host{}, "VolumeTotalSize")
	NotificationsKey             = bsonutil.MustHaveTag(Host{}, "Notifications")
	LastCommunicationTimeKey     = bsonutil.MustHaveTag(Host{}, "LastCommunicationTime")
	UserHostKey                  = bsonutil.MustHaveTag(Host{}, "UserHost")
	ZoneKey                      = bsonutil.MustHaveTag(Host{}, "Zone")
	ProjectKey                   = bsonutil.MustHaveTag(Host{}, "Project")
	ProvisionOptionsKey          = bsonutil.MustHaveTag(Host{}, "ProvisionOptions")
	ProvisionAttemptsKey         = bsonutil.MustHaveTag(Host{}, "ProvisionAttempts")
	TaskCountKey                 = bsonutil.MustHaveTag(Host{}, "TaskCount")
	StartTimeKey                 = bsonutil.MustHaveTag(Host{}, "StartTime")
	ComputeCostPerHourKey        = bsonutil.MustHaveTag(Host{}, "ComputeCostPerHour")
	TotalCostKey                 = bsonutil.MustHaveTag(Host{}, "TotalCost")
	TotalIdleTimeKey             = bsonutil.MustHaveTag(Host{}, "TotalIdleTime")
	HasContainersKey             = bsonutil.MustHaveTag(Host{}, "HasContainers")
	ParentIDKey                  = bsonutil.MustHaveTag(Host{}, "ParentID")
	ContainerImagesKey           = bsonutil.MustHaveTag(Host{}, "ContainerImages")
	ContainerBuildAttempt        = bsonutil.MustHaveTag(Host{}, "ContainerBuildAttempt")
	LastContainerFinishTimeKey   = bsonutil.MustHaveTag(Host{}, "LastContainerFinishTime")
	SpawnOptionsKey              = bsonutil.MustHaveTag(Host{}, "SpawnOptions")
	ContainerPoolSettingsKey     = bsonutil.MustHaveTag(Host{}, "ContainerPoolSettings")
	RunningTeardownForTaskKey    = bsonutil.MustHaveTag(Host{}, "RunningTeardownForTask")
	RunningTeardownSinceKey      = bsonutil.MustHaveTag(Host{}, "RunningTeardownSince")
	SpawnOptionsTaskIDKey        = bsonutil.MustHaveTag(SpawnOptions{}, "TaskID")
	SpawnOptionsBuildIDKey       = bsonutil.MustHaveTag(SpawnOptions{}, "BuildID")
	SpawnOptionsTimeoutKey       = bsonutil.MustHaveTag(SpawnOptions{}, "TimeoutTeardown")
	SpawnOptionsSpawnedByTaskKey = bsonutil.MustHaveTag(SpawnOptions{}, "SpawnedByTask")
)

var (
	HostsByDistroDistroIDKey          = bsonutil.MustHaveTag(IdleHostsByDistroID{}, "DistroID")
	HostsByDistroIdleHostsKey         = bsonutil.MustHaveTag(IdleHostsByDistroID{}, "IdleHosts")
	HostsByDistroRunningHostsCountKey = bsonutil.MustHaveTag(IdleHostsByDistroID{}, "RunningHostsCount")
)

// === Queries ===

// All is a query that returns all hosts
var All = db.Query(struct{}{})

// ByUserWithRunningStatus produces a query that returns all
// running hosts for the given user id.
func ByUserWithRunningStatus(user string) db.Q {
	return db.Query(
		bson.M{
			StartedByKey: user,
			StatusKey:    bson.M{"$ne": evergreen.HostTerminated},
		})
}

// IsLive is a query that returns all working hosts started by Evergreen
func IsLive() bson.M {
	return bson.M{
		StartedByKey: evergreen.User,
		StatusKey:    bson.M{"$in": evergreen.UpHostStatus},
	}
}

// ByUserWithUnterminatedStatus produces a query that returns all running hosts
// for the given user id.
func ByUserWithUnterminatedStatus(user string) db.Q {
	return db.Query(
		bson.M{
			StartedByKey: user,
			StatusKey:    bson.M{"$ne": evergreen.HostTerminated},
		},
	)
}

// AllIdleEphemeral finds all running ephemeral hosts without containers
// that have no running tasks.
func AllIdleEphemeral() ([]Host, error) {
	query := db.Query(bson.M{
		RunningTaskKey:   bson.M{"$exists": false},
		StartedByKey:     evergreen.User,
		StatusKey:        evergreen.HostRunning,
		ProviderKey:      bson.M{"$in": evergreen.ProviderSpawnable},
		HasContainersKey: bson.M{"$ne": true},
	})

	return Find(query)
}

// IdleEphemeralGroupedByDistroId groups and collates the following grouped and ordered by {distro.Id: 1}:
// - []host.Host of ephemeral hosts without containers which having no running task, ordered by {host.CreationTime: 1}
// - the total number of ephemeral hosts with status: evergreen.HostRunning
func IdleEphemeralGroupedByDistroId() ([]IdleHostsByDistroID, error) {
	var idlehostsByDistroID []IdleHostsByDistroID
	pipeline := []mgobson.M{
		{
			"$match": mgobson.M{
				StartedByKey:     evergreen.User,
				StatusKey:        evergreen.HostRunning,
				ProviderKey:      mgobson.M{"$in": evergreen.ProviderSpawnable},
				HasContainersKey: mgobson.M{"$ne": true},
			},
		},
		{
			"$sort": mgobson.M{CreateTimeKey: 1},
		},
		{
			"$group": mgobson.M{
				"_id":                             "$" + bsonutil.GetDottedKeyName(DistroKey, distro.IdKey),
				HostsByDistroRunningHostsCountKey: mgobson.M{"$sum": 1},
				HostsByDistroIdleHostsKey:         mgobson.M{"$push": bson.M{"$cond": []interface{}{mgobson.M{"$eq": []interface{}{"$running_task", mgobson.Undefined}}, "$$ROOT", mgobson.Undefined}}},
			},
		},
		{
			"$project": mgobson.M{"_id": 0, HostsByDistroDistroIDKey: "$_id", HostsByDistroIdleHostsKey: 1, HostsByDistroRunningHostsCountKey: 1},
		},
	}

	if err := db.Aggregate(Collection, pipeline, &idlehostsByDistroID); err != nil {
		return nil, errors.Wrap(err, "problem grouping idle hosts by Distro.Id")
	}

	return idlehostsByDistroID, nil
}

func runningHostsQuery(distroID string) bson.M {
	query := IsLive()
	if distroID != "" {
		key := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
		query[key] = distroID
	}

	return query
}

func CountRunningHosts(distroID string) (int, error) {
	num, err := Count(db.Query(runningHostsQuery(distroID)))
	return num, errors.Wrap(err, "problem finding running hosts")
}

func AllRunningHosts(distroID string) ([]Host, error) {
	allHosts, err := Find(db.Query(runningHostsQuery(distroID)))
	if err != nil {
		return nil, errors.Wrap(err, "Error finding live hosts")
	}

	return allHosts, nil
}

// AllHostsSpawnedByTasksToTerminate finds all hosts spawned by tasks that should be terminated.
func AllHostsSpawnedByTasksToTerminate() ([]Host, error) {
	catcher := grip.NewBasicCatcher()
	var hosts []Host
	timedOutHosts, err := allHostsSpawnedByTasksTimedOut()
	hosts = append(hosts, timedOutHosts...)
	catcher.Add(err)

	taskHosts, err := allHostsSpawnedByFinishedTasks()
	hosts = append(hosts, taskHosts...)
	catcher.Add(err)

	buildHosts, err := allHostsSpawnedByFinishedBuilds()
	hosts = append(hosts, buildHosts...)
	catcher.Add(err)

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}
	return hosts, nil
}

// allHostsSpawnedByTasksTimedOut finds hosts spawned by tasks that should be terminated because they are past their timeout.
func allHostsSpawnedByTasksTimedOut() ([]Host, error) {
	query := db.Query(bson.M{
		StatusKey: evergreen.HostRunning,
		bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsSpawnedByTaskKey): true,
		bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsTimeoutKey):       bson.M{"$lte": time.Now()},
	})
	return Find(query)
}

// allHostsSpawnedByFinishedTasks finds hosts spawned by tasks that should be terminated because their tasks have finished.
func allHostsSpawnedByFinishedTasks() ([]Host, error) {
	const runningTasks = "running_tasks"
	pipeline := []bson.M{
		{"$match": bson.M{
			StatusKey: bson.M{"$in": evergreen.UpHostStatus},
			bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsSpawnedByTaskKey): true}},
		{"$lookup": bson.M{
			"from":         task.Collection,
			"localField":   bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsTaskIDKey),
			"foreignField": task.IdKey,
			"as":           runningTasks,
		}},
		{"$unwind": "$" + runningTasks},
		{"$match": bson.M{bsonutil.GetDottedKeyName(runningTasks, task.StatusKey): bson.M{"$in": task.CompletedStatuses}}},
		{"$project": bson.M{runningTasks: 0}},
	}
	var hosts []Host
	if err := db.Aggregate(Collection, pipeline, &hosts); err != nil {
		return nil, errors.Wrap(err, "error getting hosts spawned by finished tasks")
	}
	return hosts, nil
}

// allHostsSpawnedByFinishedBuilds finds hosts spawned by tasks that should be terminated because their builds have finished.
func allHostsSpawnedByFinishedBuilds() ([]Host, error) {
	const runningBuilds = "running_builds"
	pipeline := []bson.M{
		{"$match": bson.M{
			StatusKey: bson.M{"$in": evergreen.UpHostStatus},
			bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsSpawnedByTaskKey): true}},
		{"$lookup": bson.M{
			"from":         build.Collection,
			"localField":   bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsBuildIDKey),
			"foreignField": build.IdKey,
			"as":           runningBuilds,
		}},
		{"$unwind": "$" + runningBuilds},
		{"$match": bson.M{bsonutil.GetDottedKeyName(runningBuilds, build.StatusKey): bson.M{"$in": build.CompletedStatuses}}},
		{"$project": bson.M{runningBuilds: 0}},
	}
	var hosts []Host
	if err := db.Aggregate(Collection, pipeline, &hosts); err != nil {
		return nil, errors.Wrap(err, "error getting hosts spawned by finished builds")
	}
	return hosts, nil
}

// ByUnprovisionedSince produces a query that returns all hosts
// Evergreen never finished setting up that were created before
// the given time.
func ByUnprovisionedSince(threshold time.Time) db.Q {
	return db.Query(bson.M{
		ProvisionedKey: false,
		CreateTimeKey:  bson.M{"$lte": threshold},
		StatusKey:      bson.M{"$ne": evergreen.HostTerminated},
		StartedByKey:   evergreen.User,
	})
}

// ByTaskSpec returns a query that finds all running hosts that are running a
// task with the given group, buildvariant, project, and version.
func NumHostsByTaskSpec(group, bv, project, version string) (int, error) {
	q := db.Query(
		bson.M{
			StatusKey: evergreen.HostRunning,
			"$or": []bson.M{
				{
					RunningTaskKey:             bson.M{"$exists": "true"},
					RunningTaskGroupKey:        group,
					RunningTaskBuildVariantKey: bv,
					RunningTaskProjectKey:      project,
					RunningTaskVersionKey:      version,
				},
				{
					LTCTaskKey:    bson.M{"$exists": "true"},
					LTCGroupKey:   group,
					LTCBVKey:      bv,
					LTCProjectKey: project,
					LTCVersionKey: version,
				},
			},
		},
	)
	hosts, err := Find(q)
	if err != nil {
		return 0, errors.Wrap(err, "error querying database for hosts")
	}
	return len(hosts), nil
}

// IsUninitialized is a query that returns all unstarted + uninitialized Evergreen hosts.
var IsUninitialized = db.Query(
	bson.M{StatusKey: evergreen.HostUninitialized},
)

// Starting returns a query that finds hosts that we do not yet know to be running.
func Starting() db.Q {
	return db.Query(bson.M{StatusKey: evergreen.HostStarting})
}

// Provisioning returns a query used by the hostinit process to determine hosts that are
// started according to the cloud provider, but have not yet been provisioned by Evergreen.
func Provisioning() db.Q {
	return db.Query(bson.M{StatusKey: evergreen.HostProvisioning})
}

func FindByFirstProvisioningAttempt() ([]Host, error) {
	return Find(db.Query(bson.M{
		ProvisionAttemptsKey: 0,
		StatusKey:            evergreen.HostProvisioning,
	}))
}

// IsRunningAndSpawned is a query that returns all running hosts
// spawned by an Evergreen user.
var IsRunningAndSpawned = db.Query(
	bson.M{
		StartedByKey: bson.M{"$ne": evergreen.User},
		StatusKey:    bson.M{"$ne": evergreen.HostTerminated},
	},
)

// IsRunningTask is a query that returns all running hosts with a running task
var IsRunningTask = db.Query(
	bson.M{
		RunningTaskKey: bson.M{"$exists": true},
		StatusKey: bson.M{
			"$ne": evergreen.HostTerminated,
		},
	},
)

// IsTerminated is a query that returns all hosts that are terminated
// (and not running a task).
var IsTerminated = db.Query(
	bson.M{
		RunningTaskKey: bson.M{"$exists": false},
		StatusKey:      evergreen.HostTerminated},
)

func ByDistroIdDoc(distroId string) bson.M {
	dId := fmt.Sprintf("%v.%v", DistroKey, distro.IdKey)
	return bson.M{
		dId:          distroId,
		StartedByKey: evergreen.User,
		StatusKey:    bson.M{"$in": evergreen.UpHostStatus},
	}
}

// ByDistroId produces a query that returns all working hosts (not terminated and
// not quarantined) of the given distro.
func ByDistroId(distroId string) db.Q {
	return db.Query(ByDistroIdDoc(distroId))
}

// ById produces a query that returns a host with the given id.
func ById(id string) db.Q {
	return db.Query(bson.D{{Key: IdKey, Value: id}})
}

// ByIds produces a query that returns all hosts in the given list of ids.
func ByIds(ids []string) db.Q {
	return db.Query(bson.D{
		{
			Key: IdKey,
			Value: bson.D{
				{
					Key:   "$in",
					Value: ids,
				},
			},
		},
	})
}

// ByRunningTaskId returns a host running the task with the given id.
func ByRunningTaskId(taskId string) db.Q {
	return db.Query(bson.D{{Key: RunningTaskKey, Value: taskId}})
}

// ByDynamicWithinTime is a query that returns all dynamic hosts running between a certain time and another time.
func ByDynamicWithinTime(startTime, endTime time.Time) db.Q {
	return db.Query(
		bson.M{
			"$or": []bson.M{
				bson.M{
					CreateTimeKey:      bson.M{"$lt": endTime},
					TerminationTimeKey: bson.M{"$gt": startTime},
					ProviderKey:        bson.M{"$ne": evergreen.HostTypeStatic},
				},
				bson.M{
					CreateTimeKey:      bson.M{"$lt": endTime},
					TerminationTimeKey: util.ZeroTime,
					StatusKey:          evergreen.HostRunning,
					ProviderKey:        bson.M{"$ne": evergreen.HostTypeStatic},
				},
			},
		})
}

var AllStatic = db.Query(
	bson.M{
		ProviderKey: evergreen.HostTypeStatic,
	})

// IsIdle is a query that returns all running Evergreen hosts with no task.
var IsIdle = db.Query(
	bson.M{
		RunningTaskKey: bson.M{"$exists": false},
		StatusKey:      evergreen.HostRunning,
		StartedByKey:   evergreen.User,
	},
)

// ByNotMonitoredSince produces a query that returns all hosts whose
// last reachability check was before the specified threshold,
// filtering out user-spawned hosts and hosts currently running tasks.
func ByNotMonitoredSince(threshold time.Time) db.Q {
	return db.Query(bson.M{
		"$and": []bson.M{
			{RunningTaskKey: bson.M{"$exists": false}},
			{StatusKey: evergreen.HostRunning},
			{StartedByKey: evergreen.User},
			{"$or": []bson.M{
				{LastCommunicationTimeKey: bson.M{"$lte": threshold}},
				{LastCommunicationTimeKey: bson.M{"$exists": false}},
			}},
		},
	})
}

// ByExpiringBetween produces a query that returns  any user-spawned hosts
// that will expire between the specified times.
func ByExpiringBetween(lowerBound time.Time, upperBound time.Time) db.Q {
	return db.Query(bson.M{
		StartedByKey: bson.M{"$ne": evergreen.User},
		StatusKey: bson.M{
			"$nin": []string{evergreen.HostTerminated, evergreen.HostQuarantined},
		},
		ExpirationTimeKey: bson.M{"$gte": lowerBound, "$lte": upperBound},
	})
}

// StateRunningTasks returns tasks documents that are currently run by a host and stale
func FindStaleRunningTasks(cutoff time.Duration) ([]task.Task, error) {
	pipeline := []bson.M{}
	pipeline = append(pipeline, bson.M{
		"$match": bson.M{
			RunningTaskKey: bson.M{
				"$exists": true,
			},
			StatusKey: bson.M{
				"$in": evergreen.UpHostStatus,
			},
		},
	})
	pipeline = append(pipeline, bson.M{
		"$lookup": bson.M{
			"from":         task.Collection,
			"localField":   RunningTaskKey,
			"foreignField": task.IdKey,
			"as":           "_task",
		},
	})
	pipeline = append(pipeline, bson.M{
		"$project": bson.M{
			"_task": 1,
			"_id":   0,
		},
	})
	pipeline = append(pipeline, bson.M{
		"$replaceRoot": bson.M{
			"newRoot": bson.M{
				"$mergeObjects": []interface{}{
					bson.M{"$arrayElemAt": []interface{}{"$_task", 0}},
					"$$ROOT",
				},
			},
		},
	})
	pipeline = append(pipeline, bson.M{
		"$project": bson.M{
			"_task": 0,
		},
	})
	pipeline = append(pipeline, bson.M{
		"$match": bson.M{
			"$or": []bson.M{
				{
					task.StatusKey:        task.SelectorTaskInProgress,
					task.LastHeartbeatKey: bson.M{"$lte": time.Now().Add(-cutoff)},
				},
				{
					task.StatusKey:        evergreen.TaskUndispatched,
					task.LastHeartbeatKey: bson.M{"$lte": time.Now().Add(-cutoff)},
					task.LastHeartbeatKey: bson.M{"$ne": util.ZeroTime},
				},
			},
		},
	})
	pipeline = append(pipeline, bson.M{
		"$project": bson.M{
			task.IdKey:        1,
			task.ExecutionKey: 1,
		},
	})

	tasks := []task.Task{}
	err := db.Aggregate(Collection, pipeline, &tasks)
	if err != nil {
		return nil, errors.Wrap(err, "error finding stale running tasks")
	}
	return tasks, nil
}

// LastCommunicationTimeElapsed returns hosts which have never communicated or have not communicated in too long.
func LastCommunicationTimeElapsed(currentTime time.Time) bson.M {
	cutoffTime := currentTime.Add(-MaxLCTInterval)
	return bson.M{
		StatusKey:        evergreen.HostRunning,
		StartedByKey:     evergreen.User,
		HasContainersKey: bson.M{"$ne": true},
		ParentIDKey:      bson.M{"$exists": false},
		RunningTaskKey:   bson.M{"$exists": false},
		"$or": []bson.M{
			{LastCommunicationTimeKey: util.ZeroTime},
			{LastCommunicationTimeKey: bson.M{"$lte": cutoffTime}},
			{LastCommunicationTimeKey: bson.M{"$exists": false}},
		},
	}
}

// NeedsNewAgentFlagSet returns hosts with NeedsNewAgent set to true.
func NeedsNewAgentFlagSet() db.Q {
	return db.Query(bson.M{
		StatusKey:        evergreen.HostRunning,
		StartedByKey:     evergreen.User,
		HasContainersKey: bson.M{"$ne": true},
		ParentIDKey:      bson.M{"$exists": false},
		RunningTaskKey:   bson.M{"$exists": false},
		NeedsNewAgentKey: true,
	})
}

// Removes host intents that have been been uninitialized for more than 3
// minutes or spawning (but not started) for more than 15 minutes for the
// specified distro.
//
// If you pass the empty string as a distroID, it will remove stale
// host intents for *all* distros.
func RemoveStaleInitializing(distroID string) error {
	query := bson.M{
		UserHostKey: false,
		ProviderKey: bson.M{"$in": evergreen.ProviderSpawnable},
		"$or": []bson.M{
			{
				StatusKey:     evergreen.HostUninitialized,
				CreateTimeKey: bson.M{"$lt": time.Now().Add(-3 * time.Minute)},
			},
			{
				StatusKey:     evergreen.HostBuilding,
				CreateTimeKey: bson.M{"$lt": time.Now().Add(-15 * time.Minute)},
			},
		},
	}

	if distroID != "" {
		key := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
		query[key] = distroID
	}

	return db.RemoveAll(Collection, query)
}

// === DB Logic ===

// FindOne gets one Host for the given query.
func FindOne(query db.Q) (*Host, error) {
	host := &Host{}
	err := db.FindOneQ(Collection, query, host)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return host, err
}

func FindOneId(id string) (*Host, error) {
	return FindOne(ById(id))
}

// FindOneByIdOrTag finds a host where the given id is stored in either the _id or tag field.
// (The tag field is used for the id from the host's original intent host.)
func FindOneByIdOrTag(id string) (*Host, error) {
	query := db.Query(bson.M{
		"$or": []bson.M{
			bson.M{TagKey: id},
			bson.M{IdKey: id},
		},
	})
	host, err := FindOne(query) // try to find by tag
	if err != nil {
		return nil, errors.Wrap(err, "error finding '%s' by _id or tag field")
	}
	return host, nil
}

// Find gets all Hosts for the given query.
func Find(query db.Q) ([]Host, error) {
	hosts := []Host{}
	return hosts, errors.WithStack(db.FindAllQ(Collection, query, &hosts))
}

// Count returns the number of hosts that satisfy the given query.
func Count(query db.Q) (int, error) {
	return db.CountQ(Collection, query)
}

// UpdateOne updates one host.
func UpdateOne(query interface{}, update interface{}) error {
	return db.Update(
		Collection,
		query,
		update,
	)
}

// UpdateAll updates all hosts.
func UpdateAll(query interface{}, update interface{}) error {
	_, err := db.UpdateAll(
		Collection,
		query,
		update,
	)
	return err
}

// UpsertOne upserts a host.
func UpsertOne(query interface{}, update interface{}) (*adb.ChangeInfo, error) {
	return db.Upsert(
		Collection,
		query,
		update,
	)
}

func GetHostsByFromIDWithStatus(id, status, user string, limit int) ([]Host, error) {
	var statusMatch interface{}
	if status != "" {
		statusMatch = status
	} else {
		statusMatch = bson.M{"$in": evergreen.UpHostStatus}
	}

	filter := bson.M{
		IdKey:     bson.M{"$gte": id},
		StatusKey: statusMatch,
	}

	if user != "" {
		filter[StartedByKey] = user
	}

	var query db.Q
	hosts, err := Find(query.Filter(filter).Sort([]string{IdKey}).Limit(limit))
	if err != nil {
		return nil, errors.Wrap(err, "Error querying database")
	}
	return hosts, nil
}

type InactiveHostCounts struct {
	HostType string `bson:"_id"`
	Count    int    `bson:"count"`
}

func inactiveHostCountPipeline() []bson.M {
	return []bson.M{
		{
			"$match": bson.M{
				StatusKey: bson.M{
					"$in": []string{evergreen.HostDecommissioned, evergreen.HostQuarantined},
				},
			},
		},
		{
			"$project": bson.M{
				IdKey:       0,
				StatusKey:   1,
				ProviderKey: 1,
			},
		},
		{
			"$group": bson.M{
				"_id": "$" + ProviderKey,
				"count": bson.M{
					"$sum": 1,
				},
			},
		},
	}
}

// FinishTime is a struct for storing pairs of host IDs and last container finish times
type FinishTime struct {
	Id         string    `bson:"_id"`
	FinishTime time.Time `bson:"finish_time"`
}

// aggregation pipeline to compute latest finish time for running hosts with child containers
func lastContainerFinishTimePipeline() []bson.M {
	const output string = "finish_time"
	return []bson.M{
		{
			// matches all running containers
			"$match": bson.M{
				ParentIDKey: bson.M{"$exists": true},
				StatusKey:   evergreen.HostRunning,
			},
		},
		{
			// joins hosts and tasks collections on task ID
			"$lookup": bson.M{
				"from":         task.Collection,
				"localField":   RunningTaskKey,
				"foreignField": IdKey,
				"as":           "task",
			},
		},
		{
			// deconstructs $lookup array
			"$unwind": "$task",
		},
		{
			// groups containers by parent host ID
			"$group": bson.M{
				"_id": "$" + ParentIDKey,
				output: bson.M{
					// computes last container finish time for each host
					"$max": bson.M{
						"$add": []interface{}{bsonutil.GetDottedKeyName("$task", "start_time"),
							// divide by 1000000 to treat duration as milliseconds rather than as nanoseconds
							bson.M{"$divide": []interface{}{bsonutil.GetDottedKeyName("$task", "duration_prediction", "value"), 1000000}},
						},
					},
				},
			},
		},
		{
			// projects only ID and finish time
			"$project": bson.M{
				output: 1,
			},
		},
	}
}

// AggregateLastContainerFinishTimes returns the latest finish time for each host with containers
func AggregateLastContainerFinishTimes() ([]FinishTime, error) {

	var times []FinishTime
	err := db.Aggregate(Collection, lastContainerFinishTimePipeline(), &times)
	if err != nil {
		return nil, errors.Wrap(err, "error aggregating parent finish times")
	}
	return times, nil

}
