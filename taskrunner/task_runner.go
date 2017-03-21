package taskrunner

import (
	"fmt"
	"path/filepath"
	"sync"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
)

const distroSleep = 10 * time.Second

type TaskRunner struct {
	*evergreen.Settings
	HostFinder
	TaskQueueFinder
	HostGateway
}

var (
	AgentPackageDirectorySubPath = filepath.Join("agent", "main")
)

func NewTaskRunner(settings *evergreen.Settings) *TaskRunner {
	// get mci home, and set the source and destination for the agent
	// executables
	evgHome := evergreen.FindEvergreenHome()

	return &TaskRunner{
		settings,
		&DBHostFinder{},
		&DBTaskQueueFinder{},
		&AgentHostGateway{
			ExecutablesDir: filepath.Join(evgHome, settings.AgentExecutablesDir),
		},
	}
}

// Runs the sequence of events that kicks off tasks on hosts.  Works by
// finding any hosts available to have a task run on them, and then figuring
// out the next appropriate task for each of the hosts and kicking them off.
// Returns an error if any error is thrown along the way.
func (self *TaskRunner) Run() error {
	grip.Info("Finding hosts available to take a task...")

	// find all hosts available to take a task
	availableHosts, err := self.FindAvailableHosts()
	if err != nil {
		return fmt.Errorf("error finding available hosts: %v", err)
	}
	grip.Infof("Found %d host(s) available to take a task", len(availableHosts))
	hostsByDistro := self.splitHostsByDistro(availableHosts)

	// assign the free hosts for each distro to the tasks they need to run
	for distroId := range hostsByDistro {
		distroStartTime := time.Now()
		if err := self.processDistro(distroId); err != nil {
			return err
		}

		distroRunTime := time.Now().Sub(distroStartTime)
		grip.Infof("It took %s to schedule tasks on %s", distroRunTime, distroId)
		// most of the time, processDistro should take less than distroSleep seconds, but
		// if it takes longer, sleep longer to give other locks a chance to preempt this one
		if distroRunTime > distroSleep {
			grip.Info("Sleeping to give other locks a chance")
			time.Sleep(distroSleep)
		}
		// always sleep one second
		time.Sleep(time.Second)
	}
	grip.Info("Finished kicking off all pending tasks")
	return nil
}

// processDistro copies and starts remote agents for the given distro.
// This function takes a global lock. Returns any errors that occur.
func (self *TaskRunner) processDistro(distroId string) error {
	lockKey := fmt.Sprintf("%v.%v", RunnerName, distroId)

	lockStartTime := time.Now()
	lockAcquired, err := db.WaitTillAcquireGlobalLock(lockKey, db.LockTimeout)
	if err != nil {
		err = fmt.Errorf("error acquiring global lock for %v: %+v", lockKey, err)
		grip.Error(err)
		return err
	}
	if !lockAcquired {
		err = fmt.Errorf("timed out acquiring global lock for %v", lockKey)
		grip.Error(err)
		return err
	}
	defer func() {
		if err = db.ReleaseGlobalLock(lockKey); err != nil {
			grip.Errorf("error releasing global lock for %s: %v", lockKey, err)
		}
	}()

	lockRunTime := time.Now().Sub(lockStartTime)
	grip.Infof("Taskrunner took %s to acquire global lock", lockRunTime)

	freeHostsForDistro, err := self.FindAvailableHostsForDistro(distroId)
	if err != nil {
		return fmt.Errorf("loading available %v hosts: %v", distroId, err)
	}

	grip.Infof("Found %d %s host(s) available for tasks", len(freeHostsForDistro), distroId)
	grip.Infof("Kicking off tasks on distro %s...", distroId)

	taskQueue, err := self.FindTaskQueue(distroId)
	if err != nil {
		return fmt.Errorf("error finding task queue for distro %v: %v",
			distroId, err)
	}
	if taskQueue == nil {
		grip.Infoln("nil task queue found for distro:", distroId)
		return nil // nothing to do
	}

	// while there are both free hosts and pending tasks left, pin tasks to hosts
	waitGroup := &sync.WaitGroup{}
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

		// use the embedded host gateway to kick the task off
		waitGroup.Add(1)
		go func(t task.Task) {
			defer waitGroup.Done()
			agentRevision, err := self.RunTaskOnHost(self.Settings,
				t, nextHost)
			if err != nil {
				grip.Errorf("error kicking off task %s on host %s: %v",
					t.Id, nextHost.Id, err)

				err = model.MarkTaskUndispatched(nextTask)

				grip.ErrorWhenf(err != nil, "error marking task %v as undispatched on host "+
					"%v: %v", t.Id, nextHost.Id, err)
				return
			}

			grip.Infof("Task %v successfully kicked off on host %v", t.Id, nextHost.Id)

			// now update the host's running task/agent revision fields
			// accordingly
			err = nextHost.SetRunningTask(t.Id, agentRevision, time.Now())
			grip.ErrorWhenf(err != nil, "error updating running task %s on host %s: %+v",
				t.Id, nextHost.Id, err)
		}(*nextTask)
	}
	waitGroup.Wait()
	return nil
}

// DispatchTaskForHost assigns the task at the head of the task queue to the
// given host, dequeues the task and then marks it as dispatched for the host
func DispatchTaskForHost(taskQueue *model.TaskQueue, assignedHost *host.Host) (
	nextTask *task.Task, err error) {
	if assignedHost == nil {
		return nil, fmt.Errorf("can not assign task to a nil host")
	}

	// only proceed if there are pending tasks left
	for !taskQueue.IsEmpty() {
		queueItem := taskQueue.NextTask()
		// pin the task to the given host and fetch the full task document from
		// the database
		nextTask, err = task.FindOne(task.ById(queueItem.Id))
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
			grip.Warningf("Skipping task %s, which was "+
				"picked up to be run but is not runnable - "+
				"status (%s) activated (%t)", nextTask.Id, nextTask.Status,
				nextTask.Activated)
			continue
		}

		// record that the task was dispatched on the host
		if err := model.MarkTaskDispatched(nextTask, assignedHost.Id, assignedHost.Distro.Id); err != nil {
			return nil, err
		}
		return nextTask, nil
	}
	return nil, nil
}

// Determines whether or not a task should be skipped over by the
// task runner. Checks if the task is not undispatched, as a sanity check that
// it is not already running.
func shouldSkipTask(task *task.Task) bool {
	return task.Status != evergreen.TaskUndispatched || !task.Activated
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
