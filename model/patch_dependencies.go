package model

import (
	"github.com/10gen-labs/slogger/v1"
	"github.com/evergreen-ci/evergreen"
)

type dependencyIncluder struct {
	Project  *Project
	included map[TVPair]bool
}

type patchDep struct {
	Variant string
	BuildVariantTask
}

// Include crawls the tasks represented by the combination of variants and tasks and
// add or removes tasks based on the dependency graph. Required and dependent tasks
// are added; tasks that depend on unpatchable tasks are pruned. New slices
// of variants and tasks are returned.
func (di *dependencyIncluder) Include(variants, tasks []string) ([]string, []string) {
	di.included = map[TVPair]bool{}
	// we get all the BuildVariantTasks requested by the patch,
	// so we can iterate through them at the finest grain.
	initialDeps := []TVPair{}
	for _, v := range variants {
		for _, t := range di.Project.FindTasksForVariant(v) {
			for _, task := range tasks {
				if t == task {
					initialDeps = append(initialDeps, TVPair{TaskName: t, Variant: v})
				}
			}
		}
	}

	// handle each pairing, recursively adding and pruning based
	// on the task's requirements and dependencies
	for _, d := range initialDeps {
		di.handle(d)
	}

	// rewrite the list of task/variant pairs to create as a single list
	// of tasks and a single list of variants.
	// TODO: this can be removed to enable support for T-shaped patches!
	var outTasks, outVariants []string
	outTasksMap := map[string]struct{}{}
	outVariantsMap := map[string]struct{}{}
	for pair, ok := range di.included {
		if ok {
			outTasksMap[pair.TaskName] = struct{}{}
			outVariantsMap[pair.Variant] = struct{}{}
		}
	}
	for t := range outTasksMap {
		outTasks = append(outTasks, t)
	}
	for v := range outVariantsMap {
		outVariants = append(outVariants, v)
	}
	return outVariants, outTasks

}

// handle finds and includes all tasks that the given task/variant pair
// requires or depends on. Returns true if the task and all of its
// dependent/required tasks are patchable, false if they are not.
func (di *dependencyIncluder) handle(pair TVPair) bool {
	if included, ok := di.included[pair]; ok {
		// we've been here before, so don't redo work
		return included
	}

	// we must load the BuildVariantTask for the task/variant pair,
	// since it contains the full scope of dependency information
	bvt := di.Project.FindTaskForVariant(pair.TaskName, pair.Variant)
	if bvt == nil {
		evergreen.Logger.Logf(slogger.ERROR, "task %v does not exist in project %v",
			pair.TaskName, di.Project.Identifier)
		di.included[pair] = false
		return false // task not found in project--skip it.
	}
	if patchable := bvt.Patchable; patchable != nil && !*patchable {
		di.included[pair] = false
		return false // task cannot be patched, so skip it
	}
	di.included[pair] = true

	// queue up all requirements and dependencies for recursive inclusion
	deps := append(
		di.expandRequirements(pair, bvt.Requires),
		di.expandDependencies(pair, bvt.DependsOn)...)
	for _, dep := range deps {
		if ok := di.handle(dep); !ok {
			di.included[pair] = false
			return false // task depends on an unpatchable task, so skip it
		}
	}

	// we've reached a point where we know it is safe to include the current task
	return true
}

// expandRequirements finds all tasks required by the current task/variant pair.
func (di *dependencyIncluder) expandRequirements(pair TVPair, reqs []TaskRequirement) []TVPair {
	deps := []TVPair{}
	for _, r := range reqs {
		if r.Variant == AllVariants {
			// the case where we depend on all variants for a task
			for _, v := range di.Project.FindVariantsWithTask(r.Name) {
				if v != pair.Variant { // skip current variant
					deps = append(deps, TVPair{TaskName: r.Name, Variant: v})
				}
			}
		} else {
			// otherwise we're depending on a single task for a single variant
			// We simply add a single task/variant and its dependencies.
			v := r.Variant
			if v == "" {
				v = pair.Variant
			}
			deps = append(deps, TVPair{TaskName: r.Name, Variant: v})
		}
	}
	return deps
}

// expandRequirements finds all tasks depended on by the current task/variant pair.
func (di *dependencyIncluder) expandDependencies(pair TVPair, depends []TaskDependency) []TVPair {
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
			for _, v := range di.Project.FindAllVariants() {
				for _, t := range di.Project.FindTasksForVariant(v) {
					if !(t == pair.TaskName && v == pair.Variant) {
						deps = append(deps, TVPair{TaskName: t, Variant: v})
					}
				}
			}

		case d.Variant == AllVariants: // specific task, variant = *
			// In the case where we depend on a task on all variants, we fetch the task's
			// dependencies, then add that task for all variants that have it.
			for _, v := range di.Project.FindVariantsWithTask(d.Name) {
				if !(pair.TaskName == d.Name && pair.Variant == v) {
					deps = append(deps, TVPair{TaskName: d.Name, Variant: v})
				}
			}

		case d.Name == AllDependencies: // task = *, specific variant
			// Here we add every task for a single variant. We add the dependent variant,
			// then add all of that variant's task, as well as their dependencies.
			v := d.Variant
			if v == "" {
				v = pair.Variant
			}
			for _, t := range di.Project.FindTasksForVariant(v) {
				if !(pair.TaskName == t && pair.Variant == v) {
					deps = append(deps, TVPair{TaskName: t, Variant: v})
				}
			}

		default: // specific name, specific variant
			// We simply add a single task/variant and its dependencies.
			v := d.Variant
			if v == "" {
				v = pair.Variant
			}
			deps = append(deps, TVPair{TaskName: d.Name, Variant: v})
		}
	}
	return deps
}
