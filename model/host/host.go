package host

import (
	"context"
	"crypto/sha256"
	"encoding/base64"
	"fmt"
	"math"
	"net"
	"net/http"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
	"github.com/robfig/cron"

	"github.com/docker/go-connections/nat"
	"github.com/evergreen-ci/birch"
	"github.com/evergreen-ci/certdepot"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
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
)

type Host struct {
	Id string `bson:"_id" json:"id"`
	// Host is the ephemeral DNS name of the host.
	Host            string        `bson:"host_id" json:"host"`
	User            string        `bson:"user" json:"user"`
	Secret          string        `bson:"secret" json:"secret"`
	ServicePassword string        `bson:"service_password,omitempty" json:"service_password,omitempty" mapstructure:"service_password,omitempty"`
	Tag             string        `bson:"tag" json:"tag"`
	Distro          distro.Distro `bson:"distro" json:"distro"`
	Provider        string        `bson:"host_type" json:"host_type"`
	// IP holds the IPv6 address when applicable.
	IP string `bson:"ip_address" json:"ip_address"`
	// IPv4 is the host's private IPv4 address.
	IPv4 string `bson:"ipv4_address" json:"ipv4_address"`
	// PersistentDNSName is the long-lived DNS name of the host, which should
	// never change once set.
	PersistentDNSName string `bson:"persistent_dns_name,omitempty" json:"persistent_dns_name,omitempty"`
	// PublicIPv4 is the host's IPv4 address.
	PublicIPv4 string `bson:"public_ipv4_address,omitempty" json:"public_ipv4_address,omitempty"`

	// secondary (external) identifier for the host
	ExternalIdentifier string `bson:"ext_identifier" json:"ext_identifier"`
	DisplayName        string `bson:"display_name" json:"display_name"`

	// Availability zone of the host.
	Zone string `bson:"zone" json:"zone"`

	// True if the app server has done all necessary host setup work (although
	// the host may need to do additional provisioning before it is running).
	Provisioned   bool      `bson:"provisioned" json:"provisioned"`
	ProvisionTime time.Time `bson:"prov_time,omitempty" json:"prov_time,omitempty"`

	ProvisionOptions *ProvisionOptions `bson:"provision_options,omitempty" json:"provision_options,omitempty"`

	// the task that is currently running on the host
	RunningTask             string `bson:"running_task,omitempty" json:"running_task,omitempty"`
	RunningTaskExecution    int    `bson:"running_task_execution" json:"running_task_execution"`
	RunningTaskBuildVariant string `bson:"running_task_bv,omitempty" json:"running_task_bv,omitempty"`
	RunningTaskVersion      string `bson:"running_task_version,omitempty" json:"running_task_version,omitempty"`
	RunningTaskProject      string `bson:"running_task_project,omitempty" json:"running_task_project,omitempty"`
	RunningTaskGroup        string `bson:"running_task_group,omitempty" json:"running_task_group,omitempty"`
	RunningTaskGroupOrder   int    `bson:"running_task_group_order,omitempty" json:"running_task_group_order,omitempty"`

	// TaskGroupTeardownStartTime represents the time when the teardown of task groups process started for the host.
	TaskGroupTeardownStartTime time.Time `bson:"teardown_start_time,omitempty" json:"teardown_start_time,omitempty"`

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
	// BillingStartTime is when billing started for the host.
	BillingStartTime time.Time `bson:"billing_start_time" json:"billing_start_time"`
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
	// NumAgentCleanupFailures represents the number of consecutive failed attempts a host has gone through
	// while trying to clean up an agent on a quarantined host.
	NumAgentCleanupFailures int `bson:"num_agent_cleanup_failures" json:"num_agent_cleanup_failures"`

	// NeedsReprovision is set if the host needs to be reprovisioned.
	// These fields must be unset if no provisioning is needed anymore.
	NeedsReprovision ReprovisionType `bson:"needs_reprovision,omitempty" json:"needs_reprovision,omitempty"`

	// JasperCredentialsID is used to match hosts to their Jasper credentials
	// for non-legacy hosts.
	JasperCredentialsID string `bson:"jasper_credentials_id" json:"jasper_credentials_id"`

	// InstanceType is the EC2 host's requested instance type. This is kept
	// up-to-date even if the instance type is changed.
	InstanceType string `bson:"instance_type" json:"instance_type,omitempty"`
	// The volumeID and device name for each volume attached to the host
	Volumes []VolumeAttachment `bson:"volumes,omitempty" json:"volumes,omitempty"`

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

	// SSHPort is the port to use when connecting to the host with SSH.
	SSHPort int `bson:"ssh_port,omitempty" json:"ssh_port,omitempty"`

	IsVirtualWorkstation bool `bson:"is_virtual_workstation" json:"is_virtual_workstation"`
	// HomeVolumeSize is the size of the home volume in GB
	HomeVolumeSize int    `bson:"home_volume_size" json:"home_volume_size"`
	HomeVolumeID   string `bson:"home_volume_id" json:"home_volume_id"`

	// SleepSchedule stores host sleep schedule information.
	SleepSchedule SleepScheduleInfo `bson:"sleep_schedule,omitempty" json:"sleep_schedule,omitempty"`
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
	// ReprovisionRestartJasper indicates that the host's Jasper service should
	// restart.
	ReprovisionRestartJasper ReprovisionType = "restart-jasper"
)

func (h *Host) UnmarshalBSON(in []byte) error { return mgobson.Unmarshal(in, h) }

// IsFree checks that the host is not running a task and is not in the process of tearing down.
func (h *Host) IsFree() bool {
	return h.RunningTask == "" && !h.IsTearingDown()
}

// IsTearingDown determines if TeardownStartTime is not zero time, therefore indicating that the host is tearing down
func (h *Host) IsTearingDown() bool {
	return !h.TaskGroupTeardownStartTime.IsZero()
}

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

// DockerOptions contains options for starting a container. This fulfills the
// ProviderSettings interface to populate container information from the distro
// settings.
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
	// StdinData is the data to pass to the container command's stdin.
	StdinData []byte `mapstructure:"stdin_data" bson:"stdin_data,omitempty" json:"stdin_data,omitempty"`
}

// FromDistroSettings loads the Docker container options from the provider
// settings.
func (opts *DockerOptions) FromDistroSettings(d distro.Distro, _ string) error {
	if len(d.ProviderSettingsList) != 0 {
		bytes, err := d.ProviderSettingsList[0].MarshalBSON()
		if err != nil {
			return errors.Wrap(err, "marshalling provider settings into BSON")
		}
		if err := bson.Unmarshal(bytes, opts); err != nil {
			return errors.Wrap(err, "unmarshalling BSON into Docker provider settings")
		}
	}
	return nil
}

// Validate checks that the settings from the config file are sane.
func (opts *DockerOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.ErrorfWhen(opts.Image == "", "image must not be empty")

	return catcher.Resolve()
}

// ProvisionOptions is struct containing options about how a new spawn host should be set up.
type ProvisionOptions struct {
	// TaskId if non-empty will trigger the CLI tool to fetch source and
	// artifacts for the given task.
	TaskId string `bson:"task_id" json:"task_id"`

	// TaskSync, if set along with TaskId, will fetch the task's sync data on
	// the spawn host instead of fetching the source and artifacts. This is
	TaskSync bool `bson:"task_sync" json:"task_sync"`

	// Owner is the user associated with the host used to populate any necessary metadata.
	OwnerId string `bson:"owner_id" json:"owner_id"`

	// SetupScript runs after other host provisioning is done (i.e. loading task data/artifacts).
	SetupScript string `bson:"setup_script" json:"setup_script"`
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

	// TaskExecutionNumber is the execution number of the task that spawned this host. This
	// field is deliberately NOT omitempty in order to support the aggregation in
	// allHostsSpawnedByFinishedTasks().
	TaskExecutionNumber int `bson:"task_execution_number" json:"task_execution_number"`

	// BuildID is the build_id of the build to which this host is pinned. When the build finishes,
	// this host should be torn down. Only one of TaskID or BuildID should be set.
	BuildID string `bson:"build_id,omitempty" json:"build_id,omitempty"`

	// Retries is the number of times Evergreen should try to spawn this host.
	Retries int `bson:"retries,omitempty" json:"retries,omitempty"`

	// Respawns is the number of spawn attempts remaining if the host is externally terminated after
	// being spawned.
	Respawns int `bson:"respawns,omitempty" json:"respawns,omitempty"`

	// SpawnedByTask indicates that this host has been spawned by a task.
	SpawnedByTask bool `bson:"spawned_by_task,omitempty" json:"spawned_by_task,omitempty"`
}

// SleepScheduleInfo stores information about a host's sleep schedule and
// related bookkeeping information.
type SleepScheduleInfo struct {
	// WholeWeekdaysOff represents whole weekdays for the host to sleep.
	WholeWeekdaysOff []time.Weekday `bson:"whole_weekdays_off,omitempty" json:"whole_weekdays_off,omitempty"`
	// DailyStartTime and DailyStopTime represent a daily schedule for when to
	// start a stopped host back up. The format is "HH:MM".
	DailyStartTime string `bson:"daily_start_time,omitempty" json:"daily_start_time,omitempty"`
	// DailyStopTime represents a daily schedule for when to stop a host. The
	// format is "HH:MM".
	DailyStopTime string `bson:"daily_stop_time,omitempty" json:"daily_stop_time,omitempty"`
	// TimeZone is the time zone for this host's sleep schedule.
	TimeZone string `bson:"time_zone" json:"time_zone"`
	// TemporarilyExemptUntil stores when a user's temporary exemption ends, if
	// any has been set. The sleep schedule will not take effect until
	// this timestamp passes.
	TemporarilyExemptUntil time.Time `bson:"temporarily_exempt_until,omitempty" json:"temporarily_exempt_until,omitempty"`
	// PermanentlyExempt determines if a host is permanently exempt from the
	// sleep schedule. If true, the sleep schedule will never take effect.
	PermanentlyExempt bool `bson:"permanently_exempt" json:"permanently_exempt"`
	// ShouldKeepOff indicates if the host should be kept off, regardless of the
	// other sleep schedule settings. This exists to permit a host to remain off
	// indefinitely until the host's owner is ready to turn it on and resume its
	// usual sleep schedule. Since this option allows the user to re-enable
	// their sleep schedule whenever they want, this is distinct from being
	// permanently exempt, which means the sleep schedule never takes effect.
	ShouldKeepOff bool `bson:"should_keep_off" json:"should_keep_off"`
	// NextStopTime is the next time that the host should stop for its sleep
	// schedule.
	NextStopTime time.Time `bson:"next_stop_time,omitempty" json:"next_stop_time,omitempty"`
	// NextStartTime is the next time that the host should start for its sleep
	// schedule.
	NextStartTime time.Time `bson:"next_start_time,omitempty" json:"next_start_time,omitempty"`
}

// NewSleepScheduleInfo creates a new sleep schedule for a host that does not
// already have one defined.
func NewSleepScheduleInfo(opts SleepScheduleOptions) (*SleepScheduleInfo, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid sleep schedule")
	}

	schedule := SleepScheduleInfo{
		WholeWeekdaysOff: opts.WholeWeekdaysOff,
		DailyStartTime:   opts.DailyStartTime,
		DailyStopTime:    opts.DailyStopTime,
		TimeZone:         opts.TimeZone,
	}

	nextStart, nextStop, err := schedule.GetNextScheduledStartAndStopTimes(time.Now())
	if err != nil {
		return nil, errors.Wrap(err, "getting next scheduled start and stop times")
	}

	schedule.NextStartTime = nextStart
	schedule.NextStopTime = nextStop

	return &schedule, nil
}

// Validate checks that the sleep schedule provided by the user is valid, or
// the host is permanently exempt.
func (i *SleepScheduleInfo) Validate() error {
	if i.PermanentlyExempt {
		return nil
	}

	opts := SleepScheduleOptions{
		WholeWeekdaysOff: i.WholeWeekdaysOff,
		DailyStartTime:   i.DailyStartTime,
		DailyStopTime:    i.DailyStopTime,
		TimeZone:         i.TimeZone,
	}

	return opts.Validate()
}

// isDailyOvernightSchedule returns whether the daily sleep schedule runs
// overnight. In other words, the host sleeps on one day, then starts up again
// on the next day.
func (i *SleepScheduleInfo) isDailyOvernightSchedule() bool {
	return i.DailyStartTime < i.DailyStopTime
}

