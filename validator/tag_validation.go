package validator

import (
	"slices"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
)

// TagValidationFailure describes a task being added to build variants by
// generate.tasks that doesn't have a tag with the required prefix.
type TagValidationFailure struct {
	// TaskName is the name of the task being added to build variants that fails
	// tag validation.
	TaskName string
	// BuildVariants are the build variants where this task is used.
	BuildVariants []string
}

// CheckGeneratedTaskTagValidation returns the tasks being added to build
// variants by generate.tasks whose tags do not match the tag validation prefix.
// Existing and merged are the translated project configurations before and
// after merging in the generated project configuration (i.e. a task/variant
// pair present only in merged was added by generate.tasks).
func CheckGeneratedTaskTagValidation(existing, merged *model.Project, prefix string) []TagValidationFailure {
	if prefix == "" {
		return nil
	}

	taskToFailingBVs := map[string][]string{}

	// checkValidTagsAndRecord checks if the task with the given name passes tag
	// validation and if not, records the task and build variant in
	// failingVariants.
	checkValidTagsAndRecord := func(project *model.Project, taskName, variantName string) {
		task := project.FindProjectTask(taskName)
		if task == nil {
			return
		}
		for _, tag := range task.Tags {
			if strings.HasPrefix(tag, prefix) {
				return
			}
		}
		taskToFailingBVs[taskName] = append(taskToFailingBVs[taskName], variantName)
	}

	existingPairs := make(map[model.TVPair]struct{})
	for _, bv := range existing.BuildVariants {
		for _, taskUnit := range bv.Tasks {
			existingPairs[model.TVPair{Variant: bv.Name, TaskName: taskUnit.Name}] = struct{}{}
		}
	}

	for _, bv := range merged.BuildVariants {
		for _, taskUnit := range bv.Tasks {
			if _, ok := existingPairs[model.TVPair{Variant: bv.Name, TaskName: taskUnit.Name}]; ok {
				continue
			}

			if taskUnit.IsGroup {
				// Task groups remain as a single unit in translated build
				// variants, so expand the task group to check each individual
				// task.
				taskGroup := merged.FindTaskGroup(taskUnit.Name)
				if taskGroup == nil {
					continue
				}
				for _, memberName := range taskGroup.Tasks {
					checkValidTagsAndRecord(merged, memberName, bv.Name)
				}
				continue
			}

			checkValidTagsAndRecord(merged, taskUnit.Name, bv.Name)
		}
	}
	if len(taskToFailingBVs) == 0 {
		return nil
	}

	failures := make([]TagValidationFailure, 0, len(taskToFailingBVs))
	for taskName, bvs := range taskToFailingBVs {
		slices.Sort(bvs)
		failures = append(failures, TagValidationFailure{
			TaskName:      taskName,
			BuildVariants: bvs,
		})
	}
	slices.SortFunc(failures, func(a, b TagValidationFailure) int {
		return strings.Compare(a.TaskName, b.TaskName)
	})
	return failures
}

// recordTagValidationFailure records the variant the named task is being added
// to if the task's definition exists and none of its own tags have the given
// prefix. Tasks with no definition are skipped; unmatched selectors are
// dropped during project translation, so such tasks should not appear here.
