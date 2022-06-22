package pod

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

// Pod represents a related collection of containers. This model holds metadata
// about containers running in a container orchestration system.
type Pod struct {
	// ID is the unique identifier for the metadata document.
	ID string `bson:"_id" json:"id"`
	// Type indicates the type of pod that this is.
	Type Type `bson:"type" json:"type"`
	// Status is the current state of the pod.
	Status Status `bson:"status"`
	// TaskCreationOpts are options to configure how a task should be
	// containerized and run in a pod.
	TaskContainerCreationOpts TaskContainerCreationOptions `bson:"task_creation_opts,omitempty" json:"task_creation_opts,omitempty"`
	// TimeInfo contains timing information for the pod's lifecycle.
	TimeInfo TimeInfo `bson:"time_info,omitempty" json:"time_info,omitempty"`
	// Resources are external resources that are owned and managed by this pod.
	Resources ResourceInfo `bson:"resource_info,omitempty" json:"resource_info,omitempty"`
	// AgentVersion is the version of the agent running on this pod if it's a
	// pod that runs tasks.
	AgentVersion string `bson:"agent_version,omitempty" json:"agent_version,omitempty"`
	// RunningTask is the ID of the task currently running on the pod.
	RunningTask string `bson:"running_task,omitempty" json:"running_task,omitempty"`
}

// TaskIntentPodOptions represents options to create an intent pod that runs
// container tasks.
type TaskIntentPodOptions struct {
	// ID is the pod identifier. If unspecified, it defaults to a new BSON
	// object ID.
	ID string
	// Secret is the shared secret value between the server and the pod for
	// authentication when the host is provisioned. If unspecified, it defaults
	// to a random string.
	Secret string

	// The remaining fields correspond to the ones in
	// TaskContainerCreationOptions.

	CPU            int
	MemoryMB       int
	OS             OS
	Arch           Arch
	WindowsVersion WindowsVersion
	Image          string
	WorkingDir     string
	RepoUsername   string
	RepoPassword   string
}

// Validate checks that the options to create a task intent pod are valid and
// sets defaults if possible.
func (o *TaskIntentPodOptions) Validate() error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(o.CPU <= 0, "CPU must be a positive non-zero value")
	catcher.NewWhen(o.MemoryMB <= 0, "memory must be a positive non-zero value")
	catcher.Wrap(o.OS.Validate(), "invalid OS")
	catcher.Wrap(o.Arch.Validate(), "invalid CPU architecture")
	if o.OS == OSWindows {
		catcher.Wrap(o.WindowsVersion.Validate(), "must specify a valid Windows version")
	}
	catcher.NewWhen(o.Image == "", "missing image")
	catcher.NewWhen(o.WorkingDir == "", "missing working directory")

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	if o.ID == "" {
		o.ID = primitive.NewObjectID().Hex()
	}
	if o.Secret == "" {
		o.Secret = utility.RandomString()
	}

	return nil
}

const (
	// PodIDEnvVar is the name of the environment variable containing the pod
	// ID.
	PodIDEnvVar = "POD_ID"
	// PodIDEnvVar is the name of the environment variable containing the shared
	// secret between the server and the pod.
	PodSecretEnvVar = "POD_SECRET"
)

// NewTaskIntentPod creates a new intent pod to run container tasks from the
// given initialization options.
func NewTaskIntentPod(opts TaskIntentPodOptions) (*Pod, error) {
	if err := opts.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	p := Pod{
		ID:     opts.ID,
		Status: StatusInitializing,
		Type:   TypeAgent,
		TaskContainerCreationOpts: TaskContainerCreationOptions{
			CPU:            opts.CPU,
			MemoryMB:       opts.MemoryMB,
			OS:             opts.OS,
			Arch:           opts.Arch,
			WindowsVersion: opts.WindowsVersion,
			Image:          opts.Image,
			WorkingDir:     opts.WorkingDir,
			RepoUsername:   opts.RepoUsername,
			RepoPassword:   opts.RepoPassword,
		},
		TimeInfo: TimeInfo{
			Initializing: time.Now(),
		},
	}
	p.TaskContainerCreationOpts.EnvVars = map[string]string{
		PodIDEnvVar: opts.ID,
	}
	p.TaskContainerCreationOpts.EnvSecrets = map[string]Secret{
		PodSecretEnvVar: {
			Value:  opts.Secret,
			Exists: utility.FalsePtr(),
			Owned:  utility.TruePtr(),
		},
	}

	return &p, nil
}

