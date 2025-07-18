package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// APICreatePod is the model to create a new pod.
type APICreatePod struct {
	Name                *string              `json:"name"`
	Memory              *int                 `json:"memory"`
	CPU                 *int                 `json:"cpu"`
	Image               *string              `json:"image"`
	RepoCredsExternalID *string              `json:"repo_creds_external_id"`
	OS                  APIPodOS             `json:"os"`
	Arch                APIPodArch           `json:"arch"`
	WindowsVersion      APIPodWindowsVersion `json:"windows_version"`
	WorkingDir          *string              `json:"working_dir"`
	PodSecretExternalID *string              `json:"pod_secret_external_id"`
	PodSecretValue      *string              `json:"pod_secret_value"`
}

type APICreatePodResponse struct {
	ID string `json:"id"`
}

// ToService returns a service layer pod.Pod using the data from APICreatePod.
func (p *APICreatePod) ToService() (*pod.Pod, error) {
	os, err := p.OS.ToService()
	if err != nil {
		return nil, err
	}
	arch, err := p.Arch.ToService()
	if err != nil {
		return nil, err
	}
	winVer, err := p.WindowsVersion.ToService()
	if err != nil {
		return nil, err
	}

	return pod.NewTaskIntentPod(evergreen.GetEnvironment().Settings().Providers.AWS.Pod.ECS, pod.TaskIntentPodOptions{
		CPU:                 utility.FromIntPtr(p.CPU),
		MemoryMB:            utility.FromIntPtr(p.Memory),
		OS:                  *os,
		Arch:                *arch,
		WindowsVersion:      *winVer,
		Image:               utility.FromStringPtr(p.Image),
		WorkingDir:          utility.FromStringPtr(p.WorkingDir),
		RepoCredsExternalID: utility.FromStringPtr(p.RepoCredsExternalID),
		PodSecretExternalID: utility.FromStringPtr(p.PodSecretExternalID),
		PodSecretValue:      utility.FromStringPtr(p.PodSecretValue),
	})
}

// APIPod represents a pod to be used and returned from the REST API.
type APIPod struct {
	ID                        *string                            `json:"id"`
	Type                      APIPodType                         `json:"type,omitempty"`
	Status                    APIPodStatus                       `json:"status,omitempty"`
	TaskContainerCreationOpts APIPodTaskContainerCreationOptions `json:"task_container_creation_opts"`
	Family                    *string                            `json:"family,omitempty"`
	TimeInfo                  APIPodTimeInfo                     `json:"time_info"`
	Resources                 APIPodResourceInfo                 `json:"resources"`
	TaskRuntimeInfo           APITaskRuntimeInfo                 `json:"task_runtime_info"`
	AgentVersion              *string                            `json:"agent_version,omitempty"`
}

// BuildFromService converts a service-layer pod model into a REST API model.
func (p *APIPod) BuildFromService(dbPod *pod.Pod) error {
	p.ID = utility.ToStringPtr(dbPod.ID)
	p.Family = utility.ToStringPtr(dbPod.Family)
	p.AgentVersion = utility.ToStringPtr(dbPod.AgentVersion)
	if err := p.Type.BuildFromService(dbPod.Type); err != nil {
		return errors.Wrap(err, "building pod type from service")
	}
	if err := p.Status.BuildFromService(dbPod.Status); err != nil {
		return errors.Wrap(err, "building status from service")
	}
	p.TimeInfo.BuildFromService(dbPod.TimeInfo)
	p.TaskContainerCreationOpts.BuildFromService(dbPod.TaskContainerCreationOpts)
	p.Resources.BuildFromService(dbPod.Resources)
	p.TaskRuntimeInfo.BuildFromService(dbPod.TaskRuntimeInfo)
	return nil
}

// ToService converts a REST API pod model into a service-layer model.
func (p *APIPod) ToService() (*pod.Pod, error) {
	s, err := p.Status.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting status to service model")
	}
	t, err := p.Type.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting pod type to service model")
	}
	taskCreationOpts, err := p.TaskContainerCreationOpts.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting task container creation options to service model")
	}
	timing := p.TimeInfo.ToService()
	resources := p.Resources.ToService()
	taskRuntime := p.TaskRuntimeInfo.ToService()
	return &pod.Pod{
		ID:                        utility.FromStringPtr(p.ID),
		Type:                      *t,
		Status:                    *s,
		TaskContainerCreationOpts: *taskCreationOpts,
		Family:                    utility.FromStringPtr(p.Family),
		TimeInfo:                  timing,
		Resources:                 resources,
		TaskRuntimeInfo:           taskRuntime,
		AgentVersion:              utility.FromStringPtr(p.AgentVersion),
	}, nil
}

