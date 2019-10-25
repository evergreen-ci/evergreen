package host

import (
	"context"
	"fmt"
	"math"
	"time"

	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	adb "github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
	mgobson "gopkg.in/mgo.v2/bson"
)

type Host struct {
	Id              string        `bson:"_id" json:"id"`
	Host            string        `bson:"host_id" json:"host"`
	User            string        `bson:"user" json:"user"`
	Secret          string        `bson:"secret" json:"secret"`
	ServicePassword string        `bson:"service_password,omitempty" json:"service_password,omitempty" mapstructure:"service_password,omitempty"`
	Tag             string        `bson:"tag" json:"tag"`
	Distro          distro.Distro `bson:"distro" json:"distro"`
	Provider        string        `bson:"host_type" json:"host_type"`
	IP              string        `bson:"ip_address" json:"ip_address"`

	// secondary (external) identifier for the host
	ExternalIdentifier string `bson:"ext_identifier" json:"ext_identifier"`

	// physical location of host
	Project string `bson:"project" json:"project"`
	Zone    string `bson:"zone" json:"zone"`

	// True if the app server has done all necessary host setup work (although
	// the host may need to do additional provisioning before it is running).
	Provisioned       bool      `bson:"provisioned" json:"provisioned"`
	ProvisionAttempts int       `bson:"priv_attempts" json:"provision_attempts"`
	ProvisionTime     time.Time `bson:"prov_time,omitempty" json:"prov_time,omitempty"`

	ProvisionOptions *ProvisionOptions `bson:"provision_options,omitempty" json:"provision_options,omitempty"`

	// the task that is currently running on the host
	RunningTask             string `bson:"running_task,omitempty" json:"running_task,omitempty"`
	RunningTaskGroup        string `bson:"running_task_group,omitempty" json:"running_task_group,omitempty"`
	RunningTaskBuildVariant string `bson:"running_task_bv,omitempty" json:"running_task_bv,omitempty"`
	RunningTaskVersion      string `bson:"running_task_version,omitempty" json:"running_task_version,omitempty"`
	RunningTaskProject      string `bson:"running_task_project,omitempty" json:"running_task_project,omitempty"`

	// the task the most recently finished running on the host
	LastTask               string    `bson:"last_task" json:"last_task"`
	LastGroup              string    `bson:"last_group,omitempty" json:"last_group,omitempty"`
	LastBuildVariant       string    `bson:"last_bv,omitempty" json:"last_bv,omitempty"`
	LastVersion            string    `bson:"last_version,omitempty" json:"last_version,omitempty"`
	LastProject            string    `bson:"last_project,omitempty" json:"last_project,omitempty"`
	RunningTeardownForTask string    `bson:"running_teardown,omitempty" json:"running_teardown,omitempty"`
	RunningTeardownSince   time.Time `bson:"running_teardown_since,omitempty" json:"running_teardown_since,omitempty"`

	// the full task struct that is running on the host (only populated by certain aggregations)
	RunningTaskFull *task.Task `bson:"task_full,omitempty" json:"task_full,omitempty"`

	ExpirationTime time.Time `bson:"expiration_time,omitempty" json:"expiration_time"`
	NoExpiration   bool      `bson:"no_expiration" json:"no_expiration"`

	// creation is when the host document was inserted to the DB, start is when it was started on the cloud provider
	CreationTime time.Time `bson:"creation_time" json:"creation_time"`
	StartTime    time.Time `bson:"start_time" json:"start_time"`
	// AgentStartTime is when the agent first initiates contact with the app
	// server.
	AgentStartTime  time.Time `bson:"agent_start_time" json:"agent_start_time"`
	TerminationTime time.Time `bson:"termination_time" json:"termination_time"`
	TaskCount       int       `bson:"task_count" json:"task_count"`

	LastTaskCompletedTime time.Time `bson:"last_task_completed_time" json:"last_task_completed_time"`
	LastCommunicationTime time.Time `bson:"last_communication" json:"last_communication"`

	Status    string `bson:"status" json:"status"`
	StartedBy string `bson:"started_by" json:"started_by"`
	// True if this host was created manually by a user (i.e. with spawnhost)
	UserHost             bool   `bson:"user_host" json:"user_host"`
	AgentRevision        string `bson:"agent_revision" json:"agent_revision"`
	NeedsNewAgent        bool   `bson:"needs_agent" json:"needs_agent"`
	NeedsNewAgentMonitor bool   `bson:"needs_agent_monitor" json:"needs_agent_monitor"`

	// JasperCredentialsID is used to match hosts to their Jasper credentials
	// for non-legacy hosts.
	JasperCredentialsID string `bson:"jasper_credentials_id" json:"jasper_credentials_id"`

	// JasperDeployAttempts is the current number of times the app server has
	// attempted to deploy a new Jasper service to the host.
	JasperDeployAttempts int `bson:"jasper_deploy_attempts" json:"jasper_deploy_attempts"`

	// for ec2 dynamic hosts, the instance type requested
	InstanceType string `bson:"instance_type" json:"instance_type,omitempty"`
	// for ec2 dynamic hosts, the total size of the volumes requested, in GiB
	VolumeTotalSize int64 `bson:"volume_total_size" json:"volume_total_size,omitempty"`

	VolumeIDs []string `bson:"volume_ids,omitempty" json:"volume_ids,omitempty"`

	// stores information on expiration notifications for spawn hosts
	Notifications map[string]bool `bson:"notifications,omitempty" json:"notifications,omitempty"`

	// ComputeCostPerHour is the compute (not storage) cost of one host for one hour. Cloud
	// managers can but are not required to cache this price.
	ComputeCostPerHour float64 `bson:"compute_cost_per_hour,omitempty" json:"compute_cost_per_hour,omitempty"`

	// incremented by task start and end stats collectors and
	// should reflect hosts total costs. Only populated for build-hosts
	// where host providers report costs.
	TotalCost float64 `bson:"total_cost,omitempty" json:"total_cost,omitempty"`

	// accrues the value of idle time.
	TotalIdleTime time.Duration `bson:"total_idle_time,omitempty" json:"total_idle_time,omitempty" yaml:"total_idle_time,omitempty"`

	// managed containers require different information based on host type
	// True if this host is a parent of containers
	HasContainers bool `bson:"has_containers,omitempty" json:"has_containers,omitempty"`
	// stores URLs of container images already downloaded on a parent
	ContainerImages map[string]bool `bson:"container_images,omitempty" json:"container_images,omitempty"`
	// stores the ID of the host a container is on
	ParentID string `bson:"parent_id,omitempty" json:"parent_id,omitempty"`
	// stores last expected finish time among all containers on the host
	LastContainerFinishTime time.Time `bson:"last_container_finish_time,omitempty" json:"last_container_finish_time,omitempty"`
	// ContainerPoolSettings
	ContainerPoolSettings *evergreen.ContainerPool `bson:"container_pool_settings,omitempty" json:"container_pool_settings,omitempty"`
	ContainerBuildAttempt int                      `bson:"container_build_attempt" json:"container_build_attempt"`

	// SpawnOptions holds data which the monitor uses to determine when to terminate hosts spawned by tasks.
	SpawnOptions SpawnOptions `bson:"spawn_options,omitempty" json:"spawn_options,omitempty"`

	// DockerOptions stores information for creating a container with a specific image and command
	DockerOptions DockerOptions `bson:"docker_options,omitempty" json:"docker_options,omitempty"`

	// InstanceTags stores user-specified tags for instances
	InstanceTags []Tag `bson:"instance_tags,omitempty" json:"instance_tags,omitempty"`
}

