package testutils

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"flag"
	"fmt"
	"gopkg.in/yaml.v2"
	"io/ioutil"
	"path/filepath"
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

// Creates a project ref local config that can be used for testing, with the string identifier given
// and the local config from a path
func CreateTestLocalConfig(testSettings *mci.MCISettings, projectName string) error {

	config, err := mci.FindMCIConfig(testSettings.ConfigDir)
	if err != nil {
		return err
	}
	projectRef, err := model.FindOneProjectRef(projectName)
	if err != nil {
		return err
	}

	if projectRef == nil {
		projectRef = &model.ProjectRef{}
	}

	projectPath := filepath.Join(config, "project", fmt.Sprintf("%v.yml", projectName))
	data, err := ioutil.ReadFile(projectPath)
	if err != nil {
		return err
	}

	project := &model.Project{}
	err = yaml.Unmarshal(data, project)
	if err != nil {
		return err
	}

	projectRef = &model.ProjectRef{
		Identifier:  projectName,
		Owner:       project.Owner,
		Repo:        project.Repo,
		Branch:      project.Branch,
		Enabled:     project.Enabled,
		LocalConfig: string(data)}

	return projectRef.Upsert()
}
