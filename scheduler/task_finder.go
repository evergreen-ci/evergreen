package scheduler

import (
	"context"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type TaskFinder func(context.Context, distro.Distro) ([]task.Task, error)

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

func RunnableTasksPipeline(ctx context.Context, d distro.Distro) ([]task.Task, error) {
	return task.FindHostRunnable(ctx, d.Id, d.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies)
}

// The old Task finderDBTaskFinder, with the dependency check implemented in Go,
// instead of using $graphLookup
func LegacyFindRunnableTasks(ctx context.Context, d distro.Distro) ([]task.Task, error) {
	// find all of the undispatched tasks
	undispatchedTasks, err := task.FindHostSchedulable(ctx, d.Id)
	if err != nil {
		return nil, err
	}

	projectRefCache, err := getProjectRefCache(ctx)
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
				"planner": d.PlannerSettings.Version,
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		canDispatch, _ := model.ProjectCanDispatchTask(&ref, &t)
		if !canDispatch {
			continue
		}

		if len(d.ValidProjects) > 0 && !utility.StringSliceContains(d.ValidProjects, ref.Id) {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "project is not valid for distro",
				"outcome": "skipping",
				"planner": d.PlannerSettings.Version,
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if d.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
			depsMet, err := t.DependenciesMet(ctx, dependencyCaches)
			if err != nil {
				grip.Warning(message.Fields{
					"runner":  RunnerName,
					"message": "error checking dependencies for task",
					"outcome": "skipping",
					"planner": d.FinderSettings.Version,
					"task":    t.Id,
					"error":   err.Error(),
				})
				continue
			}
			if !depsMet {
				continue
			}
		}

		runnableTasks = append(runnableTasks, t)
	}

	return runnableTasks, nil
}

func AlternateTaskFinder(ctx context.Context, d distro.Distro) ([]task.Task, error) {
	undispatchedTasks, err := task.FindHostSchedulable(ctx, d.Id)
	if err != nil {
		return nil, err
	}

	projectRefCache, err := getProjectRefCache(ctx)
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

	tasksToCache, err := task.FindWithFields(ctx, task.ByIds(taskIds), task.StatusKey, task.DependsOnKey)
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
				"planner": d.PlannerSettings.Version,
				"outcome": "skipping",
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		canDispatch, _ := model.ProjectCanDispatchTask(&ref, &t)
		if !canDispatch {
			continue
		}

		if len(d.ValidProjects) > 0 && !utility.StringSliceContains(d.ValidProjects, ref.Id) {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "project is not valid for distro",
				"outcome": "skipping",
				"planner": d.PlannerSettings.Version,
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if d.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
			depsMet, err := t.AllDependenciesSatisfied(ctx, cache)
			catcher.Add(err)
			if !depsMet {
				continue
			}
		}
		runnabletasks = append(runnabletasks, t)

	}
	grip.Info(message.WrapError(catcher.Resolve(), message.Fields{
		"runner":            RunnerName,
		"schedulable_tasks": len(undispatchedTasks),
	}))

	return runnabletasks, nil
}

func ParallelTaskFinder(ctx context.Context, d distro.Distro) ([]task.Task, error) {
	undispatchedTasks, err := task.FindHostSchedulable(ctx, d.Id)
	if err != nil {
		return nil, err
	}

	projectRefCache, err := getProjectRefCache(ctx)
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
				nt, err := task.FindOneIdWithFields(ctx, id, task.StatusKey, task.DependsOnKey)
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

		canDispatch, _ := model.ProjectCanDispatchTask(&ref, &t)
		if !canDispatch {
			continue
		}

		if len(d.ValidProjects) > 0 && !utility.StringSliceContains(d.ValidProjects, ref.Id) {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "project is not valid for distro",
				"outcome": "skipping",
				"planner": d.PlannerSettings.Version,
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if d.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
			depsMet, err := t.AllDependenciesSatisfied(ctx, cache)
			if err != nil {
				catcher.Add(err)
				continue
			}

			if !depsMet {
				continue
			}
		}
		runnabletasks = append(runnabletasks, t)
	}
	grip.Info(message.WrapError(catcher.Resolve(), message.Fields{
		"runner":            RunnerName,
		"planner":           d.PlannerSettings.Version,
		"schedulable_tasks": len(undispatchedTasks),
	}))

	return runnabletasks, nil
}

func getProjectRefCache(ctx context.Context) (map[string]model.ProjectRef, error) {
	out := map[string]model.ProjectRef{}
	refs, err := model.FindAllMergedProjectRefs(ctx)
	if err != nil {
		return out, err
	}

	for _, ref := range refs {
		out[ref.Id] = ref
	}

	return out, nil
}
