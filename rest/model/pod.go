package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
)

// APISecretOpts is the model for secrets in a container.
type APISecretOpts struct {
	Name   *string
	Value  *string
	Owned  *bool
	Exists *bool
}

// APIPodEnvVar is the model for environment variables in a container
type APIPodEnvVar struct {
	Name       *string
	Value      *string
	SecretOpts *APISecretOpts
}

// APITimeInfo is the model for the timing information of a pod.
type APITimeInfo struct {
	Initialized time.Time
	Started     time.Time
	Provisioned time.Time
}

// APIPod is the model to be returned by the API whenever pod.Pod is fetched.
type APIPod struct {
	Name     *string
	Memory   *int
	CPU      *int
	Image    *string
	EnvVars  []*APIPodEnvVar
	Platform *string
	Secret   *string
}

// fromAPIEnvVars converts a slice of APIPodEnvVars to a string-string map of environment variables and a string-string map of secrets.
func fromAPIEnvVars(api []*APIPodEnvVar) (map[string]string, map[string]string) {
	var envVars map[string]string
	var secrets map[string]string

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
func (p *APIPod) ToService() (interface{}, error) {
	podDB := pod.Pod{}
	podDB.ID = utility.RandomString()
	podDB.Secret = utility.FromStringPtr(p.Secret)
	envVars, secrets := fromAPIEnvVars(p.EnvVars)
	opts := pod.TaskContainerCreationOptions{
		Image:      utility.FromStringPtr(p.Image),
		MemoryMB:   utility.FromIntPtr(p.Memory),
		CPU:        utility.FromIntPtr(p.CPU),
		Platform:   evergreen.PodPlatform(utility.FromStringPtr(p.Platform)),
		EnvVars:    envVars,
		EnvSecrets: secrets,
	}
	podDB.TaskContainerCreationOpts = opts

	return podDB, nil
}