func parseDailyTime(t string) (hours int, mins int, err error) {
	hoursMins := strings.Split(t, ":")
	if len(hoursMins) != 2 {
		return 0, 0, errors.Errorf("invalid daily time format '%s', expected HH:MM", t)
	}
	hours, err = strconv.Atoi(hoursMins[0])
	if err != nil {
		return 0, 0, errors.Wrap(err, "parsing hours")
	}
	mins, err = strconv.Atoi(hoursMins[1])
	if err != nil {
		return 0, 0, errors.Wrap(err, "parsing minutes")
	}
	return hours, mins, nil
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (i SleepScheduleInfo) IsZero() bool {
	return len(i.WholeWeekdaysOff) == 0 &&
		i.DailyStartTime == "" &&
		i.DailyStopTime == "" &&
		i.TimeZone == "" &&
		utility.IsZeroTime(i.NextStopTime) &&
		utility.IsZeroTime(i.NextStartTime) &&
		utility.IsZeroTime(i.TemporarilyExemptUntil) &&
		!i.PermanentlyExempt &&
		!i.ShouldKeepOff
}

type newParentsNeededParams struct {
	numExistingParents, numContainersNeeded, numExistingContainers, maxContainers int
}

type ContainersOnParents struct {
	ParentHost Host
	Containers []Host
}

type HostModifyOptions struct {
	AddInstanceTags            []Tag    `json:"add_instance_tags"`
	DeleteInstanceTags         []string `json:"delete_instance_tags"`
	InstanceType               string   `json:"instance_type"`
	NoExpiration               *bool    `json:"no_expiration"` // whether host should never expire
	SleepScheduleOptions       `bson:",inline"`
	AddHours                   time.Duration `json:"add_hours"` // duration to extend expiration
	AddTemporaryExemptionHours int           `json:"add_temporary_exemption_hours"`
	AttachVolume               string        `json:"attach_volume"`
	DetachVolume               string        `json:"detach_volume"`
	SubscriptionType           string        `json:"subscription_type"`
	NewName                    string        `json:"new_name"`
	AddKey                     string        `json:"add_key"`
}

// SleepScheduleOptions represent options that a user can set for creating a
// sleep schedule.
type SleepScheduleOptions struct {
	// WholeWeekdaysOff are when the host is off for its sleep schedule.
	WholeWeekdaysOff []time.Weekday `bson:"whole_weekdays_off,omitempty" json:"whole_weekdays_off,omitempty"`
	// DailyStartTime is the daily time to start the host for each day the host is on
	// during its sleep schedule. The format is "HH:MM".
	DailyStartTime string `bson:"daily_start_time,omitempty" json:"daily_start_time,omitempty"`
	// DailyStopTime is the daily time to stop the host for each day the host is
	// on during its sleep schedule. The format is "HH:MM".
	DailyStopTime string `bson:"daily_stop_time,omitempty" json:"daily_stop_time,omitempty"`
	// TimeZone is the time zone in which the sleep schedule runs.
	TimeZone string `bson:"time_zone" json:"time_zone"`
}

func (o *SleepScheduleOptions) IsZero() bool {
	return len(o.WholeWeekdaysOff) == 0 && o.DailyStartTime == "" && o.DailyStopTime == "" && o.TimeZone == ""
}

// HasSchedule returns whether a sleep schedule has been defined (with or
// without an explicit time zone).
func (o *SleepScheduleOptions) HasSchedule() bool {
	return len(o.WholeWeekdaysOff) > 0 || o.DailyStartTime != "" || o.DailyStopTime != ""
}

// SetDefaultSchedule sets a default sleep schedule if none is provided. This
// does not specify a default time zone, which must be set using
// SetDefaultTimeZone.
func (o *SleepScheduleOptions) SetDefaultSchedule() {
	if o.HasSchedule() {
		return
	}
	o.WholeWeekdaysOff = []time.Weekday{time.Saturday, time.Sunday}
	o.DailyStartTime = "08:00"
	o.DailyStopTime = "20:00"
}

// SetDefaultTimeZone sets a default time zone if other sleep schedule options
// are specified and the time zone is not explicitly set. If prefers the given
// default timezone if it's not empty, but otherwise falls back to using a
// default time zone.
func (o *SleepScheduleOptions) SetDefaultTimeZone(timezone string) {
	if o.IsZero() || o.TimeZone != "" {
		return
	}
	if timezone == "" {
		timezone = evergreen.DefaultSleepScheduleTimeZone
	}
	o.TimeZone = timezone
}

// Validate checks that the sleep schedule provided by the user is valid.
func (o *SleepScheduleOptions) Validate() error {
	catcher := grip.NewBasicCatcher()

	catcher.NewWhen(len(o.WholeWeekdaysOff) == 7, "cannot create a sleep schedule where the host is off 24/7 since the host could never turn on")
	catcher.NewWhen(len(o.WholeWeekdaysOff) == 0 && o.DailyStartTime == "" && o.DailyStopTime == "", "cannot specify an empty sleep schedule that's missing both daily stop/start time and whole days off")
	catcher.NewWhen(o.TimeZone == "", "sleep schedule time zone must be set")
	catcher.ErrorfWhen(o.DailyStopTime != "" && o.DailyStartTime == "", "cannot specify a daily stop time without a daily start time")
	catcher.ErrorfWhen(o.DailyStopTime == "" && o.DailyStartTime != "", "cannot specify a daily start time without a daily stop time")

	_, err := time.LoadLocation(o.TimeZone)
	catcher.Wrapf(err, "invalid time zone '%s'", o.TimeZone)

	if o.DailyStopTime != "" {
		catcher.Wrapf(o.validateDailyTime(o.DailyStopTime), "invalid daily stop time")
	}
	if o.DailyStartTime != "" {
		catcher.Wrapf(o.validateDailyTime(o.DailyStartTime), "invalid daily start time")
	}

	catcher.ErrorfWhen(o.DailyStartTime != "" && o.DailyStopTime != "" && o.DailyStartTime == o.DailyStopTime, "daily start time and daily stop time cannot be the same (%s)", o.DailyStartTime)

	uniqueWeekdays := map[time.Weekday]struct{}{}
	for _, weekday := range o.WholeWeekdaysOff {
		if _, ok := uniqueWeekdays[weekday]; ok {
			catcher.Errorf("weekday '%s' cannot be listed more than once in weekdays off", weekday.String())
		}
		catcher.ErrorfWhen(weekday < time.Sunday || weekday > time.Saturday, "weekday '%s' is not a valid day of the week", weekday.String())
		uniqueWeekdays[weekday] = struct{}{}
	}

	const minNumHoursPerWeek = utility.Day

	weeklySleep := utility.Day * time.Duration(len(o.WholeWeekdaysOff))
	if o.DailyStopTime != "" && o.DailyStartTime != "" {
		// Add up how much time is hypothetically spent sleeping for the daily
		// sleep schedule.
		const hoursMinsFmt = "15:04"
		sampleStart, err := time.Parse(hoursMinsFmt, o.DailyStartTime)
		catcher.Wrapf(err, "parsing daily start time as a timestamp")
		sampleStop, err := time.Parse(hoursMinsFmt, o.DailyStopTime)
		catcher.Wrapf(err, "parsing daily stop time as a timestamp")
		if !utility.IsZeroTime(sampleStart) && !utility.IsZeroTime(sampleStop) {
			var dailySleep time.Duration
			if o.isDailyOvernightSchedule() {
				dailySleep = utility.Day - sampleStop.Sub(sampleStart)
			} else {
				dailySleep = sampleStart.Sub(sampleStop)
			}
			catcher.ErrorfWhen(dailySleep < time.Hour, "daily sleep schedule runs for %s per day, which is less than the minimum of 1 hour", dailySleep.String())
			weeklySleep += dailySleep * time.Duration(len(utility.Weekdays)-len(o.WholeWeekdaysOff))
		}
	}
	catcher.ErrorfWhen(weeklySleep < minNumHoursPerWeek, "sleep schedule runs for %s, which does not meet minimum requirement of %s", weeklySleep.String(), minNumHoursPerWeek.String())
	return catcher.Resolve()
}

// isDailyOvernightSchedule returns whether the daily sleep schedule runs
// overnight. In other words, the host sleeps on one day, then starts up again
// on the next day.
func (o *SleepScheduleOptions) isDailyOvernightSchedule() bool {
	return o.DailyStartTime < o.DailyStopTime
}

func (o *SleepScheduleOptions) validateDailyTime(t string) error {
	hours, mins, err := parseDailyTime(t)
	if err != nil {
		return errors.Wrap(err, "parsing daily time format")
	}

	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(hours < 0 || hours >= 24, "hours must be between 0 and 23 (inclusive)")
	catcher.NewWhen(mins < 0 || mins >= 60, "minutes must be between 0 and 59 (inclusive)")

	return catcher.Resolve()
}

type SpawnHostUsage struct {
	TotalHosts            int `bson:"total_hosts"`
	TotalStoppedHosts     int `bson:"total_stopped_hosts"`
	TotalUnexpirableHosts int `bson:"total_unexpirable_hosts"`
	NumUsersWithHosts     int `bson:"num_users_with_hosts"`

	TotalVolumes        int            `bson:"total_volumes"`
	TotalVolumeSize     int            `bson:"total_volume_size"`
	NumUsersWithVolumes int            `bson:"num_users_with_volumes"`
	InstanceTypes       map[string]int `bson:"instance_types"`
}

const (
	// MaxAgentUnresponsiveInterval is the maximum amount of time that can elapse before the
	// agent is considered dead. Once it has been successfully started (e.g.
	// once an agent has been deployed from the server), the agent must
	// regularly contact the server to ensure it is still alive.
	MaxAgentUnresponsiveInterval = 5 * time.Minute
	// MaxAgentMonitorUnresponsiveInterval is the maximum amount of time that can elapse
	// before the agent monitor is considered dead. When the host is
	// provisioning, the agent must contact the app server within this duration
	// for the agent monitor to be alive (i.e. the agent monitor has
	// successfully started its job of starting and managing the agent). After
	// initial contact, the agent must regularly contact the server so that this
	// duration does not elapse. Otherwise, the agent monitor is considered dead
	// (because it has failed to keep an agent alive that can contact the
	// server).
	MaxAgentMonitorUnresponsiveInterval = 15 * time.Minute
	// MaxStaticHostUnresponsiveInterval is the maximum amount of time that can
	// elapse before a static host is considered unresponsive and unfixable. If
	// the host is running and healthy, the agent must regularly contact the app
	// server within this duration. Otherwise, the host will be considered
	// unhealthy and require manual intervention to investigate why it's
	// unresponsive.
	// This timeout is intentionally very conservative to mitigate false
	// positives when automatically detecting unhealthy static hosts.
	MaxStaticHostUnresponsiveInterval = 120 * time.Minute

	// provisioningCutoff is the threshold before a host is considered stuck in
	// provisioning.
	provisioningCutoff = 25 * time.Minute

	MaxTagKeyLength   = 128
	MaxTagValueLength = 256

	ErrorParentNotFound        = "parent not found"
	ErrorHostAlreadyTerminated = "not changing status of already terminated host"
)

func (h *Host) GetTaskGroupString() string {
	return fmt.Sprintf("%s_%s_%s_%s", h.RunningTaskGroup, h.RunningTaskBuildVariant, h.RunningTaskProject, h.RunningTaskVersion)
}

// IdleTime returns how long has this host has been sitting idle. In this context
// idle time means the duration the host has not been running a task even though it
// could have been. The time before the host was ready to run a task does not count
// as idle time because the host needs time to come up.
func (h *Host) IdleTime() time.Duration {
	// If the host is currently running a task, it is not idle.
	if h.RunningTask != "" {
		return 0
	}

	// If the host is currently tearing down a task group, it is not idle.
	if h.IsTearingDown() {
		return 0
	}

	// If the host has run a task it's been idle since that task finished running.
	if h.LastTask != "" {
		return time.Since(h.LastTaskCompletedTime)
	}

	// If the host never ran a task it's been idle since the time it was first ready
	// to run tasks.
	if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
		// User data hosts are ready to run tasks once they've asked for a task.
		if h.AgentStartTime.After(utility.ZeroTime) {
			return time.Since(h.AgentStartTime)
		}
	} else {
		// Other provisioning methods don't run tasks until they've been provisioned.
		if h.Status == evergreen.HostRunning {
			return time.Since(h.ProvisionTime)
		}
	}

	// The host isn't ready to run tasks yet, so it's not idle.
	return 0
}

// ShouldNotifyStoppedSpawnHostIdle returns true if the stopped spawn host has been idle long enough to notify the user.
func (h *Host) ShouldNotifyStoppedSpawnHostIdle() (bool, error) {
	if !h.NoExpiration || h.Status != evergreen.HostStopped {
		return false, nil
	}
	timeToNotifyForStoppedHosts := time.Now().Add(-time.Hour * 24 * evergreen.SpawnHostExpireDays * 3)
	return event.HasNoRecentStoppedHostEvent(h.Id, timeToNotifyForStoppedHosts)
}

// WastedComputeTime returns the duration of compute we've paid for that
// wasn't used to run a task. This is the duration since the last task to run on the host
// completed or, if no task has run, the host's uptime. The time the host is alive
// before it's able to run a task also counts as wasted compute.
func (h *Host) WastedComputeTime() time.Duration {
	// If the host has run a task, return the time the last task finished running.
	if h.LastTask != "" {
		return time.Since(h.LastTaskCompletedTime)
	}

	// If the host has not yet run a task, return how long it has been billable.
	if !utility.IsZeroTime(h.BillingStartTime) {
		return time.Since(h.BillingStartTime)
	}

	// If the host hasn't run a task and its billing start time is not set, return
	// how long it has been since the host was started.
	return time.Since(h.StartTime)
}

func (h *Host) TaskStartMessage() message.Fields {
	msg := message.Fields{
		"stat":                 "host-start-task",
		"distro":               h.Distro.Id,
		"provider":             h.Distro.Provider,
		"provisioning":         h.Distro.BootstrapSettings.Method,
		"host_id":              h.Id,
		"status":               h.Status,
		"since_last_task_secs": h.WastedComputeTime().Seconds(),
		"spawn_host":           h.StartedBy != evergreen.User && !h.SpawnOptions.SpawnedByTask,
		"task_spawn_host":      h.SpawnOptions.SpawnedByTask,
		"has_containers":       h.HasContainers,
		"task_host":            h.StartedBy == evergreen.User && !h.HasContainers,
	}

	if strings.HasPrefix(h.Distro.Provider, "ec2") {
		msg["provider"] = "ec2"
	}

	if h.Provider != evergreen.ProviderNameStatic {
		msg["host_task_count"] = h.TaskCount
	}

	return msg
}

func (h *Host) GetAMI() string {
	if len(h.Distro.ProviderSettingsList) == 0 {
		return ""
	}
	ami, _ := h.Distro.ProviderSettingsList[0].Lookup("ami").StringValueOK()
	return ami
}

// GetSubnetID returns the subnet ID for the host if available, otherwise returns the empty string.
func (h *Host) GetSubnetID() string {
	if len(h.Distro.ProviderSettingsList) == 0 {
		return ""
	}
	subnetID, _ := h.Distro.ProviderSettingsList[0].Lookup("subnet_id").StringValueOK()
	return subnetID
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

// CanUpdateSpawnHost is a shared utility function to determine a users permissions to modify a spawn host
func CanUpdateSpawnHost(h *Host, usr *user.DBUser) bool {
	if usr.Username() != h.StartedBy {
		return usr.HasPermission(gimlet.PermissionOpts{
			Resource:      h.Distro.Id,
			ResourceType:  evergreen.DistroResourceType,
			Permission:    evergreen.PermissionHosts,
			RequiredLevel: evergreen.HostsEdit.Value,
		})
	}
	return true
}

// IsIntentHostId returns whether or not the host ID is for an intent host
// backed by an ephemeral cloud host. This function does not work for intent
// hosts representing Docker containers.
func IsIntentHostId(id string) bool {
	return strings.HasPrefix(id, "evg-")
}

// SetStatus updates a host's status on behalf of the given user.
// Clears last running task for hosts that are being moved to running, since this information is likely outdated.
func (h *Host) SetStatus(ctx context.Context, newStatus, user, logs string) error {
	var unset bson.M
	if newStatus == evergreen.HostRunning {
		shouldKeepOffKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleShouldKeepOffKey)
		unset = bson.M{
			LTCTimeKey:       1,
			LTCTaskKey:       1,
			LTCGroupKey:      1,
			LTCBVKey:         1,
			LTCVersionKey:    1,
			LTCProjectKey:    1,
			shouldKeepOffKey: 1,
		}
	}

	return h.setStatusAndFields(ctx, newStatus, nil, nil, unset, user, logs)
}

// setStatusAndFields sets the status as well as any of the other given fields.
// Accepts fields to query in addition to host status.
func (h *Host) setStatusAndFields(ctx context.Context, newStatus string, query, setFields, unsetFields bson.M, user, logs string) error {
	if h.Status == newStatus {
		return nil
	}

	if h.Status == evergreen.HostTerminated && h.Provider != evergreen.ProviderNameStatic {
		msg := ErrorHostAlreadyTerminated
		grip.Warning(message.Fields{
			"message": msg,
			"host_id": h.Id,
			"status":  newStatus,
		})
		return errors.New(msg)
	}

	if query == nil {
		query = bson.M{}
	}
	query[IdKey] = h.Id

	if setFields == nil {
		setFields = bson.M{}
	}
	setFields[StatusKey] = newStatus

	update := bson.M{
		"$set": setFields,
	}
	if unsetFields != nil {
		update["$unset"] = unsetFields
	}

	if err := UpdateOne(
		ctx,
		query,
		update,
	); err != nil {
		return err
	}

	event.LogHostStatusChanged(h.Id, h.Status, newStatus, user, logs)
	grip.Info(message.Fields{
		"message":    "host status changed",
		"host_id":    h.Id,
		"host_tag":   h.Tag,
		"distro":     h.Distro.Id,
		"old_status": h.Status,
		"new_status": newStatus,
	})
	h.Status = newStatus

	return nil
}

// SetStatusAtomically is the same as SetStatus but only updates the host if its
// status in the database matches currentStatus.
func (h *Host) SetStatusAtomically(ctx context.Context, newStatus, user string, logs string) error {
	if h.Status == evergreen.HostTerminated && h.Provider != evergreen.ProviderNameStatic {
		msg := ErrorHostAlreadyTerminated
		grip.Warning(message.Fields{
			"message": msg,
			"host_id": h.Id,
			"status":  newStatus,
		})
		return errors.New(msg)
	}

	err := UpdateOne(
		ctx,
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
		return errors.WithStack(err)
	}

	event.LogHostStatusChanged(h.Id, h.Status, newStatus, user, logs)
	grip.Info(message.Fields{
		"message":    "host status changed atomically",
		"host_id":    h.Id,
		"host_tag":   h.Tag,
		"distro":     h.Distro.Id,
		"old_status": h.Status,
		"new_status": newStatus,
	})
	h.Status = newStatus

	return nil
}

