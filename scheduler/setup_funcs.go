package scheduler

import (
	"fmt"
	"runtime"
	"sort"
	"sync"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	yaml "gopkg.in/yaml.v2"
)

// Function run before sorting all the tasks.  Used to fetch and store
// information needed for prioritizing the tasks.
type sortSetupFunc func(comparator *CmpBasedTaskComparator) error

// project is a type for holding a subset of the model.Project type.
type project struct {
	TaskGroups []model.TaskGroup `yaml:"task_groups"`
}

// cacheTaskGroups caches task groups by version. It uses yaml.Unmarshal instead
// of model.LoadProjectInto and only unmarshals task groups for efficiency.
func cacheTaskGroups(comparator *CmpBasedTaskComparator) error {
	comparator.projects = make(map[string]project)
	for _, v := range comparator.versions {
		p := project{}
		if err := yaml.Unmarshal([]byte(v.Config), &p); err != nil {
			return errors.Wrapf(err, "error unmarshalling task groups from version %s", v.Id)
		}
		comparator.projects[v.Id] = p
	}
	return nil
}

func backfillTaskGroups(comparator *CmpBasedTaskComparator) error {
	catcher := grip.NewBasicCatcher()

OUTERLOOP:
	for _, t := range comparator.tasks {
		if t.TaskGroup != "" && t.TaskGroupOrder == 0 {
			proj := comparator.projects[t.Version]
			for _, g := range proj.TaskGroups {
				if g.Name == t.TaskGroup {
					for idx, tn := range g.Tasks {
						if t.DisplayName == tn {
							t.TaskGroupOrder = idx + 1
							t.TaskGroupMaxHosts = g.MaxHosts
							catcher.Add(t.SetTaskGroupInfo())
							continue OUTERLOOP
						}
					}
					catcher.Errorf("task '%s' is missing from task group '%s' in version '%s'",
						t.DisplayName, t.TaskGroup, t.Version)
				}
			}
		}
	}

	return catcher.Resolve()
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
	// Reverse sort to sort task groups to the top, so that they are more
	// quickly pinned to hosts.
	sort.Sort(sort.Reverse(sort.StringSlice(taskKeys)))
	for i, k := range taskKeys {
		comparator.tasks[i] = taskMap[k]
	}
	return nil
}

func cacheExpectedDurations(comparator *CmpBasedTaskComparator) error {
	work := make(chan task.Task, len(comparator.tasks))
	output := make(chan task.Task, len(comparator.tasks))

	for _, t := range comparator.tasks {
		work <- t
	}
	close(work)

	wg := &sync.WaitGroup{}
	for i := 0; i < runtime.NumCPU(); i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for t := range work {
				_ = t.FetchExpectedDuration()
				output <- t
			}
		}()
	}
	wg.Wait()

	close(output)
	tasks := make([]task.Task, 0, len(comparator.tasks))

	for t := range output {
		tasks = append(tasks, t)
	}

	comparator.tasks = tasks

	return nil
}
