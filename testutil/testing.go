package testutil

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/mongodb/grip"
	"github.com/stretchr/testify/require"
)

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

// GetIntegrationFile returns an initialized evergreen.Settings struct. It will
// halt test execution if an override file was not provided, or if there was
// an error parsing the settings
func GetIntegrationFile(t *testing.T) *evergreen.Settings {
	if (*settingsOverride) == "" {
		require.NotZero(t, os.Getenv(evergreen.SettingsOverride), "Integration tests need a settings override file to be provided")

		*settingsOverride = os.Getenv(evergreen.SettingsOverride)
	}

	if !filepath.IsAbs(*settingsOverride) {
		*settingsOverride = filepath.Join(evergreen.FindEvergreenHome(), *settingsOverride)
	}

	// grab the file with the integration test settings
	integrationSettings, err := evergreen.NewSettings(*settingsOverride)
	require.NoError(t, err, "Error opening settings override file '%s'", *settingsOverride)

	return integrationSettings
}

func ConfigureIntegrationTest(t *testing.T, testSettings *evergreen.Settings) {
	// make sure an override file is provided
	if skip, _ := strconv.ParseBool(os.Getenv("SKIP_INTEGRATION_TESTS")); skip {
		t.Skip("SKIP_INTEGRATION_TESTS is set, skipping integration test")
	}

	integrationSettings := GetIntegrationFile(t)

	// Manually update admin settings in DB for GitHub App credentials.
	testSettings.AuthConfig = integrationSettings.AuthConfig
	err := testSettings.AuthConfig.Set(context.Background())
	require.NoError(t, err, "Error updating auth config settings in DB")

	if val, ok := integrationSettings.Expansions[evergreen.GithubAppPrivateKey]; ok {
		testSettings.Expansions[evergreen.GithubAppPrivateKey] = val
	}
	err = testSettings.Set(context.Background())
	require.NoError(t, err, "Error updating admin settings in DB")

	catcher := grip.NewBasicCatcher()
	evergreen.StoreAdminSecrets(context.Background(),
		evergreen.GetEnvironment().ParameterManager(),
		reflect.ValueOf(testSettings).Elem(),
		reflect.TypeOf(*testSettings), "", catcher)
	require.NoError(t, catcher.Resolve(), "Error storing admin secrets in parameter store")

	// Don't clobber allowed images if it doesn't exist in the override
	// A longer-term fix will be in DEVPROD-745
	testSettings.Providers = integrationSettings.Providers
	testSettings.Plugins = integrationSettings.Plugins
	testSettings.Jira = integrationSettings.Jira
	testSettings.RuntimeEnvironments = integrationSettings.RuntimeEnvironments
	testSettings.GithubPRCreatorOrg = integrationSettings.GithubPRCreatorOrg
	testSettings.Slack = integrationSettings.Slack
	testSettings.ShutdownWaitSeconds = integrationSettings.ShutdownWaitSeconds

}
