package host

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const (
	// Collection is the name of the MongoDB collection that stores hosts.
	Collection        = "hosts"
	VolumesCollection = "volumes"
)

var (
	IdKey                                  = bsonutil.MustHaveTag(Host{}, "Id")
	DNSKey                                 = bsonutil.MustHaveTag(Host{}, "Host")
	SecretKey                              = bsonutil.MustHaveTag(Host{}, "Secret")
	UserKey                                = bsonutil.MustHaveTag(Host{}, "User")
	ServicePasswordKey                     = bsonutil.MustHaveTag(Host{}, "ServicePassword")
	TagKey                                 = bsonutil.MustHaveTag(Host{}, "Tag")
	DistroKey                              = bsonutil.MustHaveTag(Host{}, "Distro")
	ProviderKey                            = bsonutil.MustHaveTag(Host{}, "Provider")
	IPKey                                  = bsonutil.MustHaveTag(Host{}, "IP")
	IPv4Key                                = bsonutil.MustHaveTag(Host{}, "IPv4")
	PersistentDNSNameKey                   = bsonutil.MustHaveTag(Host{}, "PersistentDNSName")
	PublicIPv4Key                          = bsonutil.MustHaveTag(Host{}, "PublicIPv4")
	ProvisionedKey                         = bsonutil.MustHaveTag(Host{}, "Provisioned")
	ProvisionTimeKey                       = bsonutil.MustHaveTag(Host{}, "ProvisionTime")
	ExtIdKey                               = bsonutil.MustHaveTag(Host{}, "ExternalIdentifier")
	DisplayNameKey                         = bsonutil.MustHaveTag(Host{}, "DisplayName")
	RunningTaskFullKey                     = bsonutil.MustHaveTag(Host{}, "RunningTaskFull")
	RunningTaskKey                         = bsonutil.MustHaveTag(Host{}, "RunningTask")
	RunningTaskExecutionKey                = bsonutil.MustHaveTag(Host{}, "RunningTaskExecution")
	RunningTaskGroupKey                    = bsonutil.MustHaveTag(Host{}, "RunningTaskGroup")
	RunningTaskGroupOrderKey               = bsonutil.MustHaveTag(Host{}, "RunningTaskGroupOrder")
	TaskGroupTeardownStartTimeKey          = bsonutil.MustHaveTag(Host{}, "TaskGroupTeardownStartTime")
	RunningTaskBuildVariantKey             = bsonutil.MustHaveTag(Host{}, "RunningTaskBuildVariant")
	RunningTaskVersionKey                  = bsonutil.MustHaveTag(Host{}, "RunningTaskVersion")
	RunningTaskProjectKey                  = bsonutil.MustHaveTag(Host{}, "RunningTaskProject")
	CreateTimeKey                          = bsonutil.MustHaveTag(Host{}, "CreationTime")
	ExpirationTimeKey                      = bsonutil.MustHaveTag(Host{}, "ExpirationTime")
	NoExpirationKey                        = bsonutil.MustHaveTag(Host{}, "NoExpiration")
	TerminationTimeKey                     = bsonutil.MustHaveTag(Host{}, "TerminationTime")
	LTCTimeKey                             = bsonutil.MustHaveTag(Host{}, "LastTaskCompletedTime")
	LTCTaskKey                             = bsonutil.MustHaveTag(Host{}, "LastTask")
	LTCGroupKey                            = bsonutil.MustHaveTag(Host{}, "LastGroup")
	LTCBVKey                               = bsonutil.MustHaveTag(Host{}, "LastBuildVariant")
	LTCVersionKey                          = bsonutil.MustHaveTag(Host{}, "LastVersion")
	LTCProjectKey                          = bsonutil.MustHaveTag(Host{}, "LastProject")
	StatusKey                              = bsonutil.MustHaveTag(Host{}, "Status")
	AgentRevisionKey                       = bsonutil.MustHaveTag(Host{}, "AgentRevision")
	NeedsNewAgentKey                       = bsonutil.MustHaveTag(Host{}, "NeedsNewAgent")
	NeedsNewAgentMonitorKey                = bsonutil.MustHaveTag(Host{}, "NeedsNewAgentMonitor")
	NumAgentCleanupFailuresKey             = bsonutil.MustHaveTag(Host{}, "NumAgentCleanupFailures")
	JasperCredentialsIDKey                 = bsonutil.MustHaveTag(Host{}, "JasperCredentialsID")
	NeedsReprovisionKey                    = bsonutil.MustHaveTag(Host{}, "NeedsReprovision")
	StartedByKey                           = bsonutil.MustHaveTag(Host{}, "StartedBy")
	InstanceTypeKey                        = bsonutil.MustHaveTag(Host{}, "InstanceType")
	VolumesKey                             = bsonutil.MustHaveTag(Host{}, "Volumes")
	LastCommunicationTimeKey               = bsonutil.MustHaveTag(Host{}, "LastCommunicationTime")
	UserHostKey                            = bsonutil.MustHaveTag(Host{}, "UserHost")
	ZoneKey                                = bsonutil.MustHaveTag(Host{}, "Zone")
	ProvisionOptionsKey                    = bsonutil.MustHaveTag(Host{}, "ProvisionOptions")
	TaskCountKey                           = bsonutil.MustHaveTag(Host{}, "TaskCount")
	StartTimeKey                           = bsonutil.MustHaveTag(Host{}, "StartTime")
	BillingStartTimeKey                    = bsonutil.MustHaveTag(Host{}, "BillingStartTime")
	AgentStartTimeKey                      = bsonutil.MustHaveTag(Host{}, "AgentStartTime")
	TotalIdleTimeKey                       = bsonutil.MustHaveTag(Host{}, "TotalIdleTime")
	HasContainersKey                       = bsonutil.MustHaveTag(Host{}, "HasContainers")
	ParentIDKey                            = bsonutil.MustHaveTag(Host{}, "ParentID")
	DockerOptionsKey                       = bsonutil.MustHaveTag(Host{}, "DockerOptions")
	ContainerImagesKey                     = bsonutil.MustHaveTag(Host{}, "ContainerImages")
	ContainerBuildAttemptKey               = bsonutil.MustHaveTag(Host{}, "ContainerBuildAttempt")
	LastContainerFinishTimeKey             = bsonutil.MustHaveTag(Host{}, "LastContainerFinishTime")
	SpawnOptionsKey                        = bsonutil.MustHaveTag(Host{}, "SpawnOptions")
	ContainerPoolSettingsKey               = bsonutil.MustHaveTag(Host{}, "ContainerPoolSettings")
	InstanceTagsKey                        = bsonutil.MustHaveTag(Host{}, "InstanceTags")
	SSHKeyNamesKey                         = bsonutil.MustHaveTag(Host{}, "SSHKeyNames")
	SSHPortKey                             = bsonutil.MustHaveTag(Host{}, "SSHPort")
	HomeVolumeIDKey                        = bsonutil.MustHaveTag(Host{}, "HomeVolumeID")
	PortBindingsKey                        = bsonutil.MustHaveTag(Host{}, "PortBindings")
	IsVirtualWorkstationKey                = bsonutil.MustHaveTag(Host{}, "IsVirtualWorkstation")
	SleepScheduleKey                       = bsonutil.MustHaveTag(Host{}, "SleepSchedule")
	SpawnOptionsTaskIDKey                  = bsonutil.MustHaveTag(SpawnOptions{}, "TaskID")
	SpawnOptionsTaskExecutionNumberKey     = bsonutil.MustHaveTag(SpawnOptions{}, "TaskExecutionNumber")
	SpawnOptionsBuildIDKey                 = bsonutil.MustHaveTag(SpawnOptions{}, "BuildID")
	SpawnOptionsTimeoutKey                 = bsonutil.MustHaveTag(SpawnOptions{}, "TimeoutTeardown")
	SpawnOptionsSpawnedByTaskKey           = bsonutil.MustHaveTag(SpawnOptions{}, "SpawnedByTask")
	VolumeIDKey                            = bsonutil.MustHaveTag(Volume{}, "ID")
	VolumeDisplayNameKey                   = bsonutil.MustHaveTag(Volume{}, "DisplayName")
	VolumeCreatedByKey                     = bsonutil.MustHaveTag(Volume{}, "CreatedBy")
	VolumeTypeKey                          = bsonutil.MustHaveTag(Volume{}, "Type")
	VolumeSizeKey                          = bsonutil.MustHaveTag(Volume{}, "Size")
	VolumeExpirationKey                    = bsonutil.MustHaveTag(Volume{}, "Expiration")
	VolumeNoExpirationKey                  = bsonutil.MustHaveTag(Volume{}, "NoExpiration")
	VolumeHostKey                          = bsonutil.MustHaveTag(Volume{}, "Host")
	VolumeMigratingKey                     = bsonutil.MustHaveTag(Volume{}, "Migrating")
	VolumeAttachmentIDKey                  = bsonutil.MustHaveTag(VolumeAttachment{}, "VolumeID")
	VolumeDeviceNameKey                    = bsonutil.MustHaveTag(VolumeAttachment{}, "DeviceName")
	DockerOptionsStdinDataKey              = bsonutil.MustHaveTag(DockerOptions{}, "StdinData")
	SleepScheduleNextStopTimeKey           = bsonutil.MustHaveTag(SleepScheduleInfo{}, "NextStopTime")
	SleepScheduleNextStartTimeKey          = bsonutil.MustHaveTag(SleepScheduleInfo{}, "NextStartTime")
	SleepSchedulePermanentlyExemptKey      = bsonutil.MustHaveTag(SleepScheduleInfo{}, "PermanentlyExempt")
	SleepScheduleTemporarilyExemptUntilKey = bsonutil.MustHaveTag(SleepScheduleInfo{}, "TemporarilyExemptUntil")
	SleepScheduleShouldKeepOffKey          = bsonutil.MustHaveTag(SleepScheduleInfo{}, "ShouldKeepOff")
)