// APIPodType represents a pod's type.
type APIPodType string

const (
	PodTypeAgent APIPodType = "agent"
)

// BuildFromService converts a service-layer pod type into a REST API pod type.
func (t *APIPodType) BuildFromService(pt pod.Type) error {
	var converted APIPodType
	switch pt {
	case pod.TypeAgent:
		converted = PodTypeAgent
	default:
		return errors.Errorf("unrecognized pod type '%s'", pt)
	}
	*t = converted
	return nil
}

// ToService converts a REST API pod type into a service-layer pod type.
func (t APIPodType) ToService() (*pod.Type, error) {
	var converted pod.Type
	switch t {
	case PodTypeAgent:
		converted = pod.TypeAgent
	default:
		return nil, errors.Errorf("unrecognized pod type '%s'", t)
	}
	return &converted, nil
}

// APIPodStatus represents a pod's status.
type APIPodStatus string

const (
	PodStatusInitializing   APIPodStatus = "initializing"
	PodStatusStarting       APIPodStatus = "starting"
	PodStatusRunning        APIPodStatus = "running"
	PodStatusDecommissioned APIPodStatus = "decommissioned"
	PodStatusTerminated     APIPodStatus = "terminated"
)

// BuildFromService converts a service-layer pod status into a REST API pod
// status.
func (s *APIPodStatus) BuildFromService(ps pod.Status) error {
	var converted APIPodStatus
	switch ps {
	case pod.StatusInitializing:
		converted = PodStatusInitializing
	case pod.StatusStarting:
		converted = PodStatusStarting
	case pod.StatusRunning:
		converted = PodStatusRunning
	case pod.StatusDecommissioned:
		converted = PodStatusDecommissioned
	case pod.StatusTerminated:
		converted = PodStatusTerminated
	default:
		return errors.Errorf("unrecognized pod status '%s'", ps)
	}
	*s = converted
	return nil
}

// ToService converts a REST API pod status into a service-layer pod status.
func (s APIPodStatus) ToService() (*pod.Status, error) {
	var converted pod.Status
	switch s {
	case PodStatusInitializing:
		converted = pod.StatusInitializing
	case PodStatusStarting:
		converted = pod.StatusStarting
	case PodStatusRunning:
		converted = pod.StatusRunning
	case PodStatusDecommissioned:
		converted = pod.StatusDecommissioned
	case PodStatusTerminated:
		converted = pod.StatusTerminated
	default:
		return nil, errors.Errorf("unrecognized pod status '%s'", s)
	}
	return &converted, nil
}

// APIPodTaskContainerCreationOptions represents options to apply to the task's
// container when creating a pod.
type APIPodTaskContainerCreationOptions struct {
	Image               *string                 `json:"image,omitempty"`
	RepoCredsExternalID *string                 `json:"repo_creds_external_id,omitempty"`
	MemoryMB            *int                    `json:"memory_mb,omitempty"`
	CPU                 *int                    `json:"cpu,omitempty"`
	OS                  APIPodOS                `json:"os,omitempty"`
	Arch                APIPodArch              `json:"arch,omitempty"`
	WindowsVersion      APIPodWindowsVersion    `json:"windows_version,omitempty"`
	EnvVars             map[string]string       `json:"env_vars,omitempty"`
	EnvSecrets          map[string]APIPodSecret `json:"env_secrets,omitempty"`
	WorkingDir          *string                 `json:"working_dir,omitempty"`
}

// BuildFromService converts service-layer task container creation options into
// REST API task container creation options.
func (o *APIPodTaskContainerCreationOptions) BuildFromService(opts pod.TaskContainerCreationOptions) {
	o.Image = utility.ToStringPtr(opts.Image)
	o.RepoCredsExternalID = utility.ToStringPtr(opts.RepoCredsExternalID)
	o.MemoryMB = utility.ToIntPtr(opts.MemoryMB)
	o.CPU = utility.ToIntPtr(opts.CPU)
	o.OS.BuildFromService(&opts.OS)
	o.Arch.BuildFromService(&opts.Arch)
	o.WindowsVersion.BuildFromService(&opts.WindowsVersion)
	o.EnvVars = opts.EnvVars
	envSecrets := map[string]APIPodSecret{}
	for name, secret := range opts.EnvSecrets {
		var s APIPodSecret
		s.BuildFromService(secret)
		envSecrets[name] = s
	}
	o.EnvSecrets = envSecrets
	o.WorkingDir = utility.ToStringPtr(opts.WorkingDir)
}