// SetProvisioning marks the host as initializing. Only allow this
// if the host is uninitialized.
func (h *Host) SetProvisioning(ctx context.Context) error {
	return UpdateOne(
		ctx,
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

// SetDecommissioned sets the host as decommissioned. If decommissionIfBusy
// isn't set, we only update the host if there is no running task.
func (h *Host) SetDecommissioned(ctx context.Context, user string, decommissionIfBusy bool, logs string) error {
	query := bson.M{}
	if !decommissionIfBusy {
		query[RunningTaskKey] = bson.M{"$exists": false}
	}
	if h.HasContainers {
		containers, err := h.GetContainers(ctx)
		grip.Error(message.WrapError(err, message.Fields{
			"message": "error getting containers",
			"host_id": h.Id,
		}))
		catcher := grip.NewBasicCatcher()
		failedContainerIds := []string{}
		for _, c := range containers {
			err = c.setStatusAndFields(ctx, evergreen.HostDecommissioned, query, nil, nil, user, "parent is being decommissioned")
			if err != nil && err.Error() != ErrorHostAlreadyTerminated {
				catcher.Add(err)
				failedContainerIds = append(failedContainerIds, c.Id)
			}
		}
		grip.Warning(message.WrapError(catcher.Resolve(), message.Fields{
			"message":  "error decommissioning containers",
			"host_ids": failedContainerIds,
		}))
	}
	err := h.setStatusAndFields(ctx, evergreen.HostDecommissioned, query, nil, nil, user, logs)
	// Shouldn't consider it an error if the host isn't found when checking task group,
	// because a task group may have been set for the host.
	if err != nil && !decommissionIfBusy && adb.ResultsNotFound(err) {
		return nil
	}
	return err
}

func (h *Host) SetRunning(ctx context.Context, user string) error {
	return h.SetStatus(ctx, evergreen.HostRunning, user, "")
}

func (h *Host) SetTerminated(ctx context.Context, user, reason string) error {
	return h.SetStatus(ctx, evergreen.HostTerminated, user, reason)
}

func (h *Host) SetStopping(ctx context.Context, user string) error {
	return h.SetStatus(ctx, evergreen.HostStopping, user, "")
}

// SetStopped sets a host to stopped. If shouldKeepOff is true, it will also
// keep the host off and ignore any sleep schedule until it's started up again
// by a user.
func (h *Host) SetStopped(ctx context.Context, shouldKeepOff bool, user string) error {
	setFields := bson.M{
		StatusKey:    evergreen.HostStopped,
		DNSKey:       "",
		StartTimeKey: utility.ZeroTime,
	}
	unsetFields := bson.M{}
	if shouldKeepOff {
		shouldKeepOffKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleShouldKeepOffKey)
		setFields[shouldKeepOffKey] = true

		sleepScheduleNextStartKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStartTimeKey)
		sleepScheduleNextStopKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStopTimeKey)
		unsetFields[sleepScheduleNextStartKey] = 1
		unsetFields[sleepScheduleNextStopKey] = 1
	}
	update := bson.M{"$set": setFields}
	if len(unsetFields) > 0 {
		update["$unset"] = unsetFields
	}

	err := UpdateOne(
		ctx,
		bson.M{
			IdKey:     h.Id,
			StatusKey: h.Status,
		},
		update,
	)
	if err != nil {
		return errors.Wrap(err, "setting host status to stopped")
	}

	event.LogHostStatusChanged(h.Id, h.Status, evergreen.HostStopped, user, "")
	grip.Info(message.Fields{
		"message":    "host stopped",
		"host_id":    h.Id,
		"host_tag":   h.Tag,
		"distro":     h.Distro.Id,
		"old_status": h.Status,
	})

	h.Status = evergreen.HostStopped
	h.Host = ""
	h.StartTime = utility.ZeroTime
	h.SleepSchedule.ShouldKeepOff = shouldKeepOff
	if shouldKeepOff {
		h.SleepSchedule.NextStartTime = time.Time{}
		h.SleepSchedule.NextStopTime = time.Time{}
	}

	return nil
}

func (h *Host) SetUnprovisioned(ctx context.Context) error {
	if err := UpdateOne(
		ctx,
		bson.M{
			IdKey:     h.Id,
			StatusKey: h.Status,
		},
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostProvisionFailed,
			},
		},
	); err != nil {
		return errors.WithStack(err)
	}

	h.Status = evergreen.HostProvisionFailed

	return nil
}

func (h *Host) SetQuarantined(ctx context.Context, user string, logs string) error {
	return h.SetStatus(ctx, evergreen.HostQuarantined, user, logs)
}

// CreateSecret generates a host secret and updates the host both locally
// and in the database, with the option to clear the secret rather than
// set a new one.
func (h *Host) CreateSecret(ctx context.Context, clear bool) error {
	secret := utility.RandomString()
	if clear {
		secret = ""
	}
	err := UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{SecretKey: secret}},
	)
	if err != nil {
		return err
	}
	h.Secret = secret
	return nil
}

func (h *Host) SetBillingStartTime(ctx context.Context, startTime time.Time) error {
	if err := UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{BillingStartTimeKey: startTime}},
	); err != nil {
		return errors.Wrap(err, "setting billing start time")
	}
	h.BillingStartTime = startTime
	return nil
}

func (h *Host) SetAgentStartTime(ctx context.Context) error {
	now := time.Now()
	if err := UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{AgentStartTimeKey: now}},
	); err != nil {
		return errors.Wrap(err, "setting agent start time")
	}
	h.AgentStartTime = now
	return nil
}

// SetTaskGroupTeardownStartTime sets the TaskGroupTeardownStartTime to the current time for the host
func (h *Host) SetTaskGroupTeardownStartTime(ctx context.Context) error {
	now := time.Now()
	if err := UpdateOne(ctx, bson.M{
		IdKey: h.Id,
	}, bson.M{
		"$set": bson.M{
			TaskGroupTeardownStartTimeKey: now,
		},
	}); err != nil {
		return err
	}

	h.TaskGroupTeardownStartTime = now
	return nil
}

// UnsetTaskGroupTeardownStartTime unsets the TaskGroupTeardownStartTime for the host.
func (h *Host) UnsetTaskGroupTeardownStartTime(ctx context.Context) error {
	if h.TaskGroupTeardownStartTime.IsZero() {
		return nil
	}

	if err := UpdateOne(ctx, bson.M{
		IdKey: h.Id,
	}, bson.M{
		"$unset": bson.M{
			TaskGroupTeardownStartTimeKey: 1,
		},
	}); err != nil {
		return err
	}

	h.TaskGroupTeardownStartTime = time.Time{}

	return nil
}

// JasperCredentials gets the Jasper credentials for this host's running Jasper
// service from the database. These credentials should not be used to connect to
// the Jasper service - use JasperClientCredentials for this purpose.
func (h *Host) JasperCredentials(ctx context.Context, env evergreen.Environment) (*certdepot.Credentials, error) {
	creds, err := env.CertificateDepot().Find(h.JasperCredentialsID)
	if err != nil {
		return nil, errors.Wrap(err, "finding host Jasper credentials")
	}

	return creds, nil
}

// JasperCredentialsExpiration returns the time at which the host's Jasper
// credentials will expire.
func (h *Host) JasperCredentialsExpiration(ctx context.Context, env evergreen.Environment) (time.Time, error) {
	user := &certdepot.User{}
	if err := env.DB().Collection(evergreen.CredentialsCollection).FindOne(ctx, bson.M{IdKey: h.JasperCredentialsID}).Decode(user); err != nil {
		return time.Time{}, errors.Wrap(err, "finding host Jasper credentials")
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
		return nil, errors.Wrapf(err, "finding client credentials '%s' for host '%s'", id, h.Id)
	}
	creds.ServerName = h.JasperCredentialsID
	return creds, nil
}

// GenerateJasperCredentials creates the Jasper credentials for the given host
// without saving them to the database. If credentials already exist in the
// database, they are deleted.
func (h *Host) GenerateJasperCredentials(ctx context.Context, env evergreen.Environment) (*certdepot.Credentials, error) {
	if h.JasperCredentialsID == "" {
		if err := h.UpdateJasperCredentialsID(ctx, h.Id); err != nil {
			return nil, errors.Wrap(err, "setting Jasper credentials ID")
		}
	}
	// We have to delete this host's credentials because GenerateInMemory will
	// fail if credentials already exist in the database.
	if err := h.DeleteJasperCredentials(ctx, env); err != nil {
		return nil, errors.Wrap(err, "deleting existing Jasper credentials")
	}
	addToSet := func(set []string, val string) []string {
		if utility.StringSliceContains(set, val) {
			return set
		}
		return append(set, val)
	}

	domains := []string{h.JasperCredentialsID}
	var ipAddrs []string
	if net.ParseIP(h.JasperCredentialsID) != nil {
		ipAddrs = addToSet(ipAddrs, h.JasperCredentialsID)
	}
	if h.Host != "" {
		if net.ParseIP(h.Host) != nil {
			ipAddrs = addToSet(ipAddrs, h.Host)
		} else {
			domains = addToSet(domains, h.Host)
		}
	}
	if h.IP != "" && net.ParseIP(h.IP) != nil {
		ipAddrs = addToSet(ipAddrs, h.IP)
	}
	opts := certdepot.CertificateOptions{
		CommonName: h.JasperCredentialsID,
		Host:       h.JasperCredentialsID,
		Domain:     domains,
		IP:         ipAddrs,
		CA:         evergreen.CAName,
	}
	creds, err := env.CertificateDepot().GenerateWithOptions(opts)
	if err != nil {
		return nil, errors.Wrapf(err, "generating Jasper credentials for host '%s'", h.JasperCredentialsID)
	}

	return creds, nil
}

// SaveJasperCredentials saves the given Jasper credentials in the database for
// the host.
func (h *Host) SaveJasperCredentials(ctx context.Context, env evergreen.Environment, creds *certdepot.Credentials) error {
	if h.JasperCredentialsID == "" {
		return errors.New("Jasper credentials ID is empty")
	}
	return errors.Wrapf(env.CertificateDepot().Save(h.JasperCredentialsID, creds), "finding host Jasper credentials '%s'", h.JasperCredentialsID)
}

// DeleteJasperCredentials deletes the Jasper credentials for the host and
// updates the host both in memory and in the database.
func (h *Host) DeleteJasperCredentials(ctx context.Context, env evergreen.Environment) error {
	_, err := env.DB().Collection(evergreen.CredentialsCollection).DeleteOne(ctx, bson.M{"_id": h.JasperCredentialsID})
	return errors.Wrapf(err, "deleting host Jasper credentials for '%s'", h.JasperCredentialsID)
}

// UpdateJasperCredentialsID sets the ID of the host's Jasper credentials.
func (h *Host) UpdateJasperCredentialsID(ctx context.Context, id string) error {
	if err := UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{JasperCredentialsIDKey: id}},
	); err != nil {
		return err
	}
	h.JasperCredentialsID = id
	return nil
}

// UpdateLastCommunicated sets the host's last communication time to the current time.
func (h *Host) UpdateLastCommunicated(ctx context.Context) error {
	now := time.Now()
	err := UpdateOne(
		ctx,
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
func (h *Host) ResetLastCommunicated(ctx context.Context) error {
	err := UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{LastCommunicationTimeKey: time.Unix(0, 0)}})
	if err != nil {
		return err
	}
	h.LastCommunicationTime = time.Unix(0, 0)
	return nil
}

// Terminate marks a host as terminated, sets the termination time, and unsets
// the host volumes.
func (h *Host) Terminate(ctx context.Context, user, reason string) error {
	terminatedAt := time.Now()
	if err := h.setStatusAndFields(ctx, evergreen.HostTerminated, nil, bson.M{
		TerminationTimeKey: terminatedAt,
		VolumesKey:         nil,
	}, nil, user, reason); err != nil {
		return err
	}

	h.TerminationTime = terminatedAt
	h.Volumes = nil

	return nil
}

// SetDNSName updates the DNS name for a given host once
func (h *Host) SetDNSName(ctx context.Context, dnsName string) error {
	err := UpdateOne(
		ctx,
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
		grip.Info(message.Fields{
			"message":  "set host DNS name",
			"host_id":  h.Id,
			"host_tag": h.Tag,
			"dns_name": dnsName,
		})
	}
	if adb.ResultsNotFound(err) {
		return nil
	}
	return err
}

