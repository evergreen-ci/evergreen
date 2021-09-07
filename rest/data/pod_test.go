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
				Name:   utility.ToStringPtr("name"),
				Memory: utility.ToIntPtr(128),
				CPU:    utility.ToIntPtr(128),
				Image:  utility.ToStringPtr("image"),
				EnvVars: []*model.APIPodEnvVar{
					{
						Name:   utility.ToStringPtr("env_name"),
						Value:  utility.ToStringPtr("env_value"),
						Secret: utility.ToBoolPtr(false),
					},
					{
						Name:   utility.ToStringPtr("secret_name"),
						Value:  utility.ToStringPtr("secret_value"),
						Secret: utility.ToBoolPtr(true),
					},
				},
				OS:     utility.ToStringPtr("linux"),
				Arch:   utility.ToStringPtr("amd64"),
				Secret: utility.ToStringPtr("secret"),
			}
			res, err := conn.CreatePod(p)
			require.NoError(t, err)
			require.NotZero(t, res)

			apiPod, err := conn.FindPodByID(res.ID)
			require.NoError(t, err)
			require.NotZero(t, apiPod)

			require.NotZero(t, apiPod.Status)
			assert.Equal(t, model.PodStatusInitializing, *apiPod.Status)
			assert.Equal(t, "secret", utility.FromStringPtr(apiPod.Secret))
			require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvVars)
			assert.Equal(t, "env_value", apiPod.TaskContainerCreationOpts.EnvVars["env_name"])
			assert.NotZero(t, apiPod.TaskContainerCreationOpts.EnvVars["POD_ID"])
			require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvSecrets)
			assert.Equal(t, utility.FromStringPtr(p.Secret), apiPod.TaskContainerCreationOpts.EnvSecrets["POD_SECRET"])
		},
		"FindPodByIDSucceeds": func(t *testing.T, conn Connector) {
			p := pod.Pod{
				ID:     "id",
				Status: pod.StatusInitializing,
				Secret: "secret",
			}
			require.NoError(t, p.Insert())

			apiPod, err := conn.FindPodByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, apiPod)

			assert.Equal(t, p.ID, utility.FromStringPtr(apiPod.ID))
			assert.Equal(t, p.Secret, utility.FromStringPtr(apiPod.Secret))
		},
		"FindPodByIDReturnsNilWithNonexistentPod": func(t *testing.T, conn Connector) {
			apiPod, err := conn.FindPodByID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, apiPod)
		},
		"FindPodByExternalIDSucceeds": func(t *testing.T, conn Connector) {
			p := pod.Pod{
				ID:     "id",
				Secret: "secret",
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
			assert.Equal(t, p.Secret, utility.FromStringPtr(apiPod.Secret))
		},
		"FindPodByExternalIDReturnsNilWithNonexistentPod": func(t *testing.T, conn Connector) {
			apiPod, err := conn.FindPodByExternalID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, apiPod)
		},
		"UpdatePodStatusSucceeds": func(t *testing.T, conn Connector) {
			p := pod.Pod{
				ID:     "id",
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
			assert.Equal(t, updated, *apiPod.Status)
		},
		"UpdatePodStatusFailsWithNonexistentPod": func(t *testing.T, conn Connector) {
			assert.Error(t, conn.UpdatePodStatus("nonexistent", model.PodStatusRunning, model.PodStatusTerminated))
		},
		"CheckPodSecret": func(t *testing.T, conn Connector) {
			p := pod.Pod{
				ID:     "id",
				Secret: "secret",
			}
			require.NoError(t, p.Insert())

			t.Run("Succeeds", func(t *testing.T) {
				assert.NoError(t, conn.CheckPodSecret(p.ID, p.Secret))
			})
			t.Run("FailsWithoutID", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret("", p.Secret))
			})
			t.Run("FailsWithoutSecret", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret(p.ID, ""))
			})
			t.Run("FailsWithBadID", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret("bad_id", p.Secret))
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
