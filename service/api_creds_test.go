package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildloggerV3Credentials(t *testing.T) {
	conf := testutil.TestConfig()
	conf.LoggerConfig.BuildloggerV3User = "user"
	conf.LoggerConfig.BuildloggerV3Password = "pass"
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

	url := "/api/2/agent/buildloggerv3_creds"
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
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), creds))
	assert.Equal(t, "user", creds.Username)
	assert.Equal(t, "pass", creds.Password)
}