// probably don't want to store the port mapping exactly this way
func (h *Host) SetPortMapping(ctx context.Context, portsMap PortMap) error {
	err := UpdateOne(
		ctx,
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

func (h *Host) UpdateCachedDistroProviderSettings(ctx context.Context, settingsDocuments []*birch.Document) error {
	err := UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{
			"$set": bson.M{
				bsonutil.GetDottedKeyName(DistroKey, distro.ProviderSettingsListKey): settingsDocuments,
			},
		},
	)
	if err != nil {
		return errors.Wrap(err, "updating distro provider settings")
	}

	h.Distro.ProviderSettingsList = settingsDocuments
	return nil
}

func (h *Host) MarkAsProvisioned(ctx context.Context) error {
	now := time.Now()
	err := UpdateOne(
		ctx,
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

	event.LogHostProvisioned(h.Id)
	grip.Info(message.Fields{
		"message":    "host marked provisioned",
		"host_id":    h.Id,
		"host_tag":   h.Tag,
		"distro":     h.Distro.Id,
		"old_status": h.Status,
		"operation":  "MarkAsProvisioned",
	})

	h.Status = evergreen.HostRunning
	h.Provisioned = true
	h.ProvisionTime = now

	return nil
}

// SetProvisionedNotRunning marks the host as having been provisioned by the app
// server but the host is not necessarily running yet.
func (h *Host) SetProvisionedNotRunning(ctx context.Context) error {
	now := time.Now()
	if err := UpdateOne(
		ctx,
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
		return errors.Wrap(err, "setting host as provisioned but not running")
	}

	h.Provisioned = true
	h.ProvisionTime = now
	return nil
}

// UpdateStartingToRunning changes the host status from provisioning to
// running, as well as logging that the host has finished provisioning.
func (h *Host) UpdateStartingToRunning(ctx context.Context) error {
	if h.Status != evergreen.HostStarting {
		return nil
	}

	if err := UpdateOne(
		ctx,
		bson.M{
			IdKey:          h.Id,
			StatusKey:      evergreen.HostStarting,
			ProvisionedKey: true,
		},
		bson.M{"$set": bson.M{StatusKey: evergreen.HostRunning}},
	); err != nil {
		return errors.Wrap(err, "changing host status from starting to running")
	}

	h.Status = evergreen.HostRunning

	event.LogHostProvisioned(h.Id)
	grip.Info(message.Fields{
		"message":   "host marked provisioned",
		"host_id":   h.Id,
		"host_tag":  h.Tag,
		"distro":    h.Distro.Id,
		"operation": "UpdateStartingToRunning",
	})

	return nil
}

// SetNeedsToRestartJasper sets this host as needing to have its Jasper service
// restarted as long as the host does not already need a different
// reprovisioning change. If the host is ready to reprovision now (i.e. no agent
// monitor is running), it is put in the reprovisioning state.
func (h *Host) SetNeedsToRestartJasper(ctx context.Context, user string) error {
	// Ignore hosts that are not provisioned by us (e.g. hosts spawned by
	// tasks) and legacy SSH hosts.
	if utility.StringSliceContains([]string{distro.BootstrapMethodNone, distro.BootstrapMethodLegacySSH}, h.Distro.BootstrapSettings.Method) {
		return nil
	}

	if err := h.setAwaitingJasperRestart(ctx, user); err != nil {
		return err
	}
	if h.StartedBy == evergreen.User && !h.NeedsNewAgentMonitor {
		return nil
	}
	return h.MarkAsReprovisioning(ctx)
}

// setAwaitingJasperRestart marks a host as needing Jasper to be restarted by
// the given user but is not yet ready to reprovision now (i.e. the agent
// monitor is still running).
func (h *Host) setAwaitingJasperRestart(ctx context.Context, user string) error {
	// While these checks aren't strictly necessary since they're already
	// filtered in the query, they do return more useful error messages.

	if h.NeedsReprovision == ReprovisionRestartJasper {
		return nil
	}
	if h.NeedsReprovision != ReprovisionNone {
		return errors.Errorf("cannot restart Jasper when host is already reprovisioning")
	}

	allowedStatuses := []string{evergreen.HostProvisioning, evergreen.HostRunning}
	if !utility.StringSliceContains(allowedStatuses, h.Status) {
		return errors.Errorf("cannot restart Jasper when host status is '%s'", h.Status)
	}
	allowedBootstrapMethods := []string{distro.BootstrapMethodSSH, distro.BootstrapMethodUserData}
	if !utility.StringSliceContains(allowedBootstrapMethods, h.Distro.BootstrapSettings.Method) {
		return errors.Errorf("cannot restart Jasper when host is provisioned as '%s'", h.Distro.BootstrapSettings.Method)
	}

	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	if err := UpdateOne(ctx, bson.M{
		IdKey:        h.Id,
		StatusKey:    bson.M{"$in": allowedStatuses},
		bootstrapKey: bson.M{"$in": allowedBootstrapMethods},
		"$or": []bson.M{
			{NeedsReprovisionKey: bson.M{"$exists": false}},
			{NeedsReprovisionKey: ReprovisionRestartJasper},
		},
		HasContainersKey: bson.M{"$ne": true},
		ParentIDKey:      bson.M{"$exists": false},
	}, bson.M{
		"$set": bson.M{
			NeedsReprovisionKey: ReprovisionRestartJasper,
		},
	}); err != nil {
		return err
	}

	event.LogHostJasperRestarting(h.Id, user)
	grip.Info(message.Fields{
		"message":               "set needs reprovision",
		"host_id":               h.Id,
		"host_tag":              h.Tag,
		"distro":                h.Distro.Id,
		"provider":              h.Provider,
		"old_needs_reprovision": h.NeedsReprovision,
		"new_needs_reprovision": ReprovisionRestartJasper,
	})

	h.NeedsReprovision = ReprovisionRestartJasper

	return nil
}

// SetNeedsReprovisionToNew marks a host that is currently using new
// provisioning to provision again.
func (h *Host) SetNeedsReprovisionToNew(ctx context.Context, user string) error {
	// Ignore hosts that are not provisioned by us (e.g. hosts spawned by
	// tasks) and legacy SSH hosts.
	if utility.StringSliceContains([]string{distro.BootstrapMethodNone, distro.BootstrapMethodLegacySSH}, h.Distro.BootstrapSettings.Method) {
		return nil
	}

	if err := h.setAwaitingReprovisionToNew(ctx, user); err != nil {
		return err
	}
	if h.StartedBy == evergreen.User && !h.NeedsNewAgentMonitor {
		return nil
	}

	return h.MarkAsReprovisioning(ctx)
}

func (h *Host) setAwaitingReprovisionToNew(ctx context.Context, user string) error {
	// While these checks aren't strictly necessary since they're already
	// filtered in the query, they do return more useful error messages.

	switch h.NeedsReprovision {
	case ReprovisionToNew:
		return nil
	case ReprovisionToLegacy:
		return errors.New("cannot reprovision a host that is already converting to legacy provisioning")
	}

	allowedStatuses := []string{evergreen.HostProvisioning, evergreen.HostRunning}
	if !utility.StringSliceContains(allowedStatuses, h.Status) {
		return errors.Errorf("cannot convert provisioning when host status is '%s'", h.Status)
	}

	allowedBootstrapMethods := []string{distro.BootstrapMethodSSH, distro.BootstrapMethodUserData}
	if !utility.StringSliceContains(allowedBootstrapMethods, h.Distro.BootstrapSettings.Method) {
		return errors.Errorf("cannot convert provisioning when host is provisioned as '%s'", h.Distro.BootstrapSettings.Method)
	}

	bootstrapKey := bsonutil.GetDottedKeyName(DistroKey, distro.BootstrapSettingsKey, distro.BootstrapSettingsMethodKey)
	if err := UpdateOne(ctx, bson.M{
		IdKey:        h.Id,
		StatusKey:    bson.M{"$in": allowedStatuses},
		bootstrapKey: bson.M{"$in": allowedBootstrapMethods},
		"$or": []bson.M{
			{NeedsReprovisionKey: bson.M{"$exists": false}},
			{NeedsReprovisionKey: ReprovisionToNew},
		},
		HasContainersKey: bson.M{"$ne": true},
		ParentIDKey:      bson.M{"$exists": false},
	}, bson.M{
		"$set": bson.M{
			NeedsReprovisionKey: ReprovisionToNew,
		},
	}); err != nil {
		return err
	}

	event.LogHostConvertingProvisioning(h.Id, h.Distro.BootstrapSettings.Method, user)
	grip.Info(message.Fields{
		"message":               "set needs reprovision",
		"host_id":               h.Id,
		"host_tag":              h.Tag,
		"distro":                h.Distro.Id,
		"provider":              h.Provider,
		"user":                  user,
		"old_needs_reprovision": h.NeedsReprovision,
		"new_needs_reprovision": ReprovisionToNew,
	})

	h.NeedsReprovision = ReprovisionToNew

	return nil
}

// MarkAsReprovisioning puts the host in a state that means it is ready to be
// reprovisioned immediately.
func (h *Host) MarkAsReprovisioning(ctx context.Context) error {
	allowedStatuses := []string{evergreen.HostProvisioning, evergreen.HostRunning}
	if !utility.StringSliceContains(allowedStatuses, h.Status) {
		return errors.Errorf("cannot reprovision a host when host status is '%s'", h.Status)
	}

	var needsAgent bool
	var needsAgentMonitor bool
	switch h.NeedsReprovision {
	case ReprovisionToLegacy:
		needsAgent = true
	case ReprovisionToNew:
		needsAgentMonitor = true
	case ReprovisionRestartJasper:
		needsAgentMonitor = h.StartedBy == evergreen.User
	}

	now := time.Now()
	err := UpdateOne(ctx, bson.M{
		IdKey:               h.Id,
		NeedsReprovisionKey: h.NeedsReprovision,
		StatusKey:           bson.M{"$in": allowedStatuses},
	},
		bson.M{
			"$set": bson.M{
				AgentStartTimeKey:        utility.ZeroTime,
				ProvisionedKey:           false,
				StatusKey:                evergreen.HostProvisioning,
				NeedsNewAgentKey:         needsAgent,
				NeedsNewAgentMonitorKey:  needsAgentMonitor,
				LastCommunicationTimeKey: now,
			},
		},
	)
	if err != nil {
		return errors.Wrap(err, "marking host as reprovisioning")
	}

	h.AgentStartTime = utility.ZeroTime
	h.Provisioned = false
	h.Status = evergreen.HostProvisioning
	h.NeedsNewAgent = needsAgent
	h.NeedsNewAgentMonitor = needsAgentMonitor
	h.LastCommunicationTime = now

	return nil
}

// MarkAsReprovisioned indicates that the host was successfully reprovisioned.
func (h *Host) MarkAsReprovisioned(ctx context.Context) error {
	now := time.Now()
	err := UpdateOne(
		ctx,
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
func (h *Host) ClearRunningAndSetLastTask(ctx context.Context, t *task.Task) error {
	now := time.Now()
	err := UpdateOne(
		ctx,
		bson.M{
			IdKey:                   h.Id,
			RunningTaskKey:          h.RunningTask,
			RunningTaskExecutionKey: h.RunningTaskExecution,
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
				RunningTaskExecutionKey:    1,
				RunningTaskGroupKey:        1,
				RunningTaskGroupOrderKey:   1,
				RunningTaskBuildVariantKey: 1,
				RunningTaskVersionKey:      1,
				RunningTaskProjectKey:      1,
				NumAgentCleanupFailuresKey: 1,
			},
		})

	if err != nil {
		return err
	}

	event.LogHostRunningTaskCleared(h.Id, h.RunningTask, h.RunningTaskExecution)
	grip.Info(message.Fields{
		"message":         "cleared host running task and set last task",
		"host_id":         h.Id,
		"host_tag":        h.Tag,
		"distro":          h.Distro.Id,
		"running_task_id": h.RunningTask,
		"task_execution":  h.RunningTaskExecution,
		"last_task_id":    t.Id,
	})

	h.RunningTask = ""
	h.RunningTaskExecution = 0
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

// ClearRunningTask unsets the running task on the host and logs an event
// indicating it is no longer running the task.
func (h *Host) ClearRunningTask(ctx context.Context) error {
	hadRunningTask := h.RunningTask != ""
	doUpdate := func(update bson.M) error {
		return UpdateOne(ctx, bson.M{IdKey: h.Id}, update)
	}
	if err := h.clearRunningTaskWithFunc(doUpdate); err != nil {
		return err
	}

	if hadRunningTask {
		event.LogHostRunningTaskCleared(h.Id, h.RunningTask, h.RunningTaskExecution)
		grip.Info(message.Fields{
			"message":        "cleared host running task",
			"host_id":        h.Id,
			"host_tag":       h.Tag,
			"distro":         h.Distro.Id,
			"task_id":        h.RunningTask,
			"task_execution": h.RunningTaskExecution,
		})
	}

	return nil
}

// ClearRunningTaskWithContext unsets the running task on the log. It does not
// log an event for clearing the task.
func (h *Host) ClearRunningTaskWithContext(ctx context.Context, env evergreen.Environment) error {
	doUpdate := func(update bson.M) error {
		_, err := env.DB().Collection(Collection).UpdateByID(ctx, h.Id, update)
		return err
	}
	return h.clearRunningTaskWithFunc(doUpdate)
}

func (h *Host) clearRunningTaskWithFunc(doUpdate func(update bson.M) error) error {
	if h.RunningTask == "" {
		return nil
	}

	update := bson.M{
		"$unset": bson.M{
			RunningTaskKey:             1,
			RunningTaskExecutionKey:    1,
			RunningTaskGroupKey:        1,
			RunningTaskGroupOrderKey:   1,
			RunningTaskBuildVariantKey: 1,
			RunningTaskVersionKey:      1,
			RunningTaskProjectKey:      1,
			NumAgentCleanupFailuresKey: 1,
		},
	}
	if err := doUpdate(update); err != nil {
		return err
	}

	h.RunningTask = ""
	h.RunningTaskExecution = 0
	h.RunningTaskBuildVariant = ""
	h.RunningTaskVersion = ""
	h.RunningTaskProject = ""
	h.RunningTaskGroup = ""
	h.RunningTaskGroupOrder = 0

	return nil
}

// UpdateRunningTaskWithContext updates the running task for the host. It does
// not log an event for task assignment.
func (h *Host) UpdateRunningTaskWithContext(ctx context.Context, env evergreen.Environment, t *task.Task) error {
	if t == nil {
		return errors.New("received nil task, cannot update")
	}
	if t.Id == "" {
		return errors.New("task has empty task ID, cannot update")
	}

	statuses := []string{evergreen.HostRunning}
	// User data can start anytime after the instance is created, so the app
	// server may not have marked it as running yet.
	if h.Distro.BootstrapSettings.Method == distro.BootstrapMethodUserData {
		statuses = append(statuses, evergreen.HostStarting)
	}
	query := bson.M{
		IdKey:          h.Id,
		StatusKey:      bson.M{"$in": statuses},
		RunningTaskKey: bson.M{"$exists": false},
	}

	update := bson.M{
		"$set": bson.M{
			RunningTaskKey:             t.Id,
			RunningTaskExecutionKey:    t.Execution,
			RunningTaskGroupKey:        t.TaskGroup,
			RunningTaskGroupOrderKey:   t.TaskGroupOrder,
			RunningTaskBuildVariantKey: t.BuildVariant,
			RunningTaskVersionKey:      t.Version,
			RunningTaskProjectKey:      t.Project,
		},
	}

	res, err := env.DB().Collection(Collection).UpdateOne(ctx, query, update)
	if err != nil {
		grip.DebugWhen(db.IsDuplicateKey(err), message.WrapError(err, message.Fields{
			"message": "found duplicate running task",
			"task":    t.Id,
			"host_id": h.Id,
		}))
		return err
	}
	if res.ModifiedCount != 1 {
		return errors.Errorf("expected to update 1 host to set running task information, but actually updated %d", res.ModifiedCount)
	}

	h.RunningTask = t.Id
	h.RunningTaskExecution = t.Execution
	h.RunningTaskGroup = t.TaskGroup
	h.RunningTaskGroupOrder = t.TaskGroupOrder
	h.RunningTaskBuildVariant = t.BuildVariant
	h.RunningTaskVersion = t.Version
	h.RunningTaskProject = t.Project

	return nil
}

// SetAgentRevision sets the updated agent revision for the host
func (h *Host) SetAgentRevision(ctx context.Context, agentRevision string) error {
	err := UpdateOne(ctx, bson.M{IdKey: h.Id},
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

	if h.Distro.LegacyBootstrap() && h.LastCommunicationTime.Before(time.Now().Add(-MaxAgentUnresponsiveInterval)) {
		return true
	}
	if !h.Distro.LegacyBootstrap() && h.LastCommunicationTime.Before(time.Now().Add(-MaxAgentMonitorUnresponsiveInterval)) {
		return true
	}

	return false
}

// SetNeedsNewAgent sets the "needs new agent" flag on the host.
func (h *Host) SetNeedsNewAgent(ctx context.Context, needsAgent bool) error {
	err := UpdateOne(ctx, bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{NeedsNewAgentKey: needsAgent}})
	if err != nil {
		return err
	}
	h.NeedsNewAgent = needsAgent
	return nil
}

// SetNeedsNewAgentMonitor sets the "needs new agent monitor" flag on the host
// to indicate that the host needs to have the agent monitor deployed.
func (h *Host) SetNeedsNewAgentMonitor(ctx context.Context, needsAgentMonitor bool) error {
	err := UpdateOne(ctx, bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{NeedsNewAgentMonitorKey: needsAgentMonitor}})
	if err != nil {
		return err
	}
	h.NeedsNewAgentMonitor = needsAgentMonitor
	return nil
}

// SetNeedsAgentDeploy indicates that the host's agent or agent monitor needs
// to be deployed.
func (h *Host) SetNeedsAgentDeploy(ctx context.Context, needsDeploy bool) error {
	if !h.Distro.LegacyBootstrap() {
		if err := h.SetNeedsNewAgentMonitor(ctx, needsDeploy); err != nil {
			return errors.Wrap(err, "setting host needs new agent monitor")
		}
	}
	return errors.Wrap(h.SetNeedsNewAgent(ctx, needsDeploy), "setting host needs new agent")
}

// SetExpirationTime updates the expiration time of a spawn host
func (h *Host) SetExpirationTime(ctx context.Context, expirationTime time.Time) error {
	// update the in-memory host, then the database
	h.ExpirationTime = expirationTime
	h.NoExpiration = false
	return UpdateOne(
		ctx,
		bson.M{
			IdKey: h.Id,
		},
		bson.M{
			"$set": bson.M{
				ExpirationTimeKey: expirationTime,
				NoExpirationKey:   false,
			},
		},
	)
}

func (h *Host) MarkReachable(ctx context.Context) error {
	if h.Status == evergreen.HostRunning {
		return nil
	}

	if err := UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{StatusKey: evergreen.HostRunning}}); err != nil {
		return errors.WithStack(err)
	}

	event.LogHostStatusChanged(h.Id, h.Status, evergreen.HostRunning, evergreen.User, "")
	grip.Info(message.Fields{
		"message":    "host marked reachable",
		"host_id":    h.Id,
		"host_tag":   h.Tag,
		"distro":     h.Distro.Id,
		"old_status": h.Status,
	})
	h.Status = evergreen.HostRunning

	return nil
}

func (h *Host) Upsert(ctx context.Context) (*mongo.UpdateResult, error) {
	setFields := bson.M{
		// If adding or removing fields here, make sure that all callers will work
		// correctly after the change. Any fields defined here but not set by the
		// caller will insert the zero value into the document
		DNSKey:              h.Host,
		UserKey:             h.User,
		UserHostKey:         h.UserHost,
		DistroKey:           h.Distro,
		StartedByKey:        h.StartedBy,
		ExpirationTimeKey:   h.ExpirationTime,
		ProviderKey:         h.Provider,
		TagKey:              h.Tag,
		InstanceTypeKey:     h.InstanceType,
		ZoneKey:             h.Zone,
		ProvisionedKey:      h.Provisioned,
		ProvisionOptionsKey: h.ProvisionOptions,
		StatusKey:           h.Status,
		StartTimeKey:        h.StartTime,
		HasContainersKey:    h.HasContainers,
		ContainerImagesKey:  h.ContainerImages,
	}
	unsetFields := bson.M{}
	if h.NeedsReprovision != ReprovisionNone {
		setFields[NeedsReprovisionKey] = h.NeedsReprovision
	} else {
		unsetFields[NeedsReprovisionKey] = true
	}
	if h.SSHPort != 0 {
		setFields[SSHPortKey] = h.SSHPort
	} else {
		unsetFields[SSHPortKey] = true
	}
	update := bson.M{
		"$setOnInsert": bson.M{
			CreateTimeKey: h.CreationTime,
		},
		"$set": setFields,
	}
	if len(unsetFields) != 0 {
		update["$unset"] = unsetFields
	}

	return UpsertOne(ctx, bson.M{IdKey: h.Id}, update)
}

// CloudProviderData represents data to cache in the host from its cloud
// provider.
type CloudProviderData struct {
	Zone        string
	StartedAt   time.Time
	PublicDNS   string
	PublicIPv4  string
	PrivateIPv4 string
	IPv6        string
	Volumes     []VolumeAttachment
}

// CacheAllCloudProviderData performs the same updates as
// (Host).CacheCloudProviderData but is optimized for updating many hosts at
// once.
func CacheAllCloudProviderData(ctx context.Context, env evergreen.Environment, hosts map[string]CloudProviderData) error {
	updates := make([]mongo.WriteModel, 0, len(hosts))
	for hostID, data := range hosts {
		filter := bson.M{IdKey: hostID}
		update := cacheCloudProviderDataUpdate(data)
		updates = append(updates, mongo.NewUpdateOneModel().SetFilter(filter).SetUpdate(update))
	}
	if len(updates) == 0 {
		return nil
	}
	_, err := env.DB().Collection(Collection).BulkWrite(ctx, updates, options.BulkWrite().SetOrdered(false))
	return err
}

// cacheCloudProviderDataUpdate returns an update for caching cloud provider
// data.
func cacheCloudProviderDataUpdate(data CloudProviderData) bson.M {
	return bson.M{
		"$set": bson.M{
			ZoneKey:       data.Zone,
			StartTimeKey:  data.StartedAt,
			DNSKey:        data.PublicDNS,
			PublicIPv4Key: data.PublicIPv4,
			IPv4Key:       data.PrivateIPv4,
			IPKey:         data.IPv6,
			VolumesKey:    data.Volumes,
		},
	}
}

func (h *Host) Insert(ctx context.Context) error {
	if err := InsertOne(ctx, h); err != nil {
		return errors.Wrap(err, "inserting host")
	}
	return nil
}

// InsertWithContext is the same as Insert but accepts a context for the
// operation.
func (h *Host) InsertWithContext(ctx context.Context, env evergreen.Environment) error {
	if _, err := env.DB().Collection(Collection).InsertOne(ctx, h); err != nil {
		return errors.Wrap(err, "inserting host")
	}
	return nil
}

// Remove removes the host document from the DB.
// While it's fine to use this in tests, this should generally not be called in
// production code since deleting the host document makes it difficult to trace
// what happened to it. Instead, it's preferable to set a host to a failure
// state (e.g. building-failed, decommissioned) so that it can be cleaned up by
// host termination.
func (h *Host) Remove(ctx context.Context) error {
	return errors.Wrapf(DeleteOne(ctx, ById(h.Id)), "deleting host '%s'", h.Id)
}

// RemoveStrict deletes a host and errors if the host is not found.
func RemoveStrict(ctx context.Context, env evergreen.Environment, id string) error {
	result, err := env.DB().Collection(Collection).DeleteOne(ctx, bson.M{IdKey: id})
	if err != nil {
		return err
	}
	if result.DeletedCount == 0 {
		return errors.Errorf("host '%s' not found", id)
	}
	return nil
}

// Replace overwrites an existing host document with a new one. If no existing host is found, the new one will be inserted anyway.
func (h *Host) Replace(ctx context.Context) error {
	result := evergreen.GetEnvironment().DB().Collection(Collection).FindOneAndReplace(ctx, bson.M{IdKey: h.Id}, h, options.FindOneAndReplace().SetUpsert(true))
	err := result.Err()
	if errors.Cause(err) == mongo.ErrNoDocuments {
		return nil
	}
	return errors.Wrap(err, "replacing host")
}

// GetElapsedCommunicationTime returns how long since this host has communicated with evergreen or vice versa
func (h *Host) GetElapsedCommunicationTime() time.Duration {
	// If the host is currently tearing down a task group, it is not considered idle, and
	// it is expected that it will not communicate with evergreen because the task has completed
	// and will no longer be sending heartbeat requests.
	if h.IsTearingDown() {
		return 0
	}
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

// TeardownTimeExceededMax checks if the time since the host's task group teardown start time
// has exceeded the maximum teardown threshold.
func (h *Host) TeardownTimeExceededMax() bool {
	return time.Since(h.TaskGroupTeardownStartTime) < evergreen.MaxTeardownGroupThreshold
}

// DecommissionHostsWithDistroId marks all up hosts intended for running tasks
// that have a matching distro ID as decommissioned.
func DecommissionHostsWithDistroId(ctx context.Context, distroId string) error {
	err := UpdateAll(
		ctx,
		ByDistroIDs(distroId),
		bson.M{
			"$set": bson.M{
				StatusKey: evergreen.HostDecommissioned,
			},
		},
	)
	return err
}

func (h *Host) SetExtId(ctx context.Context) error {
	return UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{ExtIdKey: h.ExternalIdentifier}},
	)
}

