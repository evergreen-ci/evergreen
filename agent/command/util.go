package command

import (
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/trace"
)

var tracer trace.Tracer

func dirExists(path string) (bool, error) {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, errors.Wrap(err, "running stat on path")
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
		return errors.Wrapf(err, "creating directory '%s'", localDir)
	}

	return nil
}

func expandModulePrefix(conf *internal.TaskConfig, module, prefix string, logger client.LoggerProducer) string {
	modulePrefix, err := conf.Expansions.ExpandString(prefix)
	if err != nil {
		logger.Task().Error(errors.Wrapf(err, "expanding module prefix '%s'", modulePrefix))
		modulePrefix = prefix
		logger.Task().Warningf("Will attempt to check out into the module prefix '%s' verbatim.", prefix)
	}
	if conf.ModulePaths == nil {
		conf.ModulePaths = map[string]string{}
	}
	conf.ModulePaths[module] = modulePrefix
	return modulePrefix
}

// getJoinedWithWorkDir joins the conf.WorkDir A with B like this:
//
//	if B is relative, return A+B.
//	if B is absolute, return B.
//
// We use this because B might be absolute.
func getJoinedWithWorkDir(conf *internal.TaskConfig, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(conf.WorkDir, path)
}

func getTracer() trace.Tracer {
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer("evergreen_command")
	}

	return tracer
}
