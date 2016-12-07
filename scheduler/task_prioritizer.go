package scheduler

import (
	"fmt"
	"sort"

	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
)

// TaskPrioritizer is responsible for taking in a slice of tasks, and ordering them
// according to which should be run first.
type TaskPrioritizer interface {
	// Takes in a slice of tasks and the current MCI settings.
	// Returns the slice of tasks, sorted in the order in which they should
	// be run, as well as an error if appropriate.
	PrioritizeTasks(settings *evergreen.Settings, tasks []task.Task) (
		[]task.Task, error)
}

// CmpBasedTaskComparator runs the tasks through a slice of comparator functions
// determining which is more important.
type CmpBasedTaskComparator struct {
	tasks          []task.Task
	errsDuringSort []error
	setupFuncs     []sortSetupFunc
	comparators    []taskPriorityCmp

	// caches for sorting
	previousTasksCache map[string]task.Task

	// cache the number of tasks that have failed in other buildvariants; tasks
	// with the same revision, project, display name and requester
	similarFailingCount map[string]int
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
func NewCmpBasedTaskComparator() *CmpBasedTaskComparator {
	return &CmpBasedTaskComparator{
		setupFuncs: []sortSetupFunc{
			cachePreviousTasks,
			cacheSimilarFailing,
		},
		comparators: []taskPriorityCmp{
			byPriority,
			byNumDeps,
			byRevisionOrderNumber,
			byCreateTime,
			bySimilarFailing,
			byRecentlyFailing,
		},
	}
}

type CmpBasedTaskPrioritizer struct{}

// PrioritizeTask prioritizes the tasks to run. First splits the tasks into slices based on
// whether they are part of patch versions or automatically created versions.
// Then prioritizes each slice, and merges them.
// Returns a full slice of the prioritized tasks, and an error if one occurs.
func (prioritizer *CmpBasedTaskPrioritizer) PrioritizeTasks(
	settings *evergreen.Settings, tasks []task.Task) ([]task.Task, error) {

	comparator := NewCmpBasedTaskComparator()
	// split the tasks into repotracker tasks and patch tasks, then prioritize
	// individually and merge
	taskQueues := comparator.splitTasksByRequester(tasks)
	prioritizedTaskLists := make([][]task.Task, 0, 3)
	for _, taskList := range [][]task.Task{taskQueues.RepotrackerTasks, taskQueues.PatchTasks, taskQueues.HighPriorityTasks} {

		comparator.tasks = taskList

		err := comparator.setupForSortingTasks()
		if err != nil {
			return nil, fmt.Errorf("Error running setup for sorting tasks: %v",
				err)
		}

		sort.Sort(comparator)

		if len(comparator.errsDuringSort) > 0 {
			errString := "The following errors were thrown while sorting:"
			for _, e := range comparator.errsDuringSort {
				errString += fmt.Sprintf("\n    %v", e)
			}
			return nil, fmt.Errorf(errString)
		}

		prioritizedTaskLists = append(prioritizedTaskLists, comparator.tasks)
	}
	prioritizedTaskQueues := CmpBasedTaskQueues{
		RepotrackerTasks:  prioritizedTaskLists[0],
		PatchTasks:        prioritizedTaskLists[1],
		HighPriorityTasks: prioritizedTaskLists[2],
	}

	comparator.tasks = comparator.mergeTasks(settings, &prioritizedTaskQueues)

	return comparator.tasks, nil
}

// Run all of the setup functions necessary for prioritizing the tasks.
// Returns an error if any of the setup funcs return an error.
func (self *CmpBasedTaskComparator) setupForSortingTasks() error {
	for _, setupFunc := range self.setupFuncs {
		if err := setupFunc(self); err != nil {
			return fmt.Errorf("Error running setup for sorting: %v", err)
		}
	}
	return nil
}

// Determine which of two tasks is more important, by running the tasks through
// the comparator functions and returning the first definitive decision on which
// is more important.
func (self *CmpBasedTaskComparator) taskMoreImportantThan(task1,
	task2 task.Task) (bool, error) {

	// run through the comparators, and return the first definitive decision on
	// which task is more important
	for _, cmp := range self.comparators {
		ret, err := cmp(task1, task2, self)
		if err != nil {
			return false, err
		}
		switch ret {
		case -1:
			return false, nil
		case 0:
			continue
		case 1:
			return true, nil
		default:
			panic("Unexpected return value from task comparator")
		}
	}

	// none of the comparators reached a definitive decision, so the return val
	// doesn't matter
	return false, nil
}

// Functions that ensure the CmdBasedTaskPrioritizer implements sort.Interface

func (self *CmpBasedTaskComparator) Len() int {
	return len(self.tasks)
}

func (self *CmpBasedTaskComparator) Less(i, j int) bool {
	moreImportant, err := self.taskMoreImportantThan(self.tasks[i],
		self.tasks[j])
	if err != nil {
		self.errsDuringSort = append(self.errsDuringSort, err)
	}
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
		case task.Requester == evergreen.RepotrackerVersionRequester:
			repoTrackerTasks = append(repoTrackerTasks, task)
		case task.Requester == evergreen.PatchVersionRequester:
			patchTasks = append(patchTasks, task)
		default:
			evergreen.Logger.Errorf(slogger.ERROR, "Unrecognized requester '%v'"+
				" for task %v:", task.Requester, task.Id)
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
func (self *CmpBasedTaskComparator) mergeTasks(settings *evergreen.Settings,
	tq *CmpBasedTaskQueues) []task.Task {

	mergedTasks := make([]task.Task, 0, len(tq.RepotrackerTasks)+
		len(tq.PatchTasks)+len(tq.HighPriorityTasks))

	toggle := settings.Scheduler.MergeToggle
	if toggle == 0 {
		toggle = 2 // defaults to interleaving evenly
	}

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
		} else if idx > 0 && (idx+1)%toggle == 0 { // turn for a repotracker task
			mergedTasks = append(mergedTasks, tq.RepotrackerTasks[rIdx])
			rIdx++
		} else { // turn for a patch task
			mergedTasks = append(mergedTasks, tq.PatchTasks[pIdx])
			pIdx++
		}
	}
	return mergedTasks
}