func (h *Host) SetDisplayName(ctx context.Context, newName string) error {
	h.DisplayName = newName
	return UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{DisplayNameKey: h.DisplayName}},
	)
}

// AddSSHKeyName adds the SSH key name for the host if it doesn't already have
// it.
func (h *Host) AddSSHKeyName(ctx context.Context, name string) error {
	var update bson.M
	if len(h.SSHKeyNames) == 0 {
		update = bson.M{"$push": bson.M{SSHKeyNamesKey: name}}
	} else {
		update = bson.M{"$addToSet": bson.M{SSHKeyNamesKey: name}}
	}
	if err := UpdateOne(ctx, bson.M{IdKey: h.Id}, update); err != nil {
		return errors.WithStack(err)
	}

	if !utility.StringSliceContains(h.SSHKeyNames, name) {
		h.SSHKeyNames = append(h.SSHKeyNames, name)
	}

	return nil
}

func FindHostsToTerminate(ctx context.Context) ([]Host, error) {
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
			{
				// Hosts that failed to be created in a cloud provider or
				// provision.
				StatusKey: bson.M{"$in": []string{
					evergreen.HostBuildingFailed,
					evergreen.HostProvisionFailed},
				},
			},
			{
				// Either:
				// - Host that does not provision with user data is taking too
				//   long to provision.
				// - Host that provisions with user data is taking too long to
				//   provision. In addition, it is not currently running a task
				//   and has not checked in recently.
				"$and": []bson.M{
					// Host is not yet done provisioning
					{"$or": []bson.M{
						{ProvisionedKey: false},
						{StatusKey: bson.M{"$in": []string{evergreen.HostStarting, evergreen.HostProvisioning}}},
					}},
					{"$or": []bson.M{
						{
							// Host is a user data host and either has not run a
							// task yet or has not started its agent monitor -
							// both are indicators that the host's agent is not
							// up. The host has either 1. failed to start the
							// agent or 2. failed to prove the agent's
							// liveliness by continuously pinging the app server
							// with requests.
							"$or": []bson.M{
								{RunningTaskKey: bson.M{"$exists": false}},
								{LTCTaskKey: ""},
							},
							LastCommunicationTimeKey: bson.M{"$lte": now.Add(-MaxAgentMonitorUnresponsiveInterval)},
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

	// In some cases, the query planner will choose a very slow plan for this
	// query. The hint guarantees that the query will use the host status index.
	hosts, err := Find(ctx, query, options.Find().SetHint(StatusIndex))

	if adb.ResultsNotFound(err) {
		return []Host{}, nil
	}

	if err != nil {
		return nil, errors.Wrap(err, "finding hosts to terminate")
	}

	return hosts, nil
}

// StatusIndex is the index that is prefixed with the host status.
var StatusIndex = bson.D{
	{
		Key:   StatusKey,
		Value: 1,
	},
	{
		Key:   bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsSpawnedByTaskKey),
		Value: 1,
	},
}

// StartedByStatusIndex is the started_by_1_status_1 index.
var StartedByStatusIndex = bson.D{
	{
		Key:   StartedByKey,
		Value: 1,
	},
	{
		Key:   StatusKey,
		Value: 1,
	},
}

// DistroIdStatusIndex is the distro_id_1_status_1 index.
var DistroIdStatusIndex = bson.D{
	{
		Key:   bsonutil.GetDottedKeyName(DistroKey, distro.IdKey),
		Value: 1,
	},
	{
		Key:   StatusKey,
		Value: 1,
	},
}

func CountInactiveHostsByProvider(ctx context.Context) ([]InactiveHostCounts, error) {
	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Aggregate(ctx, inactiveHostCountPipeline())
	if err != nil {
		return nil, errors.Wrap(err, "aggregating inactive hosts")
	}
	var counts []InactiveHostCounts
	if err = cur.All(ctx, &counts); err != nil {
		return nil, errors.Wrap(err, "decoding inactive hosts")
	}

	return counts, nil
}

// FindAllRunningContainers finds all the containers that are currently running
func FindAllRunningContainers(ctx context.Context) ([]Host, error) {
	query := bson.M{
		ParentIDKey: bson.M{"$exists": true},
		StatusKey:   evergreen.HostRunning,
	}
	hosts, err := Find(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "finding running containers")
	}

	return hosts, nil
}

// FindAllRunningParents finds all running hosts that have child containers
func FindAllRunningParents(ctx context.Context) ([]Host, error) {
	query := bson.M{
		StatusKey:        evergreen.HostRunning,
		HasContainersKey: true,
	}
	hosts, err := Find(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "finding running parents")
	}

	return hosts, nil
}

// FindAllRunningParentsOrdered finds all running hosts with child containers,
// sorted in order of soonest  to latest LastContainerFinishTime
func FindAllRunningParentsOrdered(ctx context.Context) ([]Host, error) {
	query := bson.M{
		StatusKey:        evergreen.HostRunning,
		HasContainersKey: true,
	}
	hosts, err := Find(ctx, query, options.Find().SetSort(bson.M{LastContainerFinishTimeKey: 1}))
	if err != nil {
		return nil, errors.Wrap(err, "finding ordered running parents")
	}

	return hosts, nil
}

// FindAllRunningParentsByDistroID finds all running hosts of a given distro ID
// with child containers.
func FindAllRunningParentsByDistroID(ctx context.Context, distroID string) ([]Host, error) {
	query := bson.M{
		StatusKey:        evergreen.HostRunning,
		HasContainersKey: true,
		bsonutil.GetDottedKeyName(DistroKey, distro.IdKey): distroID,
	}
	return Find(ctx, query, options.Find().SetSort(bson.M{LastContainerFinishTimeKey: 1}))
}

// GetContainers finds all the containers belonging to this host
// errors if this host is not a parent
func (h *Host) GetContainers(ctx context.Context) ([]Host, error) {
	if !h.HasContainers {
		return nil, errors.New("host is not a container parent")
	}
	query := bson.M{"$or": []bson.M{
		{ParentIDKey: h.Id},
		{ParentIDKey: h.Tag},
	}}
	hosts, err := Find(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "finding containers")
	}

	return hosts, nil
}

func (h *Host) GetActiveContainers(ctx context.Context) ([]Host, error) {
	if !h.HasContainers {
		return nil, errors.New("host is not a container parent")
	}
	query := bson.M{
		StatusKey: bson.M{
			"$in": evergreen.UpHostStatus,
		},
		"$or": []bson.M{
			{ParentIDKey: h.Id},
			{ParentIDKey: h.Tag},
		}}
	hosts, err := Find(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "finding containers")
	}

	return hosts, nil
}

// GetParent finds the parent of this container
// errors if host is not a container or if parent cannot be found
func (h *Host) GetParent(ctx context.Context) (*Host, error) {
	if h.ParentID == "" {
		return nil, errors.New("host does not have a parent")
	}
	host, err := FindOneByIdOrTag(ctx, h.ParentID)
	if err != nil {
		return nil, errors.Wrap(err, "finding container parent")
	}
	if host == nil {
		return nil, errors.New(ErrorParentNotFound)
	}
	if !host.HasContainers {
		return nil, errors.New("host found but is not a container parent")
	}

	return host, nil
}

// IsIdleParent determines whether a host has only inactive containers
func (h *Host) IsIdleParent(ctx context.Context) (bool, error) {
	const idleTimeCutoff = 20 * time.Minute
	if !h.HasContainers {
		return false, nil
	}
	// Verify that hosts are not immediately decommissioned.
	if h.IdleTime() < idleTimeCutoff {
		return false, nil
	}
	query := bson.M{
		ParentIDKey: h.Id,
		StatusKey:   bson.M{"$in": evergreen.UpHostStatus},
	}
	num, err := Count(ctx, query)
	if err != nil {
		return false, errors.Wrap(err, "counting non-terminated containers")
	}

	return num == 0, nil
}

func (h *Host) UpdateParentIDs(ctx context.Context) error {
	query := bson.M{
		ParentIDKey: h.Tag,
	}
	update := bson.M{
		"$set": bson.M{
			ParentIDKey: h.Id,
		},
	}
	return UpdateAll(ctx, query, update)
}

// ValidateExpirationExtension will prevent expirable spawn hosts
// from being extended past 30 days from its creation and not earlier
// than the current time.
func (h *Host) ValidateExpirationExtension(extension time.Duration) error {
	maxExpirationTime := h.CreationTime.Add(evergreen.SpawnHostExpireDays * utility.Day)
	proposedTime := h.ExpirationTime.Add(extension)

	if h.ExpirationTime.After(maxExpirationTime) || proposedTime.After(maxExpirationTime) {
		return errors.Errorf("spawn host cannot be extended more than %d days past creation", evergreen.SpawnHostExpireDays)
	}
	if time.Now().After(proposedTime) {
		return errors.New("spawn host cannot be extended before the current time.")
	}
	return nil
}

// UpdateLastContainerFinishTime updates latest finish time for a host with containers
func (h *Host) UpdateLastContainerFinishTime(ctx context.Context, t time.Time) error {
	selector := bson.M{
		IdKey: h.Id,
	}

	update := bson.M{
		"$set": bson.M{
			LastContainerFinishTimeKey: t,
		},
	}

	if err := UpdateOne(ctx, selector, update); err != nil {
		return errors.Wrapf(err, "updating last container finish time for host '%s'", h.Id)
	}

	return nil
}

// FindRunningHosts is the underlying query behind the hosts page's table
func FindRunningHosts(ctx context.Context, includeSpawnHosts bool) ([]Host, error) {
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

	return Aggregate(ctx, pipeline)
}

