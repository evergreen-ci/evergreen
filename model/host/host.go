package host

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/docker/go-connections/nat"
	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
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
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
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
	DisplayName        string `bson:"display_name" json:"display_name"`

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
	RunningTaskBuildVariant string `bson:"running_task_bv,omitempty" json:"running_task_bv,omitempty"`
	RunningTaskVersion      string `bson:"running_task_version,omitempty" json:"running_task_version,omitempty"`
	RunningTaskProject      string `bson:"running_task_project,omitempty" json:"running_task_project,omitempty"`
	RunningTaskGroup        string `bson:"running_task_group,omitempty" json:"running_task_group,omitempty"`
	RunningTaskGroupOrder   int    `bson:"running_task_group_order,omitempty" json:"running_task_group_order,omitempty"`

	// the task the most recently finished running on the host
	LastTask         string `bson:"last_task" json:"last_task"`
	LastGroup        string `bson:"last_group,omitempty" json:"last_group,omitempty"`
	LastBuildVariant string `bson:"last_bv,omitempty" json:"last_bv,omitempty"`
	LastVersion      string `bson:"last_version,omitempty" json:"last_version,omitempty"`
	LastProject      string `bson:"last_project,omitempty" json:"last_project,omitempty"`

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

	// NeedsReprovision is set if the host needs to be reprovisioned.
	// These fields must be unset if no provisioning is needed anymore.
	NeedsReprovision     ReprovisionType `bson:"needs_reprovision,omitempty" json:"needs_reprovision,omitempty"`
	ReprovisioningLocked bool            `bson:"reprovisioning_locked,omitempty" json:"reprovisioning_locked,omitempty"`

	// JasperCredentialsID is used to match hosts to their Jasper credentials
	// for non-legacy hosts.
	JasperCredentialsID string `bson:"jasper_credentials_id" json:"jasper_credentials_id"`

	// for ec2 dynamic hosts, the instance type requested
	InstanceType string `bson:"instance_type" json:"instance_type,omitempty"`
	// for ec2 dynamic hosts, the total size of the volumes requested, in GiB
	VolumeTotalSize int64 `bson:"volume_total_size" json:"volume_total_size,omitempty"`
	// The volumeID and device name for each volume attached to the host
	Volumes []VolumeAttachment `bson:"volumes,omitempty" json:"volumes,omitempty"`

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

	// PortBindings is populated if PublishPorts is specified when creating docker container from task
	PortBindings PortMap `bson:"port_bindings,omitempty" json:"port_bindings,omitempty"`
	// InstanceTags stores user-specified tags for instances
	InstanceTags []Tag `bson:"instance_tags,omitempty" json:"instance_tags,omitempty"`

	// SSHKeyNames contains the names of the SSH key that have been distributed
	// to this host.
	SSHKeyNames []string `bson:"ssh_key_names,omitempty" json:"ssh_key_names,omitempty"`

	IsVirtualWorkstation bool `bson:"is_virtual_workstation" json:"is_virtual_workstation"`
	// HomeVolumeSize is the size of the home volume in GB
	HomeVolumeSize int    `bson:"home_volume_size" json:"home_volume_size"`
	HomeVolumeID   string `bson:"home_volume_id" json:"home_volume_id"`
}

type Tag struct {
	Key           string `bson:"key" json:"key"`
	Value         string `bson:"value" json:"value"`
	CanBeModified bool   `bson:"can_be_modified" json:"can_be_modified"`
}

// Reprovision represents a state change in how the host is provisioned.
type ReprovisionType string

// Constants representing host provisioning changes.
const (
	ReprovisionNone ReprovisionType = ""
	// ProvisionNew indicates a transition from legacy provisioning to
	// non-legacy provisioning.
	ReprovisionToNew ReprovisionType = "convert-to-new"
	// ReprovisionToLegacy indicates a transition from non-legacy
	// provisioning to legacy provisioning.
	ReprovisionToLegacy ReprovisionType = "convert-to-legacy"
	// ReprovisionJasperRestart indicates that the host's Jasper service should
	// restart.
	ReprovisionJasperRestart ReprovisionType = "jasper-restart"
)

func (h *Host) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, h) }

type IdleHostsByDistroID struct {
	DistroID          string `bson:"distro_id"`
	IdleHosts         []Host `bson:"idle_hosts"`
	RunningHostsCount int    `bson:"running_hosts_count"`
}

func (h *IdleHostsByDistroID) MarshalBSON() ([]byte, error)  { return mgobson.Marshal(h) }
func (h *IdleHostsByDistroID) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, h) }

type HostGroup []Host

type VolumeAttachment struct {
	VolumeID   string `bson:"volume_id" json:"volume_id"`
	DeviceName string `bson:"device_name" json:"device_name"`
	IsHome     bool   `bson:"is_home" json:"is_home"`
	HostID     string `bson:"host_id" json:"host_id"`
}

// PortMap maps container port to the parent host ports (container port is formatted as <port>/<protocol>)
type PortMap map[string][]string

