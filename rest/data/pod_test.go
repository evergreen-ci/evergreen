package data

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodConnector(t *testing.T) {
	for tName, tCase := range map[string]func(t *testing.T){
		"CreatePodSucceeds": func(t *testing.T) {
			p := model.APICreatePod{
				Name:           utility.ToStringPtr("name"),
				Memory:         utility.ToIntPtr(128),
				CPU:            utility.ToIntPtr(128),
				Image:          utility.ToStringPtr("image"),
				OS:             model.APIPodOS(pod.OSWindows),
				Arch:           model.APIPodArch(pod.ArchAMD64),
				WindowsVersion: model.APIPodWindowsVersion(pod.WindowsVersionServer2019),
				Secret:         utility.ToStringPtr("secret"),
				WorkingDir:     utility.ToStringPtr("/working/dir"),
			}
			res, err := CreatePod(p)
			require.NoError(t, err)
			require.NotZero(t, res)

			apiPod, err := FindPodByID(res.ID)
			require.NoError(t, err)
			require.NotZero(t, apiPod)

			assert.Equal(t, model.PodTypeAgent, apiPod.Type)
			assert.Equal(t, model.PodStatusInitializing, apiPod.Status)
			require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvVars)
			assert.NotZero(t, apiPod.TaskContainerCreationOpts.EnvVars["POD_ID"])
			require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvSecrets)
			secret, ok := apiPod.TaskContainerCreationOpts.EnvSecrets[pod.PodSecretEnvVar]
			require.True(t, ok)
			assert.Equal(t, utility.FromStringPtr(p.Secret), utility.FromStringPtr(secret.Value))
		},
		"FindPodByIDSucceeds": func(t *testing.T) {
			p := pod.Pod{
				ID:     "id",
				Type:   pod.TypeAgent,
				Status: pod.StatusInitializing,
			}
			require.NoError(t, p.Insert())

			apiPod, err := FindPodByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, apiPod)

			assert.Equal(t, p.ID, utility.FromStringPtr(apiPod.ID))
			assert.EqualValues(t, p.Type, apiPod.Type)
			assert.EqualValues(t, p.Status, apiPod.Status)
		},
		"FindPodByIDReturnsNilWithNonexistentPod": func(t *testing.T) {
			apiPod, err := FindPodByID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, apiPod)
		},
		"CheckPodSecret": func(t *testing.T) {
			secretVal := "secret_value"
			p := pod.Pod{
				ID:   "id",
				Type: pod.TypeAgent,
				TaskContainerCreationOpts: pod.TaskContainerCreationOptions{
					EnvSecrets: map[string]pod.Secret{
						pod.PodSecretEnvVar: {
							Value:      secretVal,
							ExternalID: "external_id",
							Exists:     utility.TruePtr(),
							Owned:      utility.FalsePtr(),
						},
					},
				},
			}
			require.NoError(t, p.Insert())

			t.Run("Succeeds", func(t *testing.T) {
				assert.NoError(t, CheckPodSecret(p.ID, secretVal))
			})
			t.Run("FailsWithoutID", func(t *testing.T) {
				assert.Error(t, CheckPodSecret("", secretVal))
			})
			t.Run("FailsWithoutSecret", func(t *testing.T) {
				assert.Error(t, CheckPodSecret(p.ID, ""))
			})
			t.Run("FailsWithBadID", func(t *testing.T) {
				assert.Error(t, CheckPodSecret("bad_id", secretVal))
			})
			t.Run("FailsWithBadSecret", func(t *testing.T) {
				assert.Error(t, CheckPodSecret(p.ID, "bad_secret"))
			})
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(pod.Collection, event.AllLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(pod.Collection, event.AllLogCollection))
			}()
			tCase(t)
		})
	}
}
