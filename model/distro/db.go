package distro

import (
	"context"

	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

var (
	// bson fields for the Distro struct
	IdKey                    = bsonutil.MustHaveTag(Distro{}, "Id")
	AliasesKey               = bsonutil.MustHaveTag(Distro{}, "Aliases")
	NoteKey                  = bsonutil.MustHaveTag(Distro{}, "Note")
	ArchKey                  = bsonutil.MustHaveTag(Distro{}, "Arch")
	ProviderKey              = bsonutil.MustHaveTag(Distro{}, "Provider")
	ProviderAccountKey       = bsonutil.MustHaveTag(Distro{}, "ProviderAccount")
	ProviderSettingsListKey  = bsonutil.MustHaveTag(Distro{}, "ProviderSettingsList")
	SetupAsSudoKey           = bsonutil.MustHaveTag(Distro{}, "SetupAsSudo")
	SetupKey                 = bsonutil.MustHaveTag(Distro{}, "Setup")
	AuthorizedKeysFileKey    = bsonutil.MustHaveTag(Distro{}, "AuthorizedKeysFile")
	UserKey                  = bsonutil.MustHaveTag(Distro{}, "User")
	SSHOptionsKey            = bsonutil.MustHaveTag(Distro{}, "SSHOptions")
	BootstrapSettingsKey     = bsonutil.MustHaveTag(Distro{}, "BootstrapSettings")
	DispatcherSettingsKey    = bsonutil.MustHaveTag(Distro{}, "DispatcherSettings")
	WorkDirKey               = bsonutil.MustHaveTag(Distro{}, "WorkDir")
	SpawnAllowedKey          = bsonutil.MustHaveTag(Distro{}, "SpawnAllowed")
	ExpansionsKey            = bsonutil.MustHaveTag(Distro{}, "Expansions")
	DisabledKey              = bsonutil.MustHaveTag(Distro{}, "Disabled")
	ContainerPoolKey         = bsonutil.MustHaveTag(Distro{}, "ContainerPool")
	PlannerSettingsKey       = bsonutil.MustHaveTag(Distro{}, "PlannerSettings")
	FinderSettingsKey        = bsonutil.MustHaveTag(Distro{}, "FinderSettings")
	HomeVolumeSettingsKey    = bsonutil.MustHaveTag(Distro{}, "HomeVolumeSettings")
	HostAllocatorSettingsKey = bsonutil.MustHaveTag(Distro{}, "HostAllocatorSettings")
	DisableShallowCloneKey   = bsonutil.MustHaveTag(Distro{}, "DisableShallowClone")
	ValidProjectsKey         = bsonutil.MustHaveTag(Distro{}, "ValidProjects")
	IsVirtualWorkstationKey  = bsonutil.MustHaveTag(Distro{}, "IsVirtualWorkstation")
	IsClusterKey             = bsonutil.MustHaveTag(Distro{}, "IsCluster")
	IceCreamSettingsKey      = bsonutil.MustHaveTag(Distro{}, "IceCreamSettings")
	// ImageID is not equivalent to AMI. It is the identifier of the base image for the distro.
	ImageIDKey          = bsonutil.MustHaveTag(Distro{}, "ImageID")
	SingleTaskDistroKey = bsonutil.MustHaveTag(Distro{}, "SingleTaskDistro")

	hostAllocatorMaxHostsKey         = bsonutil.MustHaveTag(HostAllocatorSettings{}, "MaximumHosts")
	hostAllocatorAutoTuneMaxHostsKey = bsonutil.MustHaveTag(HostAllocatorSettings{}, "AutoTuneMaximumHosts")
)

var (
	// bson fields for the HostAllocatorSettings struct
	// HostAllocatorSettingsVersionKey                = bsonutil.MustHaveTag(HostAllocatorSettings{}, "Version")
	// HostAllocatorSettingsMinimumHostsKey           = bsonutil.MustHaveTag(HostAllocatorSettings{}, "MinimumHosts")
	HostAllocatorSettingsMaximumHostsKey = bsonutil.MustHaveTag(HostAllocatorSettings{}, "MaximumHosts")
	// HostAllocatorSettingsAcceptableHostIdleTimeKey = bsonutil.MustHaveTag(HostAllocatorSettings{}, "AcceptableHostIdleTime")
)

var (
	// bson fields for the BootstrapSettings struct
	BootstrapSettingsMethodKey                = bsonutil.MustHaveTag(BootstrapSettings{}, "Method")
	BootstrapSettingsCommunicationKey         = bsonutil.MustHaveTag(BootstrapSettings{}, "Communication")
	BootstrapSettingsClientDirKey             = bsonutil.MustHaveTag(BootstrapSettings{}, "ClientDir")
	BootstrapSettingsJasperBinaryDirKey       = bsonutil.MustHaveTag(BootstrapSettings{}, "JasperBinaryDir")
	BootstrapSettingsJasperCredentialsPathKey = bsonutil.MustHaveTag(BootstrapSettings{}, "JasperCredentialsPath")
	BootstrapSettingsServiceUserKey           = bsonutil.MustHaveTag(BootstrapSettings{}, "ServiceUser")
	BootstrapSettingsShellPathKey             = bsonutil.MustHaveTag(BootstrapSettings{}, "ShellPath")
	BootstrapSettingsRootDirKey               = bsonutil.MustHaveTag(BootstrapSettings{}, "RootDir")
	BootstrapSettingsEnvKey                   = bsonutil.MustHaveTag(BootstrapSettings{}, "Env")
	BootstrapSettingsResourceLimitsKey        = bsonutil.MustHaveTag(BootstrapSettings{}, "ResourceLimits")

	ResourceLimitsNumFilesKey        = bsonutil.MustHaveTag(ResourceLimits{}, "NumFiles")
	ResourceLimitsNumProcessesKey    = bsonutil.MustHaveTag(ResourceLimits{}, "NumProcesses")
	ResourceLimitsNumTasksKey        = bsonutil.MustHaveTag(ResourceLimits{}, "NumTasks")
	ResourceLimitsVirtualMemoryKBKey = bsonutil.MustHaveTag(ResourceLimits{}, "VirtualMemoryKB")
	ResourceLimitsLockedMemoryKBKey  = bsonutil.MustHaveTag(ResourceLimits{}, "LockedMemoryKB")
)

var (
	IceCreamSettingsSchedulerHostKey = bsonutil.MustHaveTag(IceCreamSettings{}, "SchedulerHost")
	IceCreamSettingsConfigPathKey    = bsonutil.MustHaveTag(IceCreamSettings{}, "ConfigPath")
)

const Collection = "distro"

// distroDB returns the database to use for distros. When a shared database is configured
// use it for distros. Otherwise, use the regular database.
func distroDB() *mongo.Database {
	if sharedDB := evergreen.GetEnvironment().SharedDB(); sharedDB != nil {
		return sharedDB
	}
	return evergreen.GetEnvironment().DB()
}

// FindOneId returns one Distro by Id.
func FindOneId(ctx context.Context, id string) (*Distro, error) {
	return FindOne(ctx, ById(id))
}

func FindOne(ctx context.Context, query bson.M, options ...*options.FindOneOptions) (*Distro, error) {
	res := distroDB().Collection(Collection).FindOne(ctx, query, options...)
	if err := res.Err(); err != nil {
		if errors.Is(err, mongo.ErrNoDocuments) {
			return nil, nil
		}
		return nil, errors.Wrap(res.Err(), "finding distro")
	}
	d := &Distro{}
	if err := res.Decode(&d); err != nil {
		return nil, errors.Wrap(err, "decoding distro")
	}

	return d, nil
}

func Find(ctx context.Context, query bson.M, options ...*options.FindOptions) ([]Distro, error) {
	cur, err := distroDB().Collection(Collection).Find(ctx, query, options...)
	if err != nil {
		return nil, errors.Wrap(err, "finding distros")
	}
	var distros []Distro
	if err := cur.All(ctx, &distros); err != nil {
		return nil, errors.Wrap(err, "decoding distros")
	}

	return distros, nil
}

// Insert writes the distro to the database.
func (d *Distro) Insert(ctx context.Context) error {
	_, err := distroDB().Collection(Collection).InsertOne(ctx, d)
	return errors.Wrap(err, "inserting distro")
}

// ReplaceOne replaces one distro.
func (d *Distro) ReplaceOne(ctx context.Context) error {
	res, err := distroDB().Collection(Collection).ReplaceOne(ctx, bson.M{IdKey: d.Id}, d)
	if err != nil {
		return errors.Wrapf(err, "updating distro ID '%s'", d.Id)
	}
	if res.MatchedCount == 0 {
		return adb.ErrNotFound
	}

	return nil
}

// Remove removes one distro.
func Remove(ctx context.Context, ID string) error {
	_, err := distroDB().Collection(Collection).DeleteOne(ctx, bson.M{IdKey: ID})
	if err != nil {
		return errors.Wrapf(err, "deleting distro ID '%s'", ID)
	}

	return nil
}

// ById returns a query that contains an Id selector on the string, id.
func ById(id string) bson.M {
	return bson.M{IdKey: id}
}

// BySpawnAllowed returns a query that contains the SpawnAllowed selector.
func BySpawnAllowed() bson.M {
	return bson.M{SpawnAllowedKey: true}
}

// ByNeedsPlanning returns a query that selects only active or static distros that don't run containers
func ByNeedsPlanning(containerPools []evergreen.ContainerPool) bson.M {
	poolDistros := []string{}
	for _, pool := range containerPools {
		poolDistros = append(poolDistros, pool.Distro)
	}
	return bson.M{
		"_id": bson.M{
			"$nin": poolDistros,
		},
		"$or": []bson.M{
			{DisabledKey: bson.M{"$exists": false}},
			{ProviderKey: evergreen.HostTypeStatic},
		}}
}

// ByNeedsHostsPlanning returns a query that selects distros that don't run containers
func ByNeedsHostsPlanning(containerPools []evergreen.ContainerPool) bson.M {
	poolDistros := []string{}
	for _, pool := range containerPools {
		poolDistros = append(poolDistros, pool.Distro)
	}
	return bson.M{
		"_id": bson.M{
			"$nin": poolDistros,
		}}
}

// ByIds creates a query that finds all distros for the given ids and implicitly
// returns them ordered by {"_id": 1}
func ByIds(ids []string) bson.M {
	return bson.M{IdKey: bson.M{"$in": ids}}
}

func FindByIdWithDefaultSettings(ctx context.Context, id string) (*Distro, error) {
	d, err := FindOneId(ctx, id)
	if err != nil {
		return d, errors.WithStack(err)
	}
	if d == nil {
		return nil, nil
	}
	if len(d.ProviderSettingsList) > 1 {
		providerSettings, err := d.GetProviderSettingByRegion(evergreen.DefaultEC2Region)
		if err != nil {
			return nil, errors.Wrapf(err, "getting provider settings for region '%s' in distro '%s'", evergreen.DefaultEC2Region, id)
		}
		d.ProviderSettingsList = []*birch.Document{providerSettings}
	}
	return d, nil
}

// FindByCanAutoTune finds all dynamically-allocated distros that can auto-tune
// their maximum hosts.
func FindByCanAutoTune(ctx context.Context) ([]Distro, error) {
	autoTuneMaxHostsKey := bsonutil.GetDottedKeyName(HostAllocatorSettingsKey, hostAllocatorAutoTuneMaxHostsKey)
	q := bson.M{
		DisabledKey:         bson.M{"$ne": true},
		ProviderKey:         bson.M{"$in": []string{evergreen.ProviderNameEc2Fleet, evergreen.ProviderNameEc2OnDemand}},
		SingleTaskDistroKey: bson.M{"$ne": true},
		autoTuneMaxHostsKey: true,
	}
	return Find(ctx, q)
}
