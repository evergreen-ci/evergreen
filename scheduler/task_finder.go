package scheduler

import (
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/anser/bsonutil"
	"github.com/mongodb/anser/db"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/bson"
)

type TaskFinder func(distro.Distro) ([]task.Task, error)

func GetTaskFinder(version string) TaskFinder {
	switch version {
	case evergreen.FinderVersionParallel:
		return ParallelTaskFinder
	case evergreen.FinderVersionLegacy:
		return LegacyFindRunnableTasks
	case evergreen.FinderVersionPipeline:
		return RunnableTasksPipeline
	case evergreen.FinderVersionAlternate:
		return AlternateTaskFinder
	default:
		return LegacyFindRunnableTasks
	}
}

func RunnableTasksPipeline(d distro.Distro) ([]task.Task, error) {
	return FindRunnable(d.Id, d.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies)
}

// The old Task finderDBTaskFinder, with the dependency check implemented in Go,
// instead of using $graphLookup
func LegacyFindRunnableTasks(d distro.Distro) ([]task.Task, error) {
	// find all of the undispatched tasks
	undispatchedTasks, err := FindSchedulable(d.Id)
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
				"planner": d.PlannerSettings.Version,
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
				"planner": d.PlannerSettings.Version,
				"project": t.Project,
			})
			continue
		}

		if len(d.ValidProjects) > 0 && !util.StringSliceContains(d.ValidProjects, ref.Identifier) {
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

		if t.IsPatchRequest() && ref.PatchingDisabled {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "patch testing disabled",
				"outcome": "skipping",
				"planner": d.PlannerSettings.Version,
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if d.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
			depsMet, err := t.DependenciesMet(dependencyCaches)
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

func AlternateTaskFinder(d distro.Distro) ([]task.Task, error) {
	undispatchedTasks, err := FindSchedulable(d.Id)
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

	tasksToCache, err := task.Find(task.ByIds(taskIds).WithFields(task.StatusKey, task.DependsOnKey))
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

		if !ref.Enabled {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "project disabled",
				"outcome": "skipping",
				"task":    t.Id,
				"planner": d.PlannerSettings.Version,
				"project": t.Project,
			})
			continue
		}

		if len(d.ValidProjects) > 0 && !util.StringSliceContains(d.ValidProjects, ref.Identifier) {
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

		if t.IsPatchRequest() && ref.PatchingDisabled {
			grip.Notice(message.Fields{
				"runner":  RunnerName,
				"message": "patch testing disabled",
				"outcome": "skipping",
				"planner": d.PlannerSettings.Version,
				"task":    t.Id,
				"project": t.Project,
			})
			continue
		}

		if d.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
			depsMet, err := t.AllDependenciesSatisfied(cache)
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

func ParallelTaskFinder(d distro.Distro) ([]task.Task, error) {
	undispatchedTasks, err := FindSchedulable(d.Id)
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
				nt, err := task.FindOneIdWithFields(id, task.StatusKey, task.DependsOnKey)
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

		if len(d.ValidProjects) > 0 && !util.StringSliceContains(d.ValidProjects, ref.Identifier) {
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

		if d.DispatcherSettings.Version != evergreen.DispatcherVersionRevisedWithDependencies {
			depsMet, err := t.AllDependenciesSatisfied(cache)
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

func FindSchedulable(distroID string) ([]task.Task, error) {
	query := task.SchedulableTasksQuery()

	if err := addApplicableDistroFilter(distroID, task.DistroIdKey, query); err != nil {
		return nil, errors.WithStack(err)
	}
	return task.Find(db.Query(query))
}

func addApplicableDistroFilter(id string, fieldName string, query bson.M) error {
	if id == "" {
		return nil
	}

	aliases, err := distro.FindApplicableDistroIDs(id)
	if err != nil {
		return errors.WithStack(err)
	}

	if len(aliases) == 1 {
		query[fieldName] = aliases[0]
	} else {
		query[fieldName] = bson.M{"$in": aliases}
	}

	return nil
}

func FindSchedulableForAlias(id string) ([]task.Task, error) {
	q := task.SchedulableTasksQuery()

	if err := addApplicableDistroFilter(id, task.DistroAliasesKey, q); err != nil {
		return nil, errors.WithStack(err)
	}

	// Single-host task groups can't be put in an alias queue, because it can
	// cause a race when assigning tasks to hosts where the tasks in the task
	// group might be assigned to different hosts.
	q[task.TaskGroupMaxHostsKey] = bson.M{"$ne": 1}

	return task.FindAll(db.Query(q))
}

func FindRunnable(distroID string, removeDeps bool) ([]task.Task, error) {
	const dependencyKey = "dependencies"

	match := task.SchedulableTasksQuery()
	var d distro.Distro
	var err error
	if distroID != "" {
		d, err = distro.FindOne(distro.ById(distroID).WithFields(distro.ValidProjectsKey))
		if err != nil {
			return nil, errors.Wrapf(err, "problem finding distro '%s'", distroID)
		}
	}

	if err = addApplicableDistroFilter(distroID, task.DistroIdKey, match); err != nil {
		return nil, errors.WithStack(err)
	}

	matchActivatedUndispatchedTasks := bson.M{
		"$match": match,
	}

	filterInvalidDistros := bson.M{
		"$match": bson.M{task.ProjectKey: bson.M{"$in": d.ValidProjects}},
	}

	removeFields := bson.M{
		"$project": bson.M{
			task.LogsKey:      0,
			task.OldTaskIdKey: 0,
			task.DependsOnKey + "." + task.DependencyUnattainableKey: 0,
		},
	}

	graphLookupTaskDeps := bson.M{
		"$graphLookup": bson.M{
			"from":             task.Collection,
			"startWith":        "$" + task.DependsOnKey + "." + task.IdKey,
			"connectFromField": task.DependsOnKey + "." + task.IdKey,
			"connectToField":   task.IdKey,
			"as":               dependencyKey,
			// restrict graphLookup to only direct dependencies
			"maxDepth": 0,
		},
	}

	unwindDependencies := bson.M{
		"$unwind": bson.M{
			"path":                       "$" + dependencyKey,
			"preserveNullAndEmptyArrays": true,
		},
	}

	unwindDependsOn := bson.M{
		"$unwind": bson.M{
			"path":                       "$" + task.DependsOnKey,
			"preserveNullAndEmptyArrays": true,
		},
	}

	matchIds := bson.M{
		"$match": bson.M{
			"$expr": bson.M{"$eq": bson.A{"$" + bsonutil.GetDottedKeyName(task.DependsOnKey, task.DependencyTaskIdKey), "$" + bsonutil.GetDottedKeyName(dependencyKey, task.IdKey)}},
		},
	}

	projectSatisfied := bson.M{
		"$addFields": bson.M{
			"satisfied_dependencies": bson.M{
				"$cond": bson.A{
					bson.M{
						"$or": []bson.M{
							{"$eq": bson.A{"$" + bsonutil.GetDottedKeyName(task.DependsOnKey, task.DependencyStatusKey), "$" + bsonutil.GetDottedKeyName(dependencyKey, task.StatusKey)}},
							{"$and": []bson.M{
								{"$eq": bson.A{"$" + bsonutil.GetDottedKeyName(task.DependsOnKey, task.DependencyStatusKey), "*"}},
								{"$or": []bson.M{
									{"$in": bson.A{"$" + bsonutil.GetDottedKeyName(dependencyKey, task.StatusKey), task.CompletedStatuses}},
									{"$anyElementTrue": "$" + bsonutil.GetDottedKeyName(dependencyKey, task.DependsOnKey, task.DependencyUnattainableKey)},
								}},
							}},
						},
					},
					true,
					false,
				},
			},
		},
	}

	regroupTasks := bson.M{
		"$group": bson.M{
			"_id":           "$_id",
			"satisfied_set": bson.M{"$addToSet": "$satisfied_dependencies"},
			"root":          bson.M{"$first": "$$ROOT"},
		},
	}

	redactUnsatisfiedDependencies := bson.M{
		"$redact": bson.M{
			"$cond": bson.A{
				bson.M{"$allElementsTrue": "$satisfied_set"},
				"$$KEEP",
				"$$PRUNE",
			},
		},
	}

	replaceRoot := bson.M{"$replaceRoot": bson.M{"newRoot": "$root"}}

	joinProjectRef := bson.M{
		"$lookup": bson.M{
			"from":         "project_ref",
			"localField":   task.ProjectKey,
			"foreignField": "identifier",
			"as":           "project_ref",
		},
	}

	filterDisabledProjects := bson.M{
		"$match": bson.M{
			"project_ref.0." + "enabled": true,
		},
	}

	filterPatchingDisabledProjects := bson.M{
		"$match": bson.M{"$or": []bson.M{
			{
				task.RequesterKey: bson.M{"$nin": evergreen.PatchRequesters},
			},
			{
				"project_ref.0." + "patching_disabled": false,
			},
		}},
	}

	removeProjectRef := bson.M{
		"$project": bson.M{
			"project_ref": 0,
		},
	}

	pipeline := []bson.M{
		matchActivatedUndispatchedTasks,
		removeFields,
		graphLookupTaskDeps,
	}

	if distroID != "" && len(d.ValidProjects) > 0 {
		pipeline = append(pipeline, filterInvalidDistros)
	}

	if removeDeps {
		pipeline = append(pipeline,
			unwindDependencies,
			unwindDependsOn,
			matchIds,
			projectSatisfied,
			regroupTasks,
			redactUnsatisfiedDependencies,
			replaceRoot,
		)
	}

	pipeline = append(pipeline,
		joinProjectRef,
		filterDisabledProjects,
		filterPatchingDisabledProjects,
		removeProjectRef,
	)

	runnableTasks := []task.Task{}
	if err := task.Aggregate(pipeline, &runnableTasks); err != nil {
		return nil, errors.Wrap(err, "failed to fetch runnable tasks")
	}

	return runnableTasks, nil
}
