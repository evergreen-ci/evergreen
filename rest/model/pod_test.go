package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodToService(t *testing.T) {
	apiPod := APICreatePod{
		Name:   utility.ToStringPtr("id"),
		Memory: utility.ToIntPtr(128),
		CPU:    utility.ToIntPtr(128),
		Image:  utility.ToStringPtr("image"),
		EnvVars: []*APIPodEnvVar{
			{
				Name:   utility.ToStringPtr("name"),
				Value:  utility.ToStringPtr("value"),
				Secret: utility.ToBoolPtr(false),
			},
			{
				Name:   utility.ToStringPtr("name1"),
				Value:  utility.ToStringPtr("value1"),
				Secret: utility.ToBoolPtr(false),
			},
			{
				Name:   utility.ToStringPtr("secret_name"),
				Value:  utility.ToStringPtr("secret_value"),
				Secret: utility.ToBoolPtr(true),
			},
		},
		Platform: utility.ToStringPtr("linux"),
		Secret:   utility.ToStringPtr("secret"),
	}

	res, err := apiPod.ToService()
	require.NoError(t, err)

	p, ok := res.(pod.Pod)
	require.True(t, ok)

	assert.Equal(t, utility.FromStringPtr(apiPod.Secret), p.Secret)
	assert.Equal(t, utility.FromStringPtr(apiPod.Image), p.TaskContainerCreationOpts.Image)
	assert.Equal(t, utility.FromIntPtr(apiPod.Memory), p.TaskContainerCreationOpts.MemoryMB)
	assert.Equal(t, utility.FromIntPtr(apiPod.CPU), p.TaskContainerCreationOpts.CPU)
	assert.Equal(t, utility.FromStringPtr(apiPod.Platform), string(p.TaskContainerCreationOpts.Platform))
	assert.Equal(t, utility.FromStringPtr(apiPod.Secret), p.Secret)
	assert.Len(t, p.TaskContainerCreationOpts.EnvVars, 2)
	assert.Len(t, p.TaskContainerCreationOpts.EnvSecrets, 1)
	assert.Equal(t, "value1", p.TaskContainerCreationOpts.EnvVars["name1"])
	assert.Equal(t, "secret_value", p.TaskContainerCreationOpts.EnvSecrets["secret_name"])
}