var (
	HostsByDistroDistroIDKey          = bsonutil.MustHaveTag(IdleHostsByDistroID{}, "DistroID")
	HostsByDistroIdleHostsKey         = bsonutil.MustHaveTag(IdleHostsByDistroID{}, "IdleHosts")
	HostsByDistroRunningHostsCountKey = bsonutil.MustHaveTag(IdleHostsByDistroID{}, "RunningHostsCount")
)

// === Queries ===

// All is a query that returns all hosts
var All = bson.M{}

// ByUserWithRunningStatus produces a query that returns all
// running hosts for the given user id.
func ByUserWithRunningStatus(user string) bson.M {
	return bson.M{
		StartedByKey: user,
		StatusKey:    bson.M{"$ne": evergreen.HostTerminated},
	}
}

// ByUserRecentlyTerminated produces a query that returns all
// terminated hosts whose TerminationTimeKey is after the given
// timestamp.
func ByUserRecentlyTerminated(user string, timestamp time.Time) bson.M {
	return bson.M{
		StartedByKey:       user,
		StatusKey:          evergreen.HostTerminated,
		TerminationTimeKey: bson.M{"$gt": timestamp},
	}
}

// byActiveForTasks returns a query that finds all task hosts that are active in
// the cloud provider.
func byActiveForTasks() bson.M {
	return bson.M{
		StartedByKey: evergreen.User,
		StatusKey:    bson.M{"$in": evergreen.ActiveStatuses},
	}
}

// byCanOrWillRunTasks returns a query that finds all task hosts that are
// currently able to or will eventually be able to run tasks.
func byCanOrWillRunTasks() bson.M {
	return bson.M{
		StartedByKey: evergreen.User,
		StatusKey:    bson.M{"$in": evergreen.IsRunningOrWillRunStatuses},
	}
}

// ByUserWithUnterminatedStatus produces a query that returns all running hosts
// for the given user id.
func ByUserWithUnterminatedStatus(user string) bson.M {
	return bson.M{
		StartedByKey: user,
		StatusKey:    bson.M{"$ne": evergreen.HostTerminated},
	}
}

// IdleEphemeralGroupedByDistroID groups and collates the following by distro.Id:
// - []host.Host of ephemeral hosts without containers which having no running task, ordered by {host.CreationTime: 1}
// - the total number of ephemeral hosts that are capable of running tasks
func IdleEphemeralGroupedByDistroID(ctx context.Context, env evergreen.Environment) ([]IdleHostsByDistroID, error) {
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	pipeline := []bson.M{
		{
			"$match": bson.M{
				StartedByKey:     evergreen.User,
				ProviderKey:      bson.M{"$in": evergreen.ProviderSpawnable},
				HasContainersKey: bson.M{"$ne": true},
				"$or": []bson.M{
					{
						StatusKey: evergreen.HostRunning,
					},
					{
						StatusKey:    evergreen.HostStarting,
						bootstrapKey: distro.BootstrapMethodUserData,
						// Idle time starts from when a host is first able
						// to run a task. For user data hosts this is the time
						// when they first made contact with the app instead
						// of when they're marked running.
						AgentStartTimeKey: bson.M{"$gt": utility.ZeroTime},
					},
				},
			},
		},
		{
			"$sort": bson.M{CreateTimeKey: 1},
		},
		{
			"$group": bson.M{
				"_id":                             "$" + bsonutil.GetDottedKeyName(DistroKey, distro.IdKey),
				HostsByDistroRunningHostsCountKey: bson.M{"$sum": 1},
				HostsByDistroIdleHostsKey:         bson.M{"$push": bson.M{"$cond": []interface{}{bson.M{"$eq": []interface{}{"$running_task", primitive.Undefined{}}}, "$$ROOT", primitive.Undefined{}}}},
			},
		},
		{
			"$project": bson.M{"_id": 0, HostsByDistroDistroIDKey: "$_id", HostsByDistroIdleHostsKey: 1, HostsByDistroRunningHostsCountKey: 1},
		},
	}

	cur, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline, options.Aggregate().SetHint(StartedByStatusIndex))
	if err != nil {
		return nil, errors.Wrap(err, "grouping idle hosts by distro ID")
	}
	var idlehostsByDistroID []IdleHostsByDistroID
	if err = cur.All(ctx, &idlehostsByDistroID); err != nil {
		return nil, errors.Wrap(err, "reading grouped idle hosts by distro ID")
	}

	return idlehostsByDistroID, nil
}

// hostsCanRunTasksQuery produces a query that returns all hosts
// that are capable of accepting and running tasks.
func hostsCanRunTasksQuery(distroID string) bson.M {
	distroIDKey := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)

	// Yes this query looks weird but it's a temporary stop gap to ensure we are able to avoid a MongoDB
	// query planner issue. This query is meant to be a temporary fix until we can update to a newer version of
	// MongoDB that does not have this bug. https://github.com/evergreen-ci/evergreen/pull/8010
	// TODO: https://jira.mongodb.org/browse/DEVPROD-8360
	return bson.M{
		"$or": []bson.M{
			{
				distroIDKey:  distroID,
				StartedByKey: evergreen.User,
				StatusKey:    evergreen.HostRunning,
			},
			{
				distroIDKey:  distroID,
				StartedByKey: evergreen.User,
				StatusKey:    evergreen.HostStarting,
				bootstrapKey: distro.BootstrapMethodUserData,
			},
		},
	}

}

func idleStartedTaskHostsQuery(distroID string) bson.M {
	query := bson.M{
		StatusKey:      bson.M{"$in": evergreen.StartedHostStatus},
		StartedByKey:   evergreen.User,
		RunningTaskKey: bson.M{"$exists": false},
	}
	if distroID != "" {
		query[bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)] = distroID
	}
	return query
}

func idleHostsQuery(distroID string) bson.M {
	query := bson.M{
		StartedByKey:     evergreen.User,
		ProviderKey:      bson.M{"$in": evergreen.ProviderSpawnable},
		RunningTaskKey:   bson.M{"$exists": false},
		HasContainersKey: bson.M{"$ne": true},
		StatusKey:        evergreen.HostRunning,
	}
	if distroID != "" {
		query[bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)] = distroID
	}
	return query
}

