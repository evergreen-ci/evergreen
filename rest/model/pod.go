package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
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
	Name     *string         `json:"name"`
	Memory   *int            `json:"memory"`
	CPU      *int            `json:"cpu"`
	Image    *string         `json:"image"`
	EnvVars  []*APIPodEnvVar `json:"env_vars"`
	Platform *string         `json:"platform"`
	Secret   *string         `json:"secret"`
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

	return pod.Pod{
		ID:     utility.RandomString(),
		Secret: utility.FromStringPtr(p.Secret),
		TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
			Image:      utility.FromStringPtr(p.Image),
			MemoryMB:   utility.FromIntPtr(p.Memory),
			CPU:        utility.FromIntPtr(p.CPU),
			Platform:   evergreen.PodPlatform(utility.FromStringPtr(p.Platform)),
			EnvVars:    envVars,
			EnvSecrets: secrets,
		},
	}, nil
}

// kim: TODO: populate with all copy-pasted fields
type APIPod struct {
	ID                        *string                             `json:"id"`
	Status                    *APIPodStatus                       `json:"status"`
	Secret                    *string                             `json:"secret"`
	TaskContainerCreationOpts *APIPodTaskContainerCreationOptions `json:"task_container_creation_opts"`
	Resources                 *APIPodResourceInfo
}

func (p *APIPod) BuildFromService(dbPod *pod.Pod) error {
	p.ID = utility.ToStringPtr(dbPod.ID)
	var s APIPodStatus
	if err := s.BuildFromService(dbPod.Status); err != nil {
		return errors.Wrap(err, "building status from service")
	}
	p.Status = &s
	p.Secret = utility.ToStringPtr(dbPod.Secret)
	var taskCreationOpts APIPodTaskContainerCreationOptions
	taskCreationOpts.BuildFromService(dbPod.TaskContainerCreationOpts)
	p.TaskContainerCreationOpts = &taskCreationOpts
	var resources APIPodResourceInfo
	resources.BuildFromService(dbPod.Resources)
	p.Resources = &resources
	return nil
}

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
		Status:                    s,
		Secret:                    utility.FromStringPtr(p.Secret),
		TaskContainerCreationOpts: *taskCreationOpts,
		Resources:                 resources,
	}, nil
}

type APIPodStatus string

const (
	PodStatusInitializing APIPodStatus = "initializing"
	PodStatusStarting     APIPodStatus = "starting"
	PodStatusRunning      APIPodStatus = "running"
	PodStatusTerminated   APIPodStatus = "terminated"
)

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
	s = &converted
	return nil
}

func (s *APIPodStatus) ToService() (pod.Status, error) {
	switch *s {
	case PodStatusInitializing:
		return pod.StatusTerminated, nil
	case PodStatusStarting:
		return pod.StatusStarting, nil
	case PodStatusRunning:
		return pod.StatusRunning, nil
	case PodStatusTerminated:
		return pod.StatusTerminated, nil
	default:
		return "", errors.Errorf("unrecognized pod status '%s'", s)
	}
}

type APIPodTaskContainerCreationOptions struct {
	Image      *string                `json:"image"`
	MemoryMB   *int                   `json:"memory_mb"`
	CPU        *int                   `json:"cpu"`
	Platform   *evergreen.PodPlatform `json:"platform"`
	EnvVars    map[string]string      `json:"env_vars"`
	EnvSecrets map[string]string      `json:"env_secrets"`
}

func (o *APIPodTaskContainerCreationOptions) BuildFromService(opts pod.TaskContainerCreationOptions) {
	o.Image = utility.ToStringPtr(opts.Image)
	o.MemoryMB = utility.ToIntPtr(opts.MemoryMB)
	o.CPU = utility.ToIntPtr(opts.CPU)
	o.Platform = &opts.Platform
	o.EnvVars = opts.EnvVars
	o.EnvSecrets = opts.EnvSecrets
}

func (i *APIPodTaskContainerCreationOptions) ToService() (*pod.TaskContainerCreationOptions, error) {
	if i.Platform == nil {
		return nil, errors.New("missing platform")
	}
	return &pod.TaskContainerCreationOptions{
		Image:      utility.FromStringPtr(i.Image),
		MemoryMB:   utility.FromIntPtr(i.MemoryMB),
		CPU:        utility.FromIntPtr(i.CPU),
		Platform:   *i.Platform,
		EnvVars:    i.EnvVars,
		EnvSecrets: i.EnvSecrets,
	}, nil
}

type APIPodResourceInfo struct {
	ID           *string  `json:"id"`
	DefinitionID *string  `json:"definition_id"`
	Cluster      *string  `json:"cluster"`
	SecretIDs    []string `json:"secret_i_ds"`
}

func (i *APIPodResourceInfo) BuildFromService(info pod.ResourceInfo) {
	i.ID = utility.ToStringPtr(info.ID)
	i.DefinitionID = utility.ToStringPtr(info.DefinitionID)
	i.Cluster = utility.ToStringPtr(info.Cluster)
	i.SecretIDs = info.SecretIDs
}

func (i *APIPodResourceInfo) ToService() pod.ResourceInfo {
	return pod.ResourceInfo{
		ID:           utility.FromStringPtr(i.ID),
		DefinitionID: utility.FromStringPtr(i.DefinitionID),
		Cluster:      utility.FromStringPtr(i.Cluster),
		SecretIDs:    i.SecretIDs,
	}
}