// FindAllHostsSpawnedByTasks finds all running hosts spawned by the
// `host.create` command.
func FindAllHostsSpawnedByTasks(ctx context.Context) ([]Host, error) {
	query := bson.M{
		StatusKey: evergreen.HostRunning,
		bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsSpawnedByTaskKey): true,
	}
	hosts, err := Find(ctx, query)
	if err != nil {
		return nil, errors.Wrap(err, "finding hosts spawned by tasks")
	}
	return hosts, nil
}

// FindHostsSpawnedByTask finds hosts spawned by the `host.create` command scoped to a given task.
func FindHostsSpawnedByTask(ctx context.Context, taskID string, execution int, statuses []string) ([]Host, error) {
	taskIDKey := bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsTaskIDKey)
	taskExecutionNumberKey := bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsTaskExecutionNumberKey)
	query := bson.M{
		StatusKey:              bson.M{"$in": statuses},
		taskIDKey:              taskID,
		taskExecutionNumberKey: execution,
	}
	hosts, err := Find(ctx, query)
	if err != nil {
		return nil, errors.Wrapf(err, "finding hosts spawned by task '%s' for execution %d", taskID, execution)
	}
	return hosts, nil
}

// FindHostsSpawnedByBuild finds hosts spawned by the `createhost` command scoped to a given build.
func FindHostsSpawnedByBuild(ctx context.Context, buildID string) ([]Host, error) {
	buildIDKey := bsonutil.GetDottedKeyName(SpawnOptionsKey, SpawnOptionsBuildIDKey)
	query := bson.M{
		StatusKey:  evergreen.HostRunning,
		buildIDKey: buildID,
	}
	hosts, err := Find(ctx, query)
	if err != nil {
		return nil, errors.Wrapf(err, "finding hosts spawned by build '%s'", buildID)
	}
	return hosts, nil
}

// FindTerminatedHostsRunningTasks finds all hosts that were running tasks when
// they were terminated.
func FindTerminatedHostsRunningTasks(ctx context.Context) ([]Host, error) {
	hosts, err := Find(ctx, bson.M{
		StatusKey: evergreen.HostTerminated,
		"$and": []bson.M{
			{RunningTaskKey: bson.M{"$exists": true}},
			{RunningTaskKey: bson.M{"$ne": ""}}},
	})
	if err != nil {
		return nil, errors.Wrap(err, "finding terminated hosts that have a running task")
	}

	return hosts, nil
}

// CountContainersOnParents counts how many containers are children of the given group of hosts
func (hosts HostGroup) CountContainersOnParents(ctx context.Context) (int, error) {
	ids := hosts.GetHostIds()
	if len(ids) == 0 {
		return 0, nil
	}

	query := bson.M{
		StatusKey:   bson.M{"$in": evergreen.UpHostStatus},
		ParentIDKey: bson.M{"$in": ids},
	}
	return Count(ctx, query)
}