// CountActiveDynamicHosts counts the number of task hosts that are active in
// the cloud provider and can be created dynamically.
func CountActiveDynamicHosts(ctx context.Context) (int, error) {
	query := byActiveForTasks()
	query[ProviderKey] = bson.M{"$in": evergreen.ProviderSpawnable}
	num, err := Count(ctx, query)
	return num, errors.Wrap(err, "counting active dynamic task hosts")
}

// CountActiveDynamicHosts counts the number of task hosts that are active in
// the cloud provider in a particular distro.
func CountActiveHostsInDistro(ctx context.Context, distroID string) (int, error) {
	query := byActiveForTasks()
	query[bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)] = distroID
	num, err := Count(ctx, query)
	return num, errors.Wrap(err, "counting active task hosts in distro")
}

// CountHostsCanRunTasks returns the number of hosts that can accept
// and run tasks for a given distro. This number is surfaced on the
// task queue.
func CountHostsCanRunTasks(ctx context.Context, distroID string) (int, error) {
	num, err := Count(ctx, hostsCanRunTasksQuery(distroID))
	return num, errors.Wrap(err, "counting hosts that can run tasks")
}

// CountHostsCanOrWillRunTasksInDistro counts all task hosts in a distro that
// can run or will eventually run tasks.
func CountHostsCanOrWillRunTasksInDistro(ctx context.Context, distroID string) (int, error) {
	q := byCanOrWillRunTasks()
	q[bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)] = distroID
	return Count(ctx, q)
}

// CountIdleStartedTaskHosts returns the count of task hosts that are starting
// and not currently running a task.
func CountIdleStartedTaskHosts(ctx context.Context) (int, error) {
	num, err := Count(ctx, idleStartedTaskHostsQuery(""))
	return num, errors.Wrap(err, "counting starting hosts")
}

// IdleHostsWithDistroID, given a distroID, returns a slice of all idle hosts in that distro
func IdleHostsWithDistroID(ctx context.Context, distroID string) ([]Host, error) {
	q := idleHostsQuery(distroID)
	idleHosts, err := Find(ctx, q)
	if err != nil {
		return nil, errors.Wrap(err, "finding idle hosts")
	}
	return idleHosts, nil
}

// AllActiveHosts produces a HostGroup for all hosts with UpHost
// status as well as quarantined hosts. These do not count spawn
// hosts.
func AllActiveHosts(ctx context.Context, distroID string) (HostGroup, error) {
	q := bson.M{
		StartedByKey: evergreen.User,
		StatusKey:    bson.M{"$in": append(evergreen.UpHostStatus, evergreen.HostQuarantined)},
	}

	if distroID != "" {
		q[bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)] = distroID
	}

	activeHosts, err := Find(ctx, q)
	if err != nil {
		return nil, errors.Wrap(err, "finding active hosts")
	}
	return activeHosts, nil
}

// AllHostsSpawnedByTasksToTerminate finds all hosts spawned by tasks that should be terminated.
func AllHostsSpawnedByTasksToTerminate(ctx context.Context) ([]Host, error) {
	catcher := grip.NewBasicCatcher()
	var hosts []Host
	timedOutHosts, err := allHostsSpawnedByTasksTimedOut(ctx)
	hosts = append(hosts, timedOutHosts...)
	catcher.Wrap(err, "finding hosts that have hit their timeout")

	taskHosts, err := allHostsSpawnedByFinishedTasks(ctx)
	hosts = append(hosts, taskHosts...)
	catcher.Wrap(err, "finding hosts whose tasks have finished")

	buildHosts, err := allHostsSpawnedByFinishedBuilds(ctx)
	hosts = append(hosts, buildHosts...)
	catcher.Wrap(err, "finding hosts whose builds have finished")

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}
	return hosts, nil
}

// allHostsSpawnedByTasksTimedOut finds hosts spawned by tasks that should be terminated because they are past their timeout.
func allHostsSpawnedByTasksTimedOut(ctx context.Context) ([]Host, error) {
	query := bson.M{
		StatusKey: evergreen.HostRunning,
		bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsSpawnedByTaskKey): true,
		bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsTimeoutKey):       bson.M{"$lte": time.Now()},
	}
	return Find(ctx, query)
}

// allHostsSpawnedByFinishedTasks finds hosts spawned by tasks that should be terminated because their tasks have finished.
func allHostsSpawnedByFinishedTasks(ctx context.Context) ([]Host, error) {
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
		{"$match": bson.M{
			"$or": []bson.M{
				// If the task is finished, then its host should be terminated.
				{
					bsonutil.GetDottedKeyName(runningTasks, task.StatusKey): bson.M{"$in": evergreen.TaskCompletedStatuses},
				},

				// If the task execution number is greater than the host's task
				// execution number, than the host belongs to an old task and should
				// be terminated.
				{
					"$expr": bson.M{
						"$gt": []string{
							"$" + bsonutil.GetDottedKeyName(runningTasks, task.ExecutionKey),
							"$" + bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsTaskExecutionNumberKey),
						},
					},
				},
			},
		}},
		{"$project": bson.M{runningTasks: 0}},
	}

	return Aggregate(ctx, pipeline)
}

// allHostsSpawnedByFinishedBuilds finds hosts spawned by tasks that should be terminated because their builds have finished.
func allHostsSpawnedByFinishedBuilds(ctx context.Context) ([]Host, error) {
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

	return Aggregate(ctx, pipeline)
}

// ByTaskSpec returns a query that finds all running hosts that are running a
// task with the given group, buildvariant, project, and version.
func ByTaskSpec(group, buildVariant, project, version string) bson.M {
	return bson.M{
		StatusKey: bson.M{"$in": []string{evergreen.HostStarting, evergreen.HostRunning}},
		"$or": []bson.M{
			{
				RunningTaskKey:             bson.M{"$exists": "true"},
				RunningTaskGroupKey:        group,
				RunningTaskBuildVariantKey: buildVariant,
				RunningTaskProjectKey:      project,
				RunningTaskVersionKey:      version,
			},
			{
				LTCTaskKey:    bson.M{"$exists": "true"},
				LTCGroupKey:   group,
				LTCBVKey:      buildVariant,
				LTCProjectKey: project,
				LTCVersionKey: version,
			},
		},
	}
}

// NumHostsByTaskSpec returns the number of running hosts that are running a task with
// the given group, buildvariant, project, and version.
func NumHostsByTaskSpec(ctx context.Context, group, buildVariant, project, version string) (int, error) {
	if group == "" || buildVariant == "" || project == "" || version == "" {
		return 0, errors.Errorf("all arguments must be non-empty strings: (group is '%s', build variant is '%s', "+
			"project is '%s' and version is '%s')", group, buildVariant, project, version)
	}

	numHosts, err := Count(ctx, ByTaskSpec(group, buildVariant, project, version))
	if err != nil {
		return 0, errors.Wrap(err, "counting hosts by task spec")
	}

	return numHosts, nil
}

// MinTaskGroupOrderRunningByTaskSpec returns the smallest task group order number for tasks with the
// given group, buildvariant, project, and version that are running on hosts.
// Returns 0 in the case of missing task group order numbers or no hosts.
func MinTaskGroupOrderRunningByTaskSpec(ctx context.Context, group, buildVariant, project, version string) (int, error) {
	if group == "" || buildVariant == "" || project == "" || version == "" {
		return 0, errors.Errorf("all arguments must be non-empty strings: (group is '%s', build variant is '%s', "+
			"project is '%s' and version is '%s')", group, buildVariant, project, version)
	}

	hosts, err := Find(ctx,
		ByTaskSpec(group, buildVariant, project, version),
		options.Find().
			SetProjection(bson.M{RunningTaskGroupOrderKey: 1}).
			SetSort(bson.M{RunningTaskGroupOrderKey: 1}),
	)
	if err != nil {
		return 0, errors.Wrap(err, "finding hosts by task spec with running task group order")
	}
	minTaskGroupOrder := 0
	//  can look at only one host because we sorted
	if len(hosts) > 0 {
		minTaskGroupOrder = hosts[0].RunningTaskGroupOrder
	}
	return minTaskGroupOrder, nil
}

// IsUninitialized is a query that returns all unstarted + uninitialized Evergreen hosts.
var IsUninitialized = bson.M{StatusKey: evergreen.HostUninitialized}

