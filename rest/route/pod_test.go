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

func TestPutPod(t *testing.T) {
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
					"value": "env_val"
				}],
				"platform": "linux",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPut, "https://example.com/rest/v2/pods/123abc", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.NoError(t, ph.Parse(ctx, req))
		},
		"ParseSucceedsWithSecrets": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"memory": 128,
				"cpu": 128,
				"image": "image",
				"env_vars": [{
					"secret_opts": {
						"name": "name",
						"value": "value
					}
				}],
				"platform": "linux",
				"secret": "secret"
			}`)
			req, err := http.NewRequest(http.MethodPut, "https://example.com/rest/v2/pods/123abc", bytes.NewBuffer(json))
			require.NoError(t, err)
			require.NoError(t, ph.Parse(ctx, req))
		},
		"RunSucceedsWithValidInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
					"memory": 128,
					"cpu": 128,
					"image": "image",
					"env_vars": [{
						"name": "env_name",
						"value": "env_val"
					}],
					"platform": "linux",
					"secret": "secret"
				}`)
			ph.body = json

			resp := ph.Run(ctx)
			require.NotNil(t, resp.Data())
			assert.Equal(t, http.StatusCreated, resp.Status())
		},
		"RunFailsWithInvalidInput": func(ctx context.Context, t *testing.T, ph *podPostHandler) {
			json := []byte(`{
				"memory": "",
				"cpu": 128,
				"image": "image",
				"env_vars": [{
					"name": "env_name",
					"value": "env_val"
				}],
				"platform": "linux",
				"secret": "secret"
			}`)
			ph.body = json

			resp := ph.Run(ctx)
			require.NotNil(t, resp.Data())
			assert.Equal(t, http.StatusInternalServerError, resp.Status())
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
