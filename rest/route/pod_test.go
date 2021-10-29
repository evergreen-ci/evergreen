package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/data"
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
				"env_vars": [{
					"name": "env_name",
					"value": "env_val",
					"secret": false
				}],
				"os": "windows",
				"arch": "arm64",
				"windows_version": "SERVER_2022",
				"secret": "secret"
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
			assert.Equal(t, "secret", utility.FromStringPtr(ph.p.Secret))
			assert.Equal(t, "env_name", utility.FromStringPtr(ph.p.EnvVars[0].Name))
			assert.Equal(t, "env_val", utility.FromStringPtr(ph.p.EnvVars[0].Value))
			assert.Equal(t, false, utility.FromBoolPtr(ph.p.EnvVars[0].Secret))
		},
		"ParseSucceedsWithSecrets": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"memory": 128,
				"cpu": 128,
				"image": "image",
				"env_vars": [{
					"name": "secret_name",
					"value": "secret_val",
					"secret": true
				}],
				"os": "linux",
				"arch": "arm64",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/pods", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.NoError(t, ph.Parse(ctx, req))
			assert.Equal(t, 128, utility.FromIntPtr(ph.p.Memory))
			assert.Equal(t, 128, utility.FromIntPtr(ph.p.CPU))
			assert.Equal(t, "image", utility.FromStringPtr(ph.p.Image))
			assert.EqualValues(t, "linux", ph.p.OS)
			assert.EqualValues(t, "arm64", ph.p.Arch)
			assert.Equal(t, "secret", utility.FromStringPtr(ph.p.Secret))
			assert.Equal(t, "secret_name", utility.FromStringPtr(ph.p.EnvVars[0].Name))
			assert.Equal(t, "secret_val", utility.FromStringPtr(ph.p.EnvVars[0].Value))
			assert.Equal(t, true, utility.FromBoolPtr(ph.p.EnvVars[0].Secret))
		},
		"ParseFailsWithInvalidJSON": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"memory": 128,
				"cpu": 128,,,
				"image": "image",
				"env_vars": [],
				"os": "linux",
				"arch": "arm64",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/pods", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.Error(t, ph.Parse(ctx, req))
		},
		"ParseFailsWithMissingInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"cpu": 128,
				"image": "image",
				"env_vars": [],
				"os": "linux",
				"arch": "arm64",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/pods", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.Error(t, ph.Parse(ctx, req))
		},
		"ParseFailsWithMissingEnvVarInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"cpu": 128,
				"image": "image",
				"env_vars": [{
					"value": "secret_val",
					"secret": true
				}],
				"os": "linux",
				"arch": "arm64",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/pods", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.Error(t, ph.Parse(ctx, req))
		},
		"ParseFailsWithInvalidInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"memory": -1,
				"cpu": 128,
				"image": "image",
				"env_vars": [],
				"os": "linux",
				"arch": "arm64",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/pods", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.Error(t, ph.Parse(ctx, req))
		},
		"RunSucceedsWithValidInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"memory": 128,
				"cpu": 128,
				"image": "image",
				"env_vars": [{
					"name": "env_name",
					"value": "env_val",
					"secret": false
				}],
				"os": "linux",
				"arch": "arm64",
				"secret": "secret"
			}`)

			req, err := http.NewRequest(http.MethodPost, "https://example.com/rest/v2/pods", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.NoError(t, ph.Parse(ctx, req))
			resp := ph.Run(ctx)
			require.NotNil(t, resp.Data())
			assert.Equal(t, http.StatusCreated, resp.Status())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sc := &data.MockConnector{}
			env := &mock.Environment{}
			require.NoError(t, env.Configure(ctx))

			p := makePostPod(env, sc)
			require.NotZero(t, p)

			tCase(ctx, t, p.(*podPostHandler))
		})
	}
}
