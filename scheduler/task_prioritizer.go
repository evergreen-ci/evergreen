package scheduler

import (
	"fmt"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// TaskPrioritizer is responsible for taking in a slice of tasks, and ordering them
// according to which should be run first.
type TaskPrioritizer interface {
	// Takes in a slice of tasks and the current MCI settings.
	// Returns the slice of tasks, sorted in the order in which they should
	// be run, as well as an error if appropriate.
	PrioritizeTasks(distroId string, tasks []task.Task, versions map[string]model.Version) ([]task.Task, map[string]map[string]string, error)
}

// CmpBasedTaskComparator runs the tasks through a slice of comparator functions
// determining which is more important.
type CmpBasedTaskComparator struct {
	runtimeID      string
	tasks          []task.Task
	versions       map[string]model.Version
	errsDuringSort []error
	setupFuncs     []sortSetupFunc
	comparators    []taskComparer
	orderingLogic  map[string]map[string]string
}

// CmpBasedTaskQueues represents the three types of queues that are created for merging together into one queue.
// The HighPriorityTasks list represent the tasks that are always placed at the front of the queue
// PatchTasks and RepotrackerTasks are interleaved after the high priority tasks.
type CmpBasedTaskQueues struct {
	HighPriorityTasks []task.Task
	PatchTasks        []task.Task
	RepotrackerTasks  []task.Task
}

// NewCmpBasedTaskComparator returns a new task prioritizer, using the default set of comparators
// as well as the setup functions necessary for those comparators.
func NewCmpBasedTaskComparator(id string) *CmpBasedTaskComparator {
	return &CmpBasedTaskComparator{
		runtimeID: id,
		setupFuncs: []sortSetupFunc{
			cacheExpectedDurations,
			groupTaskGroups,
		},
		comparators: []taskComparer{
			&byTaskGroupOrder{},
			&byCommitQueue{},
			&byPriority{},
			&byNumDeps{},
			&byGenerateTasks{},
			&byAge{},
			&byRuntime{},
		},
		orderingLogic: map[string]map[string]string{},
	}
}

type CmpBasedTaskPrioritizer struct {
	runtimeID string
}

// PrioritizeTask prioritizes the tasks to run. First splits the tasks into slices based on
// whether they are part of patch versions or automatically created versions.
// Then prioritizes each slice, and merges them.
// Returns a full slice of the prioritized tasks, and an error if one occurs.
func (prioritizer *CmpBasedTaskPrioritizer) PrioritizeTasks(distroId string, tasks []task.Task, versions map[string]model.Version) ([]task.Task, map[string]map[string]string, error) {
	comparator := NewCmpBasedTaskComparator(prioritizer.runtimeID)
	comparator.versions = versions
	// split the tasks into repotracker tasks and patch tasks, then prioritize
	// individually and merge
	taskQueues := comparator.splitTasksByRequester(tasks)
	prioritizedTaskLists := make([][]task.Task, 0, 3)

	var (
		startAt      time.Time
		setupRuntime time.Duration
		cmpRuntime   time.Duration
	)

	for _, taskList := range [][]task.Task{taskQueues.RepotrackerTasks, taskQueues.PatchTasks, taskQueues.HighPriorityTasks} {

		comparator.tasks = taskList

		startAt = time.Now()
		err := comparator.setupForSortingTasks()
		if err != nil {
			return nil, nil, errors.Wrap(err, "Error running setup for sorting tasks")
		}
		setupRuntime += time.Since(startAt)

		startAt = time.Now()
		sort.Stable(comparator)
		cmpRuntime += time.Since(startAt)

		if len(comparator.errsDuringSort) > 0 {
			errString := "The following errors were thrown while sorting:"
			for _, e := range comparator.errsDuringSort {
				errString += fmt.Sprintf("\n    %v", e)
			}
			return nil, nil, errors.New(errString)
		}

		prioritizedTaskLists = append(prioritizedTaskLists, comparator.tasks)
	}

	prioritizedTaskQueues := CmpBasedTaskQueues{
		RepotrackerTasks:  prioritizedTaskLists[0],
		PatchTasks:        prioritizedTaskLists[1],
		HighPriorityTasks: prioritizedTaskLists[2],
	}

	grip.Debug(message.Fields{
		"message":                 "finished prioritizing task queues",
		"instance":                prioritizer.runtimeID,
		"distro":                  distroId,
		"runner":                  RunnerName,
		"operation":               "prioritize tasks",
		"repotracker tasks":       len(prioritizedTaskQueues.RepotrackerTasks),
		"patch tasks":             len(prioritizedTaskQueues.PatchTasks),
		"high priority tasks":     len(prioritizedTaskQueues.HighPriorityTasks),
		"setup_runtime_secs":      setupRuntime.Seconds(),
		"comparison_runtime_secs": cmpRuntime.Seconds(),
	})

	comparator.tasks = comparator.mergeTasks(&prioritizedTaskQueues)

	return comparator.tasks, comparator.orderingLogic, nil
}

// Run all of the setup functions necessary for prioritizing the tasks.
// Returns an error if any of the setup funcs return an error.
func (self *CmpBasedTaskComparator) setupForSortingTasks() error {
	for _, setupFunc := range self.setupFuncs {
		if err := setupFunc(self); err != nil {
			return errors.Wrap(err, "Error running setup for sorting")
		}
	}
	return nil
}

// Determine which of two tasks is more important, by running the tasks through
// the comparator functions and returning the first definitive decision on which
// is more important.
func (self *CmpBasedTaskComparator) taskMoreImportantThan(task1, task2 task.Task) (bool, string, error) {
	// run through the comparators, and return the first definitive decision on
	// which task is more important
	for _, cmp := range self.comparators {
		ret, reason, err := cmp.compare(task1, task2, self)
		if err != nil {
			return false, "", errors.WithStack(err)
		}
		reason = fmt.Sprintf("%s: %s", cmp.name(), reason)

		switch ret {
		case -1:
			return false, reason, nil
		case 0:
			continue
		case 1:
			return true, reason, nil
		default:
			panic("Unexpected return value from task comparator")
		}
	}

	// none of the comparators reached a definitive decision, so the return val
	// doesn't matter
	return false, "", nil
}

// Functions that ensure the CmdBasedTaskPrioritizer implements sort.Interface

func (self *CmpBasedTaskComparator) Len() int {
	return len(self.tasks)
}

func (self *CmpBasedTaskComparator) Less(i, j int) bool {
	moreImportant, reason, err := self.taskMoreImportantThan(self.tasks[i],
		self.tasks[j])
	if err != nil {
		self.errsDuringSort = append(self.errsDuringSort, err)
	}
	thisTask, exists := self.orderingLogic[self.tasks[i].Id]
	if !exists {
		thisTask = map[string]string{}
	}
	thisTask[self.tasks[j].Id] = reason
	self.orderingLogic[self.tasks[i].Id] = thisTask

	return moreImportant
}

func (self *CmpBasedTaskComparator) Swap(i, j int) {
	self.tasks[i], self.tasks[j] = self.tasks[j], self.tasks[i]
}

// Split the tasks, based on the requester field.
// Returns two slices - the tasks requested by the repotracker, and the tasks
// requested in a patch.
func (self *CmpBasedTaskComparator) splitTasksByRequester(
	allTasks []task.Task) *CmpBasedTaskQueues {

	repoTrackerTasks := make([]task.Task, 0, len(allTasks))
	patchTasks := make([]task.Task, 0, len(allTasks))
	priorityTasks := make([]task.Task, 0, len(allTasks))

	for _, task := range allTasks {
		switch {
		case task.Priority > evergreen.MaxTaskPriority:
			priorityTasks = append(priorityTasks, task)
		case utility.StringSliceContains(evergreen.SystemVersionRequesterTypes, task.Requester):
			repoTrackerTasks = append(repoTrackerTasks, task)
		case evergreen.IsPatchRequester(task.Requester):
			patchTasks = append(patchTasks, task)
		case task.Requester == evergreen.AdHocRequester:
			patchTasks = append(patchTasks, task)
		default:
			grip.Error(message.Fields{
				"task":      task.Id,
				"requester": task.Requester,
				"runner":    RunnerName,
				"message":   "unrecognized requester",
				"priority":  task.Priority,
			})
		}
	}

	return &CmpBasedTaskQueues{
		HighPriorityTasks: priorityTasks,
		RepotrackerTasks:  repoTrackerTasks,
		PatchTasks:        patchTasks,
	}
}

// Merge the slices of tasks requested by the repotracker and in patches.
// Returns a slice of the merged tasks.
func (self *CmpBasedTaskComparator) mergeTasks(tq *CmpBasedTaskQueues) []task.Task {
	mergedTasks := make([]task.Task, 0, len(tq.RepotrackerTasks)+
		len(tq.PatchTasks)+len(tq.HighPriorityTasks))

	rIdx := 0
	pIdx := 0
	lenRepoTrackerTasks := len(tq.RepotrackerTasks)
	lenPatchTasks := len(tq.PatchTasks)

	// add the high priority tasks to the start of the queue
	mergedTasks = append(mergedTasks, tq.HighPriorityTasks...)
	for idx := 0; idx < len(tq.RepotrackerTasks)+len(tq.PatchTasks); idx++ {
		if pIdx >= lenPatchTasks { // overruns patch tasks
			mergedTasks = append(mergedTasks, tq.RepotrackerTasks[rIdx])
			rIdx++
		} else if rIdx >= lenRepoTrackerTasks { // overruns repotracker tasks
			mergedTasks = append(mergedTasks, tq.PatchTasks[pIdx])
			pIdx++
		} else if idx > 0 && (idx+1)%2 == 0 { // turn for a repotracker task
			mergedTasks = append(mergedTasks, tq.RepotrackerTasks[rIdx])
			rIdx++
		} else { // turn for a patch task
			mergedTasks = append(mergedTasks, tq.PatchTasks[pIdx])
			pIdx++
		}
	}
	return mergedTasks
}
