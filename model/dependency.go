package model

import (
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type dependencyIncluder struct {
	Project                 *Project
	requester               string
	included                map[TVPair]bool
	deactivateGeneratedDeps map[TVPair]bool
}

// IncludeDependencies takes a project and a slice of variant/task pairs names
// and returns the expanded set of variant/task pairs to include all the dependencies/requirements
// for the given set of tasks.
// If any dependency is cross-variant, it will include the variant and task for that dependency.
// This function can return an error, but it should be treated as an informational warning.
func IncludeDependencies(project *Project, tvpairs []TVPair, requester string, activationInfo *specificActivationInfo) ([]TVPair, error) {
	di := &dependencyIncluder{Project: project, requester: requester}
	return di.include(tvpairs, activationInfo, nil)
}

// IncludeDependenciesWithGenerated performs the same function as IncludeDependencies for generated projects.
// activationInfo and generatedVariants are required in the case for generate tasks to detect if
// new generated dependency's task/variant pairs are depended on by inactive tasks. If so,
// we also set these new dependencies to inactive.
func IncludeDependenciesWithGenerated(project *Project, tvpairs []TVPair, requester string, activationInfo *specificActivationInfo, generatedVariants []parserBV) ([]TVPair, error) {
	di := &dependencyIncluder{Project: project, requester: requester}
	return di.include(tvpairs, activationInfo, generatedVariants)
}

// include crawls the tasks represented by the combination of variants and tasks and
// add or removes tasks based on the dependency graph. Dependent tasks
// are added; tasks that depend on unreachable tasks are pruned. New slices
// of variants and tasks are returned.
func (di *dependencyIncluder) include(initialDeps []TVPair, activationInfo *specificActivationInfo, generatedVariants []parserBV) ([]TVPair, error) {
	di.included = map[TVPair]bool{}
	di.deactivateGeneratedDeps = map[TVPair]bool{}
	warnings := grip.NewBasicCatcher()
	// handle each pairing, recursively adding and pruning based
	// on the task's dependencies
	for _, d := range initialDeps {
		_, err := di.handle(d, activationInfo, generatedVariants, true)
		warnings.Add(err)
	}

	outPairs := []TVPair{}
	for pair, shouldInclude := range di.included {
		if shouldInclude {
			outPairs = append(outPairs, pair)
		}
	}
	// We deactivate tasks that do not have specific activation set in the
	// generated project but have been generated as dependencies of other
	// explicitly inactive tasks
	for pair, addToActivationInfo := range di.deactivateGeneratedDeps {
		if addToActivationInfo {
			activationInfo.activationTasks[pair.Variant] = append(activationInfo.activationTasks[pair.Variant], pair.TaskName)
		}
	}
	return outPairs, warnings.Resolve()
}

// handle finds and includes all tasks that the given task/variant pair depends
// on. Returns true if the task and all of its dependent tasks can be scheduled
// for the requester. Returns false if it cannot be scheduled, with an error
// explaining why.
// isRoot denotes whether the function is at its recursive root, and if so we
// update the deactivateGeneratedDeps map.
func (di *dependencyIncluder) handle(pair TVPair, activationInfo *specificActivationInfo, generatedVariants []parserBV, isRoot bool) (bool, error) {
	if included, ok := di.included[pair]; ok {
		// we've been here before, so don't redo work
		return included, nil
	}

	// For a task group, recurse on each task and add those tasks that should be
	// included.
	if tg := di.Project.FindTaskGroup(pair.TaskName); tg != nil {
		catcher := grip.NewBasicCatcher()
		for _, t := range tg.Tasks {
			_, err := di.handle(TVPair{TaskName: t, Variant: pair.Variant}, activationInfo, generatedVariants, false)
			catcher.Wrapf(err, "task group '%s' in variant '%s' contains unschedulable task '%s'", pair.TaskName, pair.Variant, t)
		}

		if catcher.HasErrors() {
			return false, catcher.Resolve()
		}

		return true, nil
	}

	// For a task group task inside a single host task group, the previous
	// tasks in the task group are implicit dependencies of the current task.
	if tg := di.Project.FindTaskGroupForTask(pair.Variant, pair.TaskName); tg != nil && tg.MaxHosts == 1 {
		catcher := grip.NewBasicCatcher()
		for _, t := range tg.Tasks {
			if t == pair.TaskName {
				// When we reach the current task, stop looping.
				// The current task being handled by the current
				// invocation of this function.
				break
			}
			_, err := di.handle(TVPair{Variant: pair.Variant, TaskName: t}, activationInfo, generatedVariants, false)
			catcher.Wrapf(err, "task group '%s' in variant '%s' contains unschedulable task '%s'", pair.TaskName, pair.Variant, t)
		}

		if catcher.HasErrors() {
			// If any of the previous tasks in the task group are unschedulable,
			// unschedule everything in the single host task group.
			di.included[pair] = false
			for _, p := range tg.Tasks {
				di.included[TVPair{Variant: pair.Variant, TaskName: p}] = false
			}
			return false, catcher.Resolve()
		}
	}

	// we must load the BuildVariantTaskUnit for the task/variant pair,
	// since it contains the full scope of dependency information
	bvt := di.Project.FindTaskForVariant(pair.TaskName, pair.Variant)
	if bvt == nil {
		di.included[pair] = false
		return false, errors.Errorf("task '%s' does not exist in project '%s' for variant '%s'", pair.TaskName,
			di.Project.Identifier, pair.Variant)
	}

	if bvt.SkipOnRequester(di.requester) {
		// TODO (DEVPROD-4776): it seems like this should not include the task,
		// but should not error either. When checking dependencies, it simply
		// skips tasks whose requester doesn't apply, so it should probably just
		// return false and no error here.
		di.included[pair] = false
		return false, errors.Errorf("task '%s' in variant '%s' cannot be run for a '%s'", pair.TaskName, pair.Variant, di.requester)
	}

	if bvt.IsDisabled() {
		di.included[pair] = false
		return false, nil
	}

	di.included[pair] = true

	// queue up all dependencies for recursive inclusion
	deps := di.expandDependencies(pair, bvt.DependsOn)

	// If this function is invoked from generate.tasks, calculate task/variant pairs
	// that were pulled in only as dependencies of inactive generated tasks so they can
	// be marked inactive. Also record roots that activate by default so they are not
	// left inactive when another generated task (incorrectly) marked them as dependency-only.
	pairSpecifiesActivation := activationInfo.taskOrVariantHasSpecificActivation(pair.Variant, pair.TaskName)
	if isRoot && generatedVariants != nil && !pairSpecifiesActivation {
		di.updateDeactivationMap(pair, false)
	}
	catcher := grip.NewBasicCatcher()
	for _, dep := range deps {
		// Since the only tasks that have activation info set are the initial unexpanded dependencies, we only need
		// to propagate the deactivateGeneratedDeps for those tasks, which only exist at the root level of each recursion.
		// Hence, if isRoot is true, we updateDeactivationMap for the full recursive set of dependencies of the task.
		if isRoot {
			propagate := !pairSpecifiesActivation || di.inactiveGeneratedRootMarksDepsInactive(pair)
			if propagate {
				di.updateDeactivationMap(dep, pairSpecifiesActivation)
			}
		}
		ok, err := di.handle(dep, activationInfo, generatedVariants, false)
		if !ok {
			di.included[pair] = false
			catcher.Wrapf(err, "task '%s' in variant '%s' has an unschedulable dependency", pair.TaskName, pair.Variant)
		}
	}

	if catcher.HasErrors() {
		return false, catcher.Resolve()
	}

	// we've reached a point where we know it is safe to include the current task
	return true, nil
}

// inactiveGeneratedRootMarksDepsInactive is true when the root is inactive only
// because activate was set false on the task or variant (generate.tasks or static
// config), not because of batchtime, cron, or disable. In that case dependency-only
// tasks should inherit inactive; batchtime-style inactivity must not pull
// prerequisites into deactivateGeneratedDeps or normal deps would never activate.
func (di *dependencyIncluder) inactiveGeneratedRootMarksDepsInactive(pair TVPair) bool {
	bvt := di.Project.FindTaskForVariant(pair.TaskName, pair.Variant)
	if bvt == nil {
		return false
	}
	if bvt.BatchTime != nil || bvt.CronBatchTime != "" || utility.FromBoolPtr(bvt.Disable) {
		return false
	}
	bv := di.Project.FindBuildVariant(pair.Variant)
	if bv != nil && (bv.BatchTime != nil || bv.CronBatchTime != "" || utility.FromBoolPtr(bv.Disable)) {
		return false
	}
	taskExplicitDeactivate := bvt.Activate != nil && !utility.FromBoolPtr(bvt.Activate)
	variantExplicitDeactivate := bv != nil && bv.Activate != nil && !utility.FromBoolPtr(bv.Activate)
	return taskExplicitDeactivate || variantExplicitDeactivate
}

func (di *dependencyIncluder) updateDeactivationMap(pair TVPair, pairSpecifiesActivation bool) {
	// deactivateGeneratedDeps[pair]==true means the task should be treated as having
	// specific activation (inactive until batch/cron/elapsed). false is the default
	// when the key is absent; we only store false to record "must activate" for a pair
	// that is also a generated root so it wins over a dependency-only inactive edge.
	// Inactive wins: never downgrade true to false; never recurse with false or we would
	// clear transitive batchtime/cron dependencies that must stay inactive.
	if pairSpecifiesActivation {
		di.deactivateGeneratedDeps[pair] = true
		di.recursivelyUpdateDeactivationMap(pair, map[TVPair]bool{})
		return
	}
	if _, ok := di.deactivateGeneratedDeps[pair]; !ok {
		di.deactivateGeneratedDeps[pair] = false
	}
}

// recursivelyUpdateDeactivationMap marks all transitive dependencies inactive for
// generate.tasks (they inherit the inactive root); only the inactive case recurses.
func (di *dependencyIncluder) recursivelyUpdateDeactivationMap(pair TVPair, dependencyIncluded map[TVPair]bool) {
	// If we've been here before, return early to avoid infinite recursion and extra work.
	if dependencyIncluded[pair] {
		return
	}
	dependencyIncluded[pair] = true
	bvt := di.Project.FindTaskForVariant(pair.TaskName, pair.Variant)
	if bvt != nil {
		deps := di.expandDependencies(pair, bvt.DependsOn)
		for _, dep := range deps {
			di.deactivateGeneratedDeps[dep] = true
			di.recursivelyUpdateDeactivationMap(dep, dependencyIncluded)
		}
	}
}

// expandDependencies finds all tasks depended on by the current task/variant pair.
func (di *dependencyIncluder) expandDependencies(pair TVPair, depends []TaskUnitDependency) []TVPair {
	deps := []TVPair{}
	for _, d := range depends {
		// don't automatically add dependencies if they are marked patch_optional
		if d.PatchOptional {
			continue
		}
		switch {
		case d.Variant == AllVariants && d.Name == AllDependencies: // task = *, variant = *
			// Here we get all variants and tasks (excluding the current task)
			// and add them to the list of tasks and variants.
			for _, v := range di.Project.BuildVariants {
				for _, t := range v.Tasks {
					if t.Name == pair.TaskName && v.Name == pair.Variant {
						continue
					}
					projectTask := di.Project.FindTaskForVariant(t.Name, v.Name)
					if projectTask != nil {
						if projectTask.IsDisabled() || projectTask.SkipOnRequester(di.requester) {
							continue
						}
						deps = append(deps, TVPair{TaskName: t.Name, Variant: v.Name})
					}
				}
			}

		case d.Variant == AllVariants: // specific task, variant = *
			// In the case where we depend on a task on all variants, we fetch the task's
			// dependencies, then add that task for all variants that have it.
			for _, v := range di.Project.BuildVariants {
				for _, t := range v.Tasks {
					if t.Name == pair.TaskName && v.Name == pair.Variant {
						continue
					}

					if t.IsGroup {
						if !di.dependencyMatchesTaskGroupTask(pair, t, d) {
							continue
						}
					} else if t.Name != d.Name {
						continue
					}

					projectTask := di.Project.FindTaskForVariant(t.Name, v.Name)
					if projectTask != nil {
						if projectTask.IsDisabled() || projectTask.SkipOnRequester(di.requester) {
							continue
						}
						if !t.IsGroup {
							deps = append(deps, TVPair{TaskName: t.Name, Variant: v.Name})
						} else {
							deps = append(deps, TVPair{TaskName: d.Name, Variant: v.Name})
						}
					}
				}
			}

		case d.Name == AllDependencies: // task = *, specific variant
			// Here we add every task for a single variant. We add the dependent variant,
			// then add all of that variant's task, as well as their dependencies.
			v := d.Variant
			if v == "" {
				v = pair.Variant
			}
			variant := di.Project.FindBuildVariant(v)
			if variant != nil {
				for _, t := range variant.Tasks {
					if t.Name == pair.TaskName && variant.Name == pair.Variant {
						continue
					}
					projectTask := di.Project.FindTaskForVariant(t.Name, v)
					if projectTask != nil {
						if projectTask.IsDisabled() || projectTask.SkipOnRequester(di.requester) {
							continue
						}
						deps = append(deps, TVPair{TaskName: t.Name, Variant: variant.Name})
					}
				}
			}

		default: // specific name, specific variant
			// We simply add a single task/variant and its dependencies. This does not do
			// the requester check above because we assume the user has configured this
			// correctly
			v := d.Variant
			if v == "" {
				v = pair.Variant
			}
			projectTask := di.Project.FindTaskForVariant(d.Name, v)
			if projectTask != nil && !projectTask.IsDisabled() {
				deps = append(deps, TVPair{TaskName: d.Name, Variant: v})
			}
		}
	}
	return deps
}

func (di *dependencyIncluder) dependencyMatchesTaskGroupTask(depSrc TVPair, bvt BuildVariantTaskUnit, dep TaskUnitDependency) bool {
	tg := di.Project.FindTaskGroup(bvt.Name)
	if tg == nil {
		return false
	}
	for _, tgTaskName := range tg.Tasks {
		if tgTaskName == depSrc.TaskName && bvt.Variant == depSrc.Variant {
			// Exclude self.
			continue
		}
		if tgTaskName == dep.Name {
			return true
		}
	}
	return false
}
