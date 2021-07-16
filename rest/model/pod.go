package model

import (
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type APISecretOpts struct {
	Name   *string
	Value  *string
	Owned  *bool
	Exists *bool
}

type APIPodEnvVar struct {
	Name       *string
	Value      *string
	SecretOpts *APISecretOpts
}

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
	Cluster  *string
	Secret   *string
	TimeInfo APITimeInfo
}

// toAPIEnvVars converts a string-string map of environment variables and a string-string map of secrets to a slice of APIPodEnvVars.
func toAPIEnvVars(envVars map[string]string, secrets map[string]string) []*APIPodEnvVar {
	var podEnvVars []*APIPodEnvVar

	for k, v := range envVars {
		apiEnvVar := &APIPodEnvVar{
			Name:  &k,
			Value: &v,
		}
		podEnvVars = append(podEnvVars, apiEnvVar)
	}

	for k, v := range secrets {
		apiEnvVar := &APIPodEnvVar{
			SecretOpts: &APISecretOpts{
				Name:   &k,
				Value:  &v,
				Owned:  utility.TruePtr(),
				Exists: utility.FalsePtr(),
			},
		}
		podEnvVars = append(podEnvVars, apiEnvVar)
	}

	return podEnvVars
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

// BuildFromService converts from service level pod.Pod to APIPod.
func (p *APIPod) BuildFromService(h interface{}) error {
	var podData pod.Pod
	switch v := h.(type) {
	case pod.Pod:
		podData = v
	case *pod.Pod:
		podData = *v
	default:
		return errors.Errorf("%T is not a supported expansion type", h)
	}

	p.CPU = &podData.TaskContainerCreationOpts.CPU
	p.Cluster = utility.ToStringPtr(string(podData.TaskContainerCreationOpts.Platform))
	p.EnvVars = toAPIEnvVars(podData.TaskContainerCreationOpts.EnvVars, podData.TaskContainerCreationOpts.EnvSecrets)
	p.Image = &podData.TaskContainerCreationOpts.Image
	p.Memory = &podData.TaskContainerCreationOpts.MemoryMB
	p.Name = &podData.ExternalID
	p.Secret = &podData.Secret

	return nil
}

// ToService returns a service layer pod.Pod using the data from APIPod.
func (p *APIPod) ToService() (interface{}, error) {
	podDB := pod.Pod{}
	podDB.ID = utility.RandomString()
	podDB.ExternalID = utility.FromStringPtr(p.Name)
	podDB.Secret = utility.FromStringPtr(p.Secret)
	podDB.TimeInfo = pod.TimeInfo{
		Initialized: p.TimeInfo.Initialized,
		Started:     p.TimeInfo.Started,
		Provisioned: p.TimeInfo.Provisioned,
	}
	envVars, secrets := fromAPIEnvVars(p.EnvVars)
	opts := pod.TaskContainerCreationOptions{
		Image:      utility.FromStringPtr(p.Image),
		MemoryMB:   utility.FromIntPtr(p.Memory),
		CPU:        utility.FromIntPtr(p.CPU),
		Platform:   evergreen.PodPlatform(utility.FromStringPtr(p.Cluster)),
		EnvVars:    envVars,
		EnvSecrets: secrets,
	}
	podDB.TaskContainerCreationOpts = opts

	return podDB, nil
}
