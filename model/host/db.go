package host

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/pkg/errors"
	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

const (
	// Collection is the name of the MongoDB collection that stores hosts.
	Collection = "hosts"
)

var (
	IdKey                      = bsonutil.MustHaveTag(Host{}, "Id")
	DNSKey                     = bsonutil.MustHaveTag(Host{}, "Host")
	SecretKey                  = bsonutil.MustHaveTag(Host{}, "Secret")
	UserKey                    = bsonutil.MustHaveTag(Host{}, "User")
	TagKey                     = bsonutil.MustHaveTag(Host{}, "Tag")
	DistroKey                  = bsonutil.MustHaveTag(Host{}, "Distro")
	ProviderKey                = bsonutil.MustHaveTag(Host{}, "Provider")
	ProvisionedKey             = bsonutil.MustHaveTag(Host{}, "Provisioned")
	ProvisionTimeKey           = bsonutil.MustHaveTag(Host{}, "ProvisionTime")
	ExtIdKey                   = bsonutil.MustHaveTag(Host{}, "ExternalIdentifier")
	RunningTaskKey             = bsonutil.MustHaveTag(Host{}, "RunningTask")
	RunningTaskGroupKey        = bsonutil.MustHaveTag(Host{}, "RunningTaskGroup")
	RunningTaskBuildVariantKey = bsonutil.MustHaveTag(Host{}, "RunningTaskBuildVariant")
	RunningTaskVersionKey      = bsonutil.MustHaveTag(Host{}, "RunningTaskVersion")
	RunningTaskProjectKey      = bsonutil.MustHaveTag(Host{}, "RunningTaskProject")
	TaskDispatchTimeKey        = bsonutil.MustHaveTag(Host{}, "TaskDispatchTime")
	CreateTimeKey              = bsonutil.MustHaveTag(Host{}, "CreationTime")
	ExpirationTimeKey          = bsonutil.MustHaveTag(Host{}, "ExpirationTime")
	TerminationTimeKey         = bsonutil.MustHaveTag(Host{}, "TerminationTime")
	LTCTimeKey                 = bsonutil.MustHaveTag(Host{}, "LastTaskCompletedTime")
	LTCTaskKey                 = bsonutil.MustHaveTag(Host{}, "LastTask")
	LTCGroupKey                = bsonutil.MustHaveTag(Host{}, "LastGroup")
	LTCBVKey                   = bsonutil.MustHaveTag(Host{}, "LastBuildVariant")
	LTCVersionKey              = bsonutil.MustHaveTag(Host{}, "LastVersion")
	LTCProjectKey              = bsonutil.MustHaveTag(Host{}, "LastProject")
	StatusKey                  = bsonutil.MustHaveTag(Host{}, "Status")
	AgentRevisionKey           = bsonutil.MustHaveTag(Host{}, "AgentRevision")
	NeedsNewAgentKey           = bsonutil.MustHaveTag(Host{}, "NeedsNewAgent")
	StartedByKey               = bsonutil.MustHaveTag(Host{}, "StartedBy")
	InstanceTypeKey            = bsonutil.MustHaveTag(Host{}, "InstanceType")
	VolumeSizeKey              = bsonutil.MustHaveTag(Host{}, "VolumeTotalSize")
	NotificationsKey           = bsonutil.MustHaveTag(Host{}, "Notifications")
	LastCommunicationTimeKey   = bsonutil.MustHaveTag(Host{}, "LastCommunicationTime")
	UserHostKey                = bsonutil.MustHaveTag(Host{}, "UserHost")
	ZoneKey                    = bsonutil.MustHaveTag(Host{}, "Zone")
	ProjectKey                 = bsonutil.MustHaveTag(Host{}, "Project")
	ProvisionOptionsKey        = bsonutil.MustHaveTag(Host{}, "ProvisionOptions")
	ProvisionAttemptsKey       = bsonutil.MustHaveTag(Host{}, "ProvisionAttempts")
	TaskCountKey               = bsonutil.MustHaveTag(Host{}, "TaskCount")
	StartTimeKey               = bsonutil.MustHaveTag(Host{}, "StartTime")
	TotalCostKey               = bsonutil.MustHaveTag(Host{}, "TotalCost")
	TotalIdleTimeKey           = bsonutil.MustHaveTag(Host{}, "TotalIdleTime")
)

// === Queries ===

// All is a query that returns all hosts
var All = db.Query(nil)

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
		StatusKey:    bson.M{"$in": evergreen.UphostStatus},
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

func AllIdleEphemeral() ([]Host, error) {
	query := db.Query(bson.M{
		RunningTaskKey: bson.M{"$exists": false},
		StartedByKey:   evergreen.User,
		StatusKey:      evergreen.HostRunning,
		ProviderKey:    bson.M{"$in": evergreen.ProviderSpawnable},
	})

	return Find(query)
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

// NeedsProvisioning returns a query used by the hostinit process to
// determine hosts that have been started, but need additional provisioning.
//
// It's likely true that Starting and Initializing are redundant, and
// EVG-2754 will address this issue.
func NeedsProvisioning() db.Q {
	return db.Query(bson.M{StatusKey: bson.M{"$in": []string{
		evergreen.HostStarting,
		evergreen.HostInitializing,
	}}})
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
		StatusKey:    bson.M{"$in": evergreen.UphostStatus},
	}
}

