package proto

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
)

// createTaskDirectory makes a directory for the agent to execute
// the current task within. It changes the necessary variables
// so that all of the agent's operations will use this folder.
func (a *Agent) createTaskDirectory(tc taskContext, taskConfig *model.TaskConfig) error {
	h := md5.New()

	_, err := h.Write([]byte(
		fmt.Sprintf("%s_%d_%d", taskConfig.Task.Id, taskConfig.Task.Execution, os.Getpid())))
	if err != nil {
		tc.logger.Execution().Errorf("Error creating task directory name: %v", err)
		return err
	}

	dirName := hex.EncodeToString(h.Sum(nil))
	newDir := filepath.Join(taskConfig.Distro.WorkDir, dirName)

	tc.logger.Execution().Infof("Making new folder for task execution: %v", newDir)
	err = os.Mkdir(newDir, 0777)
	if err != nil {
		tc.logger.Execution().Errorf("Error creating task directory: %v", err)
		return err
	}

	tc.logger.Execution().Infof("Changing into task directory: %v", newDir)
	err = os.Chdir(newDir)
	if err != nil {
		tc.logger.Execution().Errorf("Error changing into task directory: %v", err)
		return err
	}
	tc.taskDirectory = newDir
	return nil
}

// removeTaskDirectory removes the folder the agent created for the
// task it was executing.
func (a *Agent) removeTaskDirectory(tc taskContext) error {
	if tc.taskDirectory == "" {
		err := errors.New("Task directory is not set")
		tc.logger.Execution().Error(err)
		return err
	}
	tc.logger.Execution().Info("Changing directory back to distro working directory.")
	if err := os.Chdir(tc.taskConfig.Distro.WorkDir); err != nil {
		tc.logger.Execution().Errorf("Error changing directory out of task directory: %v", err)
		return err
	}

	tc.logger.Execution().Info("Deleting directory for completed task.")

	if err := os.RemoveAll(tc.taskDirectory); err != nil {
		tc.logger.Execution().Errorf("Error removing working directory for the task: %v", err)
		return err
	}
	return nil
}
