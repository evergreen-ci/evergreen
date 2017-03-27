package testutil

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v2"
)

// Creates a project ref local config that can be used for testing, with the string identifier given
// and the local config from a path
func CreateTestLocalConfig(testSettings *evergreen.Settings, projectName, projectPath string) error {
	if projectPath == "" {
		config, err := findConfig(testSettings.ConfigDir)
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

// findConfig finds the config root in the home directory.
// Returns an error if the root cannot be found or EVGHOME is unset.
func findConfig(configName string) (string, error) {
	home := evergreen.FindEvergreenHome()
	if len(home) > 0 {
		root, yes := isConfigRoot(home, configName)
		if yes {
			return root, nil
		}
		return "", errors.Errorf("Can't find evergreen config root: '%v'", root)
	}

	return "", errors.Errorf("%v environment variable must be set", evergreen.EvergreenHome)
}

func isConfigRoot(home string, configName string) (fixed string, is bool) {
	fixed = filepath.Join(home, configName)
	fixed = strings.Replace(fixed, "\\", "/", -1)
	stat, err := os.Stat(fixed)
	if err == nil && stat.IsDir() {
		is = true
	}
	return
}
