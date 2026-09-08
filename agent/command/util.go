package command

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/agent/internal/client"
	"github.com/pkg/errors"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
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

func expandModulePrefix(ctx context.Context, conf *internal.TaskConfig, module, prefix string, logger client.LoggerProducer) {
	modulePrefix, err := conf.Expansions.ExpandString(prefix)
	if err != nil {
		logger.Task().Error(ctx, errors.Wrapf(err, "expanding module prefix '%s'", modulePrefix))
		modulePrefix = prefix
		logger.Task().Warningf(ctx, "Will attempt to check out into the module prefix '%s' verbatim.", prefix)
	}
	if conf.ModulePaths == nil {
		conf.ModulePaths = map[string]string{}
	}
	conf.ModulePaths[module] = modulePrefix
}

// GetWorkingDirectory joins the conf.WorkDir A with B like this:
//
//	if B is relative, return A+B.
//	if B is absolute, return B.
//
// We use this because B might be absolute.
func GetWorkingDirectory(conf *internal.TaskConfig, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(conf.WorkDir, path)
}

// workdirBoundaryViolationAttribute records whether a command's resolved path
// falls outside the task working directory.
const workdirBoundaryViolationAttribute = "plugin.workdir_boundary_violation"

// IsWorkdirBoundaryViolation returns true when a configured path resolves
// outside conf.WorkDir. It intentionally evaluates the configured path before
// legacy command-specific rewriting so telemetry captures tasks that request
// unsupported out-of-workdir paths, even if the command ultimately executes
// from a rewritten path under the workdir. Relative paths are resolved as
// GetWorkingDirectory resolves them, then filepath.Rel determines containment.
func IsWorkdirBoundaryViolation(conf *internal.TaskConfig, path string) bool {
	if path == "" {
		return false
	}

	resolved := path
	if !filepath.IsAbs(path) {
		resolved = filepath.Join(conf.WorkDir, path)
	}

	workdir := filepath.Clean(conf.WorkDir)
	resolved = filepath.Clean(resolved)

	rel, err := filepath.Rel(workdir, resolved)
	if err != nil {
		// The path is on a different volume or otherwise incomparable.
		return true
	}

	return rel == ".." ||
		strings.HasPrefix(rel, ".."+string(filepath.Separator)) ||
		filepath.IsAbs(rel)
}

// SetWorkdirBoundaryAttribute records whether any resolved path falls outside
// the task workdir on the span from the context. This is informational only;
// it does not block or error on violations.
func SetWorkdirBoundaryAttribute(ctx context.Context, conf *internal.TaskConfig, paths ...string) {
	violation := false
	for _, path := range paths {
		if IsWorkdirBoundaryViolation(conf, path) {
			violation = true
			break
		}
	}
	trace.SpanFromContext(ctx).SetAttributes(attribute.Bool(workdirBoundaryViolationAttribute, violation))
}

// getWorkingDirectoryLegacy is a legacy function to get the working directory
// for a path, enforce that the path is always prefixed with the task working
// directory, and check that the directory exists. This is a legacy function, so
// should not be used anymore. Commands that need to get the working directory
// should instead use getWorkingDirectory.
func getWorkingDirectoryLegacy(tc *internal.TaskConfig, dir string) (string, error) {
	if dir == "" {
		dir = tc.WorkDir
	} else if strings.HasPrefix(dir, tc.WorkDir) {
		// pass
	} else {
		dir = filepath.Join(tc.WorkDir, dir)
	}

	if stat, err := os.Stat(dir); os.IsNotExist(err) {
		return "", errors.Errorf("path '%s' does not exist", dir)
	} else if err != nil || stat == nil {
		if err == nil {
			err = errors.Errorf("file stat is nil")
		}
		return "", errors.Wrapf(err, "retrieving file info for path '%s'", dir)
	} else if !stat.IsDir() {
		return "", errors.Errorf("path '%s' is not a directory", dir)
	}

	return dir, nil
}

func getTracer() trace.Tracer {
	if tracer == nil {
		tracer = otel.GetTracerProvider().Tracer("github.com/evergreen-ci/evergreen/agent/command")
	}

	return tracer
}