type Tag struct {
	Key           string `bson:"key" json:"key"`
	Value         string `bson:"value" json:"value"`
	CanBeModified bool   `bson:"can_be_modified" json:"can_be_modified"`
}

func (h *Host) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(h) }
func (h *Host) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, h) }

type IdleHostsByDistroID struct {
	DistroID          string `bson:"distro_id"`
	IdleHosts         []Host `bson:"idle_hosts"`
	RunningHostsCount int    `bson:"running_hosts_count"`
}

func (h *IdleHostsByDistroID) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(h) }
func (h *IdleHostsByDistroID) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, h) }

type HostGroup []Host

// DockerOptions contains options for starting a container
type DockerOptions struct {
	// Optional parameters to define a registry name and authentication
	RegistryName     string `mapstructure:"docker_registry_name" bson:"docker_registry_name,omitempty" json:"docker_registry_name,omitempty"`
	RegistryUsername string `mapstructure:"docker_registry_user" bson:"docker_registry_user,omitempty" json:"docker_registry_user,omitempty"`
	RegistryPassword string `mapstructure:"docker_registry_pw" bson:"docker_registry_pw,omitempty" json:"docker_registry_pw,omitempty"`

	// Image is required and specifies the image for the container.
	// This can be a URL or an image base, to be combined with a registry.
	Image string `mapstructure:"image_url" bson:"image_url,omitempty" json:"image_url,omitempty"`
	// Method is either "pull" or "import" and defines how to retrieve the image.
	Method string `mapstructure:"build_type" bson:"build_type,omitempty" json:"build_type,omitempty"`
	// Command is required and is the command to run on the docker.
	Command string `mapstructure:"command" bson:"command,omitempty" json:"command,omitempty"`
	// If the container is created from host create, we want to skip building the image with agent
	SkipImageBuild bool `mapstructure:"skip_build" bson:"skip_build,omitempty" json:"skip_build,omitempty"`
	// list of container environment variables KEY=VALUE
	EnvironmentVars []string `mapstructure:"environment_vars" bson:"environment_vars,omitempty" json:"environment_vars,omitempty"`
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

// SpawnOptions holds data which the monitor uses to determine when to terminate hosts spawned by tasks.
type SpawnOptions struct {
	// TimeoutTeardown is the time that this host should be torn down. In most cases, a host
	// should be torn down due to its task or build. TimeoutTeardown is a backstop to ensure that Evergreen
	// tears down a host if a task hangs or otherwise does not finish within an expected period of time.
	TimeoutTeardown time.Time `bson:"timeout_teardown" json:"timeout_teardown"`

	// TimeoutTeardown is the time after which Evergreen should give up trying to set up this host.
	TimeoutSetup time.Time `bson:"timeout_setup" json:"timeout_setup"`

	// TaskID is the task_id of the task to which this host is pinned. When the task finishes,
	// this host should be torn down. Only one of TaskID or BuildID should be set.
	TaskID string `bson:"task_id,omitempty" json:"task_id,omitempty"`

	// BuildID is the build_id of the build to which this host is pinned. When the build finishes,
	// this host should be torn down. Only one of TaskID or BuildID should be set.
	BuildID string `bson:"build_id,omitempty" json:"build_id,omitempty"`

	// Retries is the number of times Evergreen should try to spawn this host.
	Retries int `bson:"retries,omitempty" json:"retries,omitempty"`

	// SpawnedByTask indicates that this host has been spawned by a task.
	SpawnedByTask bool `bson:"spawned_by_task,omitempty" json:"spawned_by_task,omitempty"`
}

type newParentsNeededParams struct {
	numExistingParents, numContainersNeeded, numExistingContainers, maxContainers int
}

type ContainersOnParents struct {
	ParentHost    Host
	NumContainers int
}

type HostModifyOptions struct {
	AddInstanceTags    []Tag         // tags to add
	DeleteInstanceTags []string      // tags to remove
	InstanceType       string        // new instance type
	NoExpiration       *bool         // whether host should never expire
	AddHours           time.Duration // duration to extend expiration
}

const (
	MaxLCTInterval = 5 * time.Minute

	// Potential init systems supported by a Linux host.
	InitSystemSystemd = "systemd"
	InitSystemSysV    = "sysv"
	InitSystemUpstart = "upstart"

	// Max number of spawn hosts with no expiration for user
	MaxSpawnhostsWithNoExpirationPerUser = 1
)

func (h *Host) GetTaskGroupString() string {
	return fmt.Sprintf("%s_%s_%s_%s", h.RunningTaskGroup, h.RunningTaskBuildVariant, h.RunningTaskProject, h.RunningTaskVersion)
}

// IdleTime returns how long has this host been idle
func (h *Host) IdleTime() time.Duration {

	// if the host is currently running a task, it is not idle
	if h.RunningTask != "" {
		return time.Duration(0)
	}

	// if the host has run a task before, then the idle time is just the time
	// passed since the last task finished
	if h.LastTask != "" {
		return time.Since(h.LastTaskCompletedTime)
	}

	// if the host has been provisioned, the idle time is how long it has been provisioned
	if !util.IsZeroTime(h.ProvisionTime) {
		return time.Since(h.ProvisionTime)
	}

	// if the host has not run a task before, the idle time is just
	// how long is has been since the host was created
	return time.Since(h.CreationTime)
}

func (h *Host) IsEphemeral() bool {
	return util.StringSliceContains(evergreen.ProviderSpawnable, h.Provider)
}

func (h *Host) SetStatus(status, user string, logs string) error {
	if h.Status == evergreen.HostTerminated {
		msg := fmt.Sprintf("not changing the status of terminate host %s to %s", h.Id, status)
		grip.Warning(msg)
		return errors.New(msg)
	}

	event.LogHostStatusChanged(h.Id, h.Status, status, user, logs)

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

// SetStatusAtomically is the same as SetStatus but only updates the host if its
// status in the database matches currentStatus.
func (h *Host) SetStatusAtomically(newStatus, user string, logs string) error {
	if h.Status == evergreen.HostTerminated {
		msg := fmt.Sprintf("not changing the status of terminate host %s to %s", h.Id, newStatus)
		grip.Warning(msg)
		return errors.New(msg)
	}

	err := UpdateOne(
		bson.M{
			IdKey:     h.Id,
			StatusKey: h.Status,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: newStatus,
			},
		},
	)
	if err != nil {
		return err
	}

	h.Status = newStatus
	event.LogHostStatusChanged(h.Id, h.Status, newStatus, user, logs)

	return nil
}

// SetProvisioning marks the host as initializing. Only allow this
// if the host is uninitialized.
func (h *Host) SetProvisioning() error {
	return UpdateOne(
		bson.M{
			IdKey:     h.Id,
			StatusKey: evergreen.HostStarting,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostProvisioning,
			},
		},
	)
}

func (h *Host) SetDecommissioned(user string, logs string) error {
	return h.SetStatus(evergreen.HostDecommissioned, user, logs)
}

func (h *Host) SetRunning(user string) error {
	return h.SetStatus(evergreen.HostRunning, user, "")
}

