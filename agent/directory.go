package agent

import (
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"os/user"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/agent/globals"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/utility"
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
func (a *Agent) createTaskDirectory(ctx context.Context, tc *taskContext, taskDir string) (string, error) {
	if taskDir == "" {
		var err error
		taskDir, err = a.generateTaskDirectoryName(tc)
		if err != nil {
			tc.logger.Execution().Error(ctx, errors.Wrap(err, "generating task directory name"))
			return "", err
		}
	}

	tc.logger.Execution().Infof(ctx, "Making task directory '%s' for task execution.", taskDir)

	if err := os.MkdirAll(taskDir, 0777); err != nil {
		tc.logger.Execution().Error(ctx, errors.Wrapf(err, "creating task directory '%s'", taskDir))
		return "", err
	}

	tmpDir := filepath.Join(taskDir, "tmp")
	tc.logger.Execution().Infof(ctx, "Making task temporary directory '%s' for task execution.", tmpDir)

	if err := os.MkdirAll(tmpDir, 0777); err != nil {
		tc.logger.Execution().Warning(ctx, errors.Wrapf(err, "creating task temporary directory '%s'", tmpDir))
	}

	return taskDir, nil
}

// generateTaskDirectoryName generates a task directory path. Typically, this is
// in the format: <agent_working_directory>/<hash>.
//
// On Windows hosts, the hash portion is shorter than on other distros because
// it has a max file path length. Having a long task directory's path can cause
// issues for some tasks that have deep paths that hit the Windows path length
// limit. Shortening the typical 32-character directory name reduces the
// problem.
func (a *Agent) generateTaskDirectoryName(tc *taskContext) (string, error) {
	dirName, err := a.generateTaskDirectoryHash(fmt.Sprintf("%s_%d_%d", tc.taskConfig.Task.Id, tc.taskConfig.Task.Execution, os.Getpid()))
	if err != nil {
		return "", errors.Wrap(err, "generating hash for randomized task directory")
	}

	taskDirPath := filepath.Join(a.opts.WorkingDirectory, dirName)
	if runtime.GOOS != "windows" {
		return taskDirPath, nil
	}

	// On Windows, the task directory path's hash is shortened due to max path
	// length limits. Reducing the length of the hash also creates a new
	// potential issue by reducing the randomness of the task directory path. If
	// the agent fails to clean up task directories (ideally shouldn't happen,
	// but this does unfortunately happen sometimes in practice), then the agent
	// runs the risk of reusing a directory from a previous unrelated task. To
	// guard against this, check if the directory already exists.
	if !utility.FileExists(taskDirPath) {
		return taskDirPath, nil
	}

	// If the initially proposed shortened task directory already exists, try to
	// generate a new task directory. Practically, it's unlikely that it would
	// generate a colliding hash 10 times, this just puts a reasonable bound on
	// the maximum number of attempts.
	const maxAttempts = 10
	for i := 0; i < maxAttempts; i++ {
		dirName, err = a.generateTaskDirectoryHash(dirName)
		if err != nil {
			return "", errors.Wrapf(err, "writing hash for randomized task directory (attempt %d/%d)", i+1, maxAttempts)
		}

		taskDirPath := filepath.Join(a.opts.WorkingDirectory, dirName)
		if !utility.FileExists(taskDirPath) {
			return taskDirPath, nil
		}
	}

	return "", errors.Errorf("failed to generate a unique task directory after %d attempts", maxAttempts)
}

// generateTaskDirectoryHash generates a hashed string for a task directory.
func (a *Agent) generateTaskDirectoryHash(toHash string) (string, error) {
	h := md5.New()
	if _, err := h.Write([]byte(toHash)); err != nil {
		return "", errors.Wrap(err, "writing task directory hash")
	}

	md5Sum := h.Sum(nil)
	dirName := hex.EncodeToString(md5Sum)

	if runtime.GOOS != "windows" {
		return dirName, nil
	}

	// On Windows, the agent has to use a shorter task working directory due to
	// max file path length limitations. To accommodate this, try shortening the
	// long hash.
	const maxWindowsHashLength = 4
	return dirName[:maxWindowsHashLength], nil
}

// removeTaskDirectory removes the folder the agent created for the task it was
// executing. It does not return an error because it is executed at the end of a
// task run, and the agent loop will start another task regardless of how this
// exits. If it cannot remove the task directory, the agent may disable the host
// because leaving the task directory behind could impact later tasks.
func (a *Agent) removeTaskDirectory(ctx context.Context, tc *taskContext) {
	if tc.taskConfig == nil || tc.taskConfig.WorkDir == "" {
		grip.Info(ctx, "Task directory is not set, not removing.")
		return
	}

	dir := tc.taskConfig.WorkDir

	grip.Infof(ctx, "Deleting task directory '%s' for completed task.", dir)

	// Removing long relative paths hangs on Windows https://github.com/golang/go/issues/36375,
	// so we have to convert to an absolute path before removing.
	abs, err := filepath.Abs(dir)
	if err != nil {
		grip.Critical(ctx, errors.Wrapf(err, "getting absolute path for task directory '%s'", dir))
		return
	}
	if err := a.removeAllAndCheck(ctx, abs); err != nil {
		grip.Critical(ctx, errors.Wrapf(err, "removing task directory '%s'", dir))
	} else {
		grip.Info(ctx, message.Fields{
			"message":   "Successfully deleted directory for completed task.",
			"directory": tc.taskConfig.WorkDir,
		})
	}
}

const maxRemovalAttempts = 5

// removeAllAndCheck removes the directory and checks the data directory
// usage afterwards. If the data directory is unhealthy, the host is disabled.
func (a *Agent) removeAllAndCheck(ctx context.Context, dir string) error {
	removeErr := a.removeAll(ctx, dir)
	if removeErr == nil {
		return nil
	}

	checkCtx, checkCancel := context.WithTimeout(ctx, time.Minute)
	defer checkCancel()
	if err := a.checkDataDirectoryHealth(checkCtx); err != nil {
		grip.Error(ctx, message.WrapError(err, message.Fields{
			"message":               "failed to check data directory usage",
			"unremovable_directory": dir,
		}))
	}

	return removeErr
}

// removeAll is the same as os.RemoveAll, but recursively changes permissions
// for subdirectories and contents before removing. The permissions change fixes
// an issue where some files may be marked read-only, which prevents
// os.RemoveAll from deleting them. Removal is retried to handle background
// processes that may still be writing to the directory. If all retries are
// exhausted, it falls back to privileged removal to handle root-owned files
// left behind by containerized workloads.
func (a *Agent) removeAll(ctx context.Context, dir string) error {
	grip.Error(ctx, errors.Wrapf(filepath.WalkDir(dir, func(path string, _ fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		grip.Error(ctx, errors.Wrapf(os.Chmod(path, 0777), "changing permission before removal for path '%s'", path))
		return nil
	}), "recursively walking through path to change permissions"))

	err := utility.Retry(ctx, func() (bool, error) {
		if err := os.RemoveAll(dir); err != nil {
			grip.Warning(ctx, message.WrapError(err, message.Fields{
				"message":   "failed to remove task directory, will retry",
				"directory": dir,
			}))
			return true, err
		}
		return false, nil
	}, utility.RetryOptions{
		MaxAttempts: maxRemovalAttempts,
		MinDelay:    time.Second,
	})
	if err == nil {
		return nil
	}

	mounts := activeMountsUnder(ctx, dir)
	logRemovalFailureDiagnostics(ctx, dir, mounts)

	// Bind mounts left behind by containers prevent removal even with elevated
	// privileges, so unmount before attempting privileged removal.
	if len(mounts) > 0 {
		grip.Warningf(ctx, "Detected %d active mount(s) under task directory '%s', attempting to unmount before removal.", len(mounts), dir)
		if err := exec.Command("sudo", "umount", "-R", dir).Run(); err != nil {
			grip.Warning(ctx, message.WrapError(err, message.Fields{
				"message":   "failed to unmount task directory, will still attempt privileged removal",
				"directory": dir,
			}))
		}
	}

	// Fall back to privileged removal for files that the agent user cannot
	// remove (e.g., root-owned files written by containers into the task
	// directory via bind mounts).
	grip.Warningf(ctx, "Standard removal of task directory '%s' failed, attempting privileged removal.", dir)
	return errors.Wrapf(exec.Command("sudo", "rm", "-rf", dir).Run(), "privileged removal of task directory '%s' failed", dir)
}

// logRemovalFailureDiagnostics logs remaining file metadata and active mounts
// when removal fails, to identify whether containerized workloads left behind
// bind mounts or root-owned files that prevented cleanup.
func logRemovalFailureDiagnostics(ctx context.Context, dir string, mounts []string) {
	type fileInfo struct {
		Path  string `json:"path"`
		Mode  string `json:"mode"`
		IsDir bool   `json:"is_dir"`
		Size  int64  `json:"size"`
	}

	var remaining []fileInfo
	_ = filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			remaining = append(remaining, fileInfo{Path: path, IsDir: d.IsDir()})
			return nil
		}
		remaining = append(remaining, fileInfo{
			Path:  path,
			Mode:  info.Mode().String(),
			IsDir: d.IsDir(),
			Size:  info.Size(),
		})
		return nil
	})

	grip.Warning(ctx, message.Fields{
		"message":         "files remaining in task directory after failed removal",
		"directory":       dir,
		"remaining_count": len(remaining),
		"active_mounts":   mounts,
		"files":           remaining,
	})
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
		grip.Notice(ctx, "Refusing to clean a directory that contains '.git'.")
		return
	}

	usr, err := user.Current()
	if err != nil {
		grip.Warning(ctx, errors.Wrap(err, "getting current user"))
		return
	}

	if strings.HasPrefix(dir, usr.HomeDir) || strings.Contains(dir, "cygwin") {
		grip.Notice(ctx, "Not cleaning up directory, because it is in the home directory.")
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

	grip.Infof(ctx, "Attempting to clean up directory '%s'.", dir)
	for _, p := range paths {
		if err = a.removeAllAndCheck(ctx, p); err != nil {
			grip.Critical(ctx, errors.Wrapf(err, "removing path '%s'", p))
		}
	}
}

func (a *Agent) checkDataDirectoryHealth(ctx context.Context) error {
	usage, err := disk.UsageWithContext(ctx, a.opts.WorkingDirectory)
	if err != nil {
		return errors.Wrap(err, "getting disk usage")
	}
	return a.checkDataDirectoryHealthWithUsage(ctx, usage)
}

func (a *Agent) checkDataDirectoryHealthWithUsage(ctx context.Context, usage *disk.UsageStat) error {
	maxPercentageDataVolumeUsage := globals.MaxPercentageDataVolumeUsageDefault
	if runtime.GOOS == "darwin" {
		maxPercentageDataVolumeUsage = globals.MaxPercentageDataVolumeUsageDarwin
	}

	if usage.UsedPercent > float64(maxPercentageDataVolumeUsage) {
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
func (a *Agent) SetHomeDirectory(ctx context.Context) {
	if a.opts.HomeDirectory != "" {
		return
	}

	userHome, err := util.GetUserHome()
	if err != nil {
		grip.Warning(ctx, errors.Wrap(err, "setting the agent's home directory"))
	}

	a.opts.HomeDirectory = userHome
}
