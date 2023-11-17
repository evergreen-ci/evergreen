package testutil

import (
	"context"
	"flag"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/stretchr/testify/require"
)

const (
	EnvOverride = "SETTINGS_OVERRIDE"
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

	// Manually update admin settings in DB for GitHub App credentials.
	testSettings.AuthConfig = integrationSettings.AuthConfig
	err = testSettings.AuthConfig.Set(context.Background())
	require.NoError(t, err, "Error updating auth config settings in DB")

	if val, ok := integrationSettings.Expansions[evergreen.GithubAppPrivateKey]; ok {
		testSettings.Expansions[evergreen.GithubAppPrivateKey] = val

	}
	err = testSettings.Set(context.Background())
	require.NoError(t, err, "Error updating admin settings in DB")

	// Don't clobber allowed images if it doesn't exist in the override
	// A longer-term fix will be in EVG-20160
	allowedImages := testSettings.Providers.AWS.Pod.ECS.AllowedImages
	testSettings.Providers = integrationSettings.Providers
	if len(integrationSettings.Providers.AWS.Pod.ECS.AllowedImages) == 0 {
		testSettings.Providers.AWS.Pod.ECS.AllowedImages = allowedImages
	}

	testSettings.Credentials = integrationSettings.Credentials
	testSettings.Plugins = integrationSettings.Plugins
	testSettings.Jira = integrationSettings.Jira
	testSettings.GithubPRCreatorOrg = integrationSettings.GithubPRCreatorOrg
	testSettings.Slack = integrationSettings.Slack
	testSettings.ShutdownWaitSeconds = integrationSettings.ShutdownWaitSeconds
}