func (h *Host) SetTerminated(user, reason string) error {
	return h.SetStatus(evergreen.HostTerminated, user, reason)
}

func (h *Host) SetStopping(user string) error {
	return h.SetStatus(evergreen.HostStopping, user, "")
}

func (h *Host) SetStopped(user string) error {
	return h.SetStatus(evergreen.HostStopped, user, "")
}

func (h *Host) SetUnprovisioned() error {
	return UpdateOne(
		bson.M{
			IdKey:     h.Id,
			StatusKey: evergreen.HostProvisioning,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostProvisionFailed,
			},
		},
	)
}

func (h *Host) SetQuarantined(user string, logs string) error {
	return h.SetStatus(evergreen.HostQuarantined, user, logs)
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

func (h *Host) SetAgentStartTime() error {
	now := time.Now()
	if err := UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{AgentStartTimeKey: now}},
	); err != nil {
		return errors.Wrap(err, "could not set agent start time")
	}
	h.AgentStartTime = now
	return nil
}

// JasperCredentials gets the Jasper credentials for this host's running Jasper
// service from the database. These credentials should not be used to connect to
// the Jasper service - use JasperClientCredentials for this purpose.
func (h *Host) JasperCredentials(ctx context.Context, env evergreen.Environment) (*certdepot.Credentials, error) {
	creds, err := env.CertificateDepot().Find(h.JasperCredentialsID)
	if err != nil {
		return nil, errors.Wrap(err, "problem finding creds in depot")
	}

	return creds, nil
}

// JasperCredentialsExpiration returns the time at which the host's Jasper
// credentials will expire.
func (h *Host) JasperCredentialsExpiration(ctx context.Context, env evergreen.Environment) (time.Time, error) {
	user := &certdepot.User{}
	if err := env.DB().Collection(evergreen.CredentialsCollection).FindOne(ctx, bson.M{"_id": h.JasperCredentialsID}).Decode(user); err != nil {
		return time.Time{}, errors.Wrap(err, "problem finding credentials in the database")
	}

	return user.TTL, nil
}

// JasperClientCredentials gets the Jasper credentials for a client to
// communicate with the host's running Jasper service. These credentials should
// be used only to connect to the host's Jasper service.
func (h *Host) JasperClientCredentials(ctx context.Context, env evergreen.Environment) (*certdepot.Credentials, error) {
	id := env.Settings().DomainName
	creds, err := env.CertificateDepot().Find(id)
	if err != nil {
		return nil, errors.Wrapf(err, "problem finding '%s' for '%s [%s]'", id, h.Id, h.JasperCredentialsID)
	}
	creds.ServerName = h.JasperCredentialsID
	return creds, nil
}

// GenerateJasperCredentials creates the Jasper credentials for the given host
// without saving them to the database. If credentials already exist in the
// database, they are deleted.
func (h *Host) GenerateJasperCredentials(ctx context.Context, env evergreen.Environment) (*certdepot.Credentials, error) {
	if h.JasperCredentialsID == "" {
		if err := h.UpdateJasperCredentialsID(h.Id); err != nil {
			return nil, errors.Wrap(err, "problem setting Jasper credentials ID")
		}
	}
	// We have to delete this host's credentials because GenerateInMemory will
	// fail if credentials already exist in the database.
	if err := h.DeleteJasperCredentials(ctx, env); err != nil {
		return nil, errors.Wrap(err, "problem deleting existing Jasper credentials")
	}

	creds, err := env.CertificateDepot().Generate(h.JasperCredentialsID)
	if err != nil {
		return nil, errors.Wrapf(err, "credential generation issue for host '%s'", h.JasperCredentialsID)
	}

	return creds, nil
}

// SaveJasperCredentials saves the given Jasper credentials in the database for
// the host.
func (h *Host) SaveJasperCredentials(ctx context.Context, env evergreen.Environment, creds *certdepot.Credentials) error {
	if h.JasperCredentialsID == "" {
		return errors.New("Jasper credentials ID is empty")
	}
	return errors.Wrapf(env.CertificateDepot().Save(h.JasperCredentialsID, creds), "problem finding '%s'", h.JasperCredentialsID)
}

// DeleteJasperCredentials deletes the Jasper credentials for the host and
// updates the host both in memory and in the database.
func (h *Host) DeleteJasperCredentials(ctx context.Context, env evergreen.Environment) error {
	_, err := env.DB().Collection(evergreen.CredentialsCollection).DeleteOne(ctx, bson.M{"_id": h.JasperCredentialsID})
	return errors.Wrapf(err, "problem deleting credentials for '%s'", h.JasperCredentialsID)
}

// UpdateJasperCredentialsID sets the ID of the host's Jasper credentials.
func (h *Host) UpdateJasperCredentialsID(id string) error {
	if err := UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{JasperCredentialsIDKey: id}},
	); err != nil {
		return err
	}
	h.JasperCredentialsID = id
	return nil
}

// UpdateLastCommunicated sets the host's last communication time to the current time.
func (h *Host) UpdateLastCommunicated() error {
	now := time.Now()
	err := UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{
			LastCommunicationTimeKey: now,
		}})

	if err != nil {
		return err
	}
	h.LastCommunicationTime = now
	return nil
}

// ResetLastCommunicated sets the LastCommunicationTime to be zero.
func (h *Host) ResetLastCommunicated() error {
	err := UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{LastCommunicationTimeKey: time.Unix(0, 0)}})
	if err != nil {
		return err
	}
	h.LastCommunicationTime = time.Unix(0, 0)
	return nil
}

func (h *Host) Terminate(user, reason string) error {
	err := h.SetTerminated(user, reason)
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
	if adb.ResultsNotFound(err) {
		return nil
	}
	return err
}

func (h *Host) SetIPv6Address(ipv6Address string) error {
	err := UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				IPKey: ipv6Address,
			},
		},
	)

	if err != nil {
		return errors.Wrap(err, "error finding instance")
	}

	h.IP = ipv6Address
	return nil
}

func (h *Host) MarkAsProvisioned() error {
	event.LogHostProvisioned(h.Id)
	now := time.Now()
	err := UpdateOne(
		bson.M{
			IdKey: h.Id,
			StatusKey: bson.M{
				"$nin": evergreen.DownHostStatus,
			},
		},
		bson.M{
			"$set": bson.M{
				StatusKey:        evergreen.HostRunning,
				ProvisionedKey:   true,
				ProvisionTimeKey: now,
			},
		},
	)

	if err != nil {
		return err
	}

	h.Status = evergreen.HostRunning
	h.Provisioned = true
	h.ProvisionTime = now
	return nil
}

// SetProvisionedNotRunning marks the host as having been provisioned by the app
// server but the host is not necessarily running yet.
func (h *Host) SetProvisionedNotRunning() error {
	now := time.Now()
	if err := UpdateOne(
		bson.M{
			IdKey: h.Id,
			StatusKey: bson.M{
				"$nin": evergreen.DownHostStatus,
			},
		},
		bson.M{
			"$set": bson.M{
				ProvisionedKey:   true,
				ProvisionTimeKey: now,
			},
		},
	); err != nil {
		return errors.Wrap(err, "problem setting host as provisioned but not running")
	}

	h.Provisioned = true
	h.ProvisionTime = now
	return nil
}

