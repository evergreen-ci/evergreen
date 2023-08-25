package endpoint

import (
	"context"
	"fmt"
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
	grip.Info(message.Fields{
		"message": "got smoke test parameters",
		"params":  fmt.Sprintf("%#v", params),
	})

	appServerCmd := internal.StartAppServer(ctx, t, params.APIParams)
	defer func() {
		if appServerCmd != nil && appServerCmd.Process != nil {
			grip.Error(errors.Wrap(appServerCmd.Process.Signal(syscall.SIGTERM), "stopping app server after test completion"))
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

	grip.Info("Testing UI Endpoints")
	for url, expected := range td.UI {
		makeSmokeGetRequestAndCheck(ctx, t, params, client, url, expected)
	}

	grip.Info("Testing API Endpoints")
	for url, expected := range td.API {
		makeSmokeGetRequestAndCheck(ctx, t, params, client, "/api"+url, expected)
	}
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
	grip.Error(errors.Wrap(err, "making smoke request"))
	page := string(body)
	for _, text := range expected {
		assert.Contains(t, page, text, "should have found expected text from endpoint response")
	}
}
