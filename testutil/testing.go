package testutil

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
)

var (
	// run the integration tests
	runAllTests = flag.Bool("evergreen.all", false, "Run integration tests")
	// path to an mci settings file containing sensitive information
	settingsOverride = flag.String("evergreen.settingsOverride", "", "Settings file"+
		" to be used to override sensitive info in the testing mci settings"+
		" file")
)

// HandleTestingErr catches errors that we do not want to treat
// as relevant a goconvey statement. HandleTestingErr is used
// to terminate unit tests that fail for reasons that are orthogonal to
// the test (filesystem errors, database errors, etc).
func HandleTestingErr(err error, t *testing.T, format string, a ...interface{}) {
	if err != nil {
		_, file, line, ok := runtime.Caller(1)
		if ok {
			t.Fatalf("%v:%v: %q: %v", file, line, fmt.Sprintf(format, a), err)
		} else {
			t.Fatalf("%q: %v", fmt.Sprintf(format, a), err)
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

// SkipTestUnlessAll skips the current test.
func SkipTestUnlessAll(t *testing.T, testName string) {
	// Note: in the future we could/should be able to eliminate
	// the testName arg by using runtime.Caller(1)

	if !(*runAllTests) || !strings.Contains(os.Getenv("TEST_ARGS"), "evergreen.all") {
		t.Skip(fmt.Sprintf("skipping %v because 'evergreen.all' is not specified...",
			testName))
	}
}

func ConfigureIntegrationTest(t *testing.T, testSettings *evergreen.Settings,
	testName string) {

	SkipTestUnlessAll(t, testName)

	// make sure an override file is provided
	if (*settingsOverride) == "" {
		msg := "Integration tests need a settings override file to be provided"
		keyName := "evergreen.settingsOverride"
		if !strings.Contains(os.Getenv("TEST_ARGS"), keyName) {
			panic(msg)
		}
		for _, k := range os.Environ() {
			if strings.HasPrefix(k, keyName) {
				parts := strings.Split(k, "=")
				if len(parts) < 2 {
					panic(msg)
				}
				*settingsOverride = parts[1]
			}
		}
	}

	// grab the file with the integration test settings
	integrationSettings, err := evergreen.NewSettings(*settingsOverride)
	if err != nil {
		panic(fmt.Sprintf("Error opening settings override file %v: %v",
			*settingsOverride, err))
	}

	// override the appropriate params
	t.Logf("Loading cloud provider settings from %v", *settingsOverride)

	testSettings.Providers = integrationSettings.Providers
	testSettings.Credentials = integrationSettings.Credentials
	testSettings.AuthConfig = integrationSettings.AuthConfig
	testSettings.Plugins = integrationSettings.Plugins
	testSettings.Jira = integrationSettings.Jira
}