// UpdateProvisioningToRunning changes the host status from provisioning to
// running, as well as logging that the host has finished provisioning.
func (h *Host) UpdateProvisioningToRunning() error {
	if h.Status != evergreen.HostProvisioning {
		return nil
	}

	if err := UpdateOne(
		bson.M{
			IdKey:          h.Id,
			StatusKey:      evergreen.HostProvisioning,
			ProvisionedKey: true,
		},
		bson.M{"$set": bson.M{StatusKey: evergreen.HostRunning}},
	); err != nil {
		return errors.Wrap(err, "problem changing host status from provisioning to running")
	}

	h.Status = evergreen.HostRunning

	event.LogHostProvisioned(h.Id)

	return nil
}

// ClearRunningAndSetLastTask unsets the running task on the host and updates the last task fields.
func (h *Host) ClearRunningAndSetLastTask(t *task.Task) error {
	now := time.Now()
	err := UpdateOne(
		bson.M{
			IdKey:          h.Id,
			RunningTaskKey: h.RunningTask,
		},
		bson.M{
			"$set": bson.M{
				LTCTimeKey:    now,
				LTCTaskKey:    t.Id,
				LTCGroupKey:   t.TaskGroup,
				LTCBVKey:      t.BuildVariant,
				LTCVersionKey: t.Version,
				LTCProjectKey: t.Project,
			},
			"$unset": bson.M{
				RunningTaskKey:             1,
				RunningTaskGroupKey:        1,
				RunningTaskBuildVariantKey: 1,
				RunningTaskVersionKey:      1,
				RunningTaskProjectKey:      1,
			},
		})

	if err != nil {
		return err
	}

	event.LogHostRunningTaskCleared(h.Id, h.RunningTask)
	h.RunningTask = ""
	h.RunningTaskGroup = ""
	h.RunningTaskBuildVariant = ""
	h.RunningTaskVersion = ""
	h.RunningTaskProject = ""
	h.LastTask = t.Id
	h.LastGroup = t.TaskGroup
	h.LastBuildVariant = t.BuildVariant
	h.LastVersion = t.Version
	h.LastProject = t.Version
	h.LastTaskCompletedTime = now

	return nil
}

// ClearRunningTask unsets the running task on the host.
func (h *Host) ClearRunningTask() error {
	err := UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$unset": bson.M{
				RunningTaskKey:             1,
				RunningTaskGroupKey:        1,
				RunningTaskBuildVariantKey: 1,
				RunningTaskVersionKey:      1,
				RunningTaskProjectKey:      1,
			},
		})

	if err != nil {
		return err
	}

	event.LogHostRunningTaskCleared(h.Id, h.RunningTask)
	h.RunningTask = ""
	h.RunningTaskGroup = ""
	h.RunningTaskBuildVariant = ""
	h.RunningTaskVersion = ""
	h.RunningTaskProject = ""

	return nil
}

// UpdateRunningTask updates the running task in the host document, returns
// - true, nil on success
// - false, nil on duplicate key error, task is already assigned to another host
// - false, error on all other errors
func (h *Host) UpdateRunningTask(t *task.Task) (bool, error) {
	if t == nil {
		return false, errors.New("received nil task, cannot update")
	}
	if t.Id == "" {
		return false, errors.New("task has empty task ID, cannot update")
	}

	selector := bson.M{
		IdKey:          h.Id,
		StatusKey:      evergreen.HostRunning,
		RunningTaskKey: bson.M{"$exists": false},
	}

	update := bson.M{
		"$set": bson.M{
			RunningTaskKey:             t.Id,
			RunningTaskGroupKey:        t.TaskGroup,
			RunningTaskBuildVariantKey: t.BuildVariant,
			RunningTaskVersionKey:      t.Version,
			RunningTaskProjectKey:      t.Project,
		},
	}

	err := UpdateOne(selector, update)
	if err != nil {
		if db.IsDuplicateKey(err) {
			grip.Debug(message.Fields{
				"message": "found duplicate running task",
				"task":    t.Id,
				"host":    h.Id,
			})
			return false, nil
		}
		return false, errors.Wrapf(err, "error updating running task %s for host %s", t.Id, h.Id)
	}
	event.LogHostRunningTaskSet(h.Id, t.Id)

	return true, nil
}

// SetAgentRevision sets the updated agent revision for the host
func (h *Host) SetAgentRevision(agentRevision string) error {
	err := UpdateOne(bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{AgentRevisionKey: agentRevision}})
	if err != nil {
		return err
	}
	h.AgentRevision = agentRevision
	return nil
}

// IsWaitingForAgent provides a local predicate for the logic for
// whether the host needs either a new agent or agent monitor.
func (h *Host) IsWaitingForAgent() bool {
	if h.LegacyBootstrap() && h.NeedsNewAgent {
		return true
	}

	if !h.LegacyBootstrap() && h.NeedsNewAgentMonitor {
		return true
	}

	if util.IsZeroTime(h.LastCommunicationTime) {
		return true
	}

	if h.LastCommunicationTime.Before(time.Now().Add(-MaxLCTInterval)) {
		return true
	}

	return false
}

// SetNeedsNewAgent sets the "needs new agent" flag on the host. This is a no-op
// on non-legacy hosts.
func (h *Host) SetNeedsNewAgent(needsAgent bool) error {
	if !h.LegacyBootstrap() {
		return nil
	}

	err := UpdateOne(bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{NeedsNewAgentKey: needsAgent}})
	if err != nil {
		return err
	}
	h.NeedsNewAgent = needsAgent
	return nil
}

// SetNeedsNewAgentAtomically sets the "needs new agent" flag on the host atomically.
func (h *Host) SetNeedsNewAgentAtomically(needsAgent bool) error {
	err := UpdateOne(bson.M{IdKey: h.Id, NeedsNewAgentKey: !needsAgent},
		bson.M{"$set": bson.M{NeedsNewAgentKey: needsAgent}})
	if err != nil {
		return err
	}
	h.NeedsNewAgent = needsAgent
	return nil
}

// SetNeedsNewAgentMonitor sets the "needs new agent monitor" flag on the host
// to indicate that the host needs to have the agent monitor deployed.
func (h *Host) SetNeedsNewAgentMonitor(needsAgentMonitor bool) error {
	err := UpdateOne(bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{NeedsNewAgentMonitorKey: needsAgentMonitor}})
	if err != nil {
		return err
	}
	h.NeedsNewAgentMonitor = needsAgentMonitor
	return nil
}

// SetNeedsNewAgentMonitorAtomically is the same as SetNeedsNewAgentMonitor but
// performs an atomic update on the host in the database.
func (h *Host) SetNeedsNewAgentMonitorAtomically(needsAgentMonitor bool) error {
	if err := UpdateOne(bson.M{IdKey: h.Id, NeedsNewAgentMonitorKey: !needsAgentMonitor},
		bson.M{"$set": bson.M{NeedsNewAgentMonitorKey: needsAgentMonitor}}); err != nil {
		return err
	}
	h.NeedsNewAgentMonitor = needsAgentMonitor
	return nil
}

// LegacyBootstrap returns whether the host was bootstrapped using the legacy
// method.
func (h *Host) LegacyBootstrap() bool {
	return h.Distro.BootstrapSettings.Method == "" || h.Distro.BootstrapSettings.Method == distro.BootstrapMethodLegacySSH
}

