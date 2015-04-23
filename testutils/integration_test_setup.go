package testutils

import (
	"10gen.com/mci"
	"flag"
	"fmt"
	"testing"
)

var (
	// run the integration tests
	runIntegration = flag.Bool("mci.all", false, "Run integration tests")
	// path to an mci settings file containing sensitive information
	settingsOverride = flag.String("mci.settingsOverride", "", "Settings file"+
		" to be used to override sensitive info in the testing mci settings"+
		" file")
)

func ConfigureIntegrationTest(t *testing.T, testSettings *mci.MCISettings,
	testName string) {
	if !(*runIntegration) {
		t.Skip(fmt.Sprintf("Skipping integration test %v...", testName))
	}

	// make sure an override file is provided
	if (*settingsOverride) == "" {
		panic("Integration tests need a settings override file to be" +
			" provided")
	}

	// grab the file with the integration test settings
	integrationSettings, err := mci.NewMCISettings(*settingsOverride)
	if err != nil {
		panic(fmt.Sprintf("Error opening settings override file %v: %v",
			*settingsOverride, err))
	}

	// override the appropriate params
	t.Logf("Loading cloud provider settings from %v", *settingsOverride)
	testSettings.Providers = integrationSettings.Providers

	testSettings.Credentials = integrationSettings.Credentials
	testSettings.Crowd = integrationSettings.Crowd
}