// FindByProvisioning finds all hosts that are not yet provisioned by the app
// server.
func FindByProvisioning(ctx context.Context) ([]Host, error) {
	return Find(ctx, bson.M{
		StatusKey:           evergreen.HostProvisioning,
		NeedsReprovisionKey: bson.M{"$exists": false},
		ProvisionedKey:      false,
	})
}

// FindByShouldConvertProvisioning finds all hosts that are ready and waiting to
// convert their provisioning type.
func FindByShouldConvertProvisioning(ctx context.Context) ([]Host, error) {
	return Find(ctx, bson.M{
		StatusKey:           bson.M{"$in": []string{evergreen.HostProvisioning, evergreen.HostRunning}},
		StartedByKey:        evergreen.User,
		RunningTaskKey:      bson.M{"$exists": false},
		HasContainersKey:    bson.M{"$ne": true},
		ParentIDKey:         bson.M{"$exists": false},
		NeedsReprovisionKey: bson.M{"$in": []ReprovisionType{ReprovisionToNew, ReprovisionToLegacy}},
		"$or": []bson.M{
			{NeedsNewAgentKey: true},
			{NeedsNewAgentMonitorKey: true},
		},
	})
}

// FindByNeedsToRestartJasper finds all hosts that are ready and waiting to
// restart their Jasper service.
func FindByNeedsToRestartJasper(ctx context.Context) ([]Host, error) {
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	return Find(ctx, bson.M{
		StatusKey:           bson.M{"$in": []string{evergreen.HostProvisioning, evergreen.HostRunning}},
		bootstrapKey:        bson.M{"$in": []string{distro.BootstrapMethodSSH, distro.BootstrapMethodUserData}},
		RunningTaskKey:      bson.M{"$exists": false},
		HasContainersKey:    bson.M{"$ne": true},
		ParentIDKey:         bson.M{"$exists": false},
		NeedsReprovisionKey: ReprovisionRestartJasper,
		"$or": []bson.M{
			{"$and": []bson.M{
				{StartedByKey: bson.M{"$ne": evergreen.User}},
				{UserHostKey: true},
			}},
			{NeedsNewAgentMonitorKey: true},
		},
	})
}

// IsRunningTask is a query that returns all running hosts with a running task
var IsRunningTask = bson.M{
	RunningTaskKey: bson.M{"$exists": true},
	StatusKey: bson.M{
		"$ne": evergreen.HostTerminated,
	},
}

// IsTerminated is a query that returns all hosts that are terminated
// (and not running a task).
var IsTerminated = bson.M{
	RunningTaskKey: bson.M{"$exists": false},
	StatusKey:      evergreen.HostTerminated,
}

// ByDistroIDs produces a query that returns all up hosts of the given distros.
func ByDistroIDs(distroIDs ...string) bson.M {
	distroIDKey := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
	return bson.M{
		distroIDKey:  bson.M{"$in": distroIDs},
		StartedByKey: evergreen.User,
		StatusKey:    bson.M{"$in": evergreen.UpHostStatus},
	}
}

// ById produces a query that returns a host with the given id.
func ById(id string) bson.M {
	return bson.M{IdKey: id}
}

// ByIPAndRunning produces a query that returns a running host with the given ip address.
func ByIPAndRunning(ip string) bson.M {
	return bson.M{
		"$or": []bson.M{
			{IPKey: ip},
			{IPv4Key: ip},
		},
		StatusKey: evergreen.HostRunning,
	}
}

// ByDistroIDOrAliasesRunning returns a query that returns all hosts with
// matching distro IDs or aliases.
func ByDistroIDsOrAliasesRunning(distroNames ...string) bson.M {
	distroIDKey := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
	distroAliasesKey := bsonutil.GetDottedKeyName(DistroKey, distro.AliasesKey)
	return bson.M{
		StatusKey: evergreen.HostRunning,
		"$or": []bson.M{
			{distroIDKey: bson.M{"$in": distroNames}},
			{distroAliasesKey: bson.M{"$in": distroNames}},
		},
	}
}

// ByIds produces a query that returns all hosts in the given list of ids.
func ByIds(ids []string) bson.M {
	return bson.M{IdKey: bson.M{"$in": ids}}
}

// IsIdle is a query that returns all running Evergreen hosts with no task.
var IsIdle = bson.M{
	RunningTaskKey: bson.M{"$exists": false},
	StatusKey:      evergreen.HostRunning,
	StartedByKey:   evergreen.User,
}

// ByNotMonitoredSince produces a query that returns all hosts whose
// last reachability check was before the specified threshold,
// filtering out user-spawned hosts and hosts currently running tasks.
func ByNotMonitoredSince(threshold time.Time) bson.M {
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	return bson.M{
		"$and": []bson.M{
			{RunningTaskKey: bson.M{"$exists": false}},
			{StartedByKey: evergreen.User},
			{"$and": []bson.M{
				{"$or": []bson.M{
					{StatusKey: evergreen.HostRunning},
					// Hosts provisioned with user data which have not started
					// the agent yet may be externally terminated without our
					// knowledge, so they should be monitored.
					{
						StatusKey:      evergreen.HostStarting,
						ProvisionedKey: true,
						bootstrapKey:   distro.BootstrapMethodUserData,
					},
				}},
				{"$or": []bson.M{
					{LastCommunicationTimeKey: bson.M{"$lte": threshold}},
					{LastCommunicationTimeKey: bson.M{"$exists": false}},
				}},
			}},
		},
	}
}

// ByExpiringBetween produces a query that returns any host not running tasks
// that will expire between the specified times.
func ByExpiringBetween(lowerBound time.Time, upperBound time.Time) bson.M {
	return bson.M{
		StartedByKey: bson.M{"$ne": evergreen.User},
		StatusKey: bson.M{
			"$nin": []string{evergreen.HostTerminated, evergreen.HostQuarantined},
		},
		ExpirationTimeKey: bson.M{"$gte": lowerBound, "$lte": upperBound},
	}
}

// FindByTemporaryExemptionsExpiringBetween finds all spawn hosts with a
// temporary exemption from the sleep schedule that will expire between the
// specified times.
func FindByTemporaryExemptionsExpiringBetween(ctx context.Context, lowerBound time.Time, upperBound time.Time) ([]Host, error) {
	sleepScheduleTemporarilyExemptUntilKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleTemporarilyExemptUntilKey)
	return Find(ctx, isSleepScheduleApplicable(bson.M{
		StatusKey:                              bson.M{"$in": evergreen.SleepScheduleStatuses},
		sleepScheduleTemporarilyExemptUntilKey: bson.M{"$gte": lowerBound, "$lte": upperBound},
	}))
}

// NeedsAgentDeploy finds hosts which need the agent to be deployed because
// either they do not have an agent yet or their agents have not communicated
// recently.
func NeedsAgentDeploy(currentTime time.Time) bson.M {
	cutoffTime := currentTime.Add(-MaxAgentUnresponsiveInterval)
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	return bson.M{
		StartedByKey:     evergreen.User,
		HasContainersKey: bson.M{"$ne": true},
		ParentIDKey:      bson.M{"$exists": false},
		RunningTaskKey:   bson.M{"$exists": false},
		bootstrapKey:     distro.BootstrapMethodLegacySSH,
		"$and": []bson.M{
			{"$or": []bson.M{
				{StatusKey: evergreen.HostRunning},
				{"$and": []bson.M{
					{StatusKey: evergreen.HostProvisioning},
					{NeedsReprovisionKey: bson.M{"$exists": true, "$ne": ""}},
				}},
			}},
			{"$or": []bson.M{
				{LastCommunicationTimeKey: utility.ZeroTime},
				{LastCommunicationTimeKey: bson.M{"$lte": cutoffTime}},
				{LastCommunicationTimeKey: bson.M{"$exists": false}},
			}},
		},
	}
}