// LegacyCommunication returns whether the app server is communicating with this
// host using the legacy method.
func (h *Host) LegacyCommunication() bool {
	return h.Distro.BootstrapSettings.Communication == "" || h.Distro.BootstrapSettings.Communication == distro.CommunicationMethodLegacySSH
}

// JasperCommunication returns whether or not the app server is communicating
// with this host's Jasper service.
func (h *Host) JasperCommunication() bool {
	return h.Distro.BootstrapSettings.Communication == distro.CommunicationMethodSSH || h.Distro.BootstrapSettings.Communication == distro.CommunicationMethodRPC
}

// SetNeedsAgentDeploy indicates that the host's agent or agent monitor needs
// to be deployed.
func (h *Host) SetNeedsAgentDeploy(needsDeploy bool) error {
	if h.LegacyBootstrap() {
		return errors.Wrap(h.SetNeedsNewAgent(needsDeploy), "error setting host needs new agent")
	}

	return errors.Wrap(h.SetNeedsNewAgentMonitor(needsDeploy), "error setting host needs new agent monitor")
}

// SetExpirationTime updates the expiration time of a spawn host
func (h *Host) SetExpirationTime(expirationTime time.Time) error {
	// update the in-memory host, then the database
	h.ExpirationTime = expirationTime
	h.NoExpiration = false
	h.Notifications = make(map[string]bool)
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				ExpirationTimeKey: expirationTime,
				NoExpirationKey:   false,
			},
			"$unset": bson.M{
				NotificationsKey: 1,
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

func (h *Host) MarkReachable() error {
	if h.Status == evergreen.HostRunning {
		return nil
	}

	event.LogHostStatusChanged(h.Id, h.Status, evergreen.HostRunning, evergreen.User, "")

	h.Status = evergreen.HostRunning

	return UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{StatusKey: evergreen.HostRunning}})
}

func (h *Host) Upsert() (*adb.ChangeInfo, error) {
	return UpsertOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				// If adding or removing fields here, make sure that all callers will work
				// correctly after the change. Any fields defined here but not set by the
				// caller will insert the zero value into the document
				DNSKey:                  h.Host,
				UserKey:                 h.User,
				DistroKey:               h.Distro,
				ProvisionedKey:          h.Provisioned,
				StartedByKey:            h.StartedBy,
				ExpirationTimeKey:       h.ExpirationTime,
				ProviderKey:             h.Provider,
				TagKey:                  h.Tag,
				InstanceTypeKey:         h.InstanceType,
				ZoneKey:                 h.Zone,
				ProjectKey:              h.Project,
				ProvisionAttemptsKey:    h.ProvisionAttempts,
				ProvisionOptionsKey:     h.ProvisionOptions,
				StartTimeKey:            h.StartTime,
				HasContainersKey:        h.HasContainers,
				ContainerImagesKey:      h.ContainerImages,
				NeedsNewAgentMonitorKey: h.NeedsNewAgentMonitor,
				JasperCredentialsIDKey:  h.JasperCredentialsID,
				JasperDeployAttemptsKey: h.JasperDeployAttempts,
			},
			"$setOnInsert": bson.M{
				StatusKey:     h.Status,
				CreateTimeKey: h.CreationTime,
			},
		},
	)
}

func (h *Host) CacheHostData() error {
	_, err := UpsertOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				ZoneKey:       h.Zone,
				StartTimeKey:  h.StartTime,
				VolumeSizeKey: h.VolumeTotalSize,
				VolumeIDsKey:  h.VolumeIDs,
				DNSKey:        h.Host,
			},
		},
	)
	return err
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

// GetElapsedCommunicationTime returns how long since this host has communicated with evergreen or vice versa
func (h *Host) GetElapsedCommunicationTime() time.Duration {
	if h.LastCommunicationTime.After(h.CreationTime) {
		return time.Since(h.LastCommunicationTime)
	}
	if h.StartTime.After(h.CreationTime) {
		return time.Since(h.StartTime)
	}
	if !h.LastCommunicationTime.IsZero() {
		return time.Since(h.LastCommunicationTime)
	}
	return time.Since(h.CreationTime)
}

func DecommissionHostsWithDistroId(distroId string) error {
	err := UpdateAll(
		ByDistroIdDoc(distroId),
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostDecommissioned,
			},
		},
	)
	return err
}

func (h *Host) DisablePoisonedHost(logs string) error {
	if h.Provider == evergreen.ProviderNameStatic {
		if err := h.SetQuarantined(evergreen.User, logs); err != nil {
			return errors.WithStack(err)
		}

		grip.Error(message.Fields{
			"host":     h.Id,
			"provider": h.Provider,
			"distro":   h.Distro.Id,
			"message":  "host may be poisoned",
			"action":   "investigate recent provisioning and system failures",
		})

		return nil
	}

	return errors.WithStack(h.SetDecommissioned(evergreen.User, logs))
}

func (h *Host) SetExtId() error {
	return UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{ExtIdKey: h.ExternalIdentifier}},
	)
}

// SetRunningTeardownGroup marks the host as running teardown_group for a task group, no-oping if it's already set to the same task
func (h *Host) SetRunningTeardownGroup(taskID string) error {
	if h.RunningTeardownForTask == taskID && !util.IsZeroTime(h.RunningTeardownSince) {
		return nil
	}

	return UpdateOne(bson.M{IdKey: h.Id}, bson.M{
		"$set": bson.M{
			RunningTeardownForTaskKey: taskID,
			RunningTeardownSinceKey:   time.Now(),
		},
	})
}

func (h *Host) ClearRunningTeardownGroup() error {
	if h.RunningTeardownForTask == "" {
		return nil
	}
	return UpdateOne(bson.M{IdKey: h.Id}, bson.M{
		"$unset": bson.M{
			RunningTeardownForTaskKey: "",
			RunningTeardownSinceKey:   "",
		},
	})
}

func FindHostsToTerminate() ([]Host, error) {
	const (
		// provisioningCutoff is the threshold to consider as too long for a host to take provisioning
		provisioningCutoff = 25 * time.Minute

		// unreachableCutoff is the threshold to wait for an decommissioned host to become marked
		// as reachable again before giving up and terminating it.
		unreachableCutoff = 5 * time.Minute
	)

	now := time.Now()

	query := bson.M{
		ProviderKey: bson.M{"$in": evergreen.ProviderSpawnable},
		"$or": []bson.M{
			{ // host.ByExpiredSince(time.Now())
				StartedByKey: bson.M{"$ne": evergreen.User},
				StatusKey: bson.M{
					"$nin": []string{evergreen.HostTerminated, evergreen.HostQuarantined},
				},
				ExpirationTimeKey: bson.M{"$lte": now},
			},
			{ // host.IsProvisioningFailure
				StatusKey: evergreen.HostProvisionFailed,
			},
			{ // host.ByUnprovisionedSince
				"$or": []bson.M{
					bson.M{ProvisionedKey: false},
					bson.M{StatusKey: evergreen.HostProvisioning},
				},
				CreateTimeKey: bson.M{"$lte": now.Add(-provisioningCutoff)},
				StatusKey:     bson.M{"$ne": evergreen.HostTerminated},
				StartedByKey:  evergreen.User,
			},
			{ // host.IsDecommissioned
				RunningTaskKey: bson.M{"$exists": false},
				StatusKey:      evergreen.HostDecommissioned,
			},
			{ // decommissioned hosts that have not checked in recently
				StatusKey:                evergreen.HostDecommissioned,
				LastCommunicationTimeKey: bson.M{"$lt": now.Add(-unreachableCutoff)},
			},
		},
	}
	hosts, err := Find(db.Query(query))

	if adb.ResultsNotFound(err) {
		return []Host{}, nil
	}

	if err != nil {
		return nil, errors.Wrap(err, "database error")
	}

	return hosts, nil
}

