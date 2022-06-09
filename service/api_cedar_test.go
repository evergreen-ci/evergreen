package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCedar(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.Collection))
	conf := testutil.TestConfig()
	conf.Cedar.BaseURL = "cedar.mongodb.com"
	conf.Cedar.RPCPort = "7070"
	conf.Cedar.User = "user"
	conf.Cedar.APIKey = "api_key"
	queue := evergreen.GetEnvironment().LocalQueue()
	as, err := NewAPIServer(evergreen.GetEnvironment(), queue)
	require.NoError(t, err)
	as.Settings = *conf

	sampleHost := host.Host{
		Id: "h1",
		Distro: distro.Distro{
			Id: "d1",
		},
		Secret:        "secret",
		Provisioned:   true,
		Status:        evergreen.HostRunning,
		AgentRevision: evergreen.AgentVersion,
	}
	require.NoError(t, sampleHost.Insert())

	url := "/api/2/agent/cedar_config"
	request, err := http.NewRequest("GET", url, nil)
	request.Header.Add(evergreen.HostHeader, sampleHost.Id)
	request.Header.Add(evergreen.HostSecretHeader, sampleHost.Secret)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	handler, err := as.GetServiceApp().Handler()
	require.NoError(t, err)
	handler.ServeHTTP(w, request)

	require.Equal(t, http.StatusOK, w.Code)
	cc := &apimodels.CedarConfig{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), cc))
	assert.Equal(t, "cedar.mongodb.com", cc.BaseURL)
	assert.Equal(t, "7070", cc.RPCPort)
	assert.Equal(t, "user", cc.Username)
	assert.Equal(t, "api_key", cc.APIKey)
}