// Type is the type of pod.
type Type string

const (
	// TypeAgent indicates that it is a pod that is running the Evergreen agent
	// in a container.
	TypeAgent Type = "agent"
)

// Status represents a possible state for a pod.
type Status string

const (
	// StatusInitializing indicates that a pod is waiting to be created.
	StatusInitializing Status = "initializing"
	// StatusStarting indicates that a pod's containers are starting.
	StatusStarting Status = "starting"
	// StatusRunning indicates that the pod's containers are running.
	StatusRunning Status = "running"
	// StatusDecommissioned indicates that the pod is currently running but will
	// be terminated shortly.
	StatusDecommissioned Status = "decommissioned"
	// StatusTerminated indicates that all of the pod's containers and
	// associated resources have been deleted.
	StatusTerminated Status = "terminated"
)

// Validate checks that the pod status is recognized.
func (s Status) Validate() error {
	switch s {
	case StatusInitializing, StatusStarting, StatusRunning, StatusTerminated:
		return nil
	default:
		return errors.Errorf("unrecognized pod status '%s'", s)
	}
}

// TimeInfo represents timing information about the pod.
type TimeInfo struct {
	// Initializing is the time when this pod was initialized and is waiting to
	// be created in the container orchestration service. This should correspond
	// with the time when the pod transitions to the "initializing" status.
	Initializing time.Time `bson:"initializing,omitempty" json:"initializing,omitempty"`
	// Starting is the time when this pod was actually requested to start its
	// containers. This should correspond with the time when the pod transitions
	// to the "starting" status.
	Starting time.Time `bson:"starting,omitempty" json:"starting,omitempty"`
	// LastCommunicated is the last time that the pod connected to the
	// application server or the application server connected to the pod. This
	// is used as one indicator of liveliness.
	LastCommunicated time.Time `bson:"last_communicated,omitempty" json:"last_communicated,omitempty"`
	// AgentStarted is the time that the agent initiated first contact with the
	// application server. This only applies to agent pods.
	AgentStarted time.Time `bson:"agent_started,omitempty" json:"agent_started,omitempty"`
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (i TimeInfo) IsZero() bool {
	return i == TimeInfo{}
}

// ResourceInfo represents information about external resources associated with
// a pod.
type ResourceInfo struct {
	// ExternalID is the unique resource identifier for the aggregate collection
	// of containers running for the pod in the container service.
	ExternalID string `bson:"external_id,omitempty" json:"external_id,omitempty"`
	// DefinitionID is the resource identifier for the pod definition template.
	DefinitionID string `bson:"definition_id,omitempty" json:"definition_id,omitempty"`
	// Cluster is the namespace where the containers are running.
	Cluster string `bson:"cluster,omitempty" json:"cluster,omitempty"`
	// Containers include resource information about containers running in the
	// pod.
	Containers []ContainerResourceInfo `bson:"containers,omitempty" json:"containers,omitempty"`
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (i ResourceInfo) IsZero() bool {
	return i.ExternalID == "" && i.DefinitionID == "" && i.Cluster == "" && len(i.Containers) == 0
}

// ContainerResourceInfo represents information about external resources
// associated with a container.
type ContainerResourceInfo struct {
	// ExternalID is the unique resource identifier for the container running in
	// the container service.
	ExternalID string `bson:"external_id,omitempty" json:"external_id,omitempty"`
	// Name is the friendly name of the container.
	Name string `bson:"name,omitempty" json:"name,omitempty"`
	// SecretIDs are the resource identifiers for the secrets owned by this
	// container.
	SecretIDs []string `bson:"secret_ids,omitempty" json:"secret_ids,omitempty"`
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (i ContainerResourceInfo) IsZero() bool {
	return i.ExternalID == "" && i.Name == "" && len(i.SecretIDs) == 0
}

// TaskContainerCreationOptions are options to apply to the task's container
// when creating a pod in the container orchestration service.
type TaskContainerCreationOptions struct {
	// Image is the image that the task's container will run.
	Image string `bson:"image" json:"image"`
	// RepoUsername is the username of the repository containing the image. This
	// is only necessary if it is a private repository.
	RepoUsername string `bson:"repo_username,omitempty" json:"repo_username,omitempty"`
	// RepoPassword is the password of the repository containing the image. This
	// is only necessary if it is a private repository.
	RepoPassword string `bson:"repo_password,omitempty" json:"repo_password,omitempty"`
	// MemoryMB is the memory (in MB) that the task's container will be
	// allocated.
	MemoryMB int `bson:"memory_mb" json:"memory_mb"`
	// CPU is the CPU units that the task will be allocated. 1024 CPU units is
	// equivalent to 1vCPU.
	CPU int `bson:"cpu" json:"cpu"`
	// OS indicates which particular operating system the pod's containers run
	// on.
	OS OS `bson:"os" json:"os"`
	// Arch indicates the particular architecture that the pod's containers run
	// on.
	Arch Arch `bson:"arch" json:"arch"`
	// WindowsVersion specifies the particular version of Windows the container
	// should run in. This only applies if OS is OSWindows.
	WindowsVersion WindowsVersion `bson:"windows_version,omitempty" json:"windows_version,omitempty"`
	// EnvVars is a mapping of the non-secret environment variables to expose in
	// the task's container environment.
	EnvVars map[string]string `bson:"env_vars,omitempty" json:"env_vars,omitempty"`
	// EnvSecrets are secret values to expose in the task's container
	// environment variables. The key is the name of the environment variable
	// and the value is the configuration for the secret value.
	EnvSecrets map[string]Secret `bson:"env_secrets,omitempty" json:"env_secrets,omitempty"`
	// WorkingDir is the working directory for the task's container.
	WorkingDir string `bson:"working_dir,omitempty" json:"working_dir,omitempty"`
}

// OS represents a recognized operating system for pods.
type OS string

const (
	// OSLinux indicates that the pods will run with Linux containers.
	OSLinux OS = "linux"
	// OSWindows indicates that the pods will run with Windows containers.
	OSWindows OS = "windows"
)

// validOperatingSystems contains all the recognized pod operating systems.
var validOperatingSystems = []OS{OSLinux, OSWindows}

// Validate checks that the pod OS is recognized.
func (os OS) Validate() error {
	switch os {
	case OSLinux, OSWindows:
		return nil
	default:
		return errors.Errorf("unrecognized pod OS '%s'", os)
	}
}

// Matches returns whether or not the pod OS matches the given Evergreen ECS
// config OS.
func (os OS) Matches(other evergreen.ECSOS) bool {
	switch os {
	case OSLinux:
		return other == evergreen.ECSOSLinux
	case OSWindows:
		return other == evergreen.ECSOSWindows
	default:
		return false
	}
}

// ImportOS converts the container OS into its equivalent pod OS.
func ImportOS(os evergreen.ContainerOS) (OS, error) {
	switch os {
	case evergreen.LinuxOS:
		return OSLinux, nil
	case evergreen.WindowsOS:
		return OSWindows, nil
	default:
		return "", errors.Errorf("unrecognized OS '%s'", os)
	}
}

// Arch represents recognized architectures for pods.
type Arch string

const (
	ArchAMD64 Arch = "amd64"
	ArchARM64 Arch = "arm64"
)

// validArchitectures contains all the recognized pod CPU architectures.
var validArchitectures = []Arch{ArchAMD64, ArchARM64}

// Validate checks that the pod architecture is recognized.
func (a Arch) Validate() error {
	switch a {
	case ArchAMD64, ArchARM64:
		return nil
	default:
		return errors.Errorf("unrecognized pod architecture '%s'", a)
	}
}

// Matches returns whether or not the pod CPU architecture matches the given
// Evergreen ECS config CPU architecture.
func (a Arch) Matches(arch evergreen.ECSArch) bool {
	switch a {
	case ArchAMD64:
		return arch == evergreen.ECSArchAMD64
	case ArchARM64:
		return arch == evergreen.ECSArchARM64
	default:
		return false
	}
}

// ImportArch converts the container CPU architecture into its equivalent pod
// CPU architecture.
func ImportArch(arch evergreen.ContainerArch) (Arch, error) {
	switch arch {
	case evergreen.ArchAMD64:
		return ArchAMD64, nil
	case evergreen.ArchARM64:
		return ArchARM64, nil
	default:
		return "", errors.Errorf("unrecognized CPU architecture '%s'", arch)
	}
}

// WindowsVersion represents a specific version of Windows that a pod is allowed
// to run on. This only applies to pods running Windows containers.
type WindowsVersion string

const (
	// WindowsVersionServer2016 indicates that a pod is compatible to run on an
	// instance that is running Windows Server 2016.
	WindowsVersionServer2016 WindowsVersion = "SERVER_2016"
	// WindowsVersionServer2016 indicates that a pod is compatible to run on an
	// instance that is running Windows Server 2019.
	WindowsVersionServer2019 WindowsVersion = "SERVER_2019"
	// WindowsVersionServer2016 indicates that a pod is compatible to run on an
	// instance that is running Windows Server 2022.
	WindowsVersionServer2022 WindowsVersion = "SERVER_2022"
)

// validWindowsVersions contains all the recognized pod Windows versions.
var validWindowsVersions = []WindowsVersion{WindowsVersionServer2016, WindowsVersionServer2019, WindowsVersionServer2022}

// Validate checks that the pod Windows version is recognized.
func (v WindowsVersion) Validate() error {
	switch v {
	case WindowsVersionServer2016, WindowsVersionServer2019, WindowsVersionServer2022:
		return nil
	default:
		return errors.Errorf("unrecognized Windows version '%s'", v)
	}
}

// ImportWindowsVersion converts the container Windows version into its
// equivalent pod Windows version.
func ImportWindowsVersion(winVer evergreen.WindowsVersion) (WindowsVersion, error) {
	switch winVer {
	case evergreen.Windows2016:
		return WindowsVersionServer2016, nil
	case evergreen.Windows2019:
		return WindowsVersionServer2019, nil
	case evergreen.Windows2022:
		return WindowsVersionServer2022, nil
	default:
		return "", errors.Errorf("unrecognized Windows version '%s'", winVer)
	}
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (o TaskContainerCreationOptions) IsZero() bool {
	return o.MemoryMB == 0 && o.CPU == 0 && o.OS == "" && o.Arch == "" && o.WindowsVersion == "" && o.Image == "" && o.RepoUsername == "" && o.RepoPassword == "" && o.WorkingDir == "" && len(o.EnvVars) == 0 && len(o.EnvSecrets) == 0
}

// Secret is a sensitive secret that a pod can access. The secret is managed
// in an integrated secrets storage service.
type Secret struct {
	// Name is the friendly name of the secret.
	Name string `bson:"name,omitempty" json:"name,omitempty" yaml:"name,omitempty"`
	// ExternalID is the unique external resource identifier for a secret that
	// already exists in the secrets storage service.
	ExternalID string `bson:"external_id,omitempty" json:"external_id,omitempty" yaml:"external_id,omitempty"`
	// Value is the value of the secret. If the secret does not yet exist, it
	// will be created; otherwise, this is just a cached copy of the actual
	// value stored in the secrets storage service.
	Value string `bson:"value,omitempty" json:"value,omitempty" yaml:"value,omitempty"`
	// Exists determines whether or not the secret already exists in the secrets
	// storage service. If this is false, then a new secret will be stored.
	Exists *bool `bson:"exists,omitempty" json:"exists,omitempty" yaml:"exists,omitempty"`
	// Owned determines whether or not the secret is owned by its pod. If this
	// is true, then its lifetime is tied to the pod's lifetime, implying that
	// when the pod is cleaned up, this secret is also cleaned up.
	Owned *bool `bson:"owned,omitempty" json:"owned,omitempty" yaml:"owned,omitempty"`
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (s Secret) IsZero() bool {
	return s == Secret{}
}

// Insert inserts a new pod into the collection. This relies on the global Anser
// DB session.
func (p *Pod) Insert() error {
	return db.Insert(Collection, p)
}

// InsertWithContext is the same as Insert, but it respects the given context by
// avoiding the global Anser DB session.
func (p *Pod) InsertWithContext(ctx context.Context, env evergreen.Environment) error {
	if _, err := env.DB().Collection(Collection).InsertOne(ctx, p); err != nil {
		return err
	}
	return nil
}

// Remove removes the pod from the collection.
func (p *Pod) Remove() error {
	return db.Remove(
		Collection,
		bson.M{
			IDKey: p.ID,
		},
	)
}

// UpdateStatus updates the pod status.
func (p *Pod) UpdateStatus(s Status) error {
	ts := utility.BSONTime(time.Now())
	if err := UpdateOneStatus(p.ID, p.Status, s, ts); err != nil {
		return errors.Wrap(err, "updating status")
	}

	p.Status = s
	switch s {
	case StatusInitializing:
		p.TimeInfo.Initializing = ts
	case StatusStarting:
		p.TimeInfo.Starting = ts
	}

	return nil
}

// UpdateResources updates the pod resources.
func (p *Pod) UpdateResources(info ResourceInfo) error {
	setFields := bson.M{
		ResourcesKey: info,
	}

	if err := UpdateOne(ByID(p.ID), bson.M{
		"$set": setFields,
	}); err != nil {
		return err
	}

	p.Resources = info

	return nil
}

// GetSecret returns the shared secret between the server and the pod. If the
// secret is unset, this will return an error.
func (p *Pod) GetSecret() (*Secret, error) {
	s, ok := p.TaskContainerCreationOpts.EnvSecrets[PodSecretEnvVar]
	if !ok {
		return nil, errors.New("pod does not have a secret")
	}
	return &s, nil
}

// SetRunningTask sets the task to dispatch to the pod.
func (p *Pod) SetRunningTask(ctx context.Context, env evergreen.Environment, taskID string) error {
	query := bson.M{
		IDKey:          p.ID,
		StatusKey:      StatusRunning,
		RunningTaskKey: nil,
	}
	update := bson.M{
		"$set": bson.M{
			RunningTaskKey: taskID,
		},
	}

	res, err := env.DB().Collection(Collection).UpdateOne(ctx, query, update)
	if err != nil {
		return err
	}
	if res.ModifiedCount == 0 {
		return errors.New("pod was not updated")
	}

	p.RunningTask = taskID

	return nil
}

// ClearRunningTask clears the current task dispatched to the pod.
func (p *Pod) ClearRunningTask() error {
	if p.RunningTask == "" {
		return nil
	}

	if err := UpdateOne(bson.M{
		IDKey:          p.ID,
		RunningTaskKey: p.RunningTask,
	}, bson.M{
		"$unset": bson.M{
			RunningTaskKey: 1,
		},
	}); err != nil {
		return errors.Wrap(err, "clearing running task")
	}

	p.RunningTask = ""

	return nil
}

// SetAgentStartTime sets the time when the pod's agent started.
func (p *Pod) SetAgentStartTime() error {
	ts := utility.BSONTime(time.Now())
	if err := UpdateOne(ByID(p.ID), bson.M{
		"$set": bson.M{
			bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoAgentStartedKey): ts,
		},
	}); err != nil {
		return err
	}

	p.TimeInfo.AgentStarted = ts

	return nil
}