// ToService converts REST API task container creation options into
// service-layer task container creation options.
func (o *APIPodTaskContainerCreationOptions) ToService() (*pod.TaskContainerCreationOptions, error) {
	os, err := o.OS.ToService()
	if err != nil {
		return nil, err
	}
	arch, err := o.Arch.ToService()
	if err != nil {
		return nil, err
	}
	winVer, err := o.WindowsVersion.ToService()
	if err != nil {
		return nil, err
	}
	envSecrets := map[string]pod.Secret{}
	for name, secret := range o.EnvSecrets {
		envSecrets[name] = secret.ToService()
	}
	return &pod.TaskContainerCreationOptions{
		Image:               utility.FromStringPtr(o.Image),
		RepoCredsExternalID: utility.FromStringPtr(o.RepoCredsExternalID),
		MemoryMB:            utility.FromIntPtr(o.MemoryMB),
		CPU:                 utility.FromIntPtr(o.CPU),
		OS:                  *os,
		Arch:                *arch,
		WindowsVersion:      *winVer,
		EnvVars:             o.EnvVars,
		EnvSecrets:          envSecrets,
		WorkingDir:          utility.FromStringPtr(o.WorkingDir),
	}, nil
}

type APIPodOS string

func (os *APIPodOS) BuildFromService(dbOS *pod.OS) {
	if os == nil || dbOS == nil {
		return
	}
	*os = APIPodOS(string(*dbOS))
}

func (os *APIPodOS) ToService() (*pod.OS, error) {
	if os == nil {
		return nil, nil
	}
	dbOS := pod.OS(*os)
	if err := dbOS.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid OS")
	}
	return &dbOS, nil
}

type APIPodArch string

func (a *APIPodArch) BuildFromService(dbArch *pod.Arch) {
	if a == nil || dbArch == nil {
		return
	}
	*a = APIPodArch(*dbArch)
}

func (a *APIPodArch) ToService() (*pod.Arch, error) {
	if a == nil {
		return nil, nil
	}
	dbArch := pod.Arch(*a)
	if err := dbArch.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid architecture")
	}
	return &dbArch, nil
}

type APIPodWindowsVersion string

func (v *APIPodWindowsVersion) BuildFromService(dbWinVer *pod.WindowsVersion) {
	if v == nil || dbWinVer == nil {
		return
	}
	*v = APIPodWindowsVersion(*dbWinVer)
}

func (v *APIPodWindowsVersion) ToService() (*pod.WindowsVersion, error) {
	if v == nil {
		return nil, nil
	}
	dbWinVer := pod.WindowsVersion(*v)
	// Empty Windows version is allowed since it's optional.
	if dbWinVer == "" {
		return &dbWinVer, nil
	}
	if err := dbWinVer.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid Windows version")
	}
	return &dbWinVer, nil
}

// APIPodResourceInfo represents timing information about the pod lifecycle.
type APIPodTimeInfo struct {
	Initializing     *time.Time `json:"initializing,omitempty"`
	Starting         *time.Time `json:"starting,omitempty"`
	LastCommunicated *time.Time `json:"last_communicated,omitempty"`
}

// BuildFromService converts service-layer resource information into REST API
// timing information.
func (i *APIPodTimeInfo) BuildFromService(info pod.TimeInfo) {
	i.Initializing = utility.ToTimePtr(info.Initializing)
	i.Starting = utility.ToTimePtr(info.Starting)
	i.LastCommunicated = utility.ToTimePtr(info.LastCommunicated)
}

// BuildFromService converts service-layer resource information into REST API
// timing information.
func (i *APIPodTimeInfo) ToService() pod.TimeInfo {
	return pod.TimeInfo{
		Initializing:     utility.FromTimePtr(i.Initializing),
		Starting:         utility.FromTimePtr(i.Starting),
		LastCommunicated: utility.FromTimePtr(i.LastCommunicated),
	}
}