// NeedsAgentMonitorDeploy finds hosts which do not have an agent monitor yet or
// which should have an agent monitor but their agent has not communicated
// recently.
func NeedsAgentMonitorDeploy(currentTime time.Time) bson.M {
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	return bson.M{
		StartedByKey:     evergreen.User,
		HasContainersKey: bson.M{"$ne": true},
		ParentIDKey:      bson.M{"$exists": false},
		RunningTaskKey:   bson.M{"$exists": false},
		"$and": []bson.M{
			{"$or": []bson.M{
				{StatusKey: evergreen.HostRunning},
				{"$and": []bson.M{
					{StatusKey: evergreen.HostProvisioning},
					{NeedsReprovisionKey: bson.M{"$exists": true, "$ne": ""}},
				}},
			}},
			{"$or": []bson.M{
				{LastCommunicationTimeKey: utility.ZeroTime},
				{LastCommunicationTimeKey: bson.M{"$lte": currentTime.Add(-MaxAgentMonitorUnresponsiveInterval)}},
				{LastCommunicationTimeKey: bson.M{"$exists": false}},
			}},
		},
		bootstrapKey: bson.M{"$in": []string{distro.BootstrapMethodSSH, distro.BootstrapMethodUserData}},
	}
}

// ShouldDeployAgent returns legacy hosts with NeedsNewAgent set to true and are
// in a state in which they can deploy agents.
func ShouldDeployAgent() bson.M {
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	return bson.M{
		bootstrapKey:        distro.BootstrapMethodLegacySSH,
		StatusKey:           evergreen.HostRunning,
		StartedByKey:        evergreen.User,
		HasContainersKey:    bson.M{"$ne": true},
		ParentIDKey:         bson.M{"$exists": false},
		RunningTaskKey:      bson.M{"$exists": false},
		NeedsNewAgentKey:    true,
		NeedsReprovisionKey: bson.M{"$exists": false},
	}
}

// ShouldDeployAgentMonitor returns running hosts that need a new agent
// monitor.
func ShouldDeployAgentMonitor() bson.M {
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	return bson.M{
		bootstrapKey:            bson.M{"$in": []string{distro.BootstrapMethodSSH, distro.BootstrapMethodUserData}},
		StatusKey:               evergreen.HostRunning,
		StartedByKey:            evergreen.User,
		HasContainersKey:        bson.M{"$ne": true},
		ParentIDKey:             bson.M{"$exists": false},
		RunningTaskKey:          bson.M{"$exists": false},
		NeedsNewAgentMonitorKey: true,
		NeedsReprovisionKey:     bson.M{"$exists": false},
	}
}

// FindUserDataSpawnHostsProvisioning finds all spawn hosts that have been
// provisioned by the app server but are still being provisioned by user data.
func FindUserDataSpawnHostsProvisioning(ctx context.Context) ([]Host, error) {
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	provisioningCutoff := time.Now().Add(-30 * time.Minute)

	hosts, err := Find(ctx, bson.M{
		StatusKey:      evergreen.HostStarting,
		ProvisionedKey: true,
		// Ignore hosts that have failed to provision within the cutoff.
		ProvisionTimeKey: bson.M{"$gte": provisioningCutoff},
		StartedByKey:     bson.M{"$ne": evergreen.User},
		bootstrapKey:     distro.BootstrapMethodUserData,
	})
	if err != nil {
		return nil, errors.Wrap(err, "finding user data spawn hosts that are still provisioning themselves")
	}
	return hosts, nil
}

// Removes host intents that have been initializing for more than 3 minutes.
//
// If you pass the empty string as a distroID, it will remove stale
// host intents for *all* distros.
func RemoveStaleInitializing(ctx context.Context, distroID string) error {
	query := bson.M{
		UserHostKey: false,
		bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsSpawnedByTaskKey): bson.M{"$ne": true},
		ProviderKey:   bson.M{"$in": evergreen.ProviderSpawnable},
		StatusKey:     evergreen.HostUninitialized,
		CreateTimeKey: bson.M{"$lt": time.Now().Add(-3 * time.Minute)},
	}

	if distroID != "" {
		key := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
		query[key] = distroID
	}

	return DeleteMany(ctx, query)
}

// MarkStaleBuildingAsFailed marks building hosts that have been stuck building
// for too long as failed in order to indicate that they're stale and should be
// terminated.
func MarkStaleBuildingAsFailed(ctx context.Context, distroID string) error {
	distroIDKey := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
	spawnedByTaskKey := bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsSpawnedByTaskKey)
	query := bson.M{
		distroIDKey:      distroID,
		spawnedByTaskKey: bson.M{"$ne": true},
		ProviderKey:      bson.M{"$in": evergreen.ProviderSpawnable},
		StatusKey:        evergreen.HostBuilding,
		CreateTimeKey:    bson.M{"$lt": time.Now().Add(-15 * time.Minute)},
	}

	if distroID != "" {
		key := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
		query[key] = distroID
	}

	hosts, err := Find(ctx, query, options.Find().SetProjection(bson.M{IdKey: 1}))
	if err != nil {
		return errors.WithStack(err)
	}
	var ids []string
	for _, h := range hosts {
		ids = append(ids, h.Id)
	}
	if len(ids) == 0 {
		return nil
	}

	if err := UpdateAll(ctx, ByIds(ids), bson.M{
		"$set": bson.M{StatusKey: evergreen.HostBuildingFailed},
	}); err != nil {
		return errors.Wrap(err, "marking stale building hosts as failed")
	}

	for _, id := range ids {
		event.LogHostCreatedError(id, "stale building host took too long to start")
		grip.Info(message.Fields{
			"message": "stale building host took too long to start",
			"host_id": id,
			"distro":  distroID,
		})
	}

	return nil
}

// === DB Logic ===

// FindOne gets one Host for the given query.
func FindOne(ctx context.Context, query bson.M, options ...*options.FindOneOptions) (*Host, error) {
	res := evergreen.GetEnvironment().DB().Collection(Collection).FindOne(ctx, query, options...)
	if err := res.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, errors.Wrap(err, "finding host")
	}

	host := &Host{}
	if err := res.Decode(host); err != nil {
		return nil, errors.Wrap(err, "decoding host")
	}

	return host, nil
}

func FindOneId(ctx context.Context, id string) (*Host, error) {
	return FindOne(ctx, ById(id))
}

// FindOneByTaskIdAndExecution returns a single host with the given running task ID and execution.
func FindOneByTaskIdAndExecution(ctx context.Context, id string, execution int) (*Host, error) {
	query := bson.M{
		RunningTaskKey:          id,
		RunningTaskExecutionKey: execution,
	}
	return FindOne(ctx, query)
}

// FindOneByIdOrTag finds a host where the given id is stored in either the _id or tag field.
// (The tag field is used for the id from the host's original intent host.)
func FindOneByIdOrTag(ctx context.Context, id string) (*Host, error) {
	query := bson.M{
		"$or": []bson.M{
			{TagKey: id},
			{IdKey: id},
		},
	}
	host, err := FindOne(ctx, query)
	if err != nil {
		return nil, errors.Wrapf(err, "finding host with ID or tag '%s'", id)
	}
	return host, nil
}

func Find(ctx context.Context, query bson.M, options ...*options.FindOptions) ([]Host, error) {
	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Find(ctx, query, options...)
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts")
	}
	var hosts []Host
	if err = cur.All(ctx, &hosts); err != nil {
		return nil, errors.Wrap(err, "decoding hosts")
	}

	return hosts, nil
}

// Aggregate performs the aggregation pipeline on the host collection and returns the resulting hosts.
// Implement the aggregation directly if the result of the pipeline is not an array of hosts.
func Aggregate(ctx context.Context, pipeline []bson.M, options ...*options.AggregateOptions) ([]Host, error) {
	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Aggregate(ctx, pipeline, options...)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating hosts")
	}
	var hosts []Host
	if err = cur.All(ctx, &hosts); err != nil {
		return nil, errors.Wrap(err, "decoding hosts")
	}

	return hosts, nil
}

// Count returns the number of hosts that satisfy the given query.
func Count(ctx context.Context, query bson.M, opts ...*options.CountOptions) (int, error) {
	res, err := evergreen.GetEnvironment().DB().Collection(Collection).CountDocuments(ctx, query, opts...)
	return int(res), errors.Wrap(err, "getting host count")
}

// UpdateOne updates one host.
func UpdateOne(ctx context.Context, query bson.M, update bson.M) error {
	res, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateOne(ctx, query, update)
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return adb.ErrNotFound
	}

	return nil
}

