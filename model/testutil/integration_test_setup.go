package testutil

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/pkg/errors"
	"gopkg.in/yaml.v3"
)

// Creates a project ref local config that can be used for testing, with the string identifier given
// and the local config from a path
func CreateTestLocalConfig(ctx context.Context, testSettings *evergreen.Settings, projectName, projectPath string) error {
	if projectPath == "" {
		config, err := findConfig(testSettings.ConfigDir)
		if err != nil {
			return err
		}
		projectPath = filepath.Join(config, "project", fmt.Sprintf("%v.yml", projectName))
	}

	projectRef, err := model.FindBranchProjectRef(ctx, projectName)
	if err != nil {
		return err
	}

	if projectRef == nil {
		projectRef = &model.ProjectRef{}
	}

	data, err := os.ReadFile(projectPath)
	if err != nil {
		return err
	}

	err = yaml.Unmarshal(data, projectRef)
	if err != nil {
		return err
	}

	return projectRef.Replace(ctx)
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
		return "", errors.Errorf("could not find Evergreen config root '%s'", root)
	}

	return "", errors.Errorf("environment variable '%s' must be set", evergreen.EvergreenHome)
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
