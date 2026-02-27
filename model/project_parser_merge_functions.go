package model

// Used to detect changes within the merge functions.
// Should only be modified in conjunction with ParserProject struct.

import (
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const MergeProjectConfigError = "error merging project configs"

// mergeUnorderedUnique merges fields that are lists where the order doesn't matter.
// These fields can be defined throughout multiple yamls but cannot contain duplicate keys.
// These fields are: [task, task group, parameter, module, function]
func (pp *ParserProject) mergeUnorderedUnique(toMerge *ParserProject) error {
	catcher := grip.NewBasicCatcher()

	taskNameExist := map[string]bool{}
	for _, task := range pp.Tasks {
		taskNameExist[task.Name] = true
	}
	for _, task := range toMerge.Tasks {
		if _, ok := taskNameExist[task.Name]; ok {
			catcher.Errorf("task '%s' has been declared already", task.Name)
			continue
		}
		pp.Tasks = append(pp.Tasks, task)
		taskNameExist[task.Name] = true
	}

	taskGroupNameExist := map[string]bool{}
	for _, taskGroup := range pp.TaskGroups {
		taskGroupNameExist[taskGroup.Name] = true
	}
	for _, taskGroup := range toMerge.TaskGroups {
		if _, ok := taskGroupNameExist[taskGroup.Name]; ok {
			catcher.Errorf("task group '%s' has been declared already", taskGroup.Name)
			continue
		}
		pp.TaskGroups = append(pp.TaskGroups, taskGroup)
		taskGroupNameExist[taskGroup.Name] = true
	}

	parameterKeyExist := map[string]bool{}
	for _, parameter := range pp.Parameters {
		parameterKeyExist[parameter.Key] = true
	}
	for _, parameter := range toMerge.Parameters {
		if _, ok := parameterKeyExist[parameter.Key]; ok {
			catcher.Errorf("parameter key '%s' has been declared already", parameter.Key)
			continue
		}
		pp.Parameters = append(pp.Parameters, parameter)
		parameterKeyExist[parameter.Key] = true
	}

	moduleExist := map[string]bool{}
	for _, module := range pp.Modules {
		moduleExist[module.Name] = true
	}
	for _, module := range toMerge.Modules {
		if _, ok := moduleExist[module.Name]; ok {
			catcher.Errorf("module '%s' has been declared already", module.Name)
			continue
		}
		pp.Modules = append(pp.Modules, module)
		moduleExist[module.Name] = true
	}

	for key, val := range toMerge.Functions {
		if _, ok := pp.Functions[key]; ok {
			catcher.Errorf("function '%s' has been declared already", key)
			continue
		}
		pp.Functions[key] = val
	}

	return catcher.Resolve()
}

// mergeUnordered merges fields that are lists where the order doesn't matter.
// These fields can only be defined in one yaml and does not consider naming conflicts.
// These fields include: [ignore, loggers]
func (pp *ParserProject) mergeUnordered(toMerge *ParserProject) {
	pp.Ignore = append(pp.Ignore, toMerge.Ignore...)
}

// mergeOrderedUnique merges fields that are lists where the order does matter.
// These fields can only be defined in one yaml.
// These fields are: [pre, post, timeout, early termination]
func (pp *ParserProject) mergeOrderedUnique(toMerge *ParserProject) error {
	catcher := grip.NewBasicCatcher()

	if pp.Pre != nil && toMerge.Pre != nil {
		catcher.New("pre can only be defined in one YAML")
	} else if toMerge.Pre != nil {
		pp.Pre = toMerge.Pre
	}

	if pp.Post != nil && toMerge.Post != nil {
		catcher.New("post can only be defined in one YAML")
	} else if toMerge.Post != nil {
		pp.Post = toMerge.Post
	}

	if pp.Timeout != nil && toMerge.Timeout != nil {
		catcher.New("timeout can only be defined in one YAML")
	} else if toMerge.Timeout != nil {
		pp.Timeout = toMerge.Timeout
	}

	return catcher.Resolve()
}

// mergeUnique merges fields that are non-lists across multiple project YAML
// files.
// These fields can only be defined in one yaml.
// These fields are: [stepback, batch time, pre/post timeout, pre/post error fails task, OOM tracker, ps, display name, command type, callback/exec timeout, task annotations, build baron]
func (pp *ParserProject) mergeUnique(toMerge *ParserProject) error {
	catcher := grip.NewBasicCatcher()

	if pp.Stepback != nil && toMerge.Stepback != nil {
		catcher.New("stepback can only be defined in one YAML")
	} else if toMerge.Stepback != nil {
		pp.Stepback = toMerge.Stepback
	}

	if pp.PreTimeoutSecs != nil && toMerge.PreTimeoutSecs != nil {
		catcher.New("pre timeout secs can only be defined in one YAML")
	} else if toMerge.PreTimeoutSecs != nil {
		pp.PreTimeoutSecs = toMerge.PreTimeoutSecs
	}

	if pp.PostTimeoutSecs != nil && toMerge.PostTimeoutSecs != nil {
		catcher.New("post timeout secs can only be defined in one YAML")
	} else if toMerge.PostTimeoutSecs != nil {
		pp.PostTimeoutSecs = toMerge.PostTimeoutSecs
	}

	if pp.PreErrorFailsTask != nil && toMerge.PreErrorFailsTask != nil {
		catcher.New("pre error fails task can only be defined in one YAML")
	} else if toMerge.PreErrorFailsTask != nil {
		pp.PreErrorFailsTask = toMerge.PreErrorFailsTask
	}

	if pp.PostErrorFailsTask != nil && toMerge.PostErrorFailsTask != nil {
		catcher.New("post error fails task can only be defined in one YAML")
	} else if toMerge.PostErrorFailsTask != nil {
		pp.PostErrorFailsTask = toMerge.PostErrorFailsTask
	}

	if pp.OomTracker != nil && toMerge.OomTracker != nil {
		catcher.New("OOM tracker can only be defined in one YAML")
	} else if toMerge.OomTracker != nil {
		pp.OomTracker = toMerge.OomTracker
	}

	if pp.Ps != nil && toMerge.Ps != nil {
		catcher.New("ps can only be defined in one YAML")
	} else if toMerge.Ps != nil {
		pp.Ps = toMerge.Ps
	}

	if pp.DisplayName != nil && toMerge.DisplayName != nil {
		catcher.New("display name can only be defined in one YAML")
	} else if toMerge.DisplayName != nil {
		pp.DisplayName = toMerge.DisplayName
	}

	if pp.CommandType != nil && toMerge.CommandType != nil {
		catcher.New("command type can only be defined in one YAML")
	} else if toMerge.CommandType != nil {
		pp.CommandType = toMerge.CommandType
	}

	if pp.CallbackTimeout != nil && toMerge.CallbackTimeout != nil {
		catcher.New("callback timeout can only be defined in one YAML")
	} else if toMerge.CallbackTimeout != nil {
		pp.CallbackTimeout = toMerge.CallbackTimeout
	}

	if pp.ExecTimeoutSecs != nil && toMerge.ExecTimeoutSecs != nil {
		catcher.New("exec timeout secs can only be defined in one YAML")
	} else if toMerge.ExecTimeoutSecs != nil {
		pp.ExecTimeoutSecs = toMerge.ExecTimeoutSecs
	}

	if pp.TimeoutSecs != nil && toMerge.TimeoutSecs != nil {
		catcher.New("timeout secs can only be defined in one YAML")
	} else if toMerge.TimeoutSecs != nil {
		pp.TimeoutSecs = toMerge.TimeoutSecs
	}

	if pp.DisableMergeQueuePathFiltering != nil && toMerge.DisableMergeQueuePathFiltering != nil {
		catcher.New("disable merge queue path filtering can only be defined in one YAML")
	} else if toMerge.DisableMergeQueuePathFiltering != nil {
		pp.DisableMergeQueuePathFiltering = toMerge.DisableMergeQueuePathFiltering
	}

	return catcher.Resolve()
}

// mergeBuildVariant merges build variants.
// Build variants can only be defined once but additional tasks can be added.
func (pp *ParserProject) mergeBuildVariant(toMerge *ParserProject) error {
	catcher := grip.NewBasicCatcher()

	bvs := map[string]parserBV{}
	for _, bv := range pp.BuildVariants {
		newBv := bv
		bvs[bv.Name] = newBv
	}
	for _, bv := range toMerge.BuildVariants {
		if currentBV, ok := bvs[bv.Name]; ok {
			if !currentBV.canMerge() && !bv.canMerge() {
				catcher.Errorf("build variant '%s' has non-task fields declared already", bv.Name)
			} else {
				currentBV.Tasks = append(bvs[bv.Name].Tasks, bv.Tasks...)
				currentBV.DisplayTasks = append(bvs[bv.Name].DisplayTasks, bv.DisplayTasks...)
				bvs[bv.Name] = currentBV
			}
		} else {
			bvs[bv.Name] = bv
		}
	}
	pp.BuildVariants = make([]parserBV, 0, len(bvs))
	for _, bv := range bvs {
		pp.BuildVariants = append(pp.BuildVariants, bv)
	}

	return catcher.Resolve()
}

// mergeMatrix merges matices/axes.
// Matices/axes cannot be defined for more than one yaml.
func (pp *ParserProject) mergeMatrix(toMerge *ParserProject) error {
	catcher := grip.NewBasicCatcher()

	if pp.Axes != nil && toMerge.Axes != nil {
		catcher.New("matrixes can only be defined in one YAML")
	} else if toMerge.Axes != nil {
		pp.Axes = toMerge.Axes
	}

	return catcher.Resolve()
}

func (pp *ParserProject) mergeMultipleParserProjects(toMerge *ParserProject) error {
	catcher := grip.NewBasicCatcher()

	// Unordered list, consider naming conflict
	err := pp.mergeUnorderedUnique(toMerge)
	if err != nil {
		catcher.Add(err)
	}

	// Unordered list, don't consider naming conflict
	pp.mergeUnordered(toMerge)

	// Ordered list, cannot be defined for more than one yaml
	err = pp.mergeOrderedUnique(toMerge)
	if err != nil {
		catcher.Add(err)
	}

	// Non-list, cannot be defined for more than one yaml
	err = pp.mergeUnique(toMerge)
	if err != nil {
		catcher.Add(err)
	}

	// Build variant, can only be defined once except to add tasks
	err = pp.mergeBuildVariant(toMerge)
	if err != nil {
		catcher.Add(err)
	}

	// Matrices, can only be defined for one yaml
	err = pp.mergeMatrix(toMerge)
	if err != nil {
		catcher.Add(err)
	}

	return errors.Wrap(catcher.Resolve(), MergeProjectConfigError)
}
