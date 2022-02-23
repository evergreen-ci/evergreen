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
	for tName, tCase := range map[string]func(t *testing.T, conn Connector){
		"CreatePodSucceeds": func(t *testing.T, conn Connector) {
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
			res, err := conn.CreatePod(p)
			require.NoError(t, err)
			require.NotZero(t, res)

			apiPod, err := conn.FindPodByID(res.ID)
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
		"FindPodByIDSucceeds": func(t *testing.T, conn Connector) {
			p := pod.Pod{
				ID:     "id",
				Type:   pod.TypeAgent,
				Status: pod.StatusInitializing,
			}
			require.NoError(t, p.Insert())

			apiPod, err := conn.FindPodByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, apiPod)

			assert.Equal(t, p.ID, utility.FromStringPtr(apiPod.ID))
			assert.EqualValues(t, p.Type, apiPod.Type)
			assert.EqualValues(t, p.Status, apiPod.Status)
		},
		"FindPodByIDReturnsNilWithNonexistentPod": func(t *testing.T, conn Connector) {
			apiPod, err := conn.FindPodByID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, apiPod)
		},
		"FindPodByExternalIDSucceeds": func(t *testing.T, conn Connector) {
			p := pod.Pod{
				ID:     "id",
				Type:   pod.TypeAgent,
				Status: pod.StatusRunning,
				Resources: pod.ResourceInfo{
					ExternalID: "external_id",
				},
			}
			require.NoError(t, p.Insert())

			apiPod, err := conn.FindPodByExternalID(p.Resources.ExternalID)
			require.NoError(t, err)
			require.NotZero(t, apiPod)

			assert.Equal(t, p.ID, utility.FromStringPtr(apiPod.ID))
			assert.EqualValues(t, p.Type, apiPod.Type)
			assert.EqualValues(t, p.Status, apiPod.Status)
		},
		"FindPodByExternalIDReturnsNilWithNonexistentPod": func(t *testing.T, conn Connector) {
			apiPod, err := conn.FindPodByExternalID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, apiPod)
		},
		"UpdatePodStatusSucceeds": func(t *testing.T, conn Connector) {
			p := pod.Pod{
				ID:     "id",
				Type:   pod.TypeAgent,
				Status: pod.StatusRunning,
			}
			require.NoError(t, p.Insert())

			var current model.APIPodStatus
			require.NoError(t, current.BuildFromService(p.Status))
			updated := model.PodStatusTerminated
			require.NoError(t, conn.UpdatePodStatus(p.ID, current, updated))

			apiPod, err := conn.FindPodByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, apiPod)

			require.NotZero(t, apiPod.Status)
			assert.Equal(t, updated, apiPod.Status)
		},
		"UpdatePodStatusFailsWithNonexistentPod": func(t *testing.T, conn Connector) {
			assert.Error(t, conn.UpdatePodStatus("nonexistent", model.PodStatusRunning, model.PodStatusTerminated))
		},
		"CheckPodSecret": func(t *testing.T, conn Connector) {
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
				assert.NoError(t, conn.CheckPodSecret(p.ID, secretVal))
			})
			t.Run("FailsWithoutID", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret("", secretVal))
			})
			t.Run("FailsWithoutSecret", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret(p.ID, ""))
			})
			t.Run("FailsWithBadID", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret("bad_id", secretVal))
			})
			t.Run("FailsWithBadSecret", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret(p.ID, "bad_secret"))
			})
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(pod.Collection, event.AllLogCollection))
			defer func() {
				assert.NoError(t, db.ClearCollections(pod.Collection, event.AllLogCollection))
			}()
			tCase(t, &DBConnector{})
		})
	}
}
