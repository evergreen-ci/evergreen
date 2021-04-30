package command

import (
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/pkg/errors"
)

func dirExists(path string) (bool, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "problem running stat on path")
	}

	if !stat.IsDir() {
		return false, nil
	}

	return true, nil
}

func createEnclosingDirectoryIfNeeded(path string) error {
	localDir := filepath.Dir(path)

	exists, err := dirExists(path)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}

	if err := os.MkdirAll(localDir, 0755); err != nil {
		return errors.Wrapf(err, "problem creating directory %s", localDir)
	}

	return nil
}

func expandModulePrefix(conf *internal.TaskConfig, module, prefix string, logger client.LoggerProducer) string {
	modulePrefix, err := conf.Expansions.ExpandString(prefix)
	if err != nil {
		modulePrefix = prefix
		logger.Task().Errorf("module prefix '%s' can't be expanded: %s", prefix, err.Error())
		logger.Task().Warning("will attempt to check out into the module prefix verbatim")
	}
	if conf.ModulePaths == nil {
		conf.ModulePaths = map[string]string{}
	}
	conf.ModulePaths[module] = modulePrefix
	return modulePrefix
}