// APIPodResourceInfo represents information about external resources associated
// with a pod.
type APIPodResourceInfo struct {
	ExternalID   *string                    `json:"external_id,omitempty"`
	DefinitionID *string                    `json:"definition_id,omitempty"`
	Cluster      *string                    `json:"cluster,omitempty"`
	Containers   []APIContainerResourceInfo `json:"containers,omitempty"`
}

// BuildFromService converts service-layer pod resource information into REST
// API pod resource information.
func (i *APIPodResourceInfo) BuildFromService(info pod.ResourceInfo) {
	i.ExternalID = utility.ToStringPtr(info.ExternalID)
	i.DefinitionID = utility.ToStringPtr(info.DefinitionID)
	i.Cluster = utility.ToStringPtr(info.Cluster)
	for _, container := range info.Containers {
		var containerInfo APIContainerResourceInfo
		containerInfo.BuildFromService(container)
		i.Containers = append(i.Containers, containerInfo)
	}
}

// ToService converts REST API pod resource information into service-layer
// pod resource information.
func (i *APIPodResourceInfo) ToService() pod.ResourceInfo {
	var containers []pod.ContainerResourceInfo
	for _, container := range i.Containers {
		containers = append(containers, container.ToService())
	}
	return pod.ResourceInfo{
		ExternalID:   utility.FromStringPtr(i.ExternalID),
		DefinitionID: utility.FromStringPtr(i.DefinitionID),
		Cluster:      utility.FromStringPtr(i.Cluster),
		Containers:   containers,
	}
}

// APIPodResourceInfo represents information about external resources associated
// with a container.
type APIContainerResourceInfo struct {
	ExternalID *string  `json:"external_id,omitempty"`
	Name       *string  `json:"name,omitempty"`
	SecretIDs  []string `json:"secret_ids,omitempty"`
}

// BuildFromService converts service-layer container resource information into
// REST API container resource information.
func (i *APIContainerResourceInfo) BuildFromService(info pod.ContainerResourceInfo) {
	i.ExternalID = utility.ToStringPtr(info.ExternalID)
	i.Name = utility.ToStringPtr(info.Name)
	i.SecretIDs = info.SecretIDs
}

// ToService converts REST API container resource information into service-layer
// container resource information.
func (i *APIContainerResourceInfo) ToService() pod.ContainerResourceInfo {
	return pod.ContainerResourceInfo{
		ExternalID: utility.FromStringPtr(i.ExternalID),
		Name:       utility.FromStringPtr(i.Name),
		SecretIDs:  i.SecretIDs,
	}
}

// APIPodSecret represents a secret associated with a pod returned from the REST
// API.
type APIPodSecret struct {
	ExternalID *string `json:"external_id,omitempty"`
	Value      *string `json:"value,omitempty"`
}

// BuildFromService converts a service-layer pod secret into a REST API pod
// secret.
func (s *APIPodSecret) BuildFromService(secret pod.Secret) {
	s.ExternalID = utility.ToStringPtr(secret.ExternalID)
	s.Value = utility.ToStringPtr(secret.Value)
}

// ToService converts a REST API pod secret into a service-layer pod secret.
func (s *APIPodSecret) ToService() pod.Secret {
	return pod.Secret{
		ExternalID: utility.FromStringPtr(s.ExternalID),
		Value:      utility.FromStringPtr(s.Value),
	}
}

// APITaskRuntimeInfo represents information about tasks that a pod is running
// or has run previously.
type APITaskRuntimeInfo struct {
	RunningTaskID        *string `json:"running_task_id,omitempty"`
	RunningTaskExecution *int    `json:"running_task_execution,omitempty"`
}

// BuildFromService converts service-layer task runtime information into REST
// API task runtime information.
func (i *APITaskRuntimeInfo) BuildFromService(info pod.TaskRuntimeInfo) {
	i.RunningTaskID = utility.ToStringPtr(info.RunningTaskID)
	i.RunningTaskExecution = utility.ToIntPtr(info.RunningTaskExecution)
}

// ToService converts REST API task runtime information into service-layer task
// runtime information.
func (i *APITaskRuntimeInfo) ToService() pod.TaskRuntimeInfo {
	return pod.TaskRuntimeInfo{
		RunningTaskID:        utility.FromStringPtr(i.RunningTaskID),
		RunningTaskExecution: utility.FromIntPtr(i.RunningTaskExecution),
	}
}
