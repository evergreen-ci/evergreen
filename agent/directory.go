package agent

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/recovery"
	"github.com/pkg/errors"
	"github.com/shirou/gopsutil/v3/disk"
)

// createTaskDirectory makes a directory for the agent to execute the current
// task within and a temporary directory within that new directory. If taskDir
// is specified, it will create that task directory; otherwise, it will create
// a new task directory based on the current task data.
func (a *Agent) createTaskDirectory(tc *taskContext, taskDir string) (string, error) {
	if taskDir == "" {
		h := md5.New()

		_, err := h.Write([]byte(
			fmt.Sprintf("%s_%d_%d", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution, os.Getpid())))
		if err != nil {
			tc.logger.Execution().Error(errors.Wrap(err, "creating task directory name"))
			return "", err
		}

		dirName := hex.EncodeToString(h.Sum(nil))
		taskDir = filepath.Join(a.opts.WorkingDirectory, dirName)
	}

	tc.logger.Execution().Infof("Making task directory '%s' for task execution.", taskDir)

	if err := os.MkdirAll(taskDir, 0777); err != nil {
		tc.logger.Execution().Error(errors.Wrapf(err, "creating task directory '%s'", taskDir))
		return "", err
	}

	tmpDir := filepath.Join(taskDir, "tmp")
	tc.logger.Execution().Infof("Making task temporary directory '%s' for task execution.", tmpDir)

	if err := os.MkdirAll(tmpDir, 0777); err != nil {
		tc.logger.Execution().Warning(errors.Wrapf(err, "creating task temporary directory '%s'", tmpDir))
	}

	return taskDir, nil
}

// removeTaskDirectory removes the folder the agent created for the task it was
// executing. It does not return an error because it is executed at the end of a
// task run, and the agent loop will start another task regardless of how this
// exits. If it cannot remove the task directory, the agent may disable the host
// because leaving the task directory behind could impact later tasks.
func (a *Agent) removeTaskDirectory(ctx context.Context, tc *taskContext) {
	if tc.taskConfig == nil || tc.taskConfig.WorkDir == "" {
		grip.Info("Task directory is not set, not removing.")
		return
	}

	dir := tc.taskConfig.WorkDir

	grip.Infof("Deleting task directory '%s' for completed task.", dir)

	// Removing long relative paths hangs on Windows https://github.com/golang/go/issues/36375,
	// so we have to convert to an absolute path before removing.
	abs, err := filepath.Abs(dir)
	if err != nil {
		grip.Critical(errors.Wrapf(err, "getting absolute path for task directory '%s'", dir))
		return
	}
	if err := a.removeAllAndCheck(ctx, abs); err != nil {
		grip.Critical(errors.Wrapf(err, "removing task directory '%s'", dir))
	} else {
		grip.Info(message.Fields{
			"message":   "Successfully deleted directory for completed task.",
			"directory": tc.taskConfig.WorkDir,
		})
	}
}

// removeAllAndCheck removes the directory and checks the data directory
// usage afterwards. If the data directory is unhealthy, the host is disabled.
func (a *Agent) removeAllAndCheck(ctx context.Context, dir string) error {
	removeErr := a.removeAll(dir)
	if removeErr == nil {
		return nil
	}

	a.numTaskDirCleanupFailures++

	checkCtx, checkCancel := context.WithTimeout(ctx, time.Minute)
	defer checkCancel()
	if err := a.checkDataDirectoryHealth(checkCtx); err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message":               "failed to check data directory usage",
			"unremovable_directory": dir,
		}))
	}

	return removeErr
}

// removeAll is the same as os.RemoveAll, but recursively changes permissions
// for subdirectories and contents before removing. The permissions change fixes
// an issue where some files may be marked read-only, which prevents
// os.RemoveAll from deleting them.
func (a *Agent) removeAll(dir string) error {
	grip.Error(errors.Wrapf(filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		grip.Error(errors.Wrapf(os.Chmod(path, 0777), "changing permission before removal for path '%s'", path))
		return nil
	}), "recursively walking through path to change permissions"))
	return os.RemoveAll(dir)
}

// tryCleanupDirectory is a very conservative function that attempts
// to cleanup the working directory when the agent starts. Without
// this function, if an agent attempts to start on a system where a
// previous agent has run but crashed for some reason, the new agent
// could easily run out of disk space and will have no way to clean up
// for itself, which leads to an increased maintenance burden for
// static, and other long running hosts.
//
// By conservative, the operation does not return an error or attempt
// to retry in the case of an error, so running this function does not
// ensure that any files are necessarily removed, but the hope is that
// its better than not doing anything.
//
// Additionally the function does *not* handle log rotation or
// management, and only attempts to clean up the agent's working
// directory, so files not located in a directory may still cause
// issues.
func (a *Agent) tryCleanupDirectory(ctx context.Context, dir string) {
	defer recovery.LogStackTraceAndContinue("clean up directories")

	if dir == "" {
		return
	}

	stat, err := os.Stat(dir)
	if os.IsNotExist(err) {
		return
	}

	if !stat.IsDir() {
		return
	}

	// Don't run in a development environment
	if _, err = os.Stat(filepath.Join(dir, ".git")); !os.IsNotExist(err) {
		grip.Notice("Refusing to clean a directory that contains '.git'.")
		return
	}

	usr, err := user.Current()
	if err != nil {
		grip.Warning(errors.Wrap(err, "getting current user"))
		return
	}

	if strings.HasPrefix(dir, usr.HomeDir) || strings.Contains(dir, "cygwin") {
		grip.Notice("Not cleaning up directory, because it is in the home directory.")
		return
	}

	paths := []string{}

	err = filepath.WalkDir(dir, func(path string, di fs.DirEntry, err error) error {
		if err != nil {
			return err
		}

		if path == dir {
			return nil
		}

		if strings.HasPrefix(di.Name(), ".") {
			return nil
		}

		if di.IsDir() {
			paths = append(paths, path)
		}

		return nil
	})

	if err != nil {
		return
	}

	grip.Infof("Attempting to clean up directory '%s'.", dir)
	for _, p := range paths {
		if err = a.removeAllAndCheck(ctx, p); err != nil {
			grip.Critical(errors.Wrapf(err, "removing path '%s'", p))
		}
	}
}

func (a *Agent) checkDataDirectoryHealth(ctx context.Context) error {
	if a.numTaskDirCleanupFailures >= globals.MaxTaskDirCleanupFailures {
		err := a.comm.DisableHost(ctx, a.opts.HostID, apimodels.DisableInfo{
			Reason: fmt.Sprintf("agent has tried and failed to clean up task directories %d times", a.numTaskDirCleanupFailures),
		})
		if err != nil {
			return err
		}
	}

	usage, err := disk.UsageWithContext(ctx, a.opts.WorkingDirectory)
	if err != nil {
		return errors.Wrap(err, "getting disk usage")
	}
	if usage.UsedPercent > globals.MaxPercentageDataVolumeUsage {
		err := a.comm.DisableHost(ctx, a.opts.HostID, apimodels.DisableInfo{
			Reason: fmt.Sprintf("data directory usage (%f%%) is too high to run a new task", usage.UsedPercent),
		})
		if err != nil {
			return err
		}
	}

	return nil
}

// SetHomeDirectory sets the agent's home directory to the user's home directory
// if it is not already set.
func (a *Agent) SetHomeDirectory() {
	if a.opts.HomeDirectory != "" {
		return
	}

	homeDir, err := os.UserHomeDir()
	if err != nil {
		grip.Warning(errors.Wrap(err, "getting home directory"))
		return
	}
	a.opts.HomeDirectory = homeDir
}
