package pod

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/util"
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
	TaskContainerCreationOpts TaskContainerCreationOptions `bson:"task_creation_opts,omitempty" json:"task_creation_opts"`
	// Family is the family name of the pod definition stored in the cloud
	// provider.
	Family string `bson:"family,omitempty" json:"family,omitempty"`
	// TimeInfo contains timing information for the pod's lifecycle.
	TimeInfo TimeInfo `bson:"time_info,omitempty" json:"time_info"`
	// Resources are external resources that are owned and managed by this pod.
	Resources ResourceInfo `bson:"resource_info,omitempty" json:"resource_info"`
	// TaskRuntimeInfo contains information about the tasks that a pod is
	// assigned.
	TaskRuntimeInfo TaskRuntimeInfo `bson:"task_runtime_info,omitempty" json:"task_runtime_info"`
	// AgentVersion is the version of the agent running on this pod if it's a
	// pod that runs tasks.
	AgentVersion string `bson:"agent_version,omitempty" json:"agent_version,omitempty"`
}

// TaskIntentPodOptions represents options to create an intent pod that runs
// container tasks.
type TaskIntentPodOptions struct {
	// ID is the pod identifier. If unspecified, it defaults to a new BSON
	// object ID.
	ID string

	// The remaining fields correspond to the ones in
	// TaskContainerCreationOptions.

	CPU                 int
	MemoryMB            int
	OS                  OS
	Arch                Arch
	WindowsVersion      WindowsVersion
	Image               string
	RepoCredsExternalID string
	WorkingDir          string
	PodSecretExternalID string
	PodSecretValue      string
}

