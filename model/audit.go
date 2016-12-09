package model

import (
	"fmt"

	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
)

// HostTaskInconsistency represents a mismatch between task and host documents.
// It contains both the host and task's view of their relationship.
// Implements the Error interface, which returns a full string describing
// the nature of the mismatch.
type HostTaskInconsistency struct {
	Host          string
	HostTaskCache string
	Task          string
	TaskHostCache string
}

// Error returns a human-readible explanation of a HostTaskInconsistency.
func (i HostTaskInconsistency) Error() string {
	switch {
	case i.Task == "" && i.TaskHostCache == "":
		return fmt.Sprintf("host %s says it is running task %s, which does not exist",
			i.Host, i.HostTaskCache)
	case i.Host == "" && i.HostTaskCache == "":
		return fmt.Sprintf("task %s says it is running on host %s, which does not exist",
			i.Task, i.TaskHostCache)
	case i.HostTaskCache == i.Task:
		return fmt.Sprintf(
			"host %s says it is running task %s, but that task says it is assigned to %s",
			i.Host, i.Task, i.TaskHostCache)
	case i.TaskHostCache == i.Host:
		return fmt.Sprintf(
			"task %s says it is running on host %s, but that host says it is running %s",
			i.Task, i.Host, i.HostTaskCache)
	default:
		// this should never be hit
		return fmt.Sprintf("inconsistent mapping: %s/%s, %s/%s",
			i.Host, i.HostTaskCache, i.Task, i.TaskHostCache)
	}
}

// AuditHostTaskConsistency finds all running tasks and running hosts and compares
// their caches of what host/task they are assigned to. Returns a slice of any mappings
// that are not 1:1 and any errors that occur.
//
// NOTE: the error returned ONLY represents issues communicating with the database.
// HostTaskInconsistency implements the error interface, but it is up to the caller
// to cast the inconsistencies into an error type if they desire.
func AuditHostTaskConsistency() ([]HostTaskInconsistency, error) {
	hostToTask, taskToHost, err := loadHostTaskMapping()
	if err != nil {
		return nil, err
	}
	return auditHostTaskMapping(hostToTask, taskToHost), nil
}

// loadHostTaskMapping queries the DB for hosts with tasks, the tasks assigned in the hosts'
// running task fields, all running (or dispatched) tasks, and the hosts in those tasks'
// host id field. Returns a mapping of host Ids to task Ids and task Ids to host Ids,
// representing both directions of the relationship.
func loadHostTaskMapping() (map[string]string, map[string]string, error) {
	hostToTask := map[string]string{}
	hostTaskIds := []string{}
	taskToHost := map[string]string{}
	taskHostIds := []string{}

	// fetch all hosts with running tasks and then all of the tasks the hosts
	// say they are running.
	runningHosts, err := host.Find(host.IsRunningTask)
	if err != nil {
		return nil, nil, fmt.Errorf("querying for running hosts: %v", err)
	}
	for _, h := range runningHosts {
		hostTaskIds = append(hostTaskIds, h.RunningTask)
	}
	hostsTasks, err := task.Find(task.ByIds(hostTaskIds))
	if err != nil {
		return nil, nil, fmt.Errorf("querying for hosts' tasks: %v", err)
	}

	// fetch all tasks with an assigned host and the hosts they say
	// they are assigned to
	runningTasks, err := task.Find(task.IsDispatchedOrStarted)
	if err != nil {
		return nil, nil, fmt.Errorf("querying for running tasks: %v", err)
	}
	for _, t := range append(hostsTasks, runningTasks...) {
		taskToHost[t.Id] = t.HostId
		taskHostIds = append(taskHostIds, t.HostId)
	}
	tasksHosts, err := host.Find(host.ByIds(taskHostIds))
	if err != nil {
		return nil, nil, fmt.Errorf("querying for tasks' hosts: %v", err)
	}
	for _, h := range append(runningHosts, tasksHosts...) {
		hostToTask[h.Id] = h.RunningTask
	}

	return hostToTask, taskToHost, nil
}

// auditHostMapping takes a mapping of hosts->tasks and tasks->hosts and
// returns descriptions of any inconsistencies.
func auditHostTaskMapping(hostToTask, taskToHost map[string]string) []HostTaskInconsistency {
	found := []HostTaskInconsistency{}
	// cases where a host thinks its running a task that it isn't
	for h, t := range hostToTask {
		cachedTask, ok := taskToHost[t]
		if !ok {
			// host thinks it is running a task that does not exist
			found = append(found, HostTaskInconsistency{
				Host:          h,
				HostTaskCache: t,
			})
		} else {
			if cachedTask != h {
				found = append(found, HostTaskInconsistency{
					Host:          h,
					HostTaskCache: t,
					Task:          t,
					TaskHostCache: cachedTask,
				})
			}
		}
	}
	// cases where a task thinks it is running on a host that isnt running it
	for t, h := range taskToHost {
		cachedHost, ok := hostToTask[h]
		if !ok {
			// task thinks it is running on a host that does not exist
			found = append(found, HostTaskInconsistency{
				Task:          t,
				TaskHostCache: h,
			})
		} else {
			if cachedHost != t {
				found = append(found, HostTaskInconsistency{
					Task:          t,
					TaskHostCache: h,
					Host:          h,
					HostTaskCache: cachedHost,
				})
			}
		}
	}
	return found
}