func GetPortMap(m nat.PortMap) PortMap {
	res := map[string][]string{}
	for containerPort, bindings := range m {
		hostPorts := []string{}
		for _, binding := range bindings {
			hostPorts = append(hostPorts, binding.HostPort)
		}
		if len(hostPorts) > 0 {
			res[string(containerPort)] = hostPorts
		}
	}
	return res
}

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
	// Command is the command to run on the docker (if not specified, will use the default entrypoint).
	Command string `mapstructure:"command" bson:"command,omitempty" json:"command,omitempty"`
	// If PublishPorts is true, any port that's exposed in the image will be published
	PublishPorts bool `mapstructure:"publish_ports" bson:"publish_ports,omitempty" json:"publish_ports,omitempty"`
	// If extra hosts are provided,these will be added to /etc/hosts on the container (in the form of hostname:IP)
	ExtraHosts []string `mapstructure:"extra_hosts" bson:"extra_hosts,omitempty" json:"extra_hosts,omitempty"`
	// If the container is created from host create, we want to skip building the image with agent
	SkipImageBuild bool `mapstructure:"skip_build" bson:"skip_build,omitempty" json:"skip_build,omitempty"`
	// list of container environment variables KEY=VALUE
	EnvironmentVars []string `mapstructure:"environment_vars" bson:"environment_vars,omitempty" json:"environment_vars,omitempty"`
}

func (opts *DockerOptions) FromDistroSettings(d distro.Distro, _ string) error {
	if len(d.ProviderSettingsList) != 0 {
		bytes, err := d.ProviderSettingsList[0].MarshalBSON()
		if err != nil {
			return errors.Wrap(err, "error marshalling provider setting into bson")
		}
		if err := bson.Unmarshal(bytes, opts); err != nil {
			return errors.Wrap(err, "error unmarshalling bson into provider settings")
		}
	}
	return nil
}

// Validate checks that the settings from the config file are sane.
func (opts *DockerOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	if opts.Image == "" {
		catcher.Errorf("Image must not be empty")
	}

	for _, h := range opts.ExtraHosts {
		if len(strings.Split(h, ":")) != 2 {
			catcher.Errorf("extra host %s must be of the form hostname:IP", h)
		}
	}

	return catcher.Resolve()
}

// ProvisionOptions is struct containing options about how a new spawn host should be set up.
type ProvisionOptions struct {
	// LoadCLI indicates (if set) that while provisioning the host, the CLI binary should
	// be placed onto the host after startup.
	LoadCLI bool `bson:"load_cli" json:"load_cli"`

	// TaskId if non-empty will trigger the CLI tool to fetch source and
	// artifacts for the given task.
	// Ignored if LoadCLI is false.
	TaskId string `bson:"task_id" json:"task_id"`

	// TaskSync, if set along with TaskId, will fetch the task's sync data on
	// the spawn host instead of fetching the source and artifacts. This is
	// ignored if LoadCLI is false.
	TaskSync bool `bson:"task_sync" json:"task_sync"`

	// Owner is the user associated with the host used to populate any necessary metadata.
	OwnerId string `bson:"owner_id" json:"owner_id"`
}

// SpawnOptions holds data which the monitor uses to determine when to terminate hosts spawned by tasks.
type SpawnOptions struct {
	// TimeoutTeardown is the time that this host should be torn down. In most cases, a host
	// should be torn down due to its task or build. TimeoutTeardown is a backstop to ensure that Evergreen
	// tears down a host if a task hangs or otherwise does not finish within an expected period of time.
	TimeoutTeardown time.Time `bson:"timeout_teardown,omitempty" json:"timeout_teardown,omitempty"`

	// TimeoutTeardown is the time after which Evergreen should give up trying to set up this host.
	TimeoutSetup time.Time `bson:"timeout_setup,omitempty" json:"timeout_setup,omitempty"`

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
	ParentHost Host
	Containers []Host
}

type HostModifyOptions struct {
	AddInstanceTags    []Tag
	DeleteInstanceTags []string
	InstanceType       string
	NoExpiration       *bool         // whether host should never expire
	AddHours           time.Duration // duration to extend expiration
	AttachVolume       string
	DetachVolume       string
	SubscriptionType   string
	NewName            string
}

type SpawnHostUsage struct {
	TotalHosts            int `bson:"total_hosts"`
	TotalStoppedHosts     int `bson:"total_stopped_hosts"`
	TotalUnexpirableHosts int `bson:"total_unexpirable_hosts"`
	NumUsersWithHosts     int `bson:"num_users_with_hosts"`

	TotalVolumes              int            `bson:"total_volumes"`
	TotalVolumeSize           int            `bson:"total_volume_size"`
	NumUsersWithVolumes       int            `bson:"num_users_with_volumes"`
	InstanceTypes             map[string]int `bson:"instance_types"`
	AverageComputeCostPerHour float64        `bson:"average_compute_cost_per_hour"`
}

const (
	// MaxLCTInterval is the maximum amount of time that can elapse before the
	// agent is considered dead. Once it has been successfully started (e.g.
	// once an agent has been deployed from the server), the agent must
	// regularly contact the server to ensure it is still alive.
	MaxLCTInterval = 5 * time.Minute
	// MaxUncommunicativeInterval is the maximum amount of time that can elapse
	// before the agent monitor is considered dead. When the host is
	// provisioning, the agent must contact the app server within this duration
	// for the agent monitor to be alive (i.e. the agent monitor has
	// successfully started its job of starting and managing the agent). After
	// initial contact, the agent must regularly contact the server so that this
	// duration does not elapse. Otherwise, the agent monitor is considered dead
	// (because it has failed to keep an agent alive that can contact the
	// server).
	MaxUncommunicativeInterval = 3 * MaxLCTInterval

	// provisioningCutoff is the threshold before a host is considered stuck in
	// provisioning.
	provisioningCutoff = 25 * time.Minute

	MaxTagKeyLength   = 128
	MaxTagValueLength = 256

	ErrorParentNotFound = "Parent not found"
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
	if !utility.IsZeroTime(h.ProvisionTime) {
		return time.Since(h.ProvisionTime)
	}

	// if the host has not run a task before, the idle time is just
	// how long is has been since the host was created
	return time.Since(h.CreationTime)
}

