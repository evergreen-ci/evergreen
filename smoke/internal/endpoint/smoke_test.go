package endpoint

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/smoke/internal"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
)

// TestSmokeEndpoints runs the smoke test for the app server REST and UI
// endpoints.
func TestSmokeEndpoints(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	params := getSmokeTestParamsFromEnv(t)
	grip.Info(ctx, message.Fields{
		"message": "got smoke test parameters",
		"params":  fmt.Sprintf("%#v", params),
	})

	appServerCmd := internal.StartAppServer(ctx, t, params.APIParams)
	defer func() {
		if appServerCmd != nil {
			grip.Error(ctx, errors.Wrap(appServerCmd.Signal(ctx, syscall.SIGTERM), "stopping app server after test completion"))
		}
	}()

	defs, err := os.ReadFile(params.testFile)
	require.NoError(t, err, "should have been able to read smoke test file '%s'", params.testFile)

	td := smokeEndpointTestDefinitions{}
	require.NoError(t, yaml.Unmarshal(defs, &td), "should have unmarshalled YAML endpoint test definitions from file '%s'", params.testFile)
	require.NotEmpty(t, td, "must test at least one test definition")

	client := utility.GetHTTPClient()
	defer utility.PutHTTPClient(client)
	client.Timeout = time.Second

	internal.WaitForEvergreen(t, params.AppServerURL, client)

	grip.Info(ctx, "Testing UI Endpoints")
	for url, expected := range td.UI {
		makeSmokeGetRequestAndCheck(ctx, t, params, client, url, expected)
	}

	grip.Info(ctx, "Testing API Endpoints")
	for url, expected := range td.API {
		makeSmokeGetRequestAndCheck(ctx, t, params, client, "/api"+url, expected)
	}

	grip.Info(ctx, "Testing /select/tests POST with user auth")
	checkSelectTestsUserAuth(ctx, t, params, client)
}

// checkSelectTestsUserAuth POSTs to /rest/v2/select/tests as an authenticated
// user. Regression guard for DEVPROD-34146: before that fix, this route was
// wrapped with a middleware that rejected user-auth requests with
// 400 "task ID is required" because it required {task_id} as a URL path var
// that this route does not have.
func checkSelectTestsUserAuth(ctx context.Context, t *testing.T, params smokeTestParams, client *http.Client) {
	body := []byte(`{"project_id":"smoke","build_variant":"variant","requester":"patch","task_id":"smoke-task","task_name":"smoke-task","tests":["foo"]}`)
	url := params.AppServerURL + "/rest/v2/select/tests"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set(evergreen.APIUserHeader, params.Username)
	req.Header.Set(evergreen.APIKeyHeader, params.APIKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	respBody, err := io.ReadAll(resp.Body)
	require.NoError(t, err)

	// The fix is to never return the auth middleware's
	// 400 "task ID is required" for this route. Downstream errors (e.g. TSS
	// unreachable) are fine for smoke purposes — we only guard the
	// middleware-level regression.
	assert.NotEqual(t, http.StatusBadRequest, resp.StatusCode,
		"user-authenticated POST to /select/tests should not get a 400; body %q", string(respBody))
	assert.NotContains(t, string(respBody), "task ID is required",
		"response should not contain the old middleware's 'task ID is required'; got status %d body %q", resp.StatusCode, string(respBody))
}

type smokeTestParams struct {
	internal.APIParams
	testFile string
}

// getSmokeTestParamsFromEnv gets the necessary parameters for the app server
// endpoints smoke test. It sets defaults where possible. Note that the default
// data depends on the setup test data for the smoke test.
func getSmokeTestParamsFromEnv(t *testing.T) smokeTestParams {
	evgHome := evergreen.FindEvergreenHome()
	require.NotZero(t, evgHome, "EVGHOME must be set for smoke test")

	testFile := os.Getenv("TEST_FILE")
	if testFile == "" {
		testFile = filepath.Join(evgHome, "smoke", "internal", "testdata", "smoke_test_endpoints.yml")
	}

	return smokeTestParams{
		APIParams: internal.GetAPIParamsFromEnv(t, evgHome),
		testFile:  testFile,
	}
}

// smokeEndpointTestDefinitions describes the UI and API endpoints to verify,
// mapping the route name to the expected response text from requesting that
// route.
type smokeEndpointTestDefinitions struct {
	UI  map[string][]string `yaml:"ui,omitempty"`
	API map[string][]string `yaml:"api,omitempty"`
}

func makeSmokeGetRequestAndCheck(ctx context.Context, t *testing.T, params smokeTestParams, client *http.Client, url string, expected []string) {
	body, err := internal.MakeSmokeRequest(ctx, params.APIParams, http.MethodGet, client, url)
	grip.Error(ctx, errors.Wrap(err, "making smoke request"))
	page := string(body)
	for _, text := range expected {
		assert.Contains(t, page, text, "should have found expected text from endpoint response")
	}
}
