package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen/db"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildlogger(t *testing.T) {
	assert.NoError(t, db.ClearCollections(host.Collection))
	conf := testutil.TestConfig()
	conf.LoggerConfig.BuildloggerBaseURL = "cedar.mongodb.com"
	conf.LoggerConfig.BuildloggerRPCPort = "7070"
	conf.LoggerConfig.BuildloggerUser = "user"
	conf.LoggerConfig.BuildloggerPassword = "pass"
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
		AgentRevision: evergreen.BuildRevision,
	}
	require.NoError(t, sampleHost.Insert())

	url := "/api/2/agent/buildlogger_info"
	request, err := http.NewRequest("GET", url, nil)
	request.Header.Add(evergreen.HostHeader, sampleHost.Id)
	request.Header.Add(evergreen.HostSecretHeader, sampleHost.Secret)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	handler, err := as.GetServiceApp().Handler()
	require.NoError(t, err)
	handler.ServeHTTP(w, request)

	require.Equal(t, http.StatusOK, w.Code)
	bi := &apimodels.BuildloggerInfo{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), bi))
	assert.Equal(t, "cedar.mongodb.com", bi.BaseURL)
	assert.Equal(t, "7070", bi.RPCPort)
	assert.Equal(t, "user", bi.Username)
	assert.Equal(t, "pass", bi.Password)
}