func CountInactiveHostsByProvider() ([]InactiveHostCounts, error) {
	var counts []InactiveHostCounts
	err := db.Aggregate(Collection, inactiveHostCountPipeline(), &counts)
	if err != nil {
		return nil, errors.Wrap(err, "error aggregating inactive hosts")
	}
	return counts, nil
}

// FindAllRunningContainers finds all the containers that are currently running
func FindAllRunningContainers() ([]Host, error) {
	query := db.Query(bson.M{
		ParentIDKey: bson.M{"$exists": true},
		StatusKey:   evergreen.HostRunning,
	})
	hosts, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "Error finding running containers")
	}

	return hosts, nil
}

// FindAllRunningParents finds all running hosts that have child containers
func FindAllRunningParents() ([]Host, error) {
	query := db.Query(bson.M{
		StatusKey:        evergreen.HostRunning,
		HasContainersKey: true,
	})
	hosts, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "Error finding running parents")
	}

	return hosts, nil
}

// FindAllRunningParentsOrdered finds all running hosts with child containers,
// sorted in order of soonest  to latest LastContainerFinishTime
func FindAllRunningParentsOrdered() ([]Host, error) {
	query := db.Query(bson.M{
		StatusKey:        evergreen.HostRunning,
		HasContainersKey: true,
	}).Sort([]string{LastContainerFinishTimeKey})
	hosts, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "Error finding ordered running parents")
	}

	return hosts, nil
}

// FindAllRunningParentsOnDistro finds all running hosts of a given distro with child containers
func FindAllRunningParentsByDistro(distroId string) ([]Host, error) {
	query := db.Query(bson.M{
		StatusKey:        evergreen.HostRunning,
		HasContainersKey: true,
		bsonutil.GetDottedKeyName(DistroKey, distro.IdKey): distroId,
	}).Sort([]string{LastContainerFinishTimeKey})
	return Find(query)
}

// GetContainers finds all the containers belonging to this host
// errors if this host is not a parent
func (h *Host) GetContainers() ([]Host, error) {
	if !h.HasContainers {
		return nil, errors.New("Host does not host containers")
	}
	query := db.Query(bson.M{"$or": []bson.M{
		{ParentIDKey: h.Id},
		{ParentIDKey: h.Tag},
	}})
	hosts, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "Error finding containers")
	}

	return hosts, nil
}

// GetParent finds the parent of this container
// errors if host is not a container or if parent cannot be found
func (h *Host) GetParent() (*Host, error) {
	if h.ParentID == "" {
		return nil, errors.New("Host does not have a parent")
	}
	host, err := FindOneByIdOrTag(h.ParentID)
	if err != nil {
		return nil, errors.Wrap(err, "Error finding parent")
	}
	if host == nil {
		return nil, errors.New("Parent not found")
	}
	if !host.HasContainers {
		return nil, errors.New("Host found is not a parent")
	}

	return host, nil
}

// IsIdleParent determines whether a host with containers has exclusively
// terminated containers
func (h *Host) IsIdleParent() (bool, error) {
	const idleTimeCutoff = 20 * time.Minute
	if !h.HasContainers {
		return false, nil
	}
	// sanity check so that hosts not immediately decommissioned
	if h.IdleTime() < idleTimeCutoff {
		return false, nil
	}
	query := db.Query(bson.M{
		ParentIDKey: h.Id,
		StatusKey:   bson.M{"$ne": evergreen.HostTerminated},
	})
	num, err := Count(query)
	if err != nil {
		return false, errors.Wrap(err, "Error counting non-terminated containers")
	}

	return num == 0, nil
}

// UpdateLastContainerFinishTime updates latest finish time for a host with containers
func (h *Host) UpdateLastContainerFinishTime(t time.Time) error {
	selector := bson.M{
		IdKey: h.Id,
	}

	update := bson.M{
		"$set": bson.M{
			LastContainerFinishTimeKey: t,
		},
	}

	if err := UpdateOne(selector, update); err != nil {
		return errors.Wrapf(err, "error updating finish time for host %s", h.Id)
	}

	return nil
}

// FindRunningHosts is the underlying query behind the hosts page's table
func FindRunningHosts(includeSpawnHosts bool) ([]Host, error) {
	query := bson.M{StatusKey: bson.M{"$ne": evergreen.HostTerminated}}

	if !includeSpawnHosts {
		query[StartedByKey] = evergreen.User
	}

	pipeline := []bson.M{
		{
			"$match": query,
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
				"path":                       "$task_full",
				"preserveNullAndEmptyArrays": true,
			},
		},
	}

	var dbHosts []Host

	if err := db.Aggregate(Collection, pipeline, &dbHosts); err != nil {
		return nil, errors.WithStack(err)
	}

	return dbHosts, nil
}

// FindAllHostsSpawnedByTasks finds all running hosts spawned by the `createhost` command.
func FindAllHostsSpawnedByTasks() ([]Host, error) {
	query := db.Query(bson.M{
		StatusKey: evergreen.HostRunning,
		SpawnOptionsKey: bson.M{
			"$exists": true,
		},
	})
	hosts, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "Error finding hosts spawned by tasks")
	}
	return hosts, nil
}

// FindHostsSpawnedByTask finds hosts spawned by the `createhost` command scoped to a given task.
func FindHostsSpawnedByTask(taskID string) ([]Host, error) {
	taskIDKey := bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsTaskIDKey)
	query := db.Query(bson.M{
		StatusKey: evergreen.HostRunning,
		taskIDKey: taskID,
	})
	hosts, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "Error finding hosts spawned by tasks by task ID")
	}
	return hosts, nil
}

// FindHostsSpawnedByBuild finds hosts spawned by the `createhost` command scoped to a given build.
func FindHostsSpawnedByBuild(buildID string) ([]Host, error) {
	buildIDKey := bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsBuildIDKey)
	query := db.Query(bson.M{
		StatusKey:  evergreen.HostRunning,
		buildIDKey: buildID,
	})
	hosts, err := Find(query)
	if err != nil {
		return nil, errors.Wrap(err, "Error finding hosts spawned by builds by build ID")
	}
	return hosts, nil
}

// FindTerminatedHostsRunningTasks finds all hosts that were running tasks when
// they were either terminated or needed to be re-provisioned.
func FindTerminatedHostsRunningTasks() ([]Host, error) {
	hosts, err := Find(db.Query(bson.M{
		StatusKey: evergreen.HostTerminated,
		"$and": []bson.M{
			{RunningTaskKey: bson.M{"$exists": true}},
			{RunningTaskKey: bson.M{"$ne": ""}}},
	}))

	if adb.ResultsNotFound(err) {
		err = nil
	}

	if err != nil {
		return nil, errors.Wrap(err, "problem finding terminated hosts")
	}

	return hosts, nil
}

