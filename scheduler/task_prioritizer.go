package scheduler

import (
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"sort"
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

// CmpBasedTaskPrioritizer runs the tasks through a slice of comparator functions
// determining which is more important.
type CmpBasedTaskPrioritizer struct {
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

// NewCmpBasedTaskPrioritizer returns a new task prioritizer, using the default set of comparators
// as well as the setup functions necessary for those comparators.
func NewCmpBasedTaskPrioritizer() *CmpBasedTaskPrioritizer {
	return &CmpBasedTaskPrioritizer{
		setupFuncs: []sortSetupFunc{
			cachePreviousTasks,
			cacheSimilarFailing,
		},
		comparators: []taskPriorityCmp{
			byPriority,
			byStageName(evergreen.CompileStage),
			byNumDeps,
			byRevisionOrderNumber,
			byCreateTime,
			bySimilarFailing,
			byRecentlyFailing,
		},
	}
}

// PrioritizeTask prioritizes the tasks to run. First splits the tasks into slices based on
// whether they are part of patch versions or automatically created versions.
// Then prioritizes each slice, and merges them.
// Returns a full slice of the prioritized tasks, and an error if one occurs.
func (self *CmpBasedTaskPrioritizer) PrioritizeTasks(
	settings *evergreen.Settings, tasks []task.Task) ([]task.Task, error) {

	// split the tasks into repotracker tasks and patch tasks, then prioritize
	// individually and merge
	repoTrackerTasks, patchTasks := self.splitTasksByRequester(tasks)
	prioritizedTaskLists := make([][]task.Task, 0, 2)
	for _, taskList := range [][]task.Task{repoTrackerTasks, patchTasks} {

		self.tasks = taskList

		err := self.setupForSortingTasks()
		if err != nil {
			return nil, fmt.Errorf("Error running setup for sorting tasks: %v",
				err)
		}

		sort.Sort(self)

		if len(self.errsDuringSort) > 0 {
			errString := "The following errors were thrown while sorting:"
			for _, e := range self.errsDuringSort {
				errString += fmt.Sprintf("\n    %v", e)
			}
			return nil, fmt.Errorf(errString)
		}

		prioritizedTaskLists = append(prioritizedTaskLists, self.tasks)
	}

	self.tasks = self.mergeTasks(settings, prioritizedTaskLists[0],
		prioritizedTaskLists[1])

	return self.tasks, nil
}

// Run all of the setup functions necessary for prioritizing the tasks.
// Returns an error if any of the setup funcs return an error.
func (self *CmpBasedTaskPrioritizer) setupForSortingTasks() error {
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
func (self *CmpBasedTaskPrioritizer) taskMoreImportantThan(task1,
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

func (self *CmpBasedTaskPrioritizer) Len() int {
	return len(self.tasks)
}

func (self *CmpBasedTaskPrioritizer) Less(i, j int) bool {
	moreImportant, err := self.taskMoreImportantThan(self.tasks[i],
		self.tasks[j])
	if err != nil {
		self.errsDuringSort = append(self.errsDuringSort, err)
	}
	return moreImportant
}

func (self *CmpBasedTaskPrioritizer) Swap(i, j int) {
	self.tasks[i], self.tasks[j] = self.tasks[j], self.tasks[i]
}

// Split the tasks, based on the requester field.
// Returns two slices - the tasks requested by the repotracker, and the tasks
// requested in a patch.
func (self *CmpBasedTaskPrioritizer) splitTasksByRequester(
	allTasks []task.Task) ([]task.Task, []task.Task) {

	repoTrackerTasks := make([]task.Task, 0, len(allTasks))
	patchTasks := make([]task.Task, 0, len(allTasks))

	for _, task := range allTasks {
		switch task.Requester {
		case evergreen.RepotrackerVersionRequester:
			repoTrackerTasks = append(repoTrackerTasks, task)
		case evergreen.PatchVersionRequester:
			patchTasks = append(patchTasks, task)
		default:
			evergreen.Logger.Errorf(slogger.ERROR, "Unrecognized requester '%v'"+
				" for task %v:", task.Requester, task.Id)
		}
	}

	return repoTrackerTasks, patchTasks
}

// Merge the slices of tasks requested by the repotracker and in patches.
// Returns a slice of the merged tasks.
func (self *CmpBasedTaskPrioritizer) mergeTasks(settings *evergreen.Settings,
	repoTrackerTasks []task.Task, patchTasks []task.Task) []task.Task {

	mergedTasks := make([]task.Task, 0, len(repoTrackerTasks)+len(patchTasks))

	toggle := settings.Scheduler.MergeToggle
	if toggle == 0 {
		toggle = 2 // defaults to interleaving evenly
	}

	rIdx := 0
	pIdx := 0
	lenRepoTrackerTasks := len(repoTrackerTasks)
	lenPatchTasks := len(patchTasks)
	for idx := 0; idx < len(repoTrackerTasks)+len(patchTasks); idx++ {
		if pIdx >= lenPatchTasks { // overruns patch tasks
			mergedTasks = append(mergedTasks, repoTrackerTasks[rIdx])
			rIdx++
		} else if rIdx >= lenRepoTrackerTasks { // overruns repotracker tasks
			mergedTasks = append(mergedTasks, patchTasks[pIdx])
			pIdx++
		} else if idx > 0 && (idx+1)%toggle == 0 { // turn for a repotracker task
			mergedTasks = append(mergedTasks, repoTrackerTasks[rIdx])
			rIdx++
		} else { // turn for a patch task
			mergedTasks = append(mergedTasks, patchTasks[pIdx])
			pIdx++
		}
	}
	return mergedTasks
}
