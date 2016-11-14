package testutil

import (
	"fmt"
	"io/ioutil"
	"path/filepath"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"gopkg.in/yaml.v2"
)

// Creates a project ref local config that can be used for testing, with the string identifier given
// and the local config from a path
func CreateTestLocalConfig(testSettings *evergreen.Settings, projectName, projectPath string) error {

	if projectPath == "" {
		config, err := evergreen.FindConfig(testSettings.ConfigDir)
		if err != nil {
			return err
		}
		projectPath = filepath.Join(config, "project", fmt.Sprintf("%v.yml", projectName))
	}

	projectRef, err := model.FindOneProjectRef(projectName)
	if err != nil {
		return err
	}

	if projectRef == nil {
		projectRef = &model.ProjectRef{}
	}

	data, err := ioutil.ReadFile(projectPath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, projectRef)
	if err != nil {
		return err
	}

	projectRef.LocalConfig = string(data)

	return projectRef.Upsert()
}
