package testutil

import (
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/require"
)

const EnvOverride = "SETTINGS_OVERRIDE"

// path to an mci settings file containing sensitive information
var settingsOverride = flag.String("evergreen.settingsOverride", "", "Settings file"+
	" to be used to override sensitive info in the testing mci settings"+
	" file")

// GetDirectoryOfFile returns the path to of the file that calling
// this function. Use this to ensure that references to testdata and
// other file system locations in tests are not dependent on the working
// directory of the "go test" invocation.
func GetDirectoryOfFile() string {
	_, file, _, _ := runtime.Caller(1)

	return filepath.Dir(file)
}

func ConfigureIntegrationTest(t *testing.T, testSettings *evergreen.Settings, testName string) {
	// make sure an override file is provided
	if skip, _ := strconv.ParseBool(os.Getenv("SKIP_INTEGRATION_TESTS")); skip {
		t.Skip("SKIP_INTEGRATION_TESTS is set, skipping integration test")
	}
	if (*settingsOverride) == "" {
		require.NotZero(t, os.Getenv(EnvOverride), "Integration tests need a settings override file to be provided")

		*settingsOverride = os.Getenv(EnvOverride)
	}

	if !filepath.IsAbs(*settingsOverride) {
		*settingsOverride = filepath.Join(evergreen.FindEvergreenHome(), *settingsOverride)
	}

	// grab the file with the integration test settings
	integrationSettings, err := evergreen.NewSettings(*settingsOverride)
	require.NoError(t, err, "Error opening settings override file '%s'", *settingsOverride)

	// testSettings.Providers = integrationSettings.Providers
	testSettings.Credentials = integrationSettings.Credentials
	testSettings.AuthConfig = integrationSettings.AuthConfig
	testSettings.Plugins = integrationSettings.Plugins
	testSettings.Jira = integrationSettings.Jira
	testSettings.GithubPRCreatorOrg = integrationSettings.GithubPRCreatorOrg
	testSettings.Slack = integrationSettings.Slack
	testSettings.ShutdownWaitSeconds = integrationSettings.ShutdownWaitSeconds
}
