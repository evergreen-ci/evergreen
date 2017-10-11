package monitor

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// responsible for cleaning up any tasks that need to be stopped
type TaskMonitor struct {
	// will be used for flagging tasks that need to be cleaned up
	flaggingFuncs []taskFlaggingFunc
}

// run through the list of task flagging functions, finding all tasks that
// need to be cleaned up and taking appropriate action. takes in a map
// of project name -> project info
func (tm *TaskMonitor) CleanupTasks(ctx context.Context, projects map[string]model.Project) []error {
	grip.Info("Cleaning up tasks...")

	// used to store any errors that occur
	var errs []error

	for _, f := range tm.flaggingFuncs {
		if ctx.Err() != nil {
			return append(errs, errors.New("task monitor canceled"))
		}

		// find the next batch of tasks to be cleaned up
		tasksToCleanUp, err := f()

		// continue on error so that one wonky flagging function doesn't
		// stop others from working
		if err != nil {
			errs = append(errs, errors.Wrap(err, "error finding tasks to be cleaned up"))
			continue
		}

		// clean up all of the tasks. continue on error to allow further cleanup
		// to progress
		for _, err := range cleanUpTasks(tasksToCleanUp, projects) {
			errs = append(errs, errors.Wrap(err, "error cleaning up tasks"))
		}
	}

	grip.Info("Done cleaning up tasks")

	return errs
}

// clean up the passed-in slice of tasks
func cleanUpTasks(taskWrappers []doomedTaskWrapper, projects map[string]model.Project) []error {
	grip.Infof("Cleaning up %d tasks...", len(taskWrappers))

	// used to store any errors that occur
	var errs []error

	for _, wrapper := range taskWrappers {
		grip.Infof("Cleaning up task %s, for reason '%s'", wrapper.task.Id, wrapper.reason)

		// clean up the task. continue on error to let others be cleaned up
		if err := cleanUpTask(wrapper, projects); err != nil {
			errs = append(errs, errors.Wrapf(err,
				"error cleaning up task %v", wrapper.task.Id))
			continue
		}
		grip.Infoln("Successfully cleaned up task", wrapper.task.Id)
	}

	return errs
}

// function to clean up a single task
func cleanUpTask(wrapper doomedTaskWrapper, projects map[string]model.Project) error {

	// find the appropriate project for the task
	project, ok := projects[wrapper.task.Project]
	if !ok {
		return errors.Errorf("could not find project %v for task %v",
			wrapper.task.Project, wrapper.task.Id)
	}

	// get the host for the task
	host, err := host.FindOne(host.ById(wrapper.task.HostId))
	if err != nil {
		return errors.Wrapf(err, "error finding host %s for task %s",
			wrapper.task.HostId, wrapper.task.Id)
	}

	// if there's no relevant host, something went wrong
	if host == nil {
		grip.Errorln("no entry found for host:", wrapper.task.HostId)
		return errors.WithStack(wrapper.task.MarkUnscheduled())
	}

	// if the host still has the task as its running task, clear it.
	if host.RunningTask == wrapper.task.Id {
		// clear out the host's running task
		if err = host.ClearRunningTask(wrapper.task.Id, time.Now()); err != nil {
			return errors.Wrapf(err, "error clearing running task %v from host %v: %v",
				wrapper.task.Id, host.Id)
		}
	}

	// take different action, depending on the type of task death
	switch wrapper.reason {
	case HeartbeatTimeout:
		err = cleanUpTimedOutHeartbeat(wrapper.task, project)
	default:
		return errors.Errorf("unknown reason for cleaning up task: %v", wrapper.reason)
	}

	if err != nil {
		return errors.Wrapf(err, "error cleaning up task %s", wrapper.task.Id)
	}

	return nil

}

// clean up a task whose heartbeat has timed out
func cleanUpTimedOutHeartbeat(t task.Task, project model.Project) error {
	// mock up the failure details of the task
	detail := &apimodels.TaskEndDetail{
		Description: task.AgentHeartbeat,
		TimedOut:    true,
		Status:      evergreen.TaskFailed,
	}

	// try to reset the task
	if err := model.TryResetTask(t.Id, "", RunnerName, &project, detail); err != nil {
		return errors.Wrapf(err, "error trying to reset task %s", t.Id)
	}
	// success
	return nil
}