// UpdateAll updates all hosts.
func UpdateAll(ctx context.Context, query bson.M, update bson.M) error {
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateMany(ctx, query, update)
	return errors.Wrap(err, "updating hosts")
}

// InsertOne inserts the host into the hosts collection.
func InsertOne(ctx context.Context, h *Host, options ...*options.InsertOneOptions) error {
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertOne(ctx, h)
	return errors.Wrap(err, "inserting host")
}

// InsertMany inserts the hosts into the hosts collection.
func InsertMany(ctx context.Context, hosts []Host, options ...*options.InsertManyOptions) error {
	if len(hosts) == 0 {
		return nil
	}

	docs := make([]interface{}, len(hosts))
	for idx := range hosts {
		docs[idx] = &hosts[idx]
	}
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).InsertMany(ctx, docs)
	return errors.Wrap(err, "inserting hosts")
}

// UpsertOne upserts a host.
func UpsertOne(ctx context.Context, query bson.M, update bson.M) (*mongo.UpdateResult, error) {
	return evergreen.GetEnvironment().DB().Collection(Collection).UpdateOne(ctx, query, update, options.Update().SetUpsert(true))
}

// DeleteOne removes a single host matching the filter from the hosts collection.
func DeleteOne(ctx context.Context, filter bson.M, options ...*options.DeleteOptions) error {
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).DeleteOne(ctx, filter, options...)
	return errors.Wrap(err, "deleting host")
}

// DeleteMany removes all hosts matching the filter from the hosts collection.
func DeleteMany(ctx context.Context, filter bson.M, options ...*options.DeleteOptions) error {
	_, err := evergreen.GetEnvironment().DB().Collection(Collection).DeleteMany(ctx, filter, options...)
	return errors.Wrap(err, "deleting hosts")
}

func GetHostsByFromIDWithStatus(ctx context.Context, id, status, user string, limit int) ([]Host, error) {
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

	hosts, err := Find(ctx, filter, options.Find().SetSort(bson.M{IdKey: 1}).SetLimit(int64(limit)))
	if err != nil {
		return nil, errors.Wrapf(err, "finding hosts with an ID of '%s' or greater, status '%s', and user '%s'", id, status, user)
	}
	return hosts, nil
}

type HostsInRangeParams struct {
	CreatedBefore time.Time
	CreatedAfter  time.Time
	User          string
	Distro        string
	Status        string
	Region        string
	UserSpawned   bool
}

// FindHostsInRange is a method to find a filtered list of hosts
func FindHostsInRange(ctx context.Context, params HostsInRangeParams) ([]Host, error) {
	var statusMatch interface{}
	if params.Status != "" {
		statusMatch = params.Status
	} else {
		statusMatch = bson.M{"$in": evergreen.UpHostStatus}
	}

	createTimeFilter := bson.M{"$gt": params.CreatedAfter}
	if !utility.IsZeroTime(params.CreatedBefore) {
		createTimeFilter["$lt"] = params.CreatedBefore
	}

	filter := bson.M{
		StatusKey:     statusMatch,
		CreateTimeKey: createTimeFilter,
	}

	if params.User != "" {
		filter[StartedByKey] = params.User
	}
	if params.Distro != "" {
		filter[bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)] = params.Distro
	}

	if params.UserSpawned {
		filter[UserHostKey] = true
	}

	if params.Region != "" {
		filter[bsonutil.GetDottedKeyName(DistroKey, distro.ProviderSettingsListKey, awsRegionKey)] = params.Region
	}
	hosts, err := Find(ctx, filter)
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts by filters")
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
func AggregateLastContainerFinishTimes(ctx context.Context) ([]FinishTime, error) {
	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Aggregate(ctx, lastContainerFinishTimePipeline())
	if err != nil {
		return nil, errors.Wrap(err, "aggregating parent finish times")
	}
	var times []FinishTime
	if err = cur.All(ctx, &times); err != nil {
		return nil, errors.Wrap(err, "decoding finish times")
	}

	return times, nil
}

func (h *Host) SetVolumes(ctx context.Context, volumes []VolumeAttachment) error {
	err := UpdateOne(
		ctx,
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				VolumesKey: volumes,
			},
		})
	if err != nil {
		return errors.Wrap(err, "updating host volumes")
	}
	h.Volumes = volumes
	return nil
}

func (h *Host) AddVolumeToHost(ctx context.Context, newVolume *VolumeAttachment) error {
	res := evergreen.GetEnvironment().DB().Collection(Collection).FindOneAndUpdate(ctx,
		bson.M{IdKey: h.Id},
		bson.M{
			"$push": bson.M{
				VolumesKey: newVolume,
			},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	if err := res.Err(); err != nil {
		return errors.Wrap(err, "finding host and adding volume")
	}
	if err := res.Decode(&h); err != nil {
		return errors.Wrap(err, "decoding host")
	}

	grip.Error(message.WrapError((&Volume{ID: newVolume.VolumeID}).SetHost(h.Id),
		message.Fields{
			"host_id":   h.Id,
			"volume_id": newVolume.VolumeID,
			"op":        "host volume accounting",
			"message":   "problem setting host info on volume records",
		}))

	return nil
}

func (h *Host) RemoveVolumeFromHost(ctx context.Context, volumeId string) error {
	res := evergreen.GetEnvironment().DB().Collection(Collection).FindOneAndUpdate(ctx,
		bson.M{IdKey: h.Id},
		bson.M{
			"$pull": bson.M{
				VolumesKey: bson.M{VolumeAttachmentIDKey: volumeId},
			},
		},
		options.FindOneAndUpdate().SetReturnDocument(options.After),
	)
	if err := res.Err(); err != nil {
		return errors.Wrap(err, "finding host and removing volume")
	}
	if err := res.Decode(&h); err != nil {
		return errors.Wrap(err, "decoding host")
	}

	grip.Error(message.WrapError(UnsetVolumeHost(volumeId),
		message.Fields{
			"host_id":   h.Id,
			"volume_id": volumeId,
			"op":        "host volume accounting",
			"message":   "problem un-setting host info on volume records",
		}))

	return nil
}

// FindOne gets one Volume for the given query.
func FindOneVolume(query interface{}) (*Volume, error) {
	v := &Volume{}
	err := db.FindOneQ(VolumesCollection, db.Query(query), v)
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	return v, err
}

// updateAllVolumes updates all volumes.
func updateAllVolumes(ctx context.Context, query bson.M, update bson.M) error {
	_, err := evergreen.GetEnvironment().DB().Collection(VolumesCollection).UpdateMany(ctx, query, update)
	return errors.Wrap(err, "updating volumes")
}

func FindDistroForHost(ctx context.Context, hostID string) (string, error) {
	h, err := FindOne(ctx, ById(hostID))
	if err != nil {
		return "", err
	}
	if h == nil {
		return "", errors.New("host not found")
	}
	return h.Distro.Id, nil
}

func findVolumes(q bson.M) ([]Volume, error) {
	volumes := []Volume{}
	return volumes, db.FindAllQ(VolumesCollection, db.Query(q), &volumes)
}

type ClientOptions struct {
	Provider string `bson:"provider"`
	Region   string `bson:"region"`
	Key      string `bson:"key"`
	Secret   string `bson:"secret"`
}

type EC2ProviderSettings struct {
	Region string `bson:"region"`
	Key    string `bson:"aws_access_key_id"`
	Secret string `bson:"aws_secret_access_key"`
}

var (
	awsRegionKey = bsonutil.MustHaveTag(EC2ProviderSettings{}, "Region")
	awsKeyKey    = bsonutil.MustHaveTag(EC2ProviderSettings{}, "Key")
	awsSecretKey = bsonutil.MustHaveTag(EC2ProviderSettings{}, "Secret")
)

const defaultStartingHostsByClientLimit = 500

// FindStartingHostsByClient returns a list mapping cloud provider client
// options to hosts with those client options that are starting up. The limit
// limits the number of hosts that can be returned. The limit is applied
// separately for task hosts and spawn hosts/host.create hosts.
func FindStartingHostsByClient(ctx context.Context, limit int) ([]HostsByClient, error) {
	if limit <= 0 {
		limit = defaultStartingHostsByClientLimit
	}

	nonTaskHosts, err := findStartingNonTaskHosts(ctx, limit)
	if err != nil {
		return nil, errors.Wrap(err, "finding starting non-task hosts")
	}

	taskHosts, err := findStartingTaskHosts(ctx, limit)
	if err != nil {
		return nil, errors.Wrap(err, "finding starting task hosts")
	}

	return append(nonTaskHosts, taskHosts...), nil
}

// HostsByClient represents an aggregation of hosts with common cloud provider
// client options.
type HostsByClient struct {
	Options ClientOptions `bson:"_id"`
	Hosts   []Host        `bson:"hosts"`
}

func findStartingNonTaskHosts(ctx context.Context, limit int) ([]HostsByClient, error) {
	pipeline := hostsByClientPipeline([]bson.M{
		{
			"$match": bson.M{
				StatusKey:      evergreen.HostStarting,
				ProvisionedKey: false,
				StartedByKey:   bson.M{"$ne": evergreen.User},
			},
		},
	}, limit)
	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating starting spawn hosts and host.create hosts by client options")
	}
	results := []HostsByClient{}
	if err = cur.All(ctx, &results); err != nil {
		return nil, errors.Wrap(err, "decoding starting hosts by client options")
	}

	return results, nil
}

func findStartingTaskHosts(ctx context.Context, limit int) ([]HostsByClient, error) {
	pipeline := hostsByClientPipeline([]bson.M{
		{
			"$match": bson.M{
				StatusKey:      evergreen.HostStarting,
				ProvisionedKey: false,
				StartedByKey:   evergreen.User,
			},
		},
	}, limit)
	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating starting task hosts by client options")
	}
	results := []HostsByClient{}
	if err = cur.All(ctx, &results); err != nil {
		return nil, errors.Wrap(err, "decoding starting hosts by client options")
	}

	return results, nil
}

