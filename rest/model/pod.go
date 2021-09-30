package model

import (
	"time"

	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

// APIPodEnvVar is the model for environment variables in a container.
type APIPodEnvVar struct {
	Name   *string `json:"name"`
	Value  *string `json:"value"`
	Secret *bool   `json:"secret"`
}

// APICreatePod is the model to create a new pod.
type APICreatePod struct {
	Name         *string         `json:"name"`
	Memory       *int            `json:"memory"`
	CPU          *int            `json:"cpu"`
	Image        *string         `json:"image"`
	RepoUsername *string         `json:"repo_username"`
	RepoPassword *string         `json:"repo_password"`
	EnvVars      []*APIPodEnvVar `json:"env_vars"`
	OS           *string         `json:"os"`
	Arch         *string         `json:"arch"`
	Secret       *string         `json:"secret"`
	WorkingDir   *string         `json:"working_dir"`
}

type APICreatePodResponse struct {
	ID string `json:"id"`
}

// ToService returns a service layer pod.Pod using the data from APIPod.
func (p *APICreatePod) ToService() (interface{}, error) {
	envVars, secrets := p.splitEnvVars()
	os := pod.OS(utility.FromStringPtr(p.OS))
	if err := os.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid OS")
	}
	arch := pod.Arch(utility.FromStringPtr(p.Arch))
	if err := arch.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid architecture")
	}

	return pod.Pod{
		ID: utility.RandomString(),
		Secret: pod.Secret{
			Value:  utility.FromStringPtr(p.Secret),
			Exists: utility.FalsePtr(),
			Owned:  utility.TruePtr(),
		},
		Status: pod.StatusInitializing,
		TimeInfo: pod.TimeInfo{
			Initializing: time.Now(),
		},
		TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
			Image:        utility.FromStringPtr(p.Image),
			RepoUsername: utility.FromStringPtr(p.RepoUsername),
			RepoPassword: utility.FromStringPtr(p.RepoPassword),
			MemoryMB:     utility.FromIntPtr(p.Memory),
			CPU:          utility.FromIntPtr(p.CPU),
			OS:           os,
			Arch:         arch,
			EnvVars:      envVars,
			EnvSecrets:   secrets,
			WorkingDir:   utility.FromStringPtr(p.WorkingDir),
		},
	}, nil
}

// splitEnvVars splits its environment variables into its non-secret and secret
// environment variables.
func (p *APICreatePod) splitEnvVars() (envVars map[string]string, secrets map[string]pod.Secret) {
	envVars = make(map[string]string)
	secrets = make(map[string]pod.Secret)

	for _, envVar := range p.EnvVars {
		if utility.FromBoolPtr(envVar.Secret) {
			secrets[utility.FromStringPtr(envVar.Name)] = pod.Secret{
				Name:   utility.FromStringPtr(envVar.Name),
				Value:  utility.FromStringPtr(envVar.Value),
				Exists: utility.FalsePtr(),
				Owned:  utility.TruePtr(),
			}
		} else {
			envVars[utility.FromStringPtr(envVar.Name)] = utility.FromStringPtr(envVar.Value)
		}
	}

	return envVars, secrets
}

// APIPod represents a pod to be used and returned from the REST API.
type APIPod struct {
	ID                        *string                            `json:"id"`
	Status                    APIPodStatus                       `json:"status"`
	Secret                    APIPodSecret                       `json:"secret"`
	TaskContainerCreationOpts APIPodTaskContainerCreationOptions `json:"task_container_creation_opts"`
	TimeInfo                  APIPodTimeInfo                     `json:"time_info"`
	Resources                 APIPodResourceInfo                 `json:"resources"`
}

// BuildFromService converts a service-layer pod model into a REST API model.
func (p *APIPod) BuildFromService(dbPod *pod.Pod) error {
	p.ID = utility.ToStringPtr(dbPod.ID)
	var status APIPodStatus
	if err := status.BuildFromService(dbPod.Status); err != nil {
		return errors.Wrap(err, "building status from service")
	}
	p.Status = status
	p.Secret.BuildFromService(dbPod.Secret)
	p.TimeInfo.BuildFromService(dbPod.TimeInfo)
	p.TaskContainerCreationOpts.BuildFromService(dbPod.TaskContainerCreationOpts)
	p.Resources.BuildFromService(dbPod.Resources)
	return nil
}

