package monitor

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
)

// responsible for cleaning up any tasks that need to be stopped
type TaskMonitor struct {
	// will be used for flagging tasks that need to be cleaned up
	flaggingFuncs []taskFlaggingFunc
}

// run through the list of task flagging functions, finding all tasks that
// need to be cleaned up and taking appropriate action. takes in a map
// of project name -> project info
func (tm *TaskMonitor) CleanupTasks(projects map[string]model.Project) []error {
	grip.Info("Cleaning up tasks...")

	// used to store any errors that occur
	var errors []error

	for _, f := range tm.flaggingFuncs {
		// find the next batch of tasks to be cleaned up
		tasksToCleanUp, err := f()

		// continue on error so that one wonky flagging function doesn't
		// stop others from working
		if err != nil {
			errors = append(errors, fmt.Errorf("error finding tasks to be cleaned up: %v", err))
			continue
		}

		// clean up all of the tasks. continue on error to allow further cleanup
		// to progress
		if errs := cleanUpTasks(tasksToCleanUp, projects); errs != nil {
			for _, err := range errs {
				errors = append(errors, fmt.Errorf("error cleaning up tasks: %v", err))
			}
		}

	}

	grip.Info("Done cleaning up tasks")

	return errors

}

// clean up the passed-in slice of tasks
func cleanUpTasks(taskWrappers []doomedTaskWrapper, projects map[string]model.Project) []error {
	grip.Infof("Cleaning up %d tasks...", len(taskWrappers))

	// used to store any errors that occur
	var errors []error

	for _, wrapper := range taskWrappers {

		grip.Infof("Cleaning up task %s, for reason '%s'", wrapper.task.Id, wrapper.reason)

		// clean up the task. continue on error to let others be cleaned up
		err := cleanUpTask(wrapper, projects)
		if err != nil {
			errors = append(errors, fmt.Errorf("error cleaning up task %v: %v", wrapper.task.Id, err))
		} else {
			grip.Infoln("Successfully cleaned up task", wrapper.task.Id)
		}

	}

	return errors
}

// function to clean up a single task
func cleanUpTask(wrapper doomedTaskWrapper, projects map[string]model.Project) error {

	// find the appropriate project for the task
	project, ok := projects[wrapper.task.Project]
	if !ok {
		return fmt.Errorf("could not find project %v for task %v",
			wrapper.task.Project, wrapper.task.Id)
	}

	// get the host for the task
	host, err := host.FindOne(host.ById(wrapper.task.HostId))
	if err != nil {
		return fmt.Errorf("error finding host %v for task %v: %v",
			wrapper.task.HostId, wrapper.task.Id, err)
	}

	// if there's no relevant host, something went wrong
	if host == nil {
		grip.Errorln("no entry found for host:", wrapper.task.HostId)
		return wrapper.task.MarkUnscheduled()
	}

	// sanity check that the host is actually running the task
	if host.RunningTask != wrapper.task.Id {
		return fmt.Errorf("task %v says it is running on host %v, but the"+
			" host thinks it is running task %v", wrapper.task.Id, host.Id,
			host.RunningTask)
	}

	// take different action, depending on the type of task death
	switch wrapper.reason {
	case HeartbeatTimeout:
		err = cleanUpTimedOutHeartbeat(wrapper.task, project, host)
	default:
		return fmt.Errorf("unknown reason for cleaning up task: %v", wrapper.reason)
	}

	if err != nil {
		return fmt.Errorf("error cleaning up task %v: %v", wrapper.task.Id, err)
	}

	return nil

}

// clean up a task whose heartbeat has timed out
func cleanUpTimedOutHeartbeat(t task.Task, project model.Project, host *host.Host) error {
	// mock up the failure details of the task
	detail := &apimodels.TaskEndDetail{
		Description: task.AgentHeartbeat,
		TimedOut:    true,
		Status:      evergreen.TaskFailed,
	}

	// try to reset the task
	if err := model.TryResetTask(t.Id, "", RunnerName, &project, detail); err != nil {
		return fmt.Errorf("error trying to reset task %v: %v", t.Id, err)
	}

	// clear out the host's running task
	if err := host.ClearRunningTask(t.Id, time.Now()); err != nil {
		return fmt.Errorf("error clearing running task %v from host %v: %v",
			t.Id, host.Id, err)
	}

	// success
	return nil
}
