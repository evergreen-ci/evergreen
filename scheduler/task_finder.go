package scheduler

import (
	"sync"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/mongodb/grip"
)

type TaskFinder func() ([]task.Task, error)

func FindRunnableTasks() ([]task.Task, error) {
	return task.FindRunnable()
}

// The old Task finderDBTaskFinder, with the dependency check implemented in Go,
// instead of using $graphLookup
func LegacyFindRunnableTasks() ([]task.Task, error) {
	// find all of the undispatched tasks
	undispatchedTasks, err := task.Find(task.IsUndispatched)
	if err != nil {
		return nil, err
	}

	// filter out any tasks whose dependencies are not met
	runnableTasks := make([]task.Task, 0, len(undispatchedTasks))
	dependencyCaches := make(map[string]task.Task)
	for _, task := range undispatchedTasks {
		depsMet, err := task.DependenciesMet(dependencyCaches)
		if err != nil {
			grip.Errorf("Error checking dependencies for task %s: %+v", task.Id, err)
			continue
		}
		if depsMet {
			runnableTasks = append(runnableTasks, task)
		}
	}

	return runnableTasks, nil
}

func AlternateTaskFinder() ([]task.Task, error) {
	undispatchedTasks, err := task.Find(task.IsUndispatched)
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

	for t := range lookupSet {
		if _, ok := cache[t]; ok {
			continue
		}
		nt, err := task.FindOneId(t)
		catcher.Add(err)
		if nt == nil {
			continue
		}
		cache[t] = *nt
	}

	runnabletasks := []task.Task{}
	for _, t := range undispatchedTasks {
		depsMet, err := t.DependenciesMet(cache)
		catcher.Add(err)
		if depsMet {
			runnabletasks = append(runnabletasks, t)
		}
	}
	grip.Info(catcher.Resolve())

	return runnabletasks, nil
}

func ParallelTaskFinder() ([]task.Task, error) {
	undispatchedTasks, err := task.Find(task.IsUndispatched)
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
	for i := 0; i < 16; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for id := range toLookup {
				nt, err := task.FindOneId(id)
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
		depsMet, err := t.DependenciesMet(cache)
		catcher.Add(err)
		if depsMet {
			runnabletasks = append(runnabletasks, t)
		}
	}
	grip.Info(catcher.Resolve())

	return runnabletasks, nil
}
