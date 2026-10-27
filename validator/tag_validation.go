package validator

import (
	"slices"
	"strings"

	"github.com/evergreen-ci/evergreen/model"
)

// TagValidationFailure describes a task being added to build variants by
// generate.tasks that doesn't have a tag with the required prefix.
type TagValidationFailure struct {
	TaskName string
	Variants []string
}

// CheckGeneratedTaskTagValidation returns the tasks being added to build
// variants by generate.tasks whose own tags lack any tag with the given
// prefix, sorted by task name. Existing and merged are the translated project
// configurations before and after merging in the generated projects, so a
// task/variant pair present only in merged was added by generate.tasks.
func CheckGeneratedTaskTagValidation(existing, merged *model.Project, prefix string) []TagValidationFailure {
	if prefix == "" {
		return nil
	}

	existingPairs := make(map[model.TVPair]struct{})
	for _, bv := range existing.BuildVariants {
		for _, taskUnit := range bv.Tasks {
			existingPairs[model.TVPair{Variant: bv.Name, TaskName: taskUnit.Name}] = struct{}{}
		}
	}

	failingVariants := map[string][]string{}
	for _, bv := range merged.BuildVariants {
		for _, taskUnit := range bv.Tasks {
			if _, ok := existingPairs[model.TVPair{Variant: bv.Name, TaskName: taskUnit.Name}]; ok {
				continue
			}
			if taskUnit.IsGroup {
				// Task groups remain as a single unit in translated build variants, so the
				// group must be expanded to check each member task. The group's own tags do
				// not count towards validation; only each member task's own tags do.
				taskGroup := merged.FindTaskGroup(taskUnit.Name)
				if taskGroup == nil {
					continue
				}
				for _, memberName := range taskGroup.Tasks {
					recordTagValidationFailure(merged, memberName, bv.Name, prefix, failingVariants)
				}
				continue
			}
			recordTagValidationFailure(merged, taskUnit.Name, bv.Name, prefix, failingVariants)
		}
	}
	if len(failingVariants) == 0 {
		return nil
	}

	failures := make([]TagValidationFailure, 0, len(failingVariants))
	for taskName, variants := range failingVariants {
		slices.Sort(variants)
		failures = append(failures, TagValidationFailure{
			TaskName: taskName,
			Variants: variants,
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
func recordTagValidationFailure(project *model.Project, taskName, variantName, prefix string, failingVariants map[string][]string) {
	task := project.FindProjectTask(taskName)
	if task == nil {
		return
	}
	for _, tag := range task.Tags {
		if strings.HasPrefix(tag, prefix) {
			return
		}
	}
	failingVariants[taskName] = append(failingVariants[taskName], variantName)
}