// Validate checks that the options to create a task intent pod are valid and
// sets defaults if possible.
func (o *TaskIntentPodOptions) Validate(ecsConf evergreen.ECSConfig) error {
	catcher := grip.NewBasicCatcher()
	catcher.NewWhen(o.CPU <= 0, "CPU must be a positive non-zero value")
	catcher.ErrorfWhen(ecsConf.MaxCPU > 0 && o.CPU > ecsConf.MaxCPU, "CPU cannot exceed maximum global CPU limit of %d CPU units", ecsConf.MaxCPU)
	catcher.NewWhen(o.MemoryMB <= 0, "memory must be a positive non-zero value")
	catcher.ErrorfWhen(ecsConf.MaxMemoryMB > 0 && o.MemoryMB > ecsConf.MaxMemoryMB, "memory cannot exceed maximum global memory limit of %d MB", ecsConf.MaxCPU)
	catcher.Wrap(o.OS.Validate(), "invalid OS")
	catcher.Wrap(o.Arch.Validate(), "invalid CPU architecture")
	if o.OS == OSWindows {
		catcher.Wrap(o.WindowsVersion.Validate(), "must specify a valid Windows version")
	}
	catcher.ErrorfWhen(len(ecsConf.AllowedImages) > 0 && !util.HasAllowedImageAsPrefix(o.Image, ecsConf.AllowedImages), "image '%s' not allowed", o.Image)
	catcher.NewWhen(o.Image == "", "missing image")
	catcher.NewWhen(o.WorkingDir == "", "missing working directory")
	catcher.NewWhen(o.PodSecretExternalID == "", "missing pod secret external ID")
	catcher.NewWhen(o.PodSecretValue == "", "missing pod secret value")

	if catcher.HasErrors() {
		return catcher.Resolve()
	}

	if o.ID == "" {
		o.ID = primitive.NewObjectID().Hex()
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
func NewTaskIntentPod(ecsConf evergreen.ECSConfig, opts TaskIntentPodOptions) (*Pod, error) {
	if err := opts.Validate(ecsConf); err != nil {
		return nil, errors.Wrap(err, "invalid options")
	}

	containerOpts := TaskContainerCreationOptions{
		CPU:                 opts.CPU,
		MemoryMB:            opts.MemoryMB,
		OS:                  opts.OS,
		Arch:                opts.Arch,
		WindowsVersion:      opts.WindowsVersion,
		Image:               opts.Image,
		RepoCredsExternalID: opts.RepoCredsExternalID,
		WorkingDir:          opts.WorkingDir,
		EnvVars: map[string]string{
			PodIDEnvVar: opts.ID,
		},
		EnvSecrets: map[string]Secret{
			PodSecretEnvVar: {
				ExternalID: opts.PodSecretExternalID,
				Value:      opts.PodSecretValue,
			},
		},
	}

	p := Pod{
		ID:                        opts.ID,
		Status:                    StatusInitializing,
		Type:                      TypeAgent,
		TaskContainerCreationOpts: containerOpts,
		TimeInfo: TimeInfo{
			Initializing: time.Now(),
		},
		Family: containerOpts.GetFamily(ecsConf),
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
	// RepoCredsExternalID is the external identifier for the repository
	// credentials.
	RepoCredsExternalID string `bson:"repo_creds_external_id,omitempty" json:"repo_creds_external_id,omitempty"`
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

// Matches returns whether or not the pod Windows Version matches the given
// Evergreen ECS config Windows version.
func (v WindowsVersion) Matches(other evergreen.ECSWindowsVersion) bool {
	switch v {
	case WindowsVersionServer2016:
		return other == evergreen.ECSWindowsServer2016
	case WindowsVersionServer2019:
		return other == evergreen.ECSWindowsServer2019
	case WindowsVersionServer2022:
		return other == evergreen.ECSWindowsServer2022
	default:
		return false
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

type hashableEnvSecret struct {
	name   string
	secret Secret
}

func (hev hashableEnvSecret) hash() string {
	h := utility.NewSHA1Hash()
	h.Add(hev.name)
	h.Add(hev.secret.hash())
	return h.Sum()
}

type hashableEnvSecrets []hashableEnvSecret

func newHashableEnvSecrets(envSecrets map[string]Secret) hashableEnvSecrets {
	var hes hashableEnvSecrets
	for k, s := range envSecrets {
		hes = append(hes, hashableEnvSecret{
			name:   k,
			secret: s,
		})
	}
	sort.Sort(hes)
	return hes
}

func (hev hashableEnvSecrets) hash() string {
	if !sort.IsSorted(hev) {
		sort.Sort(hev)
	}

	h := utility.NewSHA1Hash()
	for _, ev := range hev {
		h.Add(ev.hash())
	}
	return h.Sum()
}

func (hes hashableEnvSecrets) Len() int {
	return len(hes)
}

func (hes hashableEnvSecrets) Less(i, j int) bool {
	return hes[i].name < hes[j].name
}

func (hes hashableEnvSecrets) Swap(i, j int) {
	hes[i], hes[j] = hes[j], hes[i]
}

// Hash returns the hash digest of the creation options for the container. This
// is used to create a pod definition, which acts as a template for the actual
// pod that will run the container.
func (o *TaskContainerCreationOptions) Hash() string {
	h := utility.NewSHA1Hash()
	h.Add(o.Image)
	h.Add(o.RepoCredsExternalID)
	h.Add(fmt.Sprint(o.MemoryMB))
	h.Add(fmt.Sprint(o.CPU))
	h.Add(string(o.OS))
	h.Add(string(o.Arch))
	h.Add(string(o.WindowsVersion))
	h.Add(o.WorkingDir)
	// This intentionally does not hash the plaintext environment variables
	// because some of them (such as the pod ID) vary between each pod. If they
	// were included in the pod definition, it would reduce the reusability of
	// pod definitions across different pods.
	// Instead of setting these per-pod values during pod definition creation,
	// the environment variables are injected via overriding options when the
	// pod is started, so they are not relevant to the creation options hash.
	h.Add(newHashableEnvSecrets(o.EnvSecrets).hash())
	return h.Sum()
}

// GetFamily returns the family name for the cloud pod definition to be used
// for these container creation options.
func (o *TaskContainerCreationOptions) GetFamily(ecsConf evergreen.ECSConfig) string {
	return strings.Join([]string{strings.TrimRight(ecsConf.TaskDefinitionPrefix, "-"), "task", o.Hash()}, "-")
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (o TaskContainerCreationOptions) IsZero() bool {
	return o.MemoryMB == 0 && o.CPU == 0 && o.OS == "" && o.Arch == "" && o.WindowsVersion == "" && o.Image == "" && o.RepoCredsExternalID == "" && o.WorkingDir == "" && len(o.EnvVars) == 0 && len(o.EnvSecrets) == 0
}

// Secret is a sensitive secret that a pod can access. The secret is managed
// in an integrated secrets storage service.
type Secret struct {
	// ExternalID is the unique external resource identifier for a secret that
	// already exists in the secrets storage service.
	ExternalID string `bson:"external_id,omitempty" json:"external_id,omitempty"`
	// Value is the value of the secret. This is a cached copy of the actual
	// secret value stored in the secrets storage service.
	Value string `bson:"value,omitempty" json:"value,omitempty"`
}

func (s Secret) hash() string {
	h := utility.NewSHA1Hash()
	h.Add(s.ExternalID)
	// Intentionally do not add the value, because the value is a cached copy
	// from the external secrets storage. The pod definition does not depend on
	// the particular value of the secret, just its external ID.
	return h.Sum()
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (s Secret) IsZero() bool {
	return s == Secret{}
}

// TaskRuntimeInfo contains information for pods running tasks about the tasks
// that it is running or has run previously.
type TaskRuntimeInfo struct {
	// RunningTaskID is the ID of the task currently running on the pod.
	RunningTaskID string `bson:"running_task_id,omitempty" json:"running_task_id,omitempty"`
	// RunningTaskExecution is the execution number of the task currently
	// running on the pod.
	RunningTaskExecution int `bson:"running_task_execution,omitempty" json:"running_task_execution,omitempty"`
}

// IsZero implements the bsoncodec.Zeroer interface for the sake of defining the
// zero value for BSON marshalling.
func (i TaskRuntimeInfo) IsZero() bool {
	return i == TaskRuntimeInfo{}
}

// Insert inserts a new pod into the collection. This relies on the global Anser
// DB session.
func (p *Pod) Insert(ctx context.Context) error {
	return db.Insert(ctx, Collection, p)
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
func (p *Pod) Remove(ctx context.Context) error {
	return db.Remove(
		ctx,
		Collection,
		bson.M{
			IDKey: p.ID,
		},
	)
}

// UpdateStatus updates the pod status.
func (p *Pod) UpdateStatus(ctx context.Context, s Status, reason string) error {
	ts := utility.BSONTime(time.Now())
	if err := UpdateOneStatus(ctx, p.ID, p.Status, s, ts, reason); err != nil {
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
func (p *Pod) UpdateResources(ctx context.Context, info ResourceInfo) error {
	setFields := bson.M{
		ResourcesKey: info,
	}

	if err := UpdateOne(ctx, ByID(p.ID), bson.M{
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
func (p *Pod) SetRunningTask(ctx context.Context, env evergreen.Environment, taskID string, taskExecution int) error {
	if p.TaskRuntimeInfo.RunningTaskID == taskID && p.TaskRuntimeInfo.RunningTaskExecution == taskExecution {
		return nil
	}
	if p.TaskRuntimeInfo.RunningTaskID != "" {
		return errors.Errorf("cannot set running task to '%s' execution %d when it is already set to '%s' execution %d", taskID, taskExecution, p.TaskRuntimeInfo.RunningTaskID, p.TaskRuntimeInfo.RunningTaskExecution)
	}

	runningTaskIDKey := bsonutil.GetDottedKeyName(TaskRuntimeInfoKey, TaskRuntimeInfoRunningTaskIDKey)
	runningTaskExecutionKey := bsonutil.GetDottedKeyName(TaskRuntimeInfoKey, TaskRuntimeInfoRunningTaskExecutionKey)
	query := bson.M{
		IDKey:                   p.ID,
		StatusKey:               StatusRunning,
		runningTaskIDKey:        nil,
		runningTaskExecutionKey: nil,
	}
	update := bson.M{
		"$set": bson.M{
			runningTaskIDKey:        taskID,
			runningTaskExecutionKey: taskExecution,
			// Decommissioning ensures that the pod cannot run another task.
			// TODO (PM-2618): adjust this to handle cases such as
			// single-container task groups, where the pod may be reused.
			StatusKey: StatusDecommissioned,
		},
	}

	res, err := env.DB().Collection(Collection).UpdateOne(ctx, query, update)
	if err != nil {
		return err
	}
	if res.ModifiedCount == 0 {
		return errors.New("pod was not updated")
	}

	p.TaskRuntimeInfo.RunningTaskID = taskID
	p.TaskRuntimeInfo.RunningTaskExecution = taskExecution
	p.Status = StatusDecommissioned

	event.LogPodStatusChanged(ctx, p.ID, string(StatusRunning), string(StatusDecommissioned), "pod has been assigned a task and will not be reused")

	return nil
}

// ClearRunningTask clears the current task dispatched to the pod, if one is set.
func (p *Pod) ClearRunningTask(ctx context.Context) error {
	if p.TaskRuntimeInfo.RunningTaskID == "" {
		return nil
	}

	runningTaskIDKey := bsonutil.GetDottedKeyName(TaskRuntimeInfoKey, TaskRuntimeInfoRunningTaskIDKey)
	runningTaskExecutionKey := bsonutil.GetDottedKeyName(TaskRuntimeInfoKey, TaskRuntimeInfoRunningTaskExecutionKey)
	query := bson.M{
		IDKey:            p.ID,
		runningTaskIDKey: p.TaskRuntimeInfo.RunningTaskID,
	}
	if p.TaskRuntimeInfo.RunningTaskExecution == 0 {
		query["$or"] = []bson.M{
			{runningTaskExecutionKey: p.TaskRuntimeInfo.RunningTaskExecution},
			{runningTaskExecutionKey: bson.M{"$exists": false}},
		}
	} else {
		query[runningTaskExecutionKey] = p.TaskRuntimeInfo.RunningTaskExecution
	}

	if err := UpdateOne(ctx, query, bson.M{
		"$unset": bson.M{
			runningTaskIDKey:        1,
			runningTaskExecutionKey: 1,
		},
	}); err != nil {
		return errors.Wrap(err, "clearing running task")
	}

	event.LogPodRunningTaskCleared(ctx, p.ID, p.TaskRuntimeInfo.RunningTaskID, p.TaskRuntimeInfo.RunningTaskExecution)

	p.TaskRuntimeInfo.RunningTaskID = ""
	p.TaskRuntimeInfo.RunningTaskExecution = 0

	return nil
}

// UpdateAgentStartTime updates the time when the pod's agent started to now.
func (p *Pod) UpdateAgentStartTime(ctx context.Context) error {
	ts := utility.BSONTime(time.Now())
	if err := UpdateOne(ctx, ByID(p.ID), bson.M{
		"$set": bson.M{
			bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoAgentStartedKey): ts,
		},
	}); err != nil {
		return err
	}

	p.TimeInfo.AgentStarted = ts

	return nil
}

// UpdateLastCommunicated updates the last time that the pod and app server
// successfully communicated to now, indicating that the pod is currently alive.
func (p *Pod) UpdateLastCommunicated(ctx context.Context) error {
	ts := utility.BSONTime(time.Now())
	if err := UpdateOne(ctx, ByID(p.ID), bson.M{
		"$set": bson.M{
			bsonutil.GetDottedKeyName(TimeInfoKey, TimeInfoLastCommunicatedKey): ts,
		},
	}); err != nil {
		return err
	}

	p.TimeInfo.LastCommunicated = ts

	return nil
}