// CountContainersOnParents counts how many containers are children of the given group of hosts
func (hosts HostGroup) CountContainersOnParents() (int, error) {
	ids := hosts.GetHostIds()
	query := db.Query(mgobson.M{
		StatusKey:   mgobson.M{"$in": evergreen.UpHostStatus},
		ParentIDKey: mgobson.M{"$in": ids},
	})
	return Count(query)
}

// FindUphostContainersOnParents returns the containers that are children of the given hosts
func (hosts HostGroup) FindUphostContainersOnParents() ([]Host, error) {
	ids := hosts.GetHostIds()
	query := db.Query(mgobson.M{
		StatusKey:   mgobson.M{"$in": evergreen.UpHostStatus},
		ParentIDKey: mgobson.M{"$in": ids},
	})
	return Find(query)
}

// GetHostIds returns a slice of host IDs for the given group of hosts
func (hosts HostGroup) GetHostIds() []string {
	var ids []string
	for _, h := range hosts {
		ids = append(ids, h.Id)
	}
	return ids
}

type HostGroupStats struct {
	Quarantined    int `bson:"quarantined" json:"quarantined" yaml:"quarantined"`
	Decommissioned int `bson:"decommissioned" json:"decommissioned" yaml:"decommissioned"`
	Idle           int `bson:"idle" json:"idle" yaml:"idle"`
	Active         int `bson:"active" json:"active" yaml:"active"`
	Provisioning   int `bson:"provisioning" json:"provisioning" yaml:"provisioning"`
	Total          int `bson:"total" json:"total" yaml:"total"`
}

func (hosts HostGroup) Stats() HostGroupStats {
	out := HostGroupStats{}

	for _, h := range hosts {
		out.Total++

		if h.Status == evergreen.HostQuarantined {
			out.Quarantined++
		} else if h.Status == evergreen.HostDecommissioned {
			out.Decommissioned++
		} else if h.Status != evergreen.HostRunning {
			out.Provisioning++
		} else if h.Status == evergreen.HostRunning {
			if h.RunningTask == "" {
				out.Idle++
			} else {
				out.Active++
			}
		}
	}

	return out
}

func (hosts HostGroup) Uphosts() HostGroup {
	out := HostGroup{}

	for _, h := range hosts {
		if util.StringSliceContains(evergreen.UpHostStatus, h.Status) {
			out = append(out, h)
		}
	}
	return out
}

// getNumContainersOnParents returns a slice of uphost parents and their respective
// number of current containers currently running in order of longest expected
// finish time
func GetNumContainersOnParents(d distro.Distro) ([]ContainersOnParents, error) {
	allParents, err := findUphostParentsByContainerPool(d.ContainerPool)
	if err != nil {
		return nil, errors.Wrap(err, "Could not find running parent hosts")
	}

	numContainersOnParents := make([]ContainersOnParents, 0)
	// parents come in sorted order from soonest to latest expected finish time
	for i := len(allParents) - 1; i >= 0; i-- {
		parent := allParents[i]
		currentContainers, err := parent.GetContainers()
		if err != nil && !adb.ResultsNotFound(err) {
			return nil, errors.Wrapf(err, "Problem finding containers for parent %s", parent.Id)
		}
		if len(currentContainers) < parent.ContainerPoolSettings.MaxContainers {
			numContainersOnParents = append(numContainersOnParents,
				ContainersOnParents{
					ParentHost:    parent,
					NumContainers: len(currentContainers),
				})
		}
	}

	return numContainersOnParents, nil
}

func getNumNewParentsAndHostsToSpawn(pool *evergreen.ContainerPool, newHostsNeeded int, ignoreMaxHosts bool) (int, int, error) {
	existingParents, err := findUphostParentsByContainerPool(pool.Id)
	if err != nil {
		return 0, 0, errors.Wrap(err, "could not find running parents")
	}

	// find all child containers running on those parents
	existingContainers, err := HostGroup(existingParents).FindUphostContainersOnParents()
	if err != nil {
		return 0, 0, errors.Wrap(err, "could not find running containers")
	}

	// create numParentsNeededParams struct
	parentsParams := newParentsNeededParams{
		numExistingParents:    len(existingParents),
		numExistingContainers: len(existingContainers),
		numContainersNeeded:   newHostsNeeded,
		maxContainers:         pool.MaxContainers,
	}
	// compute number of parents needed
	numNewParentsToSpawn := numNewParentsNeeded(parentsParams)
	// get parent distro from pool
	parentDistro, err := distro.FindOne(distro.ById(pool.Distro))
	if err != nil {
		return 0, 0, errors.Wrap(err, "error find parent distro")
	}

	if !ignoreMaxHosts { // only want to spawn amount of parents allowed based on pool size
		if numNewParentsToSpawn, err = parentCapacity(parentDistro, numNewParentsToSpawn, len(existingParents), pool); err != nil {
			return 0, 0, errors.Wrap(err, "could not calculate number of parents needed to spawn")
		}
	}

	// only want to spawn amount of containers we can fit on running/uninitialized parents
	newHostsNeeded = containerCapacity(len(existingParents)+numNewParentsToSpawn, len(existingContainers), newHostsNeeded, pool.MaxContainers)
	return numNewParentsToSpawn, newHostsNeeded, nil
}

// numNewParentsNeeded returns the number of additional parents needed to
// accommodate new containers
func numNewParentsNeeded(params newParentsNeededParams) int {
	existingCapacity := params.numExistingParents * params.maxContainers
	newCapacityNeeded := params.numContainersNeeded - (existingCapacity - params.numExistingContainers)

	// if we don't have enough space, calculate new parents
	if newCapacityNeeded > 0 {
		numTotalNewParents := int(math.Ceil(float64(newCapacityNeeded) / float64(params.maxContainers)))
		if numTotalNewParents < 0 {
			return 0
		}
		return numTotalNewParents
	}
	return 0

}

// containerCapacity calculates how many containers to make
// checks to make sure we do not create more containers than can fit currently
func containerCapacity(numParents, numCurrentContainers, numContainersToSpawn, maxContainers int) int {
	if numContainersToSpawn < 0 {
		return 0
	}
	numAvailableContainers := numParents*maxContainers - numCurrentContainers
	if numContainersToSpawn > numAvailableContainers {
		return numAvailableContainers
	}
	return numContainersToSpawn
}

// parentCapacity calculates number of new parents to create
// checks to make sure we do not create more parents than allowed
func parentCapacity(parent distro.Distro, numNewParents, numCurrentParents int, pool *evergreen.ContainerPool) (int, error) {
	if parent.Provider == evergreen.ProviderNameStatic {
		return 0, nil
	}
	// if there are already maximum numbers of parents running, do not spawn
	// any more parents
	if numCurrentParents >= parent.PoolSize {
		numNewParents = 0
	}
	// if adding all new parents results in more parents than allowed, only add
	// enough parents to fill to capacity
	if numNewParents+numCurrentParents > parent.PoolSize {
		numNewParents = parent.PoolSize - numCurrentParents
	}
	return numNewParents, nil
}

// findUphostParents returns the number of initializing parent host intent documents
func findUphostParentsByContainerPool(poolId string) ([]Host, error) {
	hostContainerPoolId := bsonutil.GetDottedKeyName(ContainerPoolSettingsKey, evergreen.ContainerPoolIdKey)
	query := db.Query(bson.M{
		HasContainersKey:    true,
		StatusKey:           bson.M{"$in": evergreen.UpHostStatus},
		hostContainerPoolId: poolId,
	}).Sort([]string{LastContainerFinishTimeKey})
	return Find(query)
}

