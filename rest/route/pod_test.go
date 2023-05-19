package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPostPod(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, ph *podPostHandler){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			copied := ph.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*podPostHandler)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"memory": 128,
				"cpu": 128,
				"image": "image",
				"os": "windows",
				"arch": "arm64",
				"windows_version": "SERVER_2022",
				"working_dir": "/",
				"pod_secret_external_id": "external_id",
				"pod_secret_value": "secret_value"
			}`)
			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/pods", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.NoError(t, ph.Parse(ctx, req))
			assert.Equal(t, 128, utility.FromIntPtr(ph.p.Memory))
			assert.Equal(t, 128, utility.FromIntPtr(ph.p.CPU))
			assert.Equal(t, "image", utility.FromStringPtr(ph.p.Image))
			assert.EqualValues(t, pod.OSWindows, ph.p.OS)
			assert.EqualValues(t, pod.ArchARM64, ph.p.Arch)
			assert.EqualValues(t, pod.WindowsVersionServer2022, ph.p.WindowsVersion)
			assert.Equal(t, "external_id", utility.FromStringPtr(ph.p.PodSecretExternalID))
			assert.Equal(t, "secret_value", utility.FromStringPtr(ph.p.PodSecretValue))
		},
		"RunSucceedsWithValidInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"memory": 128,
				"cpu": 128,
				"image": "image",
				"os": "linux",
				"arch": "arm64",
				"working_dir": "/",
				"pod_secret_external_id": "external_id",
				"pod_secret_value": "secret_value"
			}`)

			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/pods", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.NoError(t, ph.Parse(ctx, req))
			resp := ph.Run(ctx)
			require.NotNil(t, resp.Data())
			assert.Equal(t, http.StatusCreated, resp.Status())
		},
		"RunFailsWithInvalidInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{}`)

			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/pods", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.NoError(t, ph.Parse(ctx, req))
			resp := ph.Run(ctx)
			require.NotNil(t, resp.Data())
			assert.True(t, resp.Status() >= 400, "input should be rejected")
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			env := evergreen.GetEnvironment()
			env.Settings().Providers.AWS.Pod.ECS = evergreen.ECSConfig{
				AllowedImages: []string{
					"image",
				},
			}

			p := makePostPod(env)
			require.NotZero(t, p)

			tCase(ctx, t, p.(*podPostHandler))
		})
	}
}

func TestGetPod(t *testing.T) {
	require.NoError(t, db.ClearCollections(pod.Collection))
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, ph *podGetHandler){
		"RunSucceeds": func(ctx context.Context, t *testing.T, ph *podGetHandler) {
			podID := "id"

			podToInsert := pod.Pod{
				ID:     podID,
				Type:   pod.TypeAgent,
				Status: pod.StatusRunning,
			}
			require.NoError(t, podToInsert.Insert())

			ph.podID = podID
			resp := ph.Run(ctx)
			require.NotZero(t, resp)

			require.Equal(t, http.StatusOK, resp.Status())
			require.NotZero(t, resp.Data())
			apiPod, ok := resp.Data().(*model.APIPod)
			require.True(t, ok)
			assert.Equal(t, podID, utility.FromStringPtr(apiPod.ID))
			assert.Equal(t, model.PodTypeAgent, apiPod.Type)
			assert.Equal(t, model.PodStatusRunning, apiPod.Status)
		},
		"RunFailsWithNonexistentPod": func(ctx context.Context, t *testing.T, ph *podGetHandler) {
			ph.podID = "nonexistent"
			resp := ph.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusNotFound, resp.Status())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			p := makeGetPod(testutil.NewEnvironment(ctx, t))
			require.NotZero(t, p)

			tCase(ctx, t, p.(*podGetHandler))
		})
	}
}
