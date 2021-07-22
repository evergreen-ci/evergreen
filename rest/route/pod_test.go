package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/pod"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/send"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPodAgentSetup(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*podAgentSetup)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/pod/agent/setup", nil)
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings) {
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.AgentSetupData)
			require.True(t, ok)
			assert.Equal(t, data.SplunkServerURL, s.Splunk.ServerURL)
			assert.Equal(t, data.SplunkClientToken, s.Splunk.Token)
			assert.Equal(t, data.SplunkChannel, s.Splunk.Channel)
			assert.Equal(t, data.S3Bucket, s.Providers.AWS.S3.Bucket)
			assert.Equal(t, data.S3Key, s.Providers.AWS.S3.Key)
			assert.Equal(t, data.S3Secret, s.Providers.AWS.S3.Secret)
			assert.Equal(t, data.LogkeeperURL, s.LoggerConfig.LogkeeperURL)
		},
		"ReturnsEmpty": func(ctx context.Context, t *testing.T, rh *podAgentSetup, s *evergreen.Settings) {
			*s = evergreen.Settings{}
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.AgentSetupData)
			require.True(t, ok)
			assert.Zero(t, data)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			s := &evergreen.Settings{
				Splunk: send.SplunkConnectionInfo{
					ServerURL: "server_url",
					Token:     "token",
					Channel:   "channel",
				},
				Providers: evergreen.CloudProviders{
					AWS: evergreen.AWSConfig{
						S3: evergreen.S3Credentials{
							Bucket: "bucket",
							Key:    "key",
							Secret: "secret",
						},
						TaskSync: evergreen.S3Credentials{
							Bucket: "bucket",
							Key:    "key",
							Secret: "secret",
						},
					},
				},
				LoggerConfig: evergreen.LoggerConfig{
					LogkeeperURL: "logkeeper_url",
				},
			}

			r, ok := makePodAgentSetup(s).(*podAgentSetup)
			require.True(t, ok)

			tCase(ctx, t, r, s)
		})
	}
}

func TestPodAgentCedarConfig(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *podAgentCedarConfig, s *evergreen.Settings){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *podAgentCedarConfig, s *evergreen.Settings) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*podAgentCedarConfig)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *podAgentCedarConfig, s *evergreen.Settings) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/pod/agent/cedar_config", nil)
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *podAgentCedarConfig, s *evergreen.Settings) {
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.CedarConfig)
			require.True(t, ok)
			assert.Equal(t, data.BaseURL, s.Cedar.BaseURL)
			assert.Equal(t, data.RPCPort, s.Cedar.RPCPort)
			assert.Equal(t, data.Username, s.Cedar.User)
			assert.Equal(t, data.APIKey, s.Cedar.APIKey)
		},
		"ReturnsEmpty": func(ctx context.Context, t *testing.T, rh *podAgentCedarConfig, s *evergreen.Settings) {
			*s = evergreen.Settings{}
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.CedarConfig)
			require.True(t, ok)
			assert.Zero(t, data)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			s := &evergreen.Settings{
				Splunk: send.SplunkConnectionInfo{
					ServerURL: "server_url",
					Token:     "token",
					Channel:   "channel",
				},
				Providers: evergreen.CloudProviders{
					AWS: evergreen.AWSConfig{
						S3: evergreen.S3Credentials{
							Bucket: "bucket",
							Key:    "key",
							Secret: "secret",
						},
						TaskSync: evergreen.S3Credentials{
							Bucket: "bucket",
							Key:    "key",
							Secret: "secret",
						},
					},
				},
				LoggerConfig: evergreen.LoggerConfig{
					LogkeeperURL: "logkeeper_url",
				},
			}

			r, ok := makePodAgentCedarConfig(s).(*podAgentCedarConfig)
			require.True(t, ok)

			tCase(ctx, t, r, s)
		})
	}
}

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
				"platform": "linux",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPut, "https://example.com/rest/v2/pods/123abc", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.NoError(t, ph.Parse(ctx, req))
			assert.Equal(t, 128, utility.FromIntPtr(ph.p.Memory))
			assert.Equal(t, 128, utility.FromIntPtr(ph.p.CPU))
			assert.Equal(t, "image", utility.FromStringPtr(ph.p.Image))
			assert.Equal(t, "linux", utility.FromStringPtr(ph.p.Platform))
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
				"platform": "linux",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPut, "https://example.com/rest/v2/pods/123abc", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.NoError(t, ph.Parse(ctx, req))
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
				"platform": "linux",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPut, "https://example.com/rest/v2/pods/123abc", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.Error(t, ph.Parse(ctx, req))
		},
		"ParseFailsWithMissingInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"cpu": 128,
				"image": "image",
				"env_vars": [],
				"platform": "linux",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPut, "https://example.com/rest/v2/pods/123abc", bytes.NewBuffer(json))
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
				"platform": "linux",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPut, "https://example.com/rest/v2/pods/123abc", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.Error(t, ph.Parse(ctx, req))
		},
		"ParseFailsWithInvalidInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"memory": -1,
				"cpu": 128,
				"image": "image",
				"env_vars": [],
				"platform": "linux",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPut, "https://example.com/rest/v2/pods/123abc", bytes.NewBuffer(json))
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
				"platform": "linux",
				"secret": "secret"
			}`)
			ph.body = json

			resp := ph.Run(ctx)
			require.NotNil(t, resp.Data())
			assert.Equal(t, http.StatusCreated, resp.Status())
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			sc := &data.MockConnector{
				MockPodConnector: data.MockPodConnector{
					CachedPods: []pod.Pod{
						{
							ID: "pod1",
						},
						{
							ID: "pod2",
						},
					},
				},
			}

			p := makePostPod(sc)
			require.NotZero(t, p)

			tCase(ctx, t, p.(*podPostHandler))
		})
	}
}
