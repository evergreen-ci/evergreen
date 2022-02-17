package data

import (
	"context"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPodConnector(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env := testutil.NewEnvironment(ctx, t)
	evergreen.SetEnvironment(env)
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T){
		"CreatePodSucceeds": func(ctx context.Context, t *testing.T) {
			conn := &DBConnector{}
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
				OS:             model.APIPodOS(pod.OSWindows),
				Arch:           model.APIPodArch(pod.ArchAMD64),
				WindowsVersion: model.APIPodWindowsVersion(pod.WindowsVersionServer2019),
				Secret:         utility.ToStringPtr("secret"),
			}
			res, err := conn.CreatePod(p)
			require.NoError(t, err)
			require.NotZero(t, res)

			apiPod, err := conn.FindPodByID(res.ID)
			require.NoError(t, err)
			require.NotZero(t, apiPod)

			assert.Equal(t, model.PodTypeAgent, apiPod.Type)
			assert.Equal(t, model.PodStatusInitializing, apiPod.Status)
			assert.Equal(t, podSecretEnvVar, utility.FromStringPtr(apiPod.Secret.Name))
			assert.Equal(t, utility.FromStringPtr(p.Secret), utility.FromStringPtr(apiPod.Secret.Value))
			require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvVars)
			assert.Equal(t, "env_value", apiPod.TaskContainerCreationOpts.EnvVars["env_name"])
			assert.NotZero(t, apiPod.TaskContainerCreationOpts.EnvVars["POD_ID"])
			require.NotZero(t, apiPod.TaskContainerCreationOpts.EnvSecrets)
			secret, ok := apiPod.TaskContainerCreationOpts.EnvSecrets["POD_SECRET"]
			require.True(t, ok)
			assert.Equal(t, utility.FromStringPtr(p.Secret), utility.FromStringPtr(secret.Value))
		},
		"FindPodByIDSucceeds": func(ctx context.Context, t *testing.T) {
			conn := &DBConnector{}
			p := pod.Pod{
				ID:     "id",
				Type:   pod.TypeAgent,
				Status: pod.StatusInitializing,
				Secret: pod.Secret{
					Name:       podSecretEnvVar,
					Value:      "secret_value",
					ExternalID: "external_id",
					Exists:     utility.TruePtr(),
					Owned:      utility.FalsePtr(),
				},
			}
			require.NoError(t, p.Insert())

			apiPod, err := conn.FindPodByID(p.ID)
			require.NoError(t, err)
			require.NotZero(t, apiPod)

			assert.Equal(t, p.ID, utility.FromStringPtr(apiPod.ID))
			assert.Equal(t, p.Secret, apiPod.Secret.ToService())
		},
		"FindPodByIDReturnsNilWithNonexistentPod": func(ctx context.Context, t *testing.T) {
			conn := &DBConnector{}
			apiPod, err := conn.FindPodByID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, apiPod)
		},
		"FindPodByExternalIDSucceeds": func(ctx context.Context, t *testing.T) {
			conn := &DBConnector{}
			p := pod.Pod{
				ID:   "id",
				Type: pod.TypeAgent,
				Secret: pod.Secret{
					Name:       podSecretEnvVar,
					Value:      "secret_value",
					ExternalID: "external_id",
					Exists:     utility.TruePtr(),
					Owned:      utility.FalsePtr(),
				},
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
			assert.Equal(t, p.Secret, apiPod.Secret.ToService())
		},
		"FindPodByExternalIDReturnsNilWithNonexistentPod": func(ctx context.Context, t *testing.T) {
			conn := &DBConnector{}
			apiPod, err := conn.FindPodByExternalID("nonexistent")
			assert.NoError(t, err)
			assert.Zero(t, apiPod)
		},
		"UpdatePodStatusSucceeds": func(ctx context.Context, t *testing.T) {
			conn := &DBConnector{}
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
		"UpdatePodStatusFailsWithNonexistentPod": func(ctx context.Context, t *testing.T) {
			conn := &DBConnector{}
			assert.Error(t, conn.UpdatePodStatus("nonexistent", model.PodStatusRunning, model.PodStatusTerminated))
		},
		"CheckPodSecret": func(ctx context.Context, t *testing.T) {
			conn := &DBConnector{}
			p := pod.Pod{
				ID:   "id",
				Type: pod.TypeAgent,
				Secret: pod.Secret{
					Name:       podSecretEnvVar,
					Value:      "secret_value",
					ExternalID: "external_id",
					Exists:     utility.TruePtr(),
					Owned:      utility.FalsePtr(),
				},
			}
			require.NoError(t, p.Insert())

			t.Run("Succeeds", func(t *testing.T) {
				assert.NoError(t, conn.CheckPodSecret(p.ID, p.Secret.Value))
			})
			t.Run("FailsWithoutID", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret("", p.Secret.Value))
			})
			t.Run("FailsWithoutSecret", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret(p.ID, ""))
			})
			t.Run("FailsWithBadID", func(t *testing.T) {
				assert.Error(t, conn.CheckPodSecret("bad_id", p.Secret.Value))
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
			tCase(ctx, t)
		})
	}
}