func InsertMany(hosts []Host) error {
	docs := make([]interface{}, len(hosts))
	for idx := range hosts {
		docs[idx] = &hosts[idx]
	}

	return errors.WithStack(db.InsertMany(Collection, docs...))

}

// CountContainersRunningAtTime counts how many containers were running on the
// given parent host at the specified time, using the host StartTime and
// TerminationTime fields.
func (h *Host) CountContainersRunningAtTime(timestamp time.Time) (int, error) {
	query := db.Query(bson.M{
		ParentIDKey:  h.Id,
		StartTimeKey: bson.M{"$lt": timestamp},
		"$or": []bson.M{
			{TerminationTimeKey: bson.M{"$gt": timestamp}},
			{TerminationTimeKey: time.Time{}},
		},
	})
	return Count(query)
}

// EstimateNumberContainersForDuration estimates how many containers were running
// on a given host during the specified time interval by averaging the counts
// at the start and end. It is more accurate for shorter tasks.
func (h *Host) EstimateNumContainersForDuration(start, end time.Time) (float64, error) {
	containersAtStart, err := h.CountContainersRunningAtTime(start)
	if err != nil {
		return 0, errors.Wrapf(err, "Error counting containers running at %v", start)
	}
	containersAtEnd, err := h.CountContainersRunningAtTime(end)
	if err != nil {
		return 0, errors.Wrapf(err, "Error counting containers running at %v", end)
	}
	return float64(containersAtStart+containersAtEnd) / 2, nil
}

// StaleRunningTaskIDs finds any running tasks whose last heartbeat was at least the specified threshold ago
// and whose host thinks it's still running that task. Projects out everything but the ID and execution
func StaleRunningTaskIDs(staleness time.Duration) ([]task.Task, error) {
	var out []task.Task
	pipeline := []bson.M{
		{"$match": bson.M{
			task.StatusKey:        task.SelectorTaskInProgress,
			task.LastHeartbeatKey: bson.M{"$lte": time.Now().Add(-staleness)},
		}},
		{"$lookup": bson.M{
			"from": Collection,
			"as":   "host",
			"let":  bson.M{"id": "$" + task.IdKey},
			"pipeline": []bson.M{
				{
					"$match": bson.M{
						"$expr": bson.M{
							"$and": []bson.M{
								{"$eq": []string{"$$id", "$" + RunningTaskKey}},
								{"$or": []bson.M{ // this expression checks that the host is not currently running teardown_group of a different task
									{"$not": []bson.M{{"$ifNull": []interface{}{"$" + RunningTeardownForTaskKey, nil}}}},
									{"$eq": []string{"$" + RunningTeardownForTaskKey, ""}},
									{"$and": []bson.M{
										{"$ifNull": []interface{}{"$" + RunningTeardownForTaskKey, nil}},
										{"$lt": []interface{}{"$" + RunningTeardownSinceKey, time.Now().Add(-1*evergreen.MaxTeardownGroupTimeoutSecs*time.Second - 5*time.Minute)}},
									}},
								}},
							},
						},
					},
				},
			},
		}},
		{"$unwind": "$host"},
		{"$project": bson.M{
			task.IdKey:        1,
			task.ExecutionKey: 1,
			"host":            1,
		}},
	}

	err := db.Aggregate(task.Collection, pipeline, &out)
	return out, err
}

func (h *Host) addTag(new Tag, hasPermissions bool) {
	for i, old := range h.InstanceTags {
		if old.Key == new.Key {
			if old.CanBeModified || hasPermissions {
				h.InstanceTags[i] = new
			}
			return
		}
	}
	h.InstanceTags = append(h.InstanceTags, new)
	return
}

func (h *Host) deleteTag(key string, hasPermissions bool) {
	for i, old := range h.InstanceTags {
		if old.Key == key {
			if old.CanBeModified || hasPermissions {
				h.InstanceTags = append(h.InstanceTags[:i], h.InstanceTags[i+1:]...)
			}
			return
		}
	}
	return
}

// AddTags adds the specified tags to the host document, or modifies
// an existing tag if it can be modified. Does not allow changes to
// tags set by Evergreen.
func (h *Host) AddTags(tags []Tag) {
	for _, tag := range tags {
		h.addTag(tag, false)
	}
}

// DeleteTags removes tags specified by their keys, only if those
// keys are allowed to be deleted. Does not allow changes to tags
// set by Evergreen.
func (h *Host) DeleteTags(keys []string) {
	for _, key := range keys {
		h.deleteTag(key, false)
	}
}

// SetTags updates the host's instance tags in the database.
func (h *Host) SetTags() error {
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				InstanceTagsKey: h.InstanceTags,
			},
		},
	)
}

// SetInstanceType updates the host's instance type in the database.
func (h *Host) SetInstanceType(instanceType string) error {
	err := UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				InstanceTypeKey: instanceType,
			},
		},
	)
	if err != nil {
		return err
	}
	h.InstanceType = instanceType
	return nil
}

// CountSpawnhostsWithNoExpirationByUser returns a count of all hosts associated
// with a given users that are considered up and should never expire.
func CountSpawnhostsWithNoExpirationByUser(user string) (int, error) {
	query := db.Query(bson.M{
		StartedByKey:    user,
		NoExpirationKey: true,
		StatusKey:       bson.M{"$in": evergreen.UpHostStatus},
	})
	return Count(query)
}

// FindSpawnhostsWithNoExpirationToExtend returns all hosts that are set to never
// expire but have their expiration time within the next day and are still up.
func FindSpawnhostsWithNoExpirationToExtend() ([]Host, error) {
	query := db.Query(bson.M{
		UserHostKey:       true,
		NoExpirationKey:   true,
		StatusKey:         bson.M{"$in": evergreen.UpHostStatus},
		ExpirationTimeKey: bson.M{"$lte": time.Now().Add(24 * time.Hour)},
	})

	return Find(query)
}

func makeExpireOnTag(expireOnValue string) Tag {
	const expireOnKey = "expire-on"
	return Tag{
		Key:           expireOnKey,
		Value:         expireOnValue,
		CanBeModified: false,
	}
}

// MarkShouldNotExpire marks a host as one that should not expire
// and updates its expiration time to avoid early reaping.
func (h *Host) MarkShouldNotExpire(expireOnValue string) error {
	h.NoExpiration = true
	h.ExpirationTime = time.Now().AddDate(0, 0, 7)
	h.addTag(makeExpireOnTag(expireOnValue), true)
	return UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				NoExpirationKey:   h.NoExpiration,
				ExpirationTimeKey: h.ExpirationTime,
				InstanceTagsKey:   h.InstanceTags,
			},
		},
	)
}

// MarkShouldExpire resets a host's expiration to expire like
// a normal spawn host, after 24 hours.
func (h *Host) MarkShouldExpire(expireOnValue string) error {
	h.NoExpiration = false
	h.ExpirationTime = time.Now().Add(24 * time.Hour)
	h.addTag(makeExpireOnTag(expireOnValue), true)
	return UpdateOne(bson.M{
		IdKey: h.Id,
	},
		bson.M{
			"$set": bson.M{
				NoExpirationKey:   h.NoExpiration,
				ExpirationTimeKey: h.ExpirationTime,
				InstanceTagsKey:   h.InstanceTags,
			},
		},
	)
}
