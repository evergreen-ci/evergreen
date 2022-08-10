package route

import (
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestAgentCedarConfig(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *agentCedarConfig, c evergreen.CedarConfig){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, c evergreen.CedarConfig) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*agentCedarConfig)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, c evergreen.CedarConfig) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/agent/cedar_config", nil)
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, c evergreen.CedarConfig) {
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.CedarConfig)
			require.True(t, ok)
			assert.Equal(t, data.BaseURL, c.BaseURL)
			assert.Equal(t, data.RPCPort, c.RPCPort)
			assert.Equal(t, data.Username, c.User)
			assert.Equal(t, data.APIKey, c.APIKey)
		},
		"ReturnsEmpty": func(ctx context.Context, t *testing.T, rh *agentCedarConfig, _ evergreen.CedarConfig) {
			rh.config = evergreen.CedarConfig{}
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

			c := evergreen.CedarConfig{
				BaseURL: "url.com",
				RPCPort: "9090",
				User:    "user",
				APIKey:  "key",
			}
			r := makeAgentCedarConfig(c)

			tCase(ctx, t, r, c)
		})
	}
}

func TestAgentDataPipesConfig(t *testing.T) {
	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, c evergreen.DataPipesConfig){
		"FactorySucceeds": func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, _ evergreen.DataPipesConfig) {
			copied := rh.Factory()
			assert.NotZero(t, copied)
			_, ok := copied.(*agentDataPipesConfig)
			assert.True(t, ok)
		},
		"ParseSucceeds": func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, _ evergreen.DataPipesConfig) {
			req, err := http.NewRequest(http.MethodGet, "https://example.com/rest/v2/agent/data_pipes_config", nil)
			require.NoError(t, err)
			assert.NoError(t, rh.Parse(ctx, req))
		},
		"RunSucceeds": func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, c evergreen.DataPipesConfig) {
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.DataPipesConfig)
			require.True(t, ok)
			assert.Equal(t, data.Host, c.Host)
			assert.Equal(t, data.Region, c.Region)
			assert.Equal(t, data.AWSAccessKey, c.AWSAccessKey)
			assert.Equal(t, data.AWSSecretKey, c.AWSSecretKey)
			assert.Equal(t, data.AWSToken, c.AWSToken)
		},
		"ReturnsEmpty": func(ctx context.Context, t *testing.T, rh *agentDataPipesConfig, _ evergreen.DataPipesConfig) {
			rh.config = evergreen.DataPipesConfig{}
			resp := rh.Run(ctx)
			require.NotZero(t, resp)
			assert.Equal(t, http.StatusOK, resp.Status())

			data, ok := resp.Data().(apimodels.DataPipesConfig)
			require.True(t, ok)
			assert.Zero(t, data)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			c := evergreen.DataPipesConfig{
				Host:         "https://url.com",
				Region:       "us-east-1",
				AWSAccessKey: "access",
				AWSSecretKey: "secret",
				AWSToken:     "token",
			}
			r := makeAgentDataPipesConfig(c)

			tCase(ctx, t, r, c)
		})
	}
}
