package model

import (
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type dependencyIncluder struct {
	Project                         *Project
	requester                       string
	included                        map[TVPair]bool
	deactivateGeneratedVariantTasks map[string]bool
}

// IncludeDependencies takes a project and a slice of variant/task pairs names
// and returns the expanded set of variant/task pairs to include all the dependencies/requirements
// for the given set of tasks.
// If any dependency is cross-variant, it will include the variant and task for that dependency.
// This function can return an error, but it should be treated as an informational warning
func IncludeDependencies(project *Project, tvpairs []TVPair, requester string, activationInfo *specificActivationInfo) ([]TVPair, error) {
	di := &dependencyIncluder{Project: project, requester: requester}
	return di.include(tvpairs, activationInfo)
}

// include crawls the tasks represented by the combination of variants and tasks and
// add or removes tasks based on the dependency graph. Dependent tasks
// are added; tasks that depend on unreachable tasks are pruned. New slices
// of variants and tasks are returned.
func (di *dependencyIncluder) include(initialDeps []TVPair, activationInfo *specificActivationInfo) ([]TVPair, error) {
	di.included = map[TVPair]bool{}
	di.deactivateGeneratedVariantTasks = map[string]bool{}
	warnings := grip.NewBasicCatcher()
	// handle each pairing, recursively adding and pruning based
	// on the task's dependencies
	for _, d := range initialDeps {
		_, err := di.handle(d, activationInfo)
		warnings.Add(err)
	}

	outPairs := []TVPair{}
	for pair, shouldInclude := range di.included {
		if shouldInclude {
			outPairs = append(outPairs, pair)
		}
	}
	// we deactivate variants that do not have specific activation set in the
	// generated project but have been generated as dependencies of other
	// explicitly inactive variants
	for variant, addToActivationInfo := range di.deactivateGeneratedVariantTasks {
		if addToActivationInfo {
			activationInfo.activationVariants = append(activationInfo.activationVariants, variant)
		}
	}
	return outPairs, warnings.Resolve()
}

// handle finds and includes all tasks that the given task/variant pair depends
// on. Returns true if the task and all of its dependent tasks can be scheduled
// for the requester. Returns false if it cannot be scheduled, with an error
// explaining why
func (di *dependencyIncluder) handle(pair TVPair, activationInfo *specificActivationInfo) (bool, error) {
	if included, ok := di.included[pair]; ok {
		// we've been here before, so don't redo work
		return included, nil
	}

	// if the given task is a task group, recurse on each task
	if tg := di.Project.FindTaskGroup(pair.TaskName); tg != nil {
		for _, t := range tg.Tasks {
			ok, err := di.handle(TVPair{TaskName: t, Variant: pair.Variant}, activationInfo)
			if !ok {
				di.included[pair] = false
				return false, errors.Wrapf(err, "task group '%s' in variant '%s' contains unschedulable task '%s'", pair.TaskName, pair.Variant, t)
			}
		}
		return true, nil
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
		di.included[pair] = false
		return false, errors.Errorf("task '%s' in variant '%s' cannot be run for a '%s'", pair.TaskName, pair.Variant, di.requester)
	}

	if bvt.IsDisabled() {
		di.included[pair] = false
		return false, errors.Errorf("task '%s' in variant '%s' has been disabled", pair.TaskName, pair.Variant)
	}

	di.included[pair] = true

	// queue up all dependencies for recursive inclusion
	deps := di.expandDependencies(pair, bvt.DependsOn)

	// If this function is invoked from generate.tasks, calculate all variants
	// that need to be included as dependencies, but are not in the generated project.
	// If all the task / variant pairs that spawn these dependencies are inactive, we
	// also mark this newly generated variant as inactive.
	pairSpecifiesActivation := activationInfo != nil && activationInfo.taskOrVariantHasSpecificActivation(pair.Variant, pair.TaskName)
	for _, dep := range deps {
		if activationInfo != nil && !activationInfo.variantExistsInGeneratedProject(dep.Variant) {
			// If the new variant has not yet been added to deactivateGeneratedVariantTasks, or if the
			// original pair needs to be active, we update deactivateGeneratedVariantTasks.
			// We ultimately will only deactivate new variants where deactivateGeneratedVariantTasks[variant] = true.
			// If deactivateGeneratedVariantTasks[variant] = false it signifies that there was at least
			// one pair that depends on this new variant being active - so we cannot deactivate it.
			if _, ok := di.deactivateGeneratedVariantTasks[dep.Variant]; !ok || !pairSpecifiesActivation {
				di.deactivateGeneratedVariantTasks[dep.Variant] = pairSpecifiesActivation
			}
		}
		ok, err := di.handle(dep, activationInfo)
		if !ok {
			di.included[pair] = false
			return false, errors.Wrapf(err, "task '%s' in variant '%s' has an unschedulable dependency", pair.TaskName, pair.Variant)
		}
	}

	// we've reached a point where we know it is safe to include the current task
	return true, nil
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
					if t.Name != d.Name {
						continue
					}
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
					if t.Name == pair.TaskName {
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
