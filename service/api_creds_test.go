package service

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildloggerV3Credentials(t *testing.T) {
	conf := testutil.TestConfig()
	queue := evergreen.GetEnvironment().LocalQueue()
	remoteQueue := evergreen.GetEnvironment().RemoteQueueGroup()
	as, err := NewAPIServer(conf, queue, remoteQueue)
	require.NoError(t, err)

	env := evergreen.GetEnvironment()
	settings := &evergreen.Settings{}
	require.NoError(t, settings.Get(env))
	settings.LoggerConfig.BuildloggerV3User = "user"
	settings.LoggerConfig.BuildloggerV3Password = "pass"
	require.NoError(t, settings.Set())

	url := "/api/2/agent/buildloggerv3_creds"
	request, err := http.NewRequest("GET", url, nil)
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
