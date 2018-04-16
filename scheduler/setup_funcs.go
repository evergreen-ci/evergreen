package scheduler

import (
	"fmt"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// Function run before sorting all the tasks.  Used to fetch and store
// information needed for prioritizing the tasks.
type sortSetupFunc func(comparator *CmpBasedTaskComparator) error

// Get all of the previous completed tasks for the ones to be sorted, and cache
// them appropriately.
func cachePreviousTasks(comparator *CmpBasedTaskComparator) error {
	// get the relevant previous completed tasks
	var err error
	comparator.previousTasksCache = make(map[string]task.Task)
	for _, t := range comparator.tasks {
		prevTask := &task.Task{}

		// only relevant for repotracker tasks
		if t.Requester == evergreen.RepotrackerVersionRequester {
			prevTask, err = t.PreviousCompletedTask(t.Project, []string{})
			if err != nil {
				return errors.Wrap(err, "cachePreviousTasks")
			}
			if prevTask == nil {
				prevTask = &task.Task{}
			}
		}
		comparator.previousTasksCache[t.Id] = *prevTask
	}

	return nil
}

// cacheSimilarFailing fetches all failed tasks with the same display name,
// revision, requester and project but in other buildvariants
func cacheSimilarFailing(comparator *CmpBasedTaskComparator) error {
	// find if there are any similar failing tasks
	var err error
	comparator.similarFailingCount = make(map[string]int)
	for _, t := range comparator.tasks {
		numSimilarFailing := 0

		// only relevant for repotracker tasks
		if t.Requester == evergreen.RepotrackerVersionRequester {
			numSimilarFailing, err = t.CountSimilarFailingTasks()
			if err != nil {
				return errors.Wrap(err, "cacheSimilarFailing")
			}
		}
		comparator.similarFailingCount[t.Id] = numSimilarFailing
	}
	return nil
}

// project is a type for holding a subset of the model.Project type.
type project struct {
	TaskGroups []model.TaskGroup `yaml:"task_groups"`
}

// cacheTaskGroups caches task groups by version. It uses yaml.Unmarshal instead
// of model.LoadProjectInto and only unmarshals task groups for efficiency.
func cacheTaskGroups(comparator *CmpBasedTaskComparator) error {
	comparator.projects = make(map[string]project)
	for _, t := range comparator.tasks {
		if _, ok := comparator.projects[t.Version]; ok {
			continue
		}
		v, err := version.FindOneId(t.Version)
		if err != nil {
			return errors.Wrapf(err, "error finding version %s for task %s", t.Id, t.Version)
		}
		p := project{}
		if err := yaml.Unmarshal([]byte(v.Config), &p); err != nil {
			return errors.Wrapf(err, "error unmarshalling task groups from version %s for task %s", t.Id, t.Version)
		}
		comparator.projects[t.Version] = p
	}
	return nil
}

// groupTaskGroups puts tasks that have the same build and task group next to
// each other in the queue. This ensures that, in a stable sort,
// byTaskGroupOrder sorts task group members relative to each other.
func groupTaskGroups(comparator *CmpBasedTaskComparator) error {
	taskMap := make(map[string]task.Task)
	taskKeys := []string{}
	for _, t := range comparator.tasks {
		k := fmt.Sprintf("%s-%s-%s", t.BuildId, t.TaskGroup, t.Id)
		taskMap[k] = t
		taskKeys = append(taskKeys, k)
	}
	sort.Strings(taskKeys)
	for i, k := range taskKeys {
		comparator.tasks[i] = taskMap[k]
	}
	return nil
}
