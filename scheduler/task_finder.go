package scheduler

import (
	"sync"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type TaskFinder func(distroID string) ([]task.Task, error)

func GetTaskFinder(version string) TaskFinder {
	switch version {
	case "parallel":
		return ParallelTaskFinder
	case "legacy":
		return LegacyFindRunnableTasks
	case "pipeline":
		return RunnableTasksPipeline
	case "alternate":
		return AlternateTaskFinder
	default:
		return LegacyFindRunnableTasks
	}
}

func RunnableTasksPipeline(distroID string) ([]task.Task, error) {
	return task.FindRunnable(distroID)
}

// The old Task finderDBTaskFinder, with the dependency check implemented in Go,
// instead of using $graphLookup
func LegacyFindRunnableTasks(distroID string) ([]task.Task, error) {
	// find all of the undispatched tasks
	undispatchedTasks, err := task.FindSchedulable(distroID)
	if err != nil {
		return nil, err
	}

	projectRefCache, err := getProjectRefCache()
	if err != nil {
		return nil, err
	}

	// filter out any tasks whose dependencies are not met
	runnableTasks := make([]task.Task, 0, len(undispatchedTasks))
	dependencyCaches := make(map[string]task.Task)
	for _, t := range undispatchedTasks {
		ref, ok := projectRefCache[t.Project]
		if !ok {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "could not find project for task",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if !ref.Enabled {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "project disabled",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if t.IsPatchRequest() && ref.PatchingDisabled {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "patch testing disabled",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		depsMet, err := t.DependenciesMet(dependencyCaches)
		if err != nil {
			grip.Warning(message.Fields{
				"runner":  RunnerName,
				"message": "error checking dependencies for task",
				"outcome": "skipping",
				"task":    t.Id,
				"error":   err.Error(),
			})
			continue
		}
		if !depsMet {
			continue
		}

		runnableTasks = append(runnableTasks, t)
	}

	return runnableTasks, nil
}

func AlternateTaskFinder(distroID string) ([]task.Task, error) {
	undispatchedTasks, err := task.FindSchedulable(distroID)
	if err != nil {
		return nil, err
	}

	projectRefCache, err := getProjectRefCache()
	if err != nil {
		return nil, err
	}

	cache := make(map[string]task.Task)
	lookupSet := make(map[string]struct{})
	catcher := grip.NewBasicCatcher()

	for _, t := range undispatchedTasks {
		cache[t.Id] = t
		for _, dep := range t.DependsOn {
			lookupSet[dep.TaskId] = struct{}{}
		}
	}

	taskIds := []string{}
	for t := range lookupSet {
		if _, ok := cache[t]; ok {
			continue
		}
		taskIds = append(taskIds, t)
	}

	tasksToCache, err := task.Find(task.ByIds(taskIds).WithFields(task.StatusKey))
	if err != nil {
		return nil, errors.Wrap(err, "problem finding task dependencies")
	}

	for _, t := range tasksToCache {
		cache[t.Id] = t
	}

	runnabletasks := []task.Task{}
	for _, t := range undispatchedTasks {
		ref, ok := projectRefCache[t.Project]
		if !ok {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "could not find project for task",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if !ref.Enabled {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "project disabled",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if t.IsPatchRequest() && ref.PatchingDisabled {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "patch testing disabled",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		depsMet, err := t.AllDependenciesSatisfied(cache)
		catcher.Add(err)
		if !depsMet {
			continue
		}
		runnabletasks = append(runnabletasks, t)

	}
	grip.Info(message.WrapError(catcher.Resolve(), message.Fields{
		"runner":             RunnerName,
		"scheduleable_tasks": len(undispatchedTasks),
	}))

	return runnabletasks, nil
}

func ParallelTaskFinder(distroID string) ([]task.Task, error) {
	undispatchedTasks, err := task.FindSchedulable(distroID)
	if err != nil {
		return nil, err
	}

	projectRefCache, err := getProjectRefCache()
	if err != nil {
		return nil, err
	}

	cache := make(map[string]task.Task)
	catcher := grip.NewBasicCatcher()
	lookupSet := make(map[string]struct{})
	for _, t := range undispatchedTasks {
		cache[t.Id] = t
		for _, dep := range t.DependsOn {
			lookupSet[dep.TaskId] = struct{}{}
		}

	}

	results := make(chan *task.Task, len(lookupSet))
	toLookup := make(chan string, len(lookupSet))
	for t := range lookupSet {
		toLookup <- t
	}
	close(toLookup)

	wg := &sync.WaitGroup{}
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range toLookup {
				nt, err := task.FindOneIdWithFields(id, task.StatusKey)
				catcher.Add(err)
				if nt == nil {
					continue
				}
				results <- nt
			}
		}()
	}

	wg.Wait()
	close(results)

	for t := range results {
		cache[t.Id] = *t
	}

	runnabletasks := []task.Task{}
	for _, t := range undispatchedTasks {
		ref, ok := projectRefCache[t.Project]
		if !ok {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "could not find project for task",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})

			continue
		}

		if !ref.Enabled {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "project disabled",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if t.IsPatchRequest() && ref.PatchingDisabled {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "patch testing disabled",
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		depsMet, err := t.AllDependenciesSatisfied(cache)
		if err != nil {
			catcher.Add(err)
			continue
		}

		if !depsMet {
			continue
		}

		runnabletasks = append(runnabletasks, t)
	}
	grip.Info(message.WrapError(catcher.Resolve(), message.Fields{
		"runner":             RunnerName,
		"scheduleable_tasks": len(undispatchedTasks),
	}))

	return runnabletasks, nil
}

func getProjectRefCache() (map[string]model.ProjectRef, error) {
	out := map[string]model.ProjectRef{}
	refs, err := model.FindAllProjectRefs()
	if err != nil {
		return out, err
	}

	for _, ref := range refs {
		out[ref.Identifier] = ref
	}

	return out, nil
}

// GetRunnableTasksAndVersions finds tasks whose versions have already been
// created, and returns those tasks, as well as a map of version IDs to versions.
func filterTasksWithVersionCache(tasks []task.Task) ([]task.Task, map[string]model.Version, error) {
	ids := make(map[string]struct{})

	for _, t := range tasks {
		ids[t.Version] = struct{}{}
	}

	idlist := []string{}
	for id := range ids {
		idlist = append(idlist, id)
	}

	vs, err := model.VersionFindByIds(idlist)
	if err != nil {
		return nil, nil, errors.Wrap(err, "problem resolving version cache")
	}

	versions := make(map[string]model.Version)
	for _, v := range vs {
		versions[v.Id] = v
	}

	filteredTasks := []task.Task{}
	for _, t := range tasks {
		if _, ok := versions[t.Version]; ok {
			filteredTasks = append(filteredTasks, t)
		}
	}

	return filteredTasks, versions, nil
}