func (h *Host) IsEphemeral() bool {
	return utility.StringSliceContains(evergreen.ProviderSpawnable, h.Provider)
}

func (h *Host) IsContainer() bool {
	return utility.StringSliceContains(evergreen.ProviderContainer, h.Provider)
}

func (h *Host) NeedsPortBindings() bool {
	return h.DockerOptions.PublishPorts && h.PortBindings == nil
}

func IsIntentHostId(id string) bool {
	return strings.HasPrefix(id, "evg-")
}

func (h *Host) SetStatus(status, user string, logs string) error {
	if h.Status == evergreen.HostTerminated && h.Provider != evergreen.ProviderNameStatic {
		msg := "not changing status of already terminated host"
		grip.Warning(message.Fields{
			"message": msg,
			"host_id": h.Id,
			"status":  status,
		})
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
	if h.Status == evergreen.HostTerminated && h.Provider != evergreen.ProviderNameStatic {
		msg := "not changing status of already terminated host"
		grip.Warning(message.Fields{
			"message": msg,
			"host_id": h.Id,
			"status":  newStatus,
		})
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
	if h.HasContainers {
		containers, err := h.GetContainers()
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting containers",
			"host_id": h.Id,
		}))
		for _, c := range containers {
			err = c.SetStatus(evergreen.HostDecommissioned, user, "parent is being decommissioned")
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error decommissioning container",
				"host_id": c.Id,
			}))
		}
	}
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
	err := UpdateOne(
		bson.M{
			IdKey:     h.Id,
			StatusKey: h.Status,
		},
		bson.M{"$set": bson.M{
			StatusKey:    evergreen.HostStopped,
			DNSKey:       "",
			StartTimeKey: utility.ZeroTime,
		}},
	)
	if err != nil {
		return errors.Wrap(err, "can't set host stopped in db")
	}

	event.LogHostStatusChanged(h.Id, h.Status, evergreen.HostStopped, user, "")

	h.Status = evergreen.HostStopped
	h.Host = ""
	h.StartTime = utility.ZeroTime

	return nil
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
	secret := utility.RandomString()
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
				VolumesKey:         nil,
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

// probably don't want to store the port mapping exactly this way
func (h *Host) SetPortMapping(portsMap PortMap) error {
	err := UpdateOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				PortBindingsKey: portsMap,
			},
		},
	)
	if err != nil {
		return err
	}
	h.PortBindings = portsMap
	return nil
}

