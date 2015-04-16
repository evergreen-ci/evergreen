package taskrunner

import (
	"10gen.com/mci"
	"10gen.com/mci/model"
	"10gen.com/mci/model/host"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"path/filepath"
	"sync"
	"time"
)

type TaskRunner struct {
	*mci.MCISettings
	HostFinder
	TaskQueueFinder
	HostGateway
}

var (
	AgentPackageDirectorySubPath = filepath.Join("agent", "main")
)

func NewTaskRunner(mciSettings *mci.MCISettings) *TaskRunner {
	// get mci home, and set the source and destination for the agent
	// executables
	mciHome, err := mci.FindMCIHome()
	if err != nil {
		panic(fmt.Sprintf("error finding mci home: %v", err))
	}

	return &TaskRunner{
		mciSettings,
		&DBHostFinder{},
		&DBTaskQueueFinder{},
		&AgentBasedHostGateway{
			Compiler: &GoxcAgentCompiler{
				mciSettings,
			},
			ExecutablesDir:  filepath.Join(mciHome, mciSettings.AgentExecutablesDir),
			AgentPackageDir: filepath.Join(mciHome, AgentPackageDirectorySubPath),
		},
	}
}

// Runs the sequence of events that kicks off tasks on hosts.  Works by
// finding any hosts available to have a task run on them, and then figuring
// out the next appropriate task for each of the hosts and kicking them off.
// Returns an error if any error is thrown along the way.
func (self *TaskRunner) RunTasks() error {

	mci.Logger.Logf(slogger.INFO, "Finding hosts available to take a task...")
	// find all hosts available to take a task
	availableHosts, err := self.FindAvailableHosts()
	if err != nil {
		return fmt.Errorf("error finding available hosts: %v", err)
	}
	mci.Logger.Logf(slogger.INFO, "Found %v host(s) available to take a task",
		len(availableHosts))

	// split the hosts by distro
	hostsByDistro := self.splitHostsByDistro(availableHosts)

	// we'll need this to wait for all the setups to finish
	waitGroup := &sync.WaitGroup{}

	// assign the free hosts for each distro to the tasks they need to run
	for distroId, freeHostsForDistro := range hostsByDistro {
		mci.Logger.Logf(slogger.INFO, "Kicking off tasks on distro %v...",
			distroId)

		// load in the queue of tasks for the distro
		taskQueue, err := self.FindTaskQueue(distroId)
		if err != nil {
			return fmt.Errorf("error finding task queue for distro %v: %v",
				distroId, err)
		}

		if taskQueue == nil {
			mci.Logger.Logf(slogger.ERROR, "nil task queue found for distro '%v'", distroId)
			continue
		}

		// while there are both free hosts and pending tasks left, pin
		// tasks to hosts
		for !taskQueue.IsEmpty() && len(freeHostsForDistro) > 0 {
			nextHost := freeHostsForDistro[0]
			nextTask, err := DispatchTaskForHost(taskQueue, &nextHost)
			if err != nil {
				return err
			}

			// can only get here if the queue is empty
			if nextTask == nil {
				continue
			}

			// once allocated to a task, pop the host off the distro's free host
			// list
			freeHostsForDistro = freeHostsForDistro[1:]

			// dereference the task before running the goroutine
			dereferencedTask := *nextTask

			// use the embedded host gateway to kick the task off
			waitGroup.Add(1)
			go func() {
				defer waitGroup.Done()
				agentRevision, err := self.RunTaskOnHost(self.MCISettings,
					dereferencedTask, nextHost)
				if err != nil {
					mci.Logger.Logf(slogger.ERROR, "error kicking off task %v"+
						" on host %v: %v", dereferencedTask.Id, nextHost.Id, err)
				} else {
					mci.Logger.Logf(slogger.INFO, "task %v successfully kicked"+
						" off on host %v", dereferencedTask.Id, nextHost.Id)
				}

				// now update the host's running task/agent revision fields
				// accordingly
				err = nextHost.SetRunningTask(dereferencedTask.Id,
					agentRevision, time.Now())
				if err != nil {
					mci.Logger.Errorf(slogger.ERROR, "Error updating running "+
						"task %v on host %v: %v", dereferencedTask.Id,
						nextHost.Id, err)
				}
			}()
		}
	}

	// wait for everything to finish
	waitGroup.Wait()

	mci.Logger.Logf(slogger.INFO, "Finished kicking off all pending tasks")

	return nil
}

// DispatchTaskForHost assigns the task at the head of the task queue to the
// given host, dequeues the task and then marks it as dispatched for the host
func DispatchTaskForHost(taskQueue *model.TaskQueue, assignedHost *host.Host) (
	nextTask *model.Task, err error) {
	if assignedHost == nil {
		return nil, fmt.Errorf("can not assign task to a nil host")
	}

	// only proceed if there are pending tasks left
	for !taskQueue.IsEmpty() {
		queueItem := taskQueue.NextTask()
		// pin the task to the given host and fetch the full task document from
		// the database
		nextTask, err = model.FindTask(queueItem.Id)
		if err != nil {
			return nil, fmt.Errorf("error finding task with id %v: %v",
				queueItem.Id, err)
		}
		if nextTask == nil {
			return nil, fmt.Errorf("refusing to move forward because queued "+
				"task with id %v does not exist", queueItem.Id)
		}

		// dequeue the task from the queue
		if err = taskQueue.DequeueTask(nextTask.Id); err != nil {
			return nil, fmt.Errorf("error pulling task with id %v from "+
				"queue for distro %v: %v", nextTask.Id,
				nextTask.DistroId, err)
		}

		// validate that the task can be run, if not fetch the next one in
		// the queue
		if shouldSkipTask(nextTask) {
			mci.Logger.Logf(slogger.WARN, "Skipping task %v, which was "+
				"picked up to be run but is not runnable - "+
				"status (%v) activated (%v)", nextTask.Id, nextTask.Status,
				nextTask.Activated)
			continue
		}

		// record that the task was dispatched on the host
		err = nextTask.MarkAsDispatched(assignedHost, time.Now())
		if err != nil {
			return nil, fmt.Errorf("error marking task %v as dispatched "+
				"on host %v: %v", nextTask.Id, assignedHost.Id, err)
		}

		return nextTask, nil
	}
	return nil, nil
}

// Determines whether or not a task should be skipped over by the
// task runner. Checks if the task is not undispatched, as a sanity check that
// it is not already running.
func shouldSkipTask(task *model.Task) bool {
	return task.Status != mci.TaskUndispatched || !task.Activated
}

// Takes in a list of hosts, and returns the hosts sorted by distro, in the
// form of a map distro name -> list of hosts
func (self *TaskRunner) splitHostsByDistro(hostsToSplit []host.Host) map[string][]host.Host {
	hostsByDistro := make(map[string][]host.Host)
	for _, host := range hostsToSplit {
		hostsByDistro[host.Distro.Id] = append(hostsByDistro[host.Distro.Id], host)
	}
	return hostsByDistro
}
