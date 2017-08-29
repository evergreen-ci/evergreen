package proto

import (
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/mongodb/grip"
)

// createTaskDirectory makes a directory for the agent to execute
// the current task within. It changes the necessary variables
// so that all of the agent's operations will use this folder.
func (a *Agent) createTaskDirectory(tc *taskContext, taskConfig *model.TaskConfig) (string, error) {
	h := md5.New()

	_, err := h.Write([]byte(
		fmt.Sprintf("%s_%d_%d", taskConfig.Task.Id, taskConfig.Task.Execution, os.Getpid())))
	if err != nil {
		tc.logger.Execution().Errorf("Error creating task directory name: %v", err)
		return "", err
	}

	dirName := hex.EncodeToString(h.Sum(nil))
	newDir := filepath.Join(taskConfig.Distro.WorkDir, dirName)

	tc.logger.Execution().Infof("Making new folder for task execution: %v", newDir)
	err = os.Mkdir(newDir, 0777)
	if err != nil {
		tc.logger.Execution().Errorf("Error creating task directory: %v", err)
		return "", err
	}

	taskConfig.WorkDir = newDir
	return newDir, nil
}

// removeTaskDirectory removes the folder the agent created for the task it
// was executing. It does not return an error because it is executed at the end of
// a task run, and the agent loop will start another task regardless of how this
// exits.
func (a *Agent) removeTaskDirectory(tc *taskContext) {
	if tc.taskDirectory == "" {
		grip.Critical("Task directory is not set")
	}
	if tc.taskConfig == nil {
		grip.Critical("No taskConfig in taskContext")
		return
	}
	grip.Info("Deleting directory for completed task.")

	if err := os.RemoveAll(tc.taskDirectory); err != nil {
		grip.Criticalf("Error removing working directory for the task: %v", err)
	}
}
