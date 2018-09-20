package testutil

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/evergreen-ci/evergreen"
)

const EnvOverride = "SETTINGS_OVERRIDE"

// path to an mci settings file containing sensitive information
var settingsOverride = flag.String("evergreen.settingsOverride", "", "Settings file"+
	" to be used to override sensitive info in the testing mci settings"+
	" file")

// HandleTestingErr catches errors that we do not want to treat
// as relevant a goconvey statement. HandleTestingErr is used
// to terminate unit tests that fail for reasons that are orthogonal to
// the test (filesystem errors, database errors, etc).
func HandleTestingErr(err error, t *testing.T, format string, a ...interface{}) {
	if err != nil {
		_, file, line, ok := runtime.Caller(1)
		if ok {
			t.Fatalf("%v:%v: %q: %+v", file, line, fmt.Sprintf(format, a...), err)
		} else {
			t.Fatalf("%q: %+v", fmt.Sprintf(format, a...), err)
		}
	}
}

// GetDirectoryOfFile returns the path to of the file that calling
// this function. Use this to ensure that references to testdata and
// other file system locations in tests are not dependent on the working
// directory of the "go test" invocation.
func GetDirectoryOfFile() string {
	_, file, _, _ := runtime.Caller(1)

	return filepath.Dir(file)
}

// SkipWindows
func SkipWindows(t *testing.T, testName string) {
	// Note: in the future we could/should be able to eliminate
	// the testName arg by using runtime.Caller(1)
	if runtime.GOOS == "windows" {
		t.Skip(fmt.Sprintf("skipping test '%s' on windows", testName))
	}
}

func ConfigureIntegrationTest(t *testing.T, testSettings *evergreen.Settings, testName string) {
	// make sure an override file is provided
	if (*settingsOverride) == "" {
		if os.Getenv(EnvOverride) == "" {
			panic("Integration tests need a settings override file to be provided")
		}

		*settingsOverride = os.Getenv(EnvOverride)
	}

	if !filepath.IsAbs(*settingsOverride) {
		*settingsOverride = filepath.Join(evergreen.FindEvergreenHome(), *settingsOverride)
	}

	// grab the file with the integration test settings
	integrationSettings, err := evergreen.NewSettings(*settingsOverride)
	if err != nil {
		panic(fmt.Sprintf("Error opening settings override file %v: %v",
			*settingsOverride, err))
	}

	testSettings.Providers = integrationSettings.Providers
	testSettings.Credentials = integrationSettings.Credentials
	testSettings.AuthConfig = integrationSettings.AuthConfig
	testSettings.Plugins = integrationSettings.Plugins
	testSettings.Jira = integrationSettings.Jira
	testSettings.GithubPRCreatorOrg = integrationSettings.GithubPRCreatorOrg
	testSettings.Slack = integrationSettings.Slack
}