// FindUphostContainersOnParents returns the containers that are children of the given hosts
func (hosts HostGroup) FindUphostContainersOnParents(ctx context.Context) ([]Host, error) {
	ids := hosts.GetHostIds()
	if len(ids) == 0 {
		return nil, nil
	}

	query := bson.M{
		StatusKey:   bson.M{"$in": evergreen.UpHostStatus},
		ParentIDKey: bson.M{"$in": ids},
	}
	return Find(ctx, query)
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
			if h.IsFree() {
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

// getNumContainersOnParents returns a slice of running parents and their respective
// number of current containers currently running in order of longest expected
// finish time
func GetContainersOnParents(ctx context.Context, d distro.Distro) ([]ContainersOnParents, error) {
	allParents, err := findAllRunningParentsByContainerPool(ctx, d.ContainerPool)
	if err != nil {
		return nil, errors.Wrapf(err, "finding running container parents in container pool '%s'", d.ContainerPool)
	}

	containersOnParents := make([]ContainersOnParents, 0)
	// parents come in sorted order from soonest to latest expected finish time
	for i := len(allParents) - 1; i >= 0; i-- {
		parent := allParents[i]
		currentContainers, err := parent.GetActiveContainers(ctx)
		if err != nil && !adb.ResultsNotFound(err) {
			return nil, errors.Wrapf(err, "finding active containers for container parent '%s'", parent.Id)
		}
		containersOnParents = append(containersOnParents,
			ContainersOnParents{
				ParentHost: parent,
				Containers: currentContainers,
			})
	}

	return containersOnParents, nil
}

func getNumNewParentsAndHostsToSpawn(ctx context.Context, pool *evergreen.ContainerPool, newContainersNeeded int, ignoreMaxHosts bool) (int, int, error) {
	existingParents, err := findUphostParentsByContainerPool(ctx, pool.Id)
	if err != nil {
		return 0, 0, errors.Wrap(err, "finding container parents that are up")
	}

	// find all child containers running on those parents
	existingContainers, err := HostGroup(existingParents).FindUphostContainersOnParents(ctx)
	if err != nil {
		return 0, 0, errors.Wrap(err, "finding containers that are up")
	}

	// create numParentsNeededParams struct
	parentsParams := newParentsNeededParams{
		numExistingParents:    len(existingParents),
		numExistingContainers: len(existingContainers),
		numContainersNeeded:   newContainersNeeded,
		maxContainers:         pool.MaxContainers,
	}
	// compute number of parents needed
	numNewParentsToSpawn := numNewParentsNeeded(parentsParams)
	if numNewParentsToSpawn == 0 {
		return 0, 0, nil
	}
	// get parent distro from pool
	parentDistro, err := distro.FindOneId(ctx, pool.Distro)
	if err != nil {
		return 0, 0, errors.Wrap(err, "finding container parent's distro")
	}
	if parentDistro == nil {
		return 0, 0, errors.Errorf("distro '%s' not found", pool.Distro)
	}

	if !ignoreMaxHosts { // only want to spawn amount of parents allowed based on pool size
		if numNewParentsToSpawn, err = parentCapacity(*parentDistro, numNewParentsToSpawn, len(existingParents), pool); err != nil {
			return 0, 0, errors.Wrap(err, "calculating number of parents that need to spawn")
		}
	}

	// only want to spawn amount of containers we can fit on running/uninitialized parents
	newContainersNeeded = containerCapacity(len(existingParents)+numNewParentsToSpawn, len(existingContainers), newContainersNeeded, pool.MaxContainers)
	return numNewParentsToSpawn, newContainersNeeded, nil
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

// findAllRunningParentsByContainerPool returns a slice of hosts that are
// parents of the container pool specified by the given ID.
func findAllRunningParentsByContainerPool(ctx context.Context, poolId string) ([]Host, error) {
	hostContainerPoolId := bsonutil.GetDottedKeyName(ContainerPoolSettingsKey, evergreen.ContainerPoolIdKey)
	query := bson.M{
		HasContainersKey:    true,
		StatusKey:           evergreen.HostRunning,
		hostContainerPoolId: poolId,
	}
	return Find(ctx, query, options.Find().SetSort(bson.M{LastContainerFinishTimeKey: 1}))
}

// findUphostParents returns the container parent hosts that are up.
func findUphostParentsByContainerPool(ctx context.Context, poolId string) ([]Host, error) {
	hostContainerPoolId := bsonutil.GetDottedKeyName(ContainerPoolSettingsKey, evergreen.ContainerPoolIdKey)
	query := bson.M{
		HasContainersKey:    true,
		StatusKey:           bson.M{"$in": evergreen.UpHostStatus},
		hostContainerPoolId: poolId,
	}
	return Find(ctx, query, options.Find().SetSort(bson.M{LastContainerFinishTimeKey: 1}))
}

// CountContainersRunningAtTime counts how many containers were running on the
// given parent host at the specified time, using the host StartTime and
// TerminationTime fields.
func (h *Host) CountContainersRunningAtTime(ctx context.Context, timestamp time.Time) (int, error) {
	query := bson.M{
		ParentIDKey:  h.Id,
		StartTimeKey: bson.M{"$lt": timestamp},
		"$or": []bson.M{
			{TerminationTimeKey: bson.M{"$gt": timestamp}},
			{TerminationTimeKey: time.Time{}},
		},
	}
	return Count(ctx, query)
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
func (h *Host) SetTags(ctx context.Context) error {
	return UpdateOne(
		ctx,
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

// MakeHostTags creates and validates a map of supplied instance tags.
func MakeHostTags(tagSlice []string) ([]Tag, error) {
	catcher := grip.NewBasicCatcher()
	tagsMap := make(map[string]string)
	for _, tagString := range tagSlice {
		pair := strings.Split(tagString, "=")
		if len(pair) != 2 {
			catcher.Errorf("parsing tag '%s'", tagString)
			continue
		}

		key := pair[0]
		value := pair[1]

		// AWS tag key must contain no more than 128 characters
		catcher.ErrorfWhen(len(key) > MaxTagKeyLength, "key '%s' is longer than maximum limit of 128 characters", key)
		// AWS tag value must contain no more than 256 characters
		catcher.ErrorfWhen(len(value) > MaxTagValueLength, "value '%s' is longer than maximum limit of 256 characters", value)
		// tag prefix aws: is reserved
		catcher.ErrorfWhen(strings.HasPrefix(key, "aws:") || strings.HasPrefix(value, "aws:"), "illegal tag prefix 'aws:'")

		tagsMap[key] = value
	}

	if catcher.HasErrors() {
		return nil, catcher.Resolve()
	}

	tags := []Tag{}
	for key, value := range tagsMap {
		tags = append(tags, Tag{Key: key, Value: value, CanBeModified: true})
	}

	return tags, nil
}

// SetInstanceType updates the host's instance type in the database.
func (h *Host) SetInstanceType(ctx context.Context, instanceType string) error {
	err := UpdateOne(
		ctx,
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

// AggregateSpawnhostData returns basic metrics on spawn host/volume usage.
func AggregateSpawnhostData(ctx context.Context) (*SpawnHostUsage, error) {
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
		}},
		{"$project": bson.M{
			"_id":                     "0",
			"total_hosts":             "$hosts",
			"total_stopped_hosts":     "$stopped",
			"total_unexpirable_hosts": "$unexpirable",
			"num_users_with_hosts":    bson.M{"$size": "$users"},
		}},
	}

	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Aggregate(ctx, hostPipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating spawn host usage data")
	}
	var hostRes []SpawnHostUsage
	if err = cur.All(ctx, &hostRes); err != nil {
		return nil, errors.Wrap(err, "decoding spawn host usage data")
	}
	if len(hostRes) == 0 {
		return nil, errors.New("no host results found")
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
	cur, err = evergreen.GetEnvironment().DB().Collection(VolumesCollection).Aggregate(ctx, volumePipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating spawn host volume usage data")
	}
	var volRes []SpawnHostUsage
	if err = cur.All(ctx, &volRes); err != nil {
		return nil, errors.Wrap(err, "decoding spawn host volume usage data")
	}

	if len(volRes) == 0 {
		return nil, errors.New("no volume results found")
	}

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

	cur, err = evergreen.GetEnvironment().DB().Collection(Collection).Aggregate(ctx, instanceTypePipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating spawn host instance type usage data")
	}
	temp := []struct {
		InstanceType string `bson:"instance_type"`
		Count        int    `bson:"count"`
	}{}
	if err = cur.All(ctx, &temp); err != nil {
		return nil, errors.Wrap(err, "decoding spawn host instance type usage data")
	}

	instanceTypes := make(map[string]int, len(temp))
	for _, each := range temp {
		instanceTypes[each.InstanceType] = each.Count
	}

	return &SpawnHostUsage{
		TotalHosts:            hostRes[0].TotalHosts,
		TotalStoppedHosts:     hostRes[0].TotalStoppedHosts,
		TotalUnexpirableHosts: hostRes[0].TotalUnexpirableHosts,
		NumUsersWithHosts:     hostRes[0].NumUsersWithHosts,
		TotalVolumes:          volRes[0].TotalVolumes,
		TotalVolumeSize:       volRes[0].TotalVolumeSize,
		NumUsersWithVolumes:   volRes[0].NumUsersWithVolumes,
		InstanceTypes:         instanceTypes,
	}, nil
}

// CountSpawnhostsWithNoExpirationByUser returns a count of all hosts associated
// with a given users that are considered up and should never expire.
func CountSpawnhostsWithNoExpirationByUser(ctx context.Context, user string) (int, error) {
	query := bson.M{
		StartedByKey:    user,
		NoExpirationKey: true,
		StatusKey:       bson.M{"$in": evergreen.UpHostStatus},
	}
	return Count(ctx, query)
}

// FindSpawnhostsWithNoExpirationToExtend returns all hosts that are set to never
// expire but have their expiration time within the next day and are still up.
func FindSpawnhostsWithNoExpirationToExtend(ctx context.Context) ([]Host, error) {
	query := bson.M{
		UserHostKey:       true,
		NoExpirationKey:   true,
		StatusKey:         bson.M{"$in": evergreen.UpHostStatus},
		ExpirationTimeKey: bson.M{"$lte": time.Now().Add(utility.Day)},
	}

	return Find(ctx, query)
}

func makeExpireOnTag(expireOn string) Tag {
	return Tag{
		Key:           evergreen.TagExpireOn,
		Value:         expireOn,
		CanBeModified: false,
	}
}

// MarkShouldNotExpire marks a host as one that should not expire
// and updates its expiration time to avoid early reaping. If the host is marked
// unexpirable and has invalid/missing sleep schedule settings,  it is assigned
// the default sleep schedule.
func (h *Host) MarkShouldNotExpire(ctx context.Context, expireOnValue, userTimeZone string) error {
	h.NoExpiration = true
	h.ExpirationTime = time.Now().Add(evergreen.SpawnHostNoExpirationDuration)
	h.addTag(makeExpireOnTag(expireOnValue), true)
	if err := UpdateOne(
		ctx,
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
	); err != nil {
		return err
	}

	if h.SleepSchedule.Validate() != nil {
		// If the host is being made expirable and its sleep schedule is
		// invalid/missing, set it to the default sleep schedule. This is a
		// safety measure to ensure that a host cannot be made unexpirable
		// without having some kind of working sleep schedule (or permanent
		// exemption) in place.
		var opts SleepScheduleOptions
		opts.SetDefaultSchedule()
		opts.SetDefaultTimeZone(userTimeZone)
		schedule, err := NewSleepScheduleInfo(opts)
		if err != nil {
			return errors.Wrap(err, "creating default sleep schedule for host being marked unexpirable that has invalid schedule")
		}
		grip.Info(message.Fields{
			"message":            "host is being marked unexpirable but has an invalid sleep schedule, setting it to the default sleep schedule",
			"host_id":            h.Id,
			"started_by":         h.StartedBy,
			"old_sleep_schedule": h.SleepSchedule,
			"new_sleep_schedule": schedule,
		})
		grip.Error(message.WrapError(h.UpdateSleepSchedule(ctx, *schedule, time.Now()), message.Fields{
			"message":    "could not set default sleep schedule for host being marked unexpirable that currently has an invalid schedule",
			"host_id":    h.Id,
			"started_by": h.StartedBy,
		}))
	}

	return nil
}

// MarkShouldExpire resets a host's expiration to expire like
// a normal spawn host, after 24 hours.
func (h *Host) MarkShouldExpire(ctx context.Context, expireOnValue string) error {
	// If it's already set to expire, do nothing.
	if !h.NoExpiration {
		return nil
	}

	h.NoExpiration = false
	h.ExpirationTime = time.Now().Add(evergreen.DefaultSpawnHostExpiration)
	if expireOnValue != "" {
		h.addTag(makeExpireOnTag(expireOnValue), true)
	}
	return UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{
			"$set": bson.M{
				NoExpirationKey:   h.NoExpiration,
				ExpirationTimeKey: h.ExpirationTime,
				InstanceTagsKey:   h.InstanceTags,
			},
		},
	)
}

// UnsetHomeVolume disassociates a home volume from a (stopped) host.
// This is for internal use, and should only be used on hosts that
// will be terminated imminently; otherwise, the host will fail to boot.
func (h *Host) UnsetHomeVolume(ctx context.Context) error {
	err := UpdateOne(
		ctx,
		bson.M{IdKey: h.Id},
		bson.M{"$set": bson.M{HomeVolumeIDKey: ""}},
	)
	if err != nil {
		return err
	}

	h.HomeVolumeID = ""
	return nil
}

func (h *Host) SetHomeVolumeID(ctx context.Context, volumeID string) error {
	h.HomeVolumeID = volumeID
	return UpdateOne(ctx, bson.M{
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
func FindHostWithVolume(ctx context.Context, volumeID string) (*Host, error) {
	q := bson.M{
		StatusKey:   bson.M{"$in": evergreen.UpHostStatus},
		UserHostKey: true,
		bsonutil.GetDottedKeyName(VolumesKey, VolumeAttachmentIDKey): volumeID,
	}
	return FindOne(ctx, q)
}

// FindUpHostWithHomeVolume finds the up host associated with the
// specified home volume ID.
func FindUpHostWithHomeVolume(ctx context.Context, homeVolumeID string) (*Host, error) {
	q := bson.M{
		StatusKey:       bson.M{"$in": evergreen.UpHostStatus},
		UserHostKey:     true,
		HomeVolumeIDKey: homeVolumeID,
	}
	return FindOne(ctx, q)
}

// FindLatestTerminatedHostWithHomeVolume finds the user's most recently terminated host
// associated with the specified home volume ID.
func FindLatestTerminatedHostWithHomeVolume(ctx context.Context, homeVolumeID string, startedBy string) (*Host, error) {
	q := bson.M{
		StatusKey:       evergreen.HostTerminated,
		StartedByKey:    startedBy,
		UserHostKey:     true,
		HomeVolumeIDKey: homeVolumeID,
	}
	return FindOne(ctx, q, options.FindOne().SetSort(bson.M{TerminationTimeKey: -1}))
}

// FindStaticNeedsNewSSHKeys finds all static hosts that do not have the same
// set of SSH keys as those in the global settings.
func FindStaticNeedsNewSSHKeys(ctx context.Context, settings *evergreen.Settings) ([]Host, error) {
	if len(settings.SSHKeyPairs) == 0 {
		return nil, nil
	}

	names := []string{}
	for _, pair := range settings.SSHKeyPairs {
		names = append(names, pair.Name)
	}

	return Find(ctx, bson.M{
		StatusKey:      evergreen.HostRunning,
		ProviderKey:    evergreen.ProviderNameStatic,
		SSHKeyNamesKey: bson.M{"$not": bson.M{"$all": names}},
	})
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

// GetHostByIdOrTagWithTask finds a host by ID or tag and includes the full
// running task with the host.
func GetHostByIdOrTagWithTask(ctx context.Context, hostID string) (*Host, error) {
	pipeline := []bson.M{
		{
			"$match": bson.M{
				"$or": []bson.M{
					{TagKey: hostID},
					{IdKey: hostID},
				}},
		},
		{
			"$lookup": bson.M{
				"from":         task.Collection,
				"localField":   RunningTaskKey,
				"foreignField": task.IdKey,
				"as":           RunningTaskFullKey,
			},
		},
		{
			"$unwind": bson.M{
				"path":                       "$" + RunningTaskFullKey,
				"preserveNullAndEmptyArrays": true,
			},
		},
	}

	hosts := []Host{}
	cursor, err := evergreen.GetEnvironment().DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating host by ID or tag with task")
	}
	err = cursor.All(ctx, &hosts)
	if err != nil {
		return nil, errors.Wrap(err, "retrieving hosts by ID or tag with task")
	}
	if len(hosts) == 0 {
		return nil, errors.Wrap(err, "host not found")
	}

	return &hosts[0], nil
}

// HostsFilterOptions represents the filtering arguments for fetching hosts.
type HostsFilterOptions struct {
	HostID        string
	DistroID      string
	CurrentTaskID string
	Statuses      []string
	StartedBy     string
	SortBy        string
	SortDir       int
	Page          int
	Limit         int
}

// GetPaginatedRunningHosts gets running hosts with pagination and applies any filters.
func GetPaginatedRunningHosts(ctx context.Context, opts HostsFilterOptions) ([]Host, *int, int, error) {
	runningHostsPipeline := []bson.M{
		{
			"$match": bson.M{StatusKey: bson.M{"$ne": evergreen.HostTerminated}},
		},
		{
			"$lookup": bson.M{
				"from":         task.Collection,
				"localField":   RunningTaskKey,
				"foreignField": task.IdKey,
				"as":           RunningTaskFullKey,
			},
		},
		{
			"$unwind": bson.M{
				"path":                       "$" + RunningTaskFullKey,
				"preserveNullAndEmptyArrays": true,
			},
		},
	}

	countPipeline := []bson.M{}
	countPipeline = append(countPipeline, runningHostsPipeline...)
	countPipeline = append(countPipeline, bson.M{"$count": "count"})

	tmp := []counter{}
	env := evergreen.GetEnvironment()
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

	hasFilters := false

	if len(opts.HostID) > 0 {
		hasFilters = true

		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				"$or": []bson.M{
					{IdKey: opts.HostID},
					{DNSKey: opts.HostID},
				},
			},
		})
	}

	if len(opts.DistroID) > 0 {
		hasFilters = true

		distroIDKey := bsonutil.GetDottedKeyName(DistroKey, distro.IdKey)
		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				distroIDKey: bson.M{"$regex": opts.DistroID, "$options": "i"},
			},
		})
	}

	if len(opts.CurrentTaskID) > 0 {
		hasFilters = true

		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				RunningTaskKey: opts.CurrentTaskID,
			},
		})
	}

	if len(opts.StartedBy) > 0 {
		hasFilters = true

		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				StartedByKey: opts.StartedBy,
			},
		})
	}

	if len(opts.Statuses) > 0 {
		hasFilters = true

		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$match": bson.M{
				StatusKey: bson.M{"$in": opts.Statuses},
			},
		})
	}

	var filteredHostsCount *int

	if hasFilters {
		countPipeline = []bson.M{}
		countPipeline = append(countPipeline, runningHostsPipeline...)
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

	sorters := bson.D{}
	if len(opts.SortBy) > 0 {
		sorters = append(sorters, bson.E{Key: opts.SortBy, Value: opts.SortDir})
	}

	// If we're not sorting by ID, we need to sort by ID as a tiebreaker to ensure a consistent sort order.
	if opts.SortBy != IdKey {
		sorters = append(sorters, bson.E{Key: IdKey, Value: 1})
	}
	runningHostsPipeline = append(runningHostsPipeline, bson.M{
		"$sort": sorters,
	})

	if opts.Limit > 0 {
		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$skip": opts.Page * opts.Limit,
		})
		runningHostsPipeline = append(runningHostsPipeline, bson.M{
			"$limit": opts.Limit,
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

type VirtualWorkstationCounter struct {
	InstanceType string `bson:"instance_type" json:"instance_type"`
	Count        int    `bson:"count" json:"count"`
}

func CountVirtualWorkstationsByInstanceType(ctx context.Context) ([]VirtualWorkstationCounter, error) {
	pipeline := []bson.M{
		{"$match": bson.M{
			StatusKey:               evergreen.HostRunning,
			IsVirtualWorkstationKey: true,
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

	cur, err := evergreen.GetEnvironment().DB().Collection(Collection).Aggregate(ctx, pipeline)
	if err != nil {
		return nil, errors.Wrap(err, "aggregating virtual workstation counts by instance type")
	}
	var data []VirtualWorkstationCounter
	if err = cur.All(ctx, &data); err != nil {
		return nil, errors.Wrap(err, "decoding virtual workstation counts")
	}

	return data, nil
}

// ClearDockerStdinData clears the Docker stdin data from the host.
func (h *Host) ClearDockerStdinData(ctx context.Context) error {
	if len(h.DockerOptions.StdinData) == 0 {
		return nil
	}

	dockerStdinDataKey := bsonutil.GetDottedKeyName(DockerOptionsKey, DockerOptionsStdinDataKey)
	if err := UpdateOne(ctx, bson.M{
		IdKey: h.Id,
	},
		bson.M{
			"$unset": bson.M{dockerStdinDataKey: true},
		},
	); err != nil {
		return err
	}

	h.DockerOptions.StdinData = nil

	return nil
}

// nonAlphanumericRegexp matches any character that is not an alphanumeric
// character ([0-9A-Za-z]).
var nonAlphanumericRegexp = regexp.MustCompile("[[:^alnum:]]+")

// hexadecimalChars contains all hexademical characters (case-insensitive).
const hexadecimalChars = "abcdef0123456789"

// GeneratePersistentDNSName returns the host's persistent DNS name, or
// generates a new one if it doesn't have one currently assigned.
func (h *Host) GeneratePersistentDNSName(ctx context.Context, domain string) (string, error) {
	if h.PersistentDNSName != "" {
		return h.PersistentDNSName, nil
	}

	// DNS doesn't handle non-alphanumeric characters very well, so replace dots
	// in usernames with dashes and remove all other non-alphanumeric
	// characters.
	userParts := strings.Split(h.StartedBy, ".")
	cleanedParts := make([]string, 0, len(userParts))
	for _, part := range userParts {
		cleanedParts = append(cleanedParts, nonAlphanumericRegexp.ReplaceAllString(part, ""))
	}
	user := strings.Join(cleanedParts, "-")

	const maxRandLen = 3

	// Since each user can own multiple unexpirable hosts, just using their
	// username is not sufficient to make a unique persistent DNS name. Try
	// first generating a persistent DNS name that:
	// 1. contains only alphanumeric characters so it's more memorable, and
	// 2. is derived from the host ID.
	//
	// This is slightly preferable compared to a random alphanumeric string,
	// because a host with a particular ID will always map to the same ID and
	// will have a more short and memorable string.

	idBasedHash := sha256.Sum256([]byte(h.Id))
	encoder := base64.RawURLEncoding
	idBasedEncoding := make([]byte, encoder.EncodedLen(len(idBasedHash[:])))
	encoder.Encode(idBasedEncoding, idBasedHash[:])

	// Shorten the string and map encoding to only alphabet characters so it's
	// more memorable.
	idBasedRandom := make([]byte, maxRandLen)
	for i := range idBasedRandom {
		idBasedRandom[i] = hexadecimalChars[idBasedEncoding[i]%byte(len(hexadecimalChars))]
	}
	candidate := fmt.Sprintf("%s-%s.%s", user, string(idBasedRandom), strings.TrimPrefix(domain, "."))
	conflicting, _ := FindOneByPersistentDNSName(ctx, candidate)
	if conflicting == nil || conflicting.Id == h.Id {
		return candidate, nil
	}

	// If the host ID can't be mapped to a unique DNS name, then fall back to
	// using a random string to produce a unique DNS name. This is less
	// preferable because it's randomly-generated, but it does handle the very
	// tiny edge case where the DNS name generated above conflicts with an
	// existing one.
	const numAttempts = 5
	for i := 0; i < numAttempts; i++ {
		random := utility.RandomString()[:maxRandLen]
		candidate := fmt.Sprintf("%s-%s.%s", user, random, strings.TrimPrefix(domain, "."))

		// Ensure no other host is using this name. Since the random portion is
		// very short, it's unlikely but still possible that there's a conflict.
		conflicting, _ := FindOneByPersistentDNSName(ctx, candidate)
		if conflicting == nil || conflicting.Id == h.Id {
			return candidate, nil
		}
	}

	return "", errors.Errorf("could not generate a new persistent DNS name after %d attempts", numAttempts)
}

// SetPersistentDNSInfo sets the host's persistent DNS name and the associated
// IP address.
func (h *Host) SetPersistentDNSInfo(ctx context.Context, dnsName, ipv4Addr string) error {
	if dnsName == "" || ipv4Addr == "" {
		return errors.New("cannot set empty DNS name or IPv4 address")
	}
	if dnsName == h.PersistentDNSName && h.PublicIPv4 == ipv4Addr {
		return nil
	}

	if err := UpdateOne(ctx, bson.M{
		IdKey: h.Id,
	}, bson.M{
		"$set": bson.M{
			PersistentDNSNameKey: dnsName,
			PublicIPv4Key:        ipv4Addr,
		},
	}); err != nil {
		return err
	}

	h.PersistentDNSName = dnsName
	h.PublicIPv4 = ipv4Addr

	return nil
}

// UnsetPersistentDNSInfo unsets the host's persistent DNS name and the
// associated IP address.
func (h *Host) UnsetPersistentDNSInfo(ctx context.Context) error {
	if h.PersistentDNSName == "" && h.PublicIPv4 == "" {
		return nil
	}

	if err := UpdateOne(ctx, bson.M{
		IdKey: h.Id,
	}, bson.M{
		"$unset": bson.M{
			PersistentDNSNameKey: 1,
			PublicIPv4Key:        1,
		},
	}); err != nil {
		return err
	}

	h.PersistentDNSName = ""
	h.PublicIPv4 = ""

	return nil
}

// UpdateSleepSchedule updates the sleep schedule of an existing host. Note that
// if some fields in the sleep schedule are not being modified, the sleep
// schedule must still contain all the unmodified fields. For example, if this
// host is on a temporary exemption when their daily schedule is updated, the
// new schedule must still have the temporary exemption populated.
func (h *Host) UpdateSleepSchedule(ctx context.Context, schedule SleepScheduleInfo, now time.Time) error {
	if err := schedule.Validate(); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Wrap(err, "invalid sleep schedule").Error(),
		}
	}

	// Since the sleep schedule is being updated, recalculate the next
	// stop/start times based on the new schedule.
	schedule.NextStartTime = time.Time{}
	schedule.NextStopTime = time.Time{}

	nextStart, nextStop, err := schedule.GetNextScheduledStartAndStopTimes(now)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "determining next sleep schedule start/stop times").Error(),
		}
	}

	schedule.NextStartTime = nextStart
	schedule.NextStopTime = nextStop

	if err = setSleepSchedule(ctx, h.Id, schedule); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "setting sleep schedule").Error(),
		}
	}
	h.SleepSchedule = schedule

	return nil
}

// maxTemporaryExemptionDuration is how long after now that an unexpirable host
// can be exempt from the sleep schedule.
const maxTemporaryExemptionDuration = 32 * utility.Day

// GetTemporaryExemption validates and calculates a temporary exemption from the
// host's sleep schedule.
func (h *Host) GetTemporaryExemption(extendBy time.Duration) (time.Time, error) {
	var exemptUntil time.Time
	now := time.Now()
	if h.SleepSchedule.TemporarilyExemptUntil.After(now) {
		// Extend the existing temporary exemption if it's set and not already
		// expired.
		exemptUntil = h.SleepSchedule.TemporarilyExemptUntil.Add(extendBy)
	} else {
		exemptUntil = now.Add(extendBy)
	}

	if err := validateTemporaryExemption(exemptUntil); err != nil {
		return time.Time{}, err
	}

	return exemptUntil, nil
}

func validateTemporaryExemption(exemptUntil time.Time) error {
	if utility.IsZeroTime(exemptUntil) {
		// A zero time temporary exemption is equivalent to removing the
		// temporary exemption, which is always allowed.
		return nil
	}

	if exemptUntil.Before(time.Now()) {
		return errors.Errorf("cannot set a temporary exemption to %s because that is in the past", exemptUntil.Format(time.RFC3339))
	}
	maxTemporaryExemptionTime := time.Now().Add(maxTemporaryExemptionDuration)
	if maxTemporaryExemptionTime.Before(exemptUntil) {
		return errors.Errorf("temporary exemption cannot be extended to %s because temporary exemptions cannot be extended past %s (%s in the future)", exemptUntil.Format(time.RFC3339), maxTemporaryExemptionTime.Format(time.RFC3339), maxTemporaryExemptionDuration.String())
	}

	return nil
}

