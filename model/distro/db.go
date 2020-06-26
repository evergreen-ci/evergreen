package distro

import (
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

var (
	// bson fields for the Distro struct
	IdKey                    = bsonutil.MustHaveTag(Distro{}, "Id")
	AliasesKey               = bsonutil.MustHaveTag(Distro{}, "Aliases")
	ArchKey                  = bsonutil.MustHaveTag(Distro{}, "Arch")
	ProviderKey              = bsonutil.MustHaveTag(Distro{}, "Provider")
	ProviderSettingsListKey  = bsonutil.MustHaveTag(Distro{}, "ProviderSettingsList")
	SetupAsSudoKey           = bsonutil.MustHaveTag(Distro{}, "SetupAsSudo")
	SetupKey                 = bsonutil.MustHaveTag(Distro{}, "Setup")
	UserKey                  = bsonutil.MustHaveTag(Distro{}, "User")
	SSHKeyKey                = bsonutil.MustHaveTag(Distro{}, "SSHKey")
	SSHOptionsKey            = bsonutil.MustHaveTag(Distro{}, "SSHOptions")
	BootstrapSettingsKey     = bsonutil.MustHaveTag(Distro{}, "BootstrapSettings")
	CloneMethodKey           = bsonutil.MustHaveTag(Distro{}, "CloneMethod")
	WorkDirKey               = bsonutil.MustHaveTag(Distro{}, "WorkDir")
	SpawnAllowedKey          = bsonutil.MustHaveTag(Distro{}, "SpawnAllowed")
	ExpansionsKey            = bsonutil.MustHaveTag(Distro{}, "Expansions")
	DisabledKey              = bsonutil.MustHaveTag(Distro{}, "Disabled")
	ContainerPoolKey         = bsonutil.MustHaveTag(Distro{}, "ContainerPool")
	PlannerSettingsKey       = bsonutil.MustHaveTag(Distro{}, "PlannerSettings")
	FinderSettingsKey        = bsonutil.MustHaveTag(Distro{}, "FinderSettings")
	HostAllocatorSettingsKey = bsonutil.MustHaveTag(Distro{}, "HostAllocatorSettings")
	DisableShallowCloneKey   = bsonutil.MustHaveTag(Distro{}, "DisableShallowClone")
	UseLegacyAgentKey        = bsonutil.MustHaveTag(Distro{}, "UseLegacyAgent")
	ValidProjectsKey         = bsonutil.MustHaveTag(Distro{}, "ValidProjects")
	IsVirtualWorkstationKey  = bsonutil.MustHaveTag(Distro{}, "IsVirtualWorkstation")
	IsClusterKey             = bsonutil.MustHaveTag(Distro{}, "IsCluster")
	IcecreamSettingsKey      = bsonutil.MustHaveTag(Distro{}, "IcecreamSettings")
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
	ResourceLimitsVirtualMemoryKBKey = bsonutil.MustHaveTag(ResourceLimits{}, "VirtualMemoryKB")
	ResourceLimitsLockedMemoryKBKey  = bsonutil.MustHaveTag(ResourceLimits{}, "LockedMemoryKB")
)

var (
	IcecreamSettingsSchedulerHostKey = bsonutil.MustHaveTag(IcecreamSettings{}, "SchedulerHost")
	IcecreamSettingsConfigPathKey    = bsonutil.MustHaveTag(IcecreamSettings{}, "ConfigPath")
)

const Collection = "distro"

// All is a query that returns all distros.
var All = db.Query(nil).Sort([]string{IdKey})

// FindOne gets one Distro for the given query.
func FindOne(query db.Q) (Distro, error) {
	d := Distro{}
	return d, db.FindOneQ(Collection, query, &d)
}

// Find gets every Distro matching the given query.
func Find(query db.Q) ([]Distro, error) {
	distros := []Distro{}
	err := db.FindAllQ(Collection, query, &distros)
	return distros, err
}

func FindByID(id string) (*Distro, error) {
	d, err := FindOne(ById(id))
	if adb.ResultsNotFound(err) {
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "problem finding distro")
	}

	return &d, nil
}

func FindAll() ([]Distro, error) {
	return Find(db.Query(nil))
}

// Insert writes the distro to the database.
func (d *Distro) Insert() error {
	return db.Insert(Collection, d)
}

// Update updates one distro.
func (d *Distro) Update() error {
	return db.UpdateId(Collection, d.Id, d)
}

// Remove removes one distro.
func Remove(id string) error {
	return db.Remove(Collection, bson.M{IdKey: id})
}

// ById returns a query that contains an Id selector on the string, id.
func ById(id string) db.Q {
	return db.Query(bson.M{IdKey: id})
}

// ByProvider returns a query that contains a Provider selector on the string, p.
func ByProvider(p string) db.Q {
	return db.Query(bson.M{ProviderKey: p})
}

// BySpawnAllowed returns a query that contains the SpawnAllowed selector.
func BySpawnAllowed() db.Q {
	return db.Query(bson.M{SpawnAllowedKey: true})
}

// ByNeedsPlanning returns a query that selects only active or static distros that don't run containers
func ByNeedsPlanning(containerPools []evergreen.ContainerPool) db.Q {
	poolDistros := []string{}
	for _, pool := range containerPools {
		poolDistros = append(poolDistros, pool.Distro)
	}
	return db.Query(bson.M{
		"_id": bson.M{
			"$nin": poolDistros,
		},
		"$or": []bson.M{
			bson.M{DisabledKey: bson.M{"$exists": false}},
			bson.M{ProviderKey: evergreen.HostTypeStatic},
		}})
}

// ByNeedsHostsPlanning returns a query that selects distros that don't run containers
func ByNeedsHostsPlanning(containerPools []evergreen.ContainerPool) db.Q {
	poolDistros := []string{}
	for _, pool := range containerPools {
		poolDistros = append(poolDistros, pool.Distro)
	}
	return db.Query(bson.M{
		"_id": bson.M{
			"$nin": poolDistros,
		}})
}

// ByIsDisabled returns a query that selects distros that are disabled
func ByIsDisabled(containerPools []evergreen.ContainerPool) db.Q {
	return db.Query(bson.M{
		DisabledKey: true,
	})
}

// ByIds creates a query that finds all distros for the given ids and implicitly
// returns them ordered by {"_id": 1}
func ByIds(ids []string) db.Q {
	return db.Query(bson.M{IdKey: bson.M{"$in": ids}})
}

func FindByIdWithDefaultSettings(id string) (*Distro, error) {
	d, err := FindByID(id)
	if err != nil {
		return d, errors.WithStack(err)
	}
	if d == nil {
		return nil, nil
	}
	if len(d.ProviderSettingsList) > 1 {
		providerSettings, err := d.GetProviderSettingByRegion(evergreen.DefaultEC2Region)
		if err != nil {
			return nil, errors.Wrapf(err, "error getting %s provider settings for distro '%s'", evergreen.DefaultEC2Region, id)
		}
		d.ProviderSettingsList = []*birch.Document{providerSettings}
	}
	return d, nil
}