func (h *Host) MarkAsProvisioned() error {
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

	event.LogHostProvisioned(h.Id)

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

// SetNeedsJasperRestart sets this host as needing to have its Jasper service
// restarted as long as the host does not already need a different
// reprovisioning change. If the host is ready to reprovision now (i.e. no agent
// monitor is running), it is put in the reprovisioning state.
func (h *Host) SetNeedsJasperRestart(user string) error {
	// Ignore hosts spawned by tasks.
	if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodNone {
		return nil
	}
	if err := h.setAwaitingJasperRestart(user); err != nil {
		return err
	}
	if h.StartedBy == evergreen.User && !h.NeedsNewAgentMonitor {
		return nil
	}
	return h.MarkAsReprovisioning()
}

// setAwaitingJasperRestart marks a host as needing Jasper to be restarted by
// the given user but is not yet ready to reprovision now (i.e. the agent
// monitor is still running).
func (h *Host) setAwaitingJasperRestart(user string) error {
	if h.NeedsReprovision == ReprovisionJasperRestart {
		return nil
	}
	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	if err := UpdateOne(bson.M{
		IdKey:     h.Id,
		StatusKey: bson.M{"$in": []string{evergreen.HostProvisioning, evergreen.HostRunning}},
		bootstrapKey: bson.M{
			"$exists": true,
			"$ne":     distro.BootstrapMethodLegacySSH,
		},
		"$or": []bson.M{
			{NeedsReprovisionKey: bson.M{"$exists": false}},
			{NeedsReprovisionKey: ReprovisionJasperRestart},
		},
		ReprovisioningLockedKey: bson.M{"$ne": true},
		HasContainersKey:        bson.M{"$ne": true},
		ParentIDKey:             bson.M{"$exists": false},
	}, bson.M{
		"$set": bson.M{
			NeedsReprovisionKey: ReprovisionJasperRestart,
		},
	}); err != nil {
		return err
	}

	h.NeedsReprovision = ReprovisionJasperRestart

	event.LogHostJasperRestarting(h.Id, user)

	return nil
}

// MarkAsReprovisioning puts the host in a state that means it is going to be
// reprovisioned.
func (h *Host) MarkAsReprovisioning() error {
	var needsAgent bool
	var needsAgentMonitor bool
	switch h.NeedsReprovision {
	case ReprovisionToLegacy:
		needsAgent = true
	case ReprovisionToNew:
		needsAgentMonitor = true
	case ReprovisionJasperRestart:
		needsAgentMonitor = h.StartedBy == evergreen.User
	}

	err := UpdateOne(bson.M{IdKey: h.Id, NeedsReprovisionKey: h.NeedsReprovision},
		bson.M{
			"$set": bson.M{
				AgentStartTimeKey:       utility.ZeroTime,
				ProvisionedKey:          false,
				StatusKey:               evergreen.HostProvisioning,
				NeedsNewAgentKey:        needsAgent,
				NeedsNewAgentMonitorKey: needsAgentMonitor,
			}})
	if err != nil {
		return errors.Wrap(err, "problem marking host as reprovisioning")
	}

	h.AgentStartTime = utility.ZeroTime
	h.Provisioned = false
	h.Status = evergreen.HostProvisioning
	h.NeedsNewAgent = needsAgent
	h.NeedsNewAgentMonitor = needsAgentMonitor

	return nil
}

// MarkAsReprovisioned indicates that the host was successfully reprovisioned.
func (h *Host) MarkAsReprovisioned() error {
	now := time.Now()
	err := UpdateOne(
		bson.M{
			IdKey: h.Id,
			StatusKey: bson.M{
				"$nin": evergreen.DownHostStatus,
			},
			NeedsReprovisionKey: h.NeedsReprovision,
		},
		bson.M{
			"$set": bson.M{
				StatusKey:        evergreen.HostRunning,
				ProvisionedKey:   true,
				ProvisionTimeKey: now,
			},
			"$unset": bson.M{NeedsReprovisionKey: ReprovisionNone},
		},
	)

	if err != nil {
		return err
	}

	h.Status = evergreen.HostRunning
	h.Provisioned = true
	h.ProvisionTime = now
	h.NeedsReprovision = ReprovisionNone

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
				RunningTaskGroupOrderKey:   1,
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
	h.RunningTaskBuildVariant = ""
	h.RunningTaskVersion = ""
	h.RunningTaskProject = ""
	h.RunningTaskGroup = ""
	h.RunningTaskGroupOrder = 0
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
				RunningTaskGroupOrderKey:   1,
				RunningTaskBuildVariantKey: 1,
				RunningTaskVersionKey:      1,
				RunningTaskProjectKey:      1,
			},
		})

	if err != nil {
		return err
	}

	if h.RunningTask != "" {
		event.LogHostRunningTaskCleared(h.Id, h.RunningTask)
	}
	h.RunningTask = ""
	h.RunningTaskBuildVariant = ""
	h.RunningTaskVersion = ""
	h.RunningTaskProject = ""
	h.RunningTaskGroup = ""
	h.RunningTaskGroupOrder = 0

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

	statuses := []string{evergreen.HostRunning}
	// User data can start anytime after the instance is created, so the app
	// server may not have marked it as running yet.
	if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
		statuses = append(statuses, evergreen.HostStarting, evergreen.HostProvisioning)
	}
	selector := bson.M{
		IdKey:          h.Id,
		StatusKey:      bson.M{"$in": statuses},
		RunningTaskKey: bson.M{"$exists": false},
	}

	update := bson.M{
		"$set": bson.M{
			RunningTaskKey:             t.Id,
			RunningTaskGroupKey:        t.TaskGroup,
			RunningTaskGroupOrderKey:   t.TaskGroupOrder,
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
				"host_id": h.Id,
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
	if h.Distro.LegacyBootstrap() && h.NeedsNewAgent {
		return true
	}

	if !h.Distro.LegacyBootstrap() && h.NeedsNewAgentMonitor {
		return true
	}

	if utility.IsZeroTime(h.LastCommunicationTime) {
		return true
	}

	if h.Distro.LegacyBootstrap() && h.LastCommunicationTime.Before(time.Now().Add(-MaxLCTInterval)) {
		return true
	}
	if !h.Distro.LegacyBootstrap() && h.LastCommunicationTime.Before(time.Now().Add(-MaxUncommunicativeInterval)) {
		return true
	}

	return false
}

// SetNeedsNewAgent sets the "needs new agent" flag on the host.
func (h *Host) SetNeedsNewAgent(needsAgent bool) error {
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

// SetReprovisioningLocked sets the "provisioning is locked" flag on the host to
// indicate that provisioning jobs should not run.
func (h *Host) SetReprovisioningLocked(locked bool) error {
	var update bson.M
	if locked {
		update = bson.M{"$set": bson.M{ReprovisioningLockedKey: true}}
	} else {
		update = bson.M{"$unset": bson.M{ReprovisioningLockedKey: false}}
	}
	if err := UpdateOne(bson.M{IdKey: h.Id}, update); err != nil {
		return err
	}
	h.ReprovisioningLocked = locked
	return nil
}

// SetReprovisioningLockedAtomically is the same as SetReprovisioningLocked but
// performs an atomic update on the host in the database.
func (h *Host) SetReprovisioningLockedAtomically(locked bool) error {
	query := bson.M{IdKey: h.Id}
	var update bson.M
	if locked {
		query[ReprovisioningLockedKey] = bson.M{"$ne": true}
		update = bson.M{"$set": bson.M{ReprovisioningLockedKey: true}}
	} else {
		query[ReprovisioningLockedKey] = true
		update = bson.M{"$unset": bson.M{ReprovisioningLockedKey: false}}
	}
	if err := UpdateOne(query, update); err != nil {
		return err
	}
	h.ReprovisioningLocked = locked
	return nil
}

// SetNeedsAgentDeploy indicates that the host's agent or agent monitor needs
// to be deployed.
func (h *Host) SetNeedsAgentDeploy(needsDeploy bool) error {
	if !h.Distro.LegacyBootstrap() {
		if err := h.SetNeedsNewAgentMonitor(needsDeploy); err != nil {
			return errors.Wrap(err, "error setting host needs new agent monitor")
		}
	}
	return errors.Wrap(h.SetNeedsNewAgent(needsDeploy), "error setting host needs new agent")
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
	setFields := bson.M{
		// If adding or removing fields here, make sure that all callers will work
		// correctly after the change. Any fields defined here but not set by the
		// caller will insert the zero value into the document
		DNSKey:               h.Host,
		UserKey:              h.User,
		DistroKey:            h.Distro,
		StartedByKey:         h.StartedBy,
		ExpirationTimeKey:    h.ExpirationTime,
		ProviderKey:          h.Provider,
		TagKey:               h.Tag,
		InstanceTypeKey:      h.InstanceType,
		ZoneKey:              h.Zone,
		ProjectKey:           h.Project,
		ProvisionedKey:       h.Provisioned,
		ProvisionAttemptsKey: h.ProvisionAttempts,
		ProvisionOptionsKey:  h.ProvisionOptions,
		StatusKey:            h.Status,
		StartTimeKey:         h.StartTime,
		HasContainersKey:     h.HasContainers,
		ContainerImagesKey:   h.ContainerImages,
	}
	update := bson.M{
		"$setOnInsert": bson.M{
			CreateTimeKey: h.CreationTime,
		},
	}
	if h.NeedsReprovision != ReprovisionNone {
		setFields[NeedsReprovisionKey] = h.NeedsReprovision
	} else {
		update["$unset"] = bson.M{NeedsReprovisionKey: ReprovisionNone}
	}
	update["$set"] = setFields

	return UpsertOne(bson.M{IdKey: h.Id}, update)
}

func (h *Host) CacheHostData() error {
	_, err := UpsertOne(
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				ZoneKey:               h.Zone,
				StartTimeKey:          h.StartTime,
				VolumeTotalSizeKey:    h.VolumeTotalSize,
				VolumesKey:            h.Volumes,
				DNSKey:                h.Host,
				ComputeCostPerHourKey: h.ComputeCostPerHour,
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

// RemoveStrict deletes a host and errors if the host is not found
func RemoveStrict(id string) error {
	ctx, cancel := evergreen.GetEnvironment().Context()
	defer cancel()
	result, err := evergreen.GetEnvironment().DB().Collection(Collection).DeleteOne(ctx, bson.M{IdKey: id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return errors.Errorf("host %s not found", id)
	}
	return nil
}

// Replace overwrites an existing host document with a new one. If no existing host is found, the new one will be inserted anyway.
func (h *Host) Replace() error {
	ctx, cancel := evergreen.GetEnvironment().Context()
	defer cancel()
	result := evergreen.GetEnvironment().DB().Collection(Collection).FindOneAndReplace(ctx, bson.M{IdKey: h.Id}, h, options.FindOneAndReplace().SetUpsert(true))
	err := result.Err()
	if errors.Cause(err) == mongo.ErrNoDocuments {
		return nil
	}
	return errors.Wrap(err, "error replacing host")
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

// DecommissionHostsWithDistroId marks all up hosts intended for running tasks
// that have a matching distro ID as decommissioned.
func DecommissionHostsWithDistroId(distroId string) error {
	err := UpdateAll(
		ByDistroIDs(distroId),
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
			"host_id":  h.Id,
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

func (h *Host) SetDisplayName(newName string) error {
	h.DisplayName = newName
	return UpdateOne(
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{DisplayNameKey: h.DisplayName}},
	)
}

// AddSSHKeyName adds the SSH key name for the host if it doesn't already have
// it.
func (h *Host) AddSSHKeyName(name string) error {
	var update bson.M
	if len(h.SSHKeyNames) == 0 {
		update = bson.M{"$push": bson.M{SSHKeyNamesKey: name}}
	} else {
		update = bson.M{"$addToSet": bson.M{SSHKeyNamesKey: name}}
	}
	if err := UpdateOne(bson.M{IdKey: h.Id}, update); err != nil {
		return errors.WithStack(err)
	}

	if !utility.StringSliceContains(h.SSHKeyNames, name) {
		h.SSHKeyNames = append(h.SSHKeyNames, name)
	}

	return nil
}

func FindHostsToTerminate() ([]Host, error) {
	// unreachableCutoff is the threshold to wait for an decommissioned host to
	// become marked as reachable again before giving up and terminating it.
	const unreachableCutoff = 5 * time.Minute

	now := time.Now()

	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	query := bson.M{
		ProviderKey: bson.M{"$in": evergreen.ProviderSpawnable},
		"$or": []bson.M{
			{ // expired spawn hosts
				StartedByKey: bson.M{"$ne": evergreen.User},
				StatusKey: bson.M{
					"$nin": []string{evergreen.HostTerminated, evergreen.HostQuarantined},
				},
				ExpirationTimeKey: bson.M{"$lte": now},
			},
			{ // hosts that failed to provision
				StatusKey: evergreen.HostProvisionFailed,
			},
			{
				// Either:
				// - Host that does not provision with user data is taking too
				//   long to provision
				// - Host that provisions with user data is taking too long to
				//   provision, but has already started running tasks and not
				//   checked in recently.
				"$and": []bson.M{
					// Host is not yet done provisioning
					{"$or": []bson.M{
						{ProvisionedKey: false},
						{StatusKey: evergreen.HostProvisioning},
					}},
					{"$or": []bson.M{
						{
							// Host is a user data host and either has not run a
							// task yet or has not started its agent monitor -
							// both are indicators that the host failed to start
							// the agent in a reasonable amount of time.
							"$or": []bson.M{
								{RunningTaskKey: bson.M{"$exists": false}},
								{LTCTaskKey: ""},
							},
							LastCommunicationTimeKey: bson.M{"$lte": now.Add(-MaxUncommunicativeInterval)},
						}, {
							// Host is not a user data host so cannot run tasks
							// until done provisioning.
							bootstrapKey: bson.M{"$ne": distro.BootstrapMethodUserData},
						},
					}},
				},
				CreateTimeKey: bson.M{"$lte": now.Add(-provisioningCutoff)},
				StatusKey:     bson.M{"$ne": evergreen.HostTerminated},
				StartedByKey:  evergreen.User,
				ProviderKey:   bson.M{"$ne": evergreen.ProviderNameStatic},
			},
			{ // decommissioned hosts not running tasks
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

// FindAllRunningParentsByDistroID finds all running hosts of a given distro ID
// with child containers.
func FindAllRunningParentsByDistroID(distroID string) ([]Host, error) {
	query := db.Query(bson.M{
		StatusKey:        evergreen.HostRunning,
		HasContainersKey: true,
		bsonutil.GetDottedKeyName(DistroKey, distro.IdKey): distroID,
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

func (h *Host) GetActiveContainers() ([]Host, error) {
	if !h.HasContainers {
		return nil, errors.New("Host does not host containers")
	}
	query := db.Query(bson.M{
		StatusKey: bson.M{
			"$in": evergreen.UpHostStatus,
		},
		"$or": []bson.M{
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
		return nil, errors.New(ErrorParentNotFound)
	}
	if !host.HasContainers {
		return nil, errors.New("Host found is not a parent")
	}

	return host, nil
}

// IsIdleParent determines whether a host has only inactive containers
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
		StatusKey:   bson.M{"$in": evergreen.UpHostStatus},
	})
	num, err := Count(query)
	if err != nil {
		return false, errors.Wrap(err, "Error counting non-terminated containers")
	}

	return num == 0, nil
}

func (h *Host) UpdateParentIDs() error {
	query := bson.M{
		ParentIDKey: h.Tag,
	}
	update := bson.M{
		"$set": bson.M{
			ParentIDKey: h.Id,
		},
	}
	return UpdateAll(query, update)
}

// For spawn hosts that have never been set unexpirable, this will
// prevent spawn hosts from being set further than 30 days.
// For unexpirable hosts, this will prevent them from being extended any further
// than the new 30 day expiration (unless it is set to unexpirable again #loophole)
func (h *Host) PastMaxExpiration(extension time.Duration) error {
	maxExpirationTime := h.CreationTime.Add(evergreen.SpawnHostExpireDays * time.Hour * 24)
	if h.ExpirationTime.After(maxExpirationTime) {
		return errors.New("Spawn host cannot be extended further")
	}

	proposedTime := h.ExpirationTime.Add(extension)
	if proposedTime.After(maxExpirationTime) {
		return errors.Errorf("Spawn host cannot be extended more than %d days past creation", evergreen.SpawnHostExpireDays)
	}
	return nil
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
		bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsSpawnedByTaskKey): true,
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
		if utility.StringSliceContains(evergreen.UpHostStatus, h.Status) {
			out = append(out, h)
		}
	}
	return out
}

// getNumContainersOnParents returns a slice of uphost parents and their respective
// number of current containers currently running in order of longest expected
// finish time
func GetContainersOnParents(d distro.Distro) ([]ContainersOnParents, error) {
	allParents, err := findUphostParentsByContainerPool(d.ContainerPool)
	if err != nil {
		return nil, errors.Wrap(err, "Could not find running parent hosts")
	}

	containersOnParents := make([]ContainersOnParents, 0)
	// parents come in sorted order from soonest to latest expected finish time
	for i := len(allParents) - 1; i >= 0; i-- {
		parent := allParents[i]
		currentContainers, err := parent.GetActiveContainers()
		if err != nil && !adb.ResultsNotFound(err) {
			return nil, errors.Wrapf(err, "Problem finding containers for parent %s", parent.Id)
		}
		containersOnParents = append(containersOnParents,
			ContainersOnParents{
				ParentHost: parent,
				Containers: currentContainers,
			})
	}

	return containersOnParents, nil
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
	if numCurrentParents >= parent.HostAllocatorSettings.MaximumHosts {
		numNewParents = 0
	}
	// if adding all new parents results in more parents than allowed, only add
	// enough parents to fill to capacity
	if numNewParents+numCurrentParents > parent.HostAllocatorSettings.MaximumHosts {
		numNewParents = parent.HostAllocatorSettings.MaximumHosts - numCurrentParents
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

// MakeHostTags creates and validates a map of supplied instance tags
func MakeHostTags(tagSlice []string) ([]Tag, error) {
	catcher := grip.NewBasicCatcher()
	tagsMap := make(map[string]string)
	for _, tagString := range tagSlice {
		pair := strings.Split(tagString, "=")
		if len(pair) != 2 {
			catcher.Add(errors.Errorf("problem parsing tag '%s'", tagString))
			continue
		}

		key := pair[0]
		value := pair[1]

		// AWS tag key must contain no more than 128 characters
		if len(key) > MaxTagKeyLength {
			catcher.Add(errors.Errorf("key '%s' is longer than 128 characters", key))
		}
		// AWS tag value must contain no more than 256 characters
		if len(value) > MaxTagValueLength {
			catcher.Add(errors.Errorf("value '%s' is longer than 256 characters", value))
		}
		// tag prefix aws: is reserved
		if strings.HasPrefix(key, "aws:") || strings.HasPrefix(value, "aws:") {
			catcher.Add(errors.Errorf("illegal tag prefix 'aws:'"))
		}

		tagsMap[key] = value
	}

	// Make slice of host.Tag structs from map
	tags := []Tag{}
	for key, value := range tagsMap {
		tags = append(tags, Tag{Key: key, Value: value, CanBeModified: true})
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	return tags, nil
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

func AggregateSpawnhostData() (*SpawnHostUsage, error) {
	res := []SpawnHostUsage{}
	hostPipeline := []bson.M{
		{"$match": bson.M{
			UserHostKey: bson.M{"$eq": true},
			StatusKey:   bson.M{"$in": evergreen.UpHostStatus},
		}},
		{"$group": bson.M{
			"_id":         nil,
			"hosts":       bson.M{"$sum": 1},
			"stopped":     bson.M{"$sum": bson.M{"$cond": []interface{}{bson.M{"$eq": []string{"$" + StatusKey, evergreen.HostStopped}}, 1, 0}}},
			"unexpirable": bson.M{"$sum": bson.M{"$cond": []interface{}{"$" + NoExpirationKey, 1, 0}}},
			"users":       bson.M{"$addToSet": "$" + StartedByKey},
			"cost":        bson.M{"$avg": "$" + ComputeCostPerHourKey},
		}},
		{"$project": bson.M{
			"_id":                           "0",
			"total_hosts":                   "$hosts",
			"total_stopped_hosts":           "$stopped",
			"total_unexpirable_hosts":       "$unexpirable",
			"num_users_with_hosts":          bson.M{"$size": "$users"},
			"average_compute_cost_per_hour": "$cost",
		}},
	}

	if err := db.Aggregate(Collection, hostPipeline, &res); err != nil {
		return nil, errors.Wrap(err, "error aggregating hosts")
	}

	volumePipeline := []bson.M{
		{"$group": bson.M{
			"_id":     nil,
			"volumes": bson.M{"$sum": 1},
			"size":    bson.M{"$sum": "$" + VolumeSizeKey},
			"users":   bson.M{"$addToSet": "$" + VolumeCreatedByKey},
		}},
		{"$project": bson.M{
			"_id":                    "0",
			"total_volumes":          "$volumes",
			"total_volume_size":      "$size",
			"num_users_with_volumes": bson.M{"$size": "$users"},
		}},
	}
	if err := db.Aggregate(VolumesCollection, volumePipeline, &res); err != nil {
		return nil, errors.Wrap(err, "error aggregating volumes")
	}
	if len(res) == 0 {
		return nil, errors.New("no host/volume results found")
	}

	temp := []struct {
		InstanceType string `bson:"instance_type"`
		Count        int    `bson:"count"`
	}{}

	instanceTypePipeline := []bson.M{
		{"$match": bson.M{
			UserHostKey: bson.M{"$eq": true},
			StatusKey:   bson.M{"$in": evergreen.UpHostStatus},
		}},
		{"$group": bson.M{
			"_id":   "$" + InstanceTypeKey,
			"count": bson.M{"$sum": 1},
		}},
		{"$project": bson.M{
			"_id":           "0",
			"instance_type": "$_id",
			"count":         "$count",
		}},
	}

	if err := db.Aggregate(Collection, instanceTypePipeline, &temp); err != nil {
		return nil, errors.Wrap(err, "error aggregating instance types")
	}

	res[0].InstanceTypes = map[string]int{}
	for _, each := range temp {
		res[0].InstanceTypes[each.InstanceType] = each.Count
	}
	return &res[0], nil
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

func makeExpireOnTag(expireOn string) Tag {
	return Tag{
		Key:           evergreen.TagExpireOn,
		Value:         expireOn,
		CanBeModified: false,
	}
}

// MarkShouldNotExpire marks a host as one that should not expire
// and updates its expiration time to avoid early reaping.
func (h *Host) MarkShouldNotExpire(expireOnValue string) error {
	h.NoExpiration = true
	h.ExpirationTime = time.Now().Add(evergreen.SpawnHostNoExpirationDuration)
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
	// If it's already set to expire, do nothing.
	if !h.NoExpiration {
		return nil
	}

	h.NoExpiration = false
	h.ExpirationTime = time.Now().Add(evergreen.DefaultSpawnHostExpiration)
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

func (h *Host) SetHomeVolumeID(volumeID string) error {
	h.HomeVolumeID = volumeID
	return UpdateOne(bson.M{
		IdKey:           h.Id,
		HomeVolumeIDKey: "",
	},
		bson.M{
			"$set": bson.M{HomeVolumeIDKey: volumeID},
		},
	)
}

func (h *Host) HomeVolume() *VolumeAttachment {
	for _, vol := range h.Volumes {
		if vol.IsHome {
			return &vol
		}
	}

	return nil
}

func (h *Host) HostVolumeDeviceNames() []string {
	res := []string{}
	for _, vol := range h.Volumes {
		res = append(res, vol.DeviceName)
	}
	return res
}

// FindHostWithVolume finds the host associated with the
// specified volume ID.
func FindHostWithVolume(volumeID string) (*Host, error) {
	q := db.Query(
		bson.M{
			StatusKey:   bson.M{"$in": evergreen.UpHostStatus},
			UserHostKey: true,
			bsonutil.GetDottedKeyName(VolumesKey, VolumeAttachmentIDKey): volumeID,
		},
	)
	return FindOne(q)
}

// FindStaticNeedsNewSSHKeys finds all static hosts that do not have the same
// set of SSH keys as those in the global settings.
func FindStaticNeedsNewSSHKeys(settings *evergreen.Settings) ([]Host, error) {
	if len(settings.SSHKeyPairs) == 0 {
		return nil, nil
	}

	names := []string{}
	for _, pair := range settings.SSHKeyPairs {
		names = append(names, pair.Name)
	}

	return Find(db.Query(bson.M{
		StatusKey:      evergreen.HostRunning,
		ProviderKey:    evergreen.ProviderNameStatic,
		SSHKeyNamesKey: bson.M{"$not": bson.M{"$all": names}},
	}))
}

func (h *Host) IsSubjectToHostCreationThrottle() bool {
	if h.UserHost {
		return false
	}

	if h.SpawnOptions.SpawnedByTask {
		return false
	}

	if h.HasContainers || h.ParentID != "" {
		return false
	}

	if h.Status == evergreen.HostBuilding {
		return false
	}

	return true
}

func GetHostByIdWithTask(hostID string) (*Host, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{IdKey: hostID},
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

	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	hosts := []Host{}

	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	err = cursor.All(ctx, &hosts)
	if err != nil {
		return nil, err
	}

	return &hosts[0], nil
}

func GetPaginatedRunningHosts(hostID, distroID, currentTaskID string, statuses []string, startedBy string, sortBy string, sortDir, page, limit int) ([]Host, *int, int, error) {
	// PIPELINE FOR ALL RUNNING HOSTS
	runningHostsPipeline := []bson.M{
		{
			"$match": bson.M{StatusKey: bson.M{"$ne": evergreen.HostTerminated}},
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

	// CONTEXT
	env := evergreen.GetEnvironment()
	ctx, cancel := env.Context()
	defer cancel()

	// TOTAL RUNNING HOSTS COUNT
	countPipeline := []bson.M{}
	for _, stage := range runningHostsPipeline {
		countPipeline = append(countPipeline, stage)
	}
	countPipeline = append(countPipeline, bson.M{"$count": "count"})

	tmp := []counter{}
	cursor, err := env.DB().Collection(Collection).Aggregate(ctx, countPipeline)
	if err != nil {
		return nil, nil, 0, err
	}
	err = cursor.All(ctx, &tmp)
	if err != nil {
		return nil, nil, 0, err
	}
	totalRunningHostsCount := 0
	if len(tmp) > 0 {
		totalRunningHostsCount = tmp[0].Count
	}

	// APPLY FILTERS TO PIPELINE
	hasFilters := false

	if len(hostID) > 0 {
		hasFilters = true

		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				IdKey: hostID,
			},
		})
	}

	if len(distroID) > 0 {
		hasFilters = true

		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				"distro._id": distroID,
			},
		})
	}

	if len(currentTaskID) > 0 {
		hasFilters = true

		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				RunningTaskKey: currentTaskID,
			},
		})
	}

	if len(startedBy) > 0 {
		hasFilters = true

		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				StartedByKey: startedBy,
			},
		})
	}

	if len(statuses) > 0 {
		hasFilters = true

		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				StatusKey: bson.M{"$in": statuses},
			},
		})
	}

	// FILTERED HOSTS COUNT
	var filteredHostsCount *int

	if hasFilters {
		countPipeline = []bson.M{}
		for _, stage := range runningHostsPipeline {
			countPipeline = append(countPipeline, stage)
		}
		countPipeline = append(countPipeline, bson.M{"$count": "count"})

		tmp = []counter{}
		cursor, err = env.DB().Collection(Collection).Aggregate(ctx, countPipeline)
		if err != nil {
			return nil, nil, 0, err
		}
		err = cursor.All(ctx, &tmp)
		if err != nil {
			return nil, nil, 0, err
		}
		if len(tmp) > 0 {
			filteredHostsCount = &tmp[0].Count
		}
	}

	// APPLY SORTERS TO PIPELINE
	sorters := bson.D{}
	if len(sortBy) > 0 {
		sorters = append(sorters, bson.E{Key: sortBy, Value: sortDir})
	}
	// _id must be the LAST item in sort array to ensure a consistent sort order when previous sort keys result in a tie
	sorters = append(sorters, bson.E{Key: IdKey, Value: 1})
	runningHostsPipeline = append(runningHostsPipeline, bson.M{
		"$sort": sorters,
	})

	// APPLY PAGINATION
	if limit > 0 {
		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$skip": page * limit,
		})
		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$limit": limit,
		})
	}

	hosts := []Host{}

	cursor, err = env.DB().Collection(Collection).Aggregate(ctx, runningHostsPipeline)
	if err != nil {
		return nil, nil, 0, err
	}
	err = cursor.All(ctx, &hosts)
	if err != nil {
		return nil, nil, 0, err
	}

	return hosts, filteredHostsCount, totalRunningHostsCount, err
}

type counter struct {
	Count int `bson:"count"`
}
