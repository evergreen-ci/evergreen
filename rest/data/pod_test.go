package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreatePod(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(pod.Collection))
	}()
	require.NoError(t, db.ClearCollections(pod.Collection))

	env := evergreen.GetEnvironment()
	env.Settings().Providers.AWS.Pod.ECS = evergreen.ECSConfig{
		AllowedImages: []string{
			"image",
		},
	}

	p := model.APICreatePod{
		Name:                utility.ToStringPtr("name"),
		Memory:              utility.ToIntPtr(128),
		CPU:                 utility.ToIntPtr(128),
		Image:               utility.ToStringPtr("image"),
		OS:                  model.APIPodOS(pod.OSWindows),
		Arch:                model.APIPodArch(pod.ArchAMD64),
		WindowsVersion:      model.APIPodWindowsVersion(pod.WindowsVersionServer2019),
		WorkingDir:          utility.ToStringPtr("/working/dir"),
		PodSecretExternalID: utility.ToStringPtr("pod_secret_external_id"),
		PodSecretValue:      utility.ToStringPtr("pod_secret_value"),
	}
	res, err := CreatePod(p)
	require.NoError(t, err)
	require.NotZero(t, res)

	apiPod, err := FindAPIPodByID(res.ID)
	require.NoError(t, err)
	require.NotZero(t, apiPod)

	assert.Equal(t, model.PodTypeAgent, apiPod.Type)
	assert.Equal(t, model.PodStatusInitializing, apiPod.Status)
	require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvVars)
	assert.NotZero(t, apiPod.TaskContainerCreationOpts.EnvVars["POD_ID"])
	require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvSecrets)
	secret, ok := apiPod.TaskContainerCreationOpts.EnvSecrets[pod.PodSecretEnvVar]
	require.True(t, ok)
	assert.Equal(t, utility.FromStringPtr(p.PodSecretExternalID), utility.FromStringPtr(secret.ExternalID))
	assert.Equal(t, utility.FromStringPtr(p.PodSecretValue), utility.FromStringPtr(secret.Value))
}

func TestFindAPIPodByID(t *testing.T) {
	defer func() {
		assert.NoError(t, db.ClearCollections(pod.Collection))
	}()
	require.NoError(t, db.ClearCollections(pod.Collection))

	t.Run("Succeeds", func(t *testing.T) {
		p := pod.Pod{
			ID:     "id",
			Type:   pod.TypeAgent,
			Status: pod.StatusInitializing,
		}
		require.NoError(t, p.Insert())

		apiPod, err := FindAPIPodByID(p.ID)
		require.NoError(t, err)
		require.NotZero(t, apiPod)

		assert.Equal(t, p.ID, utility.FromStringPtr(apiPod.ID))
		assert.EqualValues(t, p.Type, apiPod.Type)
		assert.EqualValues(t, p.Status, apiPod.Status)
	})
	t.Run("FailsWithNonexistentPod", func(t *testing.T) {
		apiPod, err := FindAPIPodByID("nonexistent")
		assert.Error(t, err)
		assert.Zero(t, apiPod)
	})
}

func TestCheckPodSecret(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T, p pod.Pod, secret string){
		"Succeeds": func(t *testing.T, p pod.Pod, secret string) {
			assert.NoError(t, CheckPodSecret(p.ID, secret))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.NotZero(t, dbPod.TimeInfo.LastCommunicated)
		},
		"FailsWithoutID": func(t *testing.T, p pod.Pod, secret string) {
			assert.Error(t, CheckPodSecret("", secret))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TimeInfo.LastCommunicated)
		},
		"FailsWithoutSecret": func(t *testing.T, p pod.Pod, secret string) {
			assert.Error(t, CheckPodSecret(p.ID, ""))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TimeInfo.LastCommunicated)
		},
		"FailsWithBadID": func(t *testing.T, p pod.Pod, secret string) {
			assert.Error(t, CheckPodSecret("bad_id", secret))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TimeInfo.LastCommunicated)
		},
		"FailsWithBadSecret": func(t *testing.T, p pod.Pod, secret string) {
			assert.Error(t, CheckPodSecret(p.ID, "bad_secret"))

			dbPod, err := pod.FindOneByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, dbPod)
			assert.Zero(t, dbPod.TimeInfo.LastCommunicated)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(pod.Collection, event.EventCollection))

			secretVal := "secret_value"
			p := pod.Pod{
				ID:   "id",
				Type: pod.TypeAgent,
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					EnvSecrets: map[string]pod.Secret{
						pod.PodSecretEnvVar: {
							Value:      secretVal,
							ExternalID: "external_id",
						},
					},
				},
			}
			require.NoError(t, p.Insert())

			tCase(t, p, secretVal)
		})
	}
}