func hostsByClientPipeline(pipeline []bson.M, limit int) []bson.M {
	return append(pipeline, []bson.M{
		{
			"$sort": bson.M{
				CreateTimeKey: 1,
			},
		},
		{
			"$limit": limit,
		},
		{
			"$project": bson.M{
				"host":          "$$ROOT",
				"settings_list": "$" + bsonutil.GetDottedKeyName(DistroKey, distro.ProviderSettingsListKey),
			},
		},
		{
			"$unwind": "$settings_list",
		},
		{
			"$group": bson.M{
				"_id": bson.M{
					"provider": bsonutil.GetDottedKeyName("$host", DistroKey, distro.ProviderKey),
					"region":   bsonutil.GetDottedKeyName("$settings_list", awsRegionKey),
					"key":      bsonutil.GetDottedKeyName("$settings_list", awsKeyKey),
					"secret":   bsonutil.GetDottedKeyName("$settings_list", awsSecretKey),
				},
				"hosts": bson.M{"$push": "$host"},
			},
		},
	}...)
}

// UnsafeReplace atomically removes the host given by the idToRemove and inserts
// a new host toInsert. This is typically done to replace the old host with a
// new one. While the atomic swap is safer than doing it non-atomically, it is
// not sufficient to guarantee application correctness, because other threads
// may still be using the old host document.
func UnsafeReplace(ctx context.Context, env evergreen.Environment, idToRemove string, toInsert *Host) error {
	if idToRemove == toInsert.Id {
		return nil
	}

	sess, err := env.Client().StartSession()
	if err != nil {
		return errors.Wrap(err, "starting transaction session")
	}
	defer sess.EndSession(ctx)

	replaceHost := func(sessCtx mongo.SessionContext) (interface{}, error) {
		if err := RemoveStrict(sessCtx, env, idToRemove); err != nil {
			return nil, errors.Wrapf(err, "removing old host '%s'", idToRemove)
		}

		if err := toInsert.InsertWithContext(sessCtx, env); err != nil {
			return nil, errors.Wrapf(err, "inserting new host '%s'", toInsert.Id)
		}
		grip.Info(message.Fields{
			"message":  "inserted host to replace intent host",
			"host_id":  toInsert.Id,
			"host_tag": toInsert.Tag,
			"distro":   toInsert.Distro.Id,
		})
		return nil, nil
	}

	txnStart := time.Now()
	if _, err = sess.WithTransaction(ctx, replaceHost); err != nil {
		return errors.Wrap(err, "atomic removal of old host and insertion of new host")
	}

	grip.Info(message.Fields{
		"message":                   "successfully replaced host document",
		"host_id":                   toInsert.Id,
		"host_tag":                  toInsert.Tag,
		"distro":                    toInsert.Distro.Id,
		"old_host_id":               idToRemove,
		"transaction_duration_secs": time.Since(txnStart).Seconds(),
	})

	return nil
}

// ConsolidateHostsForUser moves any unterminated hosts/volumes owned by oldUser to be assigned to the newUser.
func ConsolidateHostsForUser(ctx context.Context, oldUser, newUser string) error {
	catcher := grip.NewBasicCatcher()
	catcher.Add(UpdateAll(ctx, ByUserWithUnterminatedStatus(oldUser),
		bson.M{"$set": bson.M{StartedByKey: newUser}},
	))
	catcher.Add(updateAllVolumes(ctx, bson.M{VolumeCreatedByKey: oldUser},
		bson.M{"$set": bson.M{VolumeCreatedByKey: newUser}},
	))
	return catcher.Resolve()
}

// FindUnexpirableRunning returns all unexpirable spawn hosts that are
// currently running.
func FindUnexpirableRunning() ([]Host, error) {
	hosts := []Host{}
	q := bson.M{
		StatusKey:       evergreen.HostRunning,
		StartedByKey:    bson.M{"$ne": evergreen.User},
		NoExpirationKey: true,
	}
	return hosts, db.FindAllQ(Collection, db.Query(q), &hosts)
}

// FindOneByPersistentDNSName returns hosts that have a matching persistent DNS
// name.
func FindOneByPersistentDNSName(ctx context.Context, dnsName string) (*Host, error) {
	return FindOne(ctx, bson.M{
		PersistentDNSNameKey: dnsName,
	})
}

// IncrementNumAgentCleanupFailures will increment the NumAgentCleanupFailures field by 1.
func (h *Host) IncrementNumAgentCleanupFailures(ctx context.Context) error {
	if err := UpdateOne(
		ctx,
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$inc": bson.M{
				NumAgentCleanupFailuresKey: 1,
			},
		},
	); err != nil {
		return errors.Wrapf(err, "incrementing number of agent cleanup failures for host '%s'", h.Id)
	}
	h.NumAgentCleanupFailures++
	return nil
}

// UnsetNumAgentCleanupFailures unsets the NumAgentCleanupFailures field.
func (h *Host) UnsetNumAgentCleanupFailures(ctx context.Context) error {
	err := UpdateOne(
		ctx,
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				NumAgentCleanupFailuresKey: 0,
			},
		},
	)
	return errors.Wrapf(err, "unsetting number of agent cleanup failures for host '%s'", h.Id)
}

// FindUnexpirableRunningWithoutPersistentDNSName finds unexpirable hosts that
// are currently running and do not have a persistent DNS name assigned to
// them.
func FindUnexpirableRunningWithoutPersistentDNSName(ctx context.Context, limit int) ([]Host, error) {
	return Find(ctx, bson.M{
		StatusKey:            evergreen.HostRunning,
		StartedByKey:         bson.M{"$ne": evergreen.User},
		NoExpirationKey:      true,
		PersistentDNSNameKey: nil,
	}, options.Find().SetLimit(int64(limit)))
}

// isSleepScheduleApplicable returns a query that finds hosts which can use a
// sleep schedule.
func isSleepScheduleApplicable(q bson.M) bson.M {
	if q == nil {
		q = bson.M{}
	}
	q[StartedByKey] = bson.M{"$ne": evergreen.User}
	q[NoExpirationKey] = true
	return q
}