// ToService converts a REST API pod model into a service-layer model.
func (p *APIPod) ToService() (*pod.Pod, error) {
	s, err := p.Status.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting status to service")
	}
	taskCreationOpts, err := p.TaskContainerCreationOpts.ToService()
	if err != nil {
		return nil, errors.Wrap(err, "converting task container creation options to service")
	}
	secret := p.Secret.ToService()
	timing := p.TimeInfo.ToService()
	resources := p.Resources.ToService()
	return &pod.Pod{
		ID:                        utility.FromStringPtr(p.ID),
		Status:                    *s,
		Secret:                    secret,
		TaskContainerCreationOpts: *taskCreationOpts,
		TimeInfo:                  timing,
		Resources:                 resources,
	}, nil
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
func (s *APIPodStatus) ToService() (*pod.Status, error) {
	if s == nil {
		return nil, errors.New("nonexistent pod status")
	}
	var converted pod.Status
	switch *s {
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
		return nil, errors.Errorf("unrecognized pod status '%s'", *s)
	}
	return &converted, nil
}

// APIPodTaskContainerCreationOptions represents options to apply to the task's
// container when creating a pod.
type APIPodTaskContainerCreationOptions struct {
	Image        *string                 `json:"image,omitempty"`
	RepoUsername *string                 `json:"repo_username,omitempty"`
	RepoPassword *string                 `json:"repo_password,omitempty"`
	MemoryMB     *int                    `json:"memory_mb,omitempty"`
	CPU          *int                    `json:"cpu,omitempty"`
	OS           *string                 `json:"os,omitempty"`
	Arch         *string                 `json:"arch,omitempty"`
	EnvVars      map[string]string       `json:"env_vars,omitempty"`
	EnvSecrets   map[string]APIPodSecret `json:"env_secrets,omitempty"`
	WorkingDir   *string                 `json:"working_dir,omitempty"`
}

// BuildFromService converts service-layer task container creation options into
// REST API task container creation options.
func (o *APIPodTaskContainerCreationOptions) BuildFromService(opts pod.TaskContainerCreationOptions) {
	o.Image = utility.ToStringPtr(opts.Image)
	o.RepoUsername = utility.ToStringPtr(opts.RepoUsername)
	o.RepoPassword = utility.ToStringPtr(opts.RepoPassword)
	o.MemoryMB = utility.ToIntPtr(opts.MemoryMB)
	o.CPU = utility.ToIntPtr(opts.CPU)
	o.OS = utility.ToStringPtr(string(opts.OS))
	o.Arch = utility.ToStringPtr(string(opts.Arch))
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
	os := pod.OS(utility.FromStringPtr(o.OS))
	if err := os.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid OS")
	}
	arch := pod.Arch(utility.FromStringPtr(o.Arch))
	if err := arch.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid architecture")
	}
	envSecrets := map[string]pod.Secret{}
	for name, secret := range o.EnvSecrets {
		envSecrets[name] = secret.ToService()
	}
	return &pod.TaskContainerCreationOptions{
		Image:        utility.FromStringPtr(o.Image),
		RepoUsername: utility.FromStringPtr(o.RepoUsername),
		RepoPassword: utility.FromStringPtr(o.RepoPassword),
		MemoryMB:     utility.FromIntPtr(o.MemoryMB),
		CPU:          utility.FromIntPtr(o.CPU),
		OS:           os,
		Arch:         arch,
		EnvVars:      o.EnvVars,
		EnvSecrets:   envSecrets,
		WorkingDir:   utility.FromStringPtr(o.WorkingDir),
	}, nil
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
	Name       *string `json:"name,omitempty"`
	ExternalID *string `json:"external_id,omitempty"`
	Value      *string `json:"value,omitempty"`
	Exists     *bool   `json:"exists,omitempty"`
	Owned      *bool   `json:"owned,omitempty"`
}

// BuildFromService converts a service-layer pod secret into a REST API pod
// secret.
func (s *APIPodSecret) BuildFromService(secret pod.Secret) {
	s.Name = utility.ToStringPtr(secret.Name)
	s.ExternalID = utility.ToStringPtr(secret.ExternalID)
	s.Value = utility.ToStringPtr(secret.Value)
	s.Exists = secret.Exists
	s.Owned = secret.Owned
}

// ToService converts a REST API pod secret into a service-layer pod secret.
func (s *APIPodSecret) ToService() pod.Secret {
	return pod.Secret{
		Name:       utility.FromStringPtr(s.Name),
		ExternalID: utility.FromStringPtr(s.ExternalID),
		Value:      utility.FromStringPtr(s.Value),
		Exists:     s.Exists,
		Owned:      s.Owned,
	}
}
