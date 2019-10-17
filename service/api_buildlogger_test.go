package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildlogger(t *testing.T) {
	conf := testutil.TestConfig()
	conf.LoggerConfig.BuildloggerBaseURL = "cedar.mongodb.com"
	conf.LoggerConfig.BuildloggerRPCPort = "7070"
	conf.LoggerConfig.BuildloggerUser = "user"
	conf.LoggerConfig.BuildloggerPassword = "pass"
	queue := evergreen.GetEnvironment().LocalQueue()
	remoteQueue := evergreen.GetEnvironment().RemoteQueueGroup()
	as, err := NewAPIServer(conf, queue, remoteQueue)
	require.NoError(t, err)

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

	url := "/api/2/agent/buildlogger_creds"
	request, err := http.NewRequest("GET", url, nil)
	request.Header.Add(evergreen.HostHeader, sampleHost.Id)
	request.Header.Add(evergreen.HostSecretHeader, sampleHost.Secret)
	require.NoError(t, err)
	w := httptest.NewRecorder()

	handler, err := as.GetServiceApp().Handler()
	require.NoError(t, err)
	handler.ServeHTTP(w, request)

	require.Equal(t, http.StatusOK, w.Code)
	creds := &Credentials{}
	bl := &apimodels.BuildloggerInfo{}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), creds))
	assert.Equal(t, "cedar.mongodb.com", bl.BaseURL)
	assert.Equal(t, "7070", bl.RPCPort)
	assert.Equal(t, "user", bl.Username)
	assert.Equal(t, "pass", bl.Password)
}