// isSleepScheduleEnabledQuery returns a query that finds unexpirable hosts for
// which the sleep schedule should take effect.
func isSleepScheduleEnabledQuery(q bson.M, now time.Time) bson.M {
	if q == nil {
		q = bson.M{}
	}

	q = isSleepScheduleApplicable(q)
	sleepSchedulePermanentlyExemptKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepSchedulePermanentlyExemptKey)
	sleepScheduleTemporarilyExemptUntil := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleTemporarilyExemptUntilKey)
	sleepScheduleShouldKeepOff := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleShouldKeepOffKey)

	if _, ok := q[StatusKey]; !ok {
		// Use all sleep schedule statuses if the query hasn't already specified
		// a more specific set of statuses.
		q[StatusKey] = bson.M{"$in": evergreen.SleepScheduleStatuses}
	}

	q[sleepSchedulePermanentlyExemptKey] = bson.M{"$ne": true}
	q[sleepScheduleShouldKeepOff] = bson.M{"$ne": true}

	notTemporarilyExempt := []bson.M{
		{
			sleepScheduleTemporarilyExemptUntil: nil,
		},
		{
			sleepScheduleTemporarilyExemptUntil: bson.M{"$lte": now},
		},
	}

	andClauses := []interface{}{bson.M{"$or": notTemporarilyExempt}}
	if andClause, ok := q["$and"]; ok {
		// Combine $and/$or clauses in case $and is already defined.
		andClauses = append(andClauses, andClause)
	}
	q["$and"] = andClauses

	return q
}

// FindHostsScheduledToStop finds all unexpirable hosts that are due to stop due to their sleep
// schedule settings.
func FindHostsScheduledToStop(ctx context.Context) ([]Host, error) {
	now := time.Now()
	sleepScheduleNextStopKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStopTimeKey)

	q := isSleepScheduleEnabledQuery(bson.M{
		StatusKey:                bson.M{"$in": []string{evergreen.HostRunning, evergreen.HostStopping}},
		sleepScheduleNextStopKey: bson.M{"$lte": now},
	}, now)

	return Find(ctx, q)
}

// PreStartThreshold is how long in advance Evergreen can check for hosts that
// are scheduled to start up soon.
const PreStartThreshold = 10 * time.Minute

// FindHostsToSleep finds all unexpirable hosts that are due to start soon due
// to their sleep schedule settings.
func FindHostsScheduledToStart(ctx context.Context) ([]Host, error) {
	now := time.Now()
	sleepScheduleNextStartKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStartTimeKey)

	q := isSleepScheduleEnabledQuery(bson.M{
		StatusKey: bson.M{"$in": []string{evergreen.HostStopped, evergreen.HostStopping}},
		// Include hosts that are imminently about to reach their wakeup time to
		// better ensure the host is running at the scheduled time.
		sleepScheduleNextStartKey: bson.M{"$lte": now.Add(PreStartThreshold)},
	}, now)

	return Find(ctx, q)
}

// setSleepSchedule sets the sleep schedule for a given host.
func setSleepSchedule(ctx context.Context, hostId string, schedule SleepScheduleInfo) error {
	if err := UpdateOne(ctx, bson.M{
		IdKey: hostId,
	}, bson.M{
		"$set": bson.M{
			SleepScheduleKey: schedule,
		},
	}); err != nil {
		return err
	}
	return nil
}

// FindMissingNextSleepScheduleTime finds hosts that are subject to the sleep
// schedule but are missing a next scheduled stop/start time.
func FindMissingNextSleepScheduleTime(ctx context.Context) ([]Host, error) {
	now := time.Now()
	sleepScheduleNextStartKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStartTimeKey)
	sleepScheduleNextStopKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStopTimeKey)
	q := isSleepScheduleEnabledQuery(bson.M{
		"$or": []bson.M{
			{
				sleepScheduleNextStartKey: nil,
			},
			{
				sleepScheduleNextStopKey: nil,
			},
		},
	}, now)
	return Find(ctx, q)
}

const SleepScheduleActionTimeout = 30 * time.Minute

// FindExceedsSleepScheduleTimeout finds hosts that are subject to the
// sleep schedule and are scheduled to stop/start, but have taken a long time to
// do so.
func FindExceedsSleepScheduleTimeout(ctx context.Context) ([]Host, error) {
	now := time.Now()
	sleepScheduleNextStartKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStartTimeKey)
	sleepScheduleNextStopKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStopTimeKey)
	q := isSleepScheduleEnabledQuery(bson.M{
		"$or": []bson.M{
			{
				sleepScheduleNextStartKey: bson.M{"$lte": now.Add(-SleepScheduleActionTimeout)},
			},
			{
				sleepScheduleNextStopKey: bson.M{"$lte": now.Add(-SleepScheduleActionTimeout)},
			},
		},
	}, now)
	return Find(ctx, q)
}

// ClearExpiredTemporaryExemptions clears all temporary exemptions from the
// sleep schedule that have expired.
func ClearExpiredTemporaryExemptions(ctx context.Context) error {
	sleepScheduleTemporarilyExemptUntilKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleTemporarilyExemptUntilKey)

	res, err := evergreen.GetEnvironment().DB().Collection(Collection).UpdateMany(ctx, isSleepScheduleApplicable(bson.M{
		StatusKey:                              bson.M{"$in": evergreen.SleepScheduleStatuses},
		sleepScheduleTemporarilyExemptUntilKey: bson.M{"$lte": time.Now()},
	}), bson.M{
		"$unset": bson.M{
			sleepScheduleTemporarilyExemptUntilKey: 1,
		},
	})
	if err != nil {
		return err
	}

	grip.InfoWhen(res.ModifiedCount > 0, message.Fields{
		"message":   "cleared expired temporary exemptions from hosts",
		"num_hosts": res.ModifiedCount,
	})

	return nil
}

// SyncPermanentExemptions finds two sets of unexpirable hosts based
// on the authoritative list of permanently exempt hosts. The function returns:
//  1. Hosts that are on the list of permanent exemptions but are not marked as
//     permanently exempt (i.e. should be marked as permanently exempt).
//  2. Hosts that are marked as permanently exempt but are not on the list of
//     permanent exemptions (i.e. should be marked as not permanently exempt).
func SyncPermanentExemptions(ctx context.Context, permanentlyExempt []string) error {
	sleepSchedulePermanentlyExemptKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepSchedulePermanentlyExemptKey)
	sleepScheduleNextStartKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStartTimeKey)
	sleepScheduleNextStopKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStopTimeKey)

	if permanentlyExempt == nil {
		permanentlyExempt = []string{}
	}

	catcher := grip.NewBasicCatcher()
	coll := evergreen.GetEnvironment().DB().Collection(Collection)

	if len(permanentlyExempt) > 0 {
		res, err := coll.UpdateMany(ctx, isSleepScheduleApplicable(bson.M{
			IdKey:                             bson.M{"$in": permanentlyExempt},
			StatusKey:                         bson.M{"$in": evergreen.SleepScheduleStatuses},
			sleepSchedulePermanentlyExemptKey: bson.M{"$ne": true},
		}), bson.M{
			"$set": bson.M{
				sleepSchedulePermanentlyExemptKey: true,
			},
			"$unset": bson.M{
				sleepScheduleNextStartKey: 1,
				sleepScheduleNextStopKey:  1,
			},
		})
		catcher.Wrap(err, "marking newly-added hosts as permanently exempt")
		if res != nil && res.ModifiedCount > 0 {
			grip.Info(message.Fields{
				"message":   "marked newly-added hosts as permanently exempt",
				"num_hosts": res.ModifiedCount,
			})
		}
	}

	res, err := coll.UpdateMany(ctx, isSleepScheduleApplicable(bson.M{
		IdKey:                             bson.M{"$nin": permanentlyExempt},
		StatusKey:                         bson.M{"$in": evergreen.SleepScheduleStatuses},
		sleepSchedulePermanentlyExemptKey: true,
	}), bson.M{
		"$set": bson.M{
			sleepSchedulePermanentlyExemptKey: false,
		},
	})
	catcher.Wrap(err, "marking newly-removed hosts as no longer permanently exempt")
	if res != nil && res.ModifiedCount > 0 {
		grip.Info(message.Fields{
			"message":   "marked newly-removed hosts as no longer permanently exempt",
			"num_hosts": res.ModifiedCount,
		})
	}

	return catcher.Resolve()
}
