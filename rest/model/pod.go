package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
)

// APISecretOpts is the model for secrets in a container.
type APISecretOpts struct {
	Name  *string `json:"name"`
	Value *string `json:"value"`
}

// APIPodEnvVar is the model for environment variables in a container.
type APIPodEnvVar struct {
	Name       *string        `json:"name"`
	Value      *string        `json:"value"`
	SecretOpts *APISecretOpts `json:"secret_opts"`
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
		if envVar.SecretOpts == nil {
			envVars[utility.FromStringPtr(envVar.Name)] = utility.FromStringPtr(envVar.Value)
		} else {
			secrets[utility.FromStringPtr(envVar.SecretOpts.Name)] = utility.FromStringPtr(envVar.SecretOpts.Value)
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