// SetTemporaryExemption sets a temporary exemption from the host's sleep
// schedule.
func (h *Host) SetTemporaryExemption(ctx context.Context, exemptUntil time.Time) error {
	if h.SleepSchedule.TemporarilyExemptUntil.Equal(exemptUntil) {
		return nil
	}

	if err := validateTemporaryExemption(exemptUntil); err != nil {
		return err
	}

	var extendedBy time.Duration
	now := time.Now()
	if h.SleepSchedule.TemporarilyExemptUntil.After(now) {
		extendedBy = exemptUntil.Sub(h.SleepSchedule.TemporarilyExemptUntil)
	} else {
		extendedBy = exemptUntil.Sub(now)
	}
	grip.Info(message.Fields{
		"message":                  "creating/extending temporary exemption from the sleep schedule",
		"host_id":                  h.Id,
		"distro_id":                h.Distro.Id,
		"exempt_until":             exemptUntil.Format(time.RFC3339),
		"additional_exemption_hrs": extendedBy.Hours(),
		"dashboard":                "evergreen sleep schedule health",
	})

	temporarilyExemptUntilKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleTemporarilyExemptUntilKey)
	sleepScheduleStartKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStartTimeKey)
	sleepScheduleStopKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStopTimeKey)
	update := bson.M{}
	unset := bson.M{
		sleepScheduleStartKey: 1,
		sleepScheduleStopKey:  1,
	}
	if utility.IsZeroTime(exemptUntil) {
		unset[temporarilyExemptUntilKey] = 1
	} else {
		update["$set"] = bson.M{temporarilyExemptUntilKey: exemptUntil}
	}
	update["$unset"] = unset

	if err := UpdateOne(ctx, bson.M{IdKey: h.Id}, update); err != nil {
		return err
	}

	h.SleepSchedule.TemporarilyExemptUntil = exemptUntil

	return nil
}

// IsSleepScheduleEnabled returns whether or not a sleep schedule is enabled
// for the host, taking into account that some hosts are not subject to the
// sleep schedule (e.g. expirable hosts).
func (h *Host) IsSleepScheduleEnabled() bool {
	if !h.NoExpiration {
		return false
	}
	if !utility.StringSliceContains(evergreen.SleepScheduleStatuses, h.Status) {
		return false
	}
	if h.SleepSchedule.IsExempt(time.Now()) {
		return false
	}
	return true
}

// IsExempt returns whether or not the sleep schedule has an exemption.
func (s *SleepScheduleInfo) IsExempt(now time.Time) bool {
	return s.PermanentlyExempt || s.ShouldKeepOff || s.TemporarilyExemptUntil.After(now)
}

// GetNextScheduledStopTime returns the next time a host should be
// stopped according to its sleep schedule.
func (s *SleepScheduleInfo) GetNextScheduledStopTime(now time.Time) (time.Time, error) {
	if s.IsExempt(now) {
		return time.Time{}, nil
	}

	userTimeZone, err := time.LoadLocation(s.TimeZone)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "loading user time zone '%s'", s.TimeZone)
	}

	var calculateTimeAfter time.Time
	if s.NextStartTime.After(now) {
		// If the next start time is known and is in the future, do not try to
		// stop the host again until after it's scheduled to start back up. This
		// is a necessary safeguard against a rare edge case where the host is
		// stopped for the sleep schedule but the user manually starts the host
		// back up again in between a daily stop time and a whole weekday off.
		//
		// As an illustrative example, say the user sets a schedule to stop
		// every day from 10 pm to 6 am, and also has Saturday and Sunday off.
		// It's Friday at 11:30 PM and their host is currently stopped for the
		// sleep schedule. They then manually start up their host. If the next
		// stop time was calculated to be Saturday at midnight (the next
		// scheduled whole day off), they may see their host abruptly shut down
		// at midnight in the middle of working on it. A more graceful behavior
		// would be to avoid stopping the host again until the next time the
		// user would have expected their host to wake up (i.e. Monday at 6 am).
		//
		// To avoid this unfortunate scenario, once the host has been stopped
		// for its sleep schedule, it should not try to stop the host again
		// until after the next time the schedule would typically start it back
		// up.
		calculateTimeAfter = s.NextStartTime
	} else {
		calculateTimeAfter = now
	}
	// It's critical to do all the time calculations in the user's time zone and
	// not in UTC. If the time calculations were all in UTC, they wouldn't
	// account for timezone-specific quirks like daylight savings time changes.
	calculateTimeAfter = calculateTimeAfter.In(userTimeZone)

	// Partition the weekdays into the first day in a series of whole days off
	// (if any) and the weekdays when the daily sleep schedule applies (if any).
	var dailyWeekdays []string
	var firstContiguousWeekdayOff []string
	for _, weekday := range utility.Weekdays {
		isOff := utility.FilterSlice(s.WholeWeekdaysOff, func(weekdayToCheck time.Weekday) bool { return weekdayToCheck == weekday })
		if len(isOff) == 0 {
			dailyWeekdays = append(dailyWeekdays, fmt.Sprintf("%d", weekday))
		} else {
			prevWeekdayOff := (weekday - 1) % time.Weekday(len(utility.Weekdays))
			if prevWeekdayOff < 0 {
				// Convert negative modulo to positive value (because -1 mod 7 = -1).
				prevWeekdayOff += time.Weekday(len(utility.Weekdays))
			}
			isPrevWeekdayOff := utility.FilterSlice(s.WholeWeekdaysOff, func(weekdayToCheck time.Weekday) bool { return weekdayToCheck == prevWeekdayOff })
			if len(isPrevWeekdayOff) == 0 {
				// It's the first day in a contiguous series of weekdays off,
				// which means the host can stop on this day.
				firstContiguousWeekdayOff = append(firstContiguousWeekdayOff, fmt.Sprintf("%d", weekday))
			}
		}
	}

	var nextDailyTrigger time.Time
	if s.DailyStopTime != "" {
		hours, mins, err := parseDailyTime(s.DailyStopTime)
		if err != nil {
			return time.Time{}, errors.Wrap(err, "parsing daily stop time")
		}
		// Get the next time the host can stop for its daily schedule.
		nextDailyTrigger, err = getNextScheduledTime(calculateTimeAfter, fmt.Sprintf("%d %d %s", mins, hours, strings.Join(dailyWeekdays, ",")))
		if err != nil {
			return time.Time{}, errors.Wrap(err, "determining next scheduled stop time from daily stop time")
		}
	}

	var nextWeekdayOffTrigger time.Time
	if len(firstContiguousWeekdayOff) > 0 {
		// Get the next time the host can stop for the first day in a contiguous
		// series of whole weekdays off.
		nextWeekdayOffTrigger, err = getNextScheduledTime(calculateTimeAfter, fmt.Sprintf("00 00 %s", strings.Join(firstContiguousWeekdayOff, ",")))
		if err != nil {
			return time.Time{}, errors.Wrap(err, "determining next scheduled stop time from weekdays off")
		}
	}

	if !utility.IsZeroTime(nextDailyTrigger) && !utility.IsZeroTime(nextWeekdayOffTrigger) {
		if nextDailyTrigger.Before(nextWeekdayOffTrigger) {
			return nextDailyTrigger, nil
		}
		return nextWeekdayOffTrigger, nil
	}
	if !utility.IsZeroTime(nextWeekdayOffTrigger) {
		return nextWeekdayOffTrigger, nil
	}
	if !utility.IsZeroTime(nextDailyTrigger) {
		return nextDailyTrigger, nil
	}

	return time.Time{}, errors.New("neither daily nor whole days off schedule could determine a next stop time")
}

// GetNextScheduledStartTime returns the next time a host should be
// started according to its sleep schedule.
func (s *SleepScheduleInfo) GetNextScheduledStartTime(now time.Time) (time.Time, error) {
	if s.IsExempt(now) {
		return time.Time{}, nil
	}

	userTimeZone, err := time.LoadLocation(s.TimeZone)
	if err != nil {
		return time.Time{}, errors.Wrapf(err, "loading user time zone '%s'", s.TimeZone)
	}

	calculateTimeAfter := now
	// It's critical to do all the time calculations in the user's time zone and
	// not in UTC. If the time calculations were all in UTC, they wouldn't
	// account for timezone-specific quirks like daylight savings time changes.
	calculateTimeAfter = calculateTimeAfter.In(userTimeZone)

	// Partition the weekdays into the first day after a series of whole days
	// off (if any) and the weekdays when the daily waking schedule applies (if
	// any).
	var dailyWeekdays []string
	var afterContiguousWeekdaysOff []string
	for _, weekday := range utility.Weekdays {
		isOff := utility.FilterSlice(s.WholeWeekdaysOff, func(weekdayToCheck time.Weekday) bool { return weekdayToCheck == weekday })
		if len(isOff) == 0 {
			dailyWeekdays = append(dailyWeekdays, fmt.Sprintf("%d", weekday))
		} else {
			nextWeekday := (weekday + 1) % time.Weekday(len(utility.Weekdays))
			isNextWeekdayOff := utility.FilterSlice(s.WholeWeekdaysOff, func(weekdayToCheck time.Weekday) bool { return weekdayToCheck == nextWeekday })
			if len(isNextWeekdayOff) == 0 {
				// It's the end of a contiguous series of weekdays off, which
				// means the host can start up again on this day.
				afterContiguousWeekdaysOff = append(afterContiguousWeekdaysOff, fmt.Sprintf("%d", nextWeekday))
			}
		}
	}

	var nextDailyTrigger time.Time
	if s.DailyStartTime != "" {
		hours, mins, err := parseDailyTime(s.DailyStartTime)
		if err != nil {
			return time.Time{}, errors.Wrap(err, "parsing daily start time")
		}
		// Get the next time the host can start for its daily schedule.
		nextDailyTrigger, err = getNextScheduledTime(calculateTimeAfter, fmt.Sprintf("%d %d %s", mins, hours, strings.Join(dailyWeekdays, ",")))
		if err != nil {
			return time.Time{}, errors.Wrap(err, "determining next start time from daily start time")
		}
	}

	var nextAfterWeekdayOffTrigger time.Time
	if len(afterContiguousWeekdaysOff) > 0 {
		// Get the next time the host can start for the end of a series of whole
		// weekdays off.
		nextAfterWeekdayOffTrigger, err = getNextScheduledTime(calculateTimeAfter, fmt.Sprintf("00 00 %s", strings.Join(afterContiguousWeekdaysOff, ",")))
		if err != nil {
			return time.Time{}, errors.Wrap(err, "determining next start time to start after weekdays off")
		}
	}

	if !utility.IsZeroTime(nextDailyTrigger) && !utility.IsZeroTime(nextAfterWeekdayOffTrigger) {
		if nextDailyTrigger.Year() == nextAfterWeekdayOffTrigger.Year() &&
			nextDailyTrigger.Month() == nextAfterWeekdayOffTrigger.Month() &&
			nextDailyTrigger.Day() == nextAfterWeekdayOffTrigger.Day() &&
			s.isDailyOvernightSchedule() {
			// If the two proposed start times are on the same date and the
			// daily one runs overnight, choose the daily one. For example, if
			// the host has Sunday off and a daily sleep schedule from 10 pm to
			// 4 am, it should start at 4 am on Monday (rather than 12 am)
			// because the sleep schedule is contiguous.
			return nextDailyTrigger, nil
		}

		if nextDailyTrigger.Before(nextAfterWeekdayOffTrigger) {
			return nextDailyTrigger, nil
		}
		return nextAfterWeekdayOffTrigger, nil
	}
	if !utility.IsZeroTime(nextAfterWeekdayOffTrigger) {
		return nextAfterWeekdayOffTrigger, nil
	}
	if !utility.IsZeroTime(nextDailyTrigger) {
		return nextDailyTrigger, nil
	}

	return time.Time{}, errors.New("neither daily nor whole days off schedule could determine a next start time")
}

// GetNextScheduledStartAndStopTimes is a convenience function to return both
// the next time a host should be started and the next time a host should be
// stopped according to its sleep schedule.
func (s *SleepScheduleInfo) GetNextScheduledStartAndStopTimes(now time.Time) (nextStart, nextStop time.Time, err error) {
	catcher := grip.NewBasicCatcher()
	nextStart, err = s.GetNextScheduledStartTime(now)
	catcher.Wrap(err, "getting next start time")
	nextStop, err = s.GetNextScheduledStopTime(now)
	catcher.Wrap(err, "getting next stop time")
	if catcher.HasErrors() {
		return time.Time{}, time.Time{}, catcher.Resolve()
	}
	return nextStart, nextStop, nil
}

// getNextScheduledTime returns the next time a cron schedule should trigger
// after the given time. The spec must be in the format "<minute> <hour> <weekday>".
func getNextScheduledTime(after time.Time, spec string) (time.Time, error) {
	schedule, err := cron.NewParser(cron.Minute | cron.Hour | cron.Dow).Parse(spec)
	if err != nil {
		return time.Time{}, errors.Wrap(err, "creating schedule")
	}
	nextTriggerAt := schedule.Next(after)
	if utility.IsZeroTime(nextTriggerAt) {
		return time.Time{}, errors.New("could not determine next scheduled time")
	}
	return nextTriggerAt, nil
}

// SetNextScheduledStartTime sets the next time the host is planned to start for its
// sleep schedule.
func (h *Host) SetNextScheduledStartTime(ctx context.Context, t time.Time) error {
	sleepScheduleStartKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStartTimeKey)
	update := bson.M{}
	if utility.IsZeroTime(t) {
		update["$unset"] = bson.M{sleepScheduleStartKey: 1}
	} else {
		update["$set"] = bson.M{sleepScheduleStartKey: t}
	}
	if err := UpdateOne(ctx,
		bson.M{IdKey: h.Id},
		update,
	); err != nil {
		return err
	}

	h.SleepSchedule.NextStartTime = t

	return nil
}

// SetNextScheduledStopTime sets the next time the host is planned to stop for its
// sleep schedule.
func (h *Host) SetNextScheduledStopTime(ctx context.Context, t time.Time) error {
	sleepScheduleStopKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStopTimeKey)
	update := bson.M{}
	if utility.IsZeroTime(t) {
		update["$unset"] = bson.M{sleepScheduleStopKey: 1}
	} else {
		update["$set"] = bson.M{sleepScheduleStopKey: t}
	}

	if err := UpdateOne(ctx,
		bson.M{IdKey: h.Id},
		update,
	); err != nil {
		return err
	}

	h.SleepSchedule.NextStopTime = t

	return nil
}

// SetNextScheduledStartAndStopTimes sets both the next time the host is planned
// to start and the next time the host is planned to stop for its sleep
// schedule.
func (h *Host) SetNextScheduledStartAndStopTimes(ctx context.Context, nextStart, nextStop time.Time) error {
	update := bson.M{}
	unset := bson.M{}
	set := bson.M{}

	sleepScheduleNextStartKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStartTimeKey)
	if utility.IsZeroTime(nextStart) {
		unset[sleepScheduleNextStartKey] = 1
	} else {
		set[sleepScheduleNextStartKey] = nextStart
	}

	sleepScheduleNextStopKey := bsonutil.GetDottedKeyName(SleepScheduleKey, SleepScheduleNextStopTimeKey)
	if utility.IsZeroTime(nextStop) {
		unset[sleepScheduleNextStopKey] = 1
	} else {
		set[sleepScheduleNextStopKey] = nextStop
	}
	if len(unset) > 0 {
		update["$unset"] = unset
	}
	if len(set) > 0 {
		update["$set"] = set
	}

	if err := UpdateOne(ctx,
		bson.M{IdKey: h.Id},
		update,
	); err != nil {
		return err
	}

	h.SleepSchedule.NextStartTime = nextStart
	h.SleepSchedule.NextStopTime = nextStop

	return nil
}
