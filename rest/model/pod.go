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

// APITimeInfo is the model for the timing information of a pod.
type APITimeInfo struct {
	Initialized time.Time `json:"initialized"`
	Started     time.Time `json:"started"`
	Provisioned time.Time `json:"provisioned"`
}

// APICreatePod is the model to create a new pod.
type APICreatePod struct {
	Name    *string         `json:"name"`
	Memory  *int            `json:"memory"`
	CPU     *int            `json:"cpu"`
	Image   *string         `json:"image"`
	EnvVars []*APIPodEnvVar `json:"env_vars"`
	OS      *string         `json:"os"`
	Arch    *string         `json:"arch"`
	Secret  *string         `json:"secret"`
}

type APICreatePodResponse struct {
	ID string `json:"id"`
}

// fromAPIEnvVars converts a slice of APIPodEnvVars to a map of environment variables and a map of secrets.
func (p *APICreatePod) fromAPIEnvVars(api []*APIPodEnvVar) (envVars map[string]string, secrets map[string]string) {
	envVars = make(map[string]string)
	secrets = make(map[string]string)

	for _, envVar := range api {
		if utility.FromBoolPtr(envVar.Secret) {
			secrets[utility.FromStringPtr(envVar.Name)] = utility.FromStringPtr(envVar.Value)
		} else {
			envVars[utility.FromStringPtr(envVar.Name)] = utility.FromStringPtr(envVar.Value)
		}
	}

	return envVars, secrets
}

// ToService returns a service layer pod.Pod using the data from APIPod.
func (p *APICreatePod) ToService() (interface{}, error) {
	envVars, secrets := p.fromAPIEnvVars(p.EnvVars)
	os := pod.OS(utility.FromStringPtr(p.OS))
	if err := os.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid OS")
	}
	arch := pod.Arch(utility.FromStringPtr(p.Arch))
	if err := arch.Validate(); err != nil {
		return nil, errors.Wrap(err, "invalid architecture")
	}

	return pod.Pod{
		ID:     utility.RandomString(),
		Secret: utility.FromStringPtr(p.Secret),
		Status: pod.StatusInitializing,
		TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
			Image:      utility.FromStringPtr(p.Image),
			MemoryMB:   utility.FromIntPtr(p.Memory),
			CPU:        utility.FromIntPtr(p.CPU),
			OS:         os,
			Arch:       arch,
			EnvVars:    envVars,
			EnvSecrets: secrets,
		},
	}, nil
}

// APIPod represents a pod to be used and returned from the REST API.
type APIPod struct {
	ID                        *string                            `json:"id"`
	Status                    *APIPodStatus                      `json:"status"`
	Secret                    *string                            `json:"secret,omitempty"`
	TaskContainerCreationOpts APIPodTaskContainerCreationOptions `json:"task_container_creation_opts,omitempty"`
	Resources                 APIPodResourceInfo                 `json:"resources,omitempty"`
}

// BuildFromService converts a service-layer pod model into a REST API model.
func (p *APIPod) BuildFromService(dbPod *pod.Pod) error {
	p.ID = utility.ToStringPtr(dbPod.ID)
	var status APIPodStatus
	if err := status.BuildFromService(dbPod.Status); err != nil {
		return errors.Wrap(err, "building status from service")
	}
	p.Status = &status
	p.Secret = utility.ToStringPtr(dbPod.Secret)
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
	resources := p.Resources.ToService()
	return &pod.Pod{
		ID:                        utility.FromStringPtr(p.ID),
		Status:                    *s,
		Secret:                    utility.FromStringPtr(p.Secret),
		TaskContainerCreationOpts: *taskCreationOpts,
		Resources:                 resources,
	}, nil
}

// APIPodStatus represents a pod's status.
type APIPodStatus string

const (
	PodStatusInitializing APIPodStatus = "initializing"
	PodStatusStarting     APIPodStatus = "starting"
	PodStatusRunning      APIPodStatus = "running"
	PodStatusTerminated   APIPodStatus = "terminated"
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
		converted = pod.StatusTerminated
	case PodStatusStarting:
		converted = pod.StatusStarting
	case PodStatusRunning:
		converted = pod.StatusRunning
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
	Image      *string           `json:"image,omitempty"`
	MemoryMB   *int              `json:"memory_mb,omitempty"`
	CPU        *int              `json:"cpu,omitempty"`
	OS         *string           `json:"os,omitempty"`
	Arch       *string           `json:"arch,omitempty"`
	EnvVars    map[string]string `json:"env_vars,omitempty"`
	EnvSecrets map[string]string `json:"env_secrets,omitempty"`
}

// BuildFromService converts service-layer task container creation options into
// REST API task container creation options.
func (o *APIPodTaskContainerCreationOptions) BuildFromService(opts pod.TaskContainerCreationOptions) {
	o.Image = utility.ToStringPtr(opts.Image)
	o.MemoryMB = utility.ToIntPtr(opts.MemoryMB)
	o.CPU = utility.ToIntPtr(opts.CPU)
	o.OS = utility.ToStringPtr(string(opts.OS))
	o.Arch = utility.ToStringPtr(string(opts.Arch))
	o.EnvVars = opts.EnvVars
	o.EnvSecrets = opts.EnvSecrets
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
	return &pod.TaskContainerCreationOptions{
		Image:      utility.FromStringPtr(o.Image),
		MemoryMB:   utility.FromIntPtr(o.MemoryMB),
		CPU:        utility.FromIntPtr(o.CPU),
		OS:         os,
		Arch:       arch,
		EnvVars:    o.EnvVars,
		EnvSecrets: o.EnvSecrets,
	}, nil
}

// APIPodResourceInfo represents information about external resources associated
// with a pod.
type APIPodResourceInfo struct {
	ID           *string  `json:"id,omitempty"`
	DefinitionID *string  `json:"definition_id,omitempty"`
	Cluster      *string  `json:"cluster,omitempty"`
	SecretIDs    []string `json:"secret_ids,omitempty"`
}

// BuildFromService converts service-layer resource information into REST API
// resource information.
func (i *APIPodResourceInfo) BuildFromService(info pod.ResourceInfo) {
	i.ID = utility.ToStringPtr(info.ID)
	i.DefinitionID = utility.ToStringPtr(info.DefinitionID)
	i.Cluster = utility.ToStringPtr(info.Cluster)
	i.SecretIDs = info.SecretIDs
}

// ToService converts REST API resource information into service-layer resource
// information.
func (i *APIPodResourceInfo) ToService() pod.ResourceInfo {
	return pod.ResourceInfo{
		ID:           utility.FromStringPtr(i.ID),
		DefinitionID: utility.FromStringPtr(i.DefinitionID),
		Cluster:      utility.FromStringPtr(i.Cluster),
		SecretIDs:    i.SecretIDs,
	}
}