// ByDistroId produces a query that returns all working hosts (not terminated and
// not quarantined) of the given distro.
func ByDistroId(distroId string) db.Q {
	return db.Query(ByDistroIdDoc(distroId))
}

// ById produces a query that returns a host with the given id.
func ById(id string) db.Q {
	return db.Query(bson.D{{Name: IdKey, Value: id}})
}

// ByIds produces a query that returns all hosts in the given list of ids.
func ByIds(ids []string) db.Q {
	return db.Query(bson.D{
		{
			Name: IdKey,
			Value: bson.D{
				{
					Name:  "$in",
					Value: ids,
				},
			},
		},
	})
}

// ByRunningTaskId returns a host running the task with the given id.
func ByRunningTaskId(taskId string) db.Q {
	return db.Query(bson.D{{Name: RunningTaskKey, Value: taskId}})
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

// IsActive is a query that returns all Evergreen hosts that are working or
// capable of being assigned work to do.
var IsActive = db.Query(
	bson.M{
		StartedByKey: evergreen.User,
		StatusKey: bson.M{
			"$nin": []string{
				evergreen.HostTerminated, evergreen.HostDecommissioned, evergreen.HostInitializing,
			},
		},
	},
)

// ByNotMonitoredSince produces a query that returns all hosts whose
// last reachability check was before the specified threshold,
// filtering out user-spawned hosts and hosts currently running tasks.
func ByNotMonitoredSince(threshold time.Time) db.Q {
	return db.Query(bson.M{
		"$and": []bson.M{
			{RunningTaskKey: bson.M{"$exists": false}},
			{StatusKey: bson.M{
				"$in": []string{evergreen.HostRunning, evergreen.HostUnreachable},
			}},
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

// NeedsNewAgent returns hosts that are running and need a new agent, have no Last Commmunication Time,
// or have one that exists that is greater than the MaxLTCInterval duration away from the current time.
func NeedsNewAgent(currentTime time.Time) db.Q {
	cutoffTime := currentTime.Add(-MaxLCTInterval)
	return db.Query(bson.M{
		StatusKey:    evergreen.HostRunning,
		StartedByKey: evergreen.User,
		"$or": []bson.M{
			{LastCommunicationTimeKey: util.ZeroTime},
			{LastCommunicationTimeKey: bson.M{"$lte": cutoffTime}},
			{LastCommunicationTimeKey: bson.M{"$exists": false}},
			{NeedsNewAgentKey: true},
		},
	})
}

// Removes host intents that have been been pending for more than 3
// minutes for the specified distro.
//
// If you pass the empty string as a distroID, it will remove stale
// host intents for *all* distros.
func RemoveStaleInitializing(distroID string) error {
	query := bson.M{
		StatusKey:     evergreen.HostUninitialized,
		UserHostKey:   false,
		CreateTimeKey: bson.M{"$lt": time.Now().Add(-3 * time.Minute)},
		ProviderKey:   bson.M{"$in": evergreen.ProviderSpawnable},
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
	if err == mgo.ErrNotFound {
		return nil, nil
	}
	return host, err
}

func FindOneId(id string) (*Host, error) {
	return FindOne(ById(id))
}

// Find gets all Hosts for the given query.
func Find(query db.Q) ([]Host, error) {
	hosts := []Host{}
	err := db.FindAllQ(Collection, query, &hosts)
	return hosts, err
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
func UpsertOne(query interface{}, update interface{}) (*mgo.ChangeInfo, error) {
	return db.Upsert(
		Collection,
		query,
		update,
	)
}

func GetHostsByFromIdWithStatus(id, status, user string, limit, sortDir int) ([]Host, error) {
	sort := []string{IdKey}
	sortOperator := "$gte"
	if sortDir < 0 {
		sortOperator = "$lte"
		sort = []string{"-" + IdKey}
	}
	var statusMatch interface{}
	if status != "" {
		statusMatch = status
	} else {
		statusMatch = bson.M{"$in": evergreen.UphostStatus}
	}

	filter := bson.M{
		IdKey:     bson.M{sortOperator: id},
		StatusKey: statusMatch,
	}
	if user != "" {
		filter[StartedByKey] = user
	}
	query := db.Q{}
	query = query.Filter(filter)
	query = query.Sort(sort)
	query = query.Limit(limit)

	hosts, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "Error querying database")
	}
	return hosts, nil
}

// QueryWithFullTaskPipeline returns a pipeline to match hosts and embeds the
// task document within the host, if it's running a task
func QueryWithFullTaskPipeline(match bson.M) []bson.M {
	return []bson.M{
		{
			"$match": match,
		},
		{
			"$lookup": bson.M{
				"from":         task.Collection,
				"localField":   RunningTaskKey,
				"foreignField": task.IdKey,
				"as":           "task_full",
			},
		},
		{
			"$unwind": bson.M{
				"path": "$task_full",
				"preserveNullAndEmptyArrays": true,
			},
		},
	}
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
