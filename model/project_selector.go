package model

import (
	"bytes"
	"fmt"
	"github.com/evergreen-ci/evergreen/util"
	"reflect"
	"strings"
)

// Selectors are used in a project file to select groups of tasks/axes based on user-defined tags.
// Selection syntax is currently defined as a whitespace-delimited set of criteria, where each
// criterion is a different name or tag with optional modifiers.
// Formally, we define the syntax as:
//   Selector := [whitespace-delimited list of Criterion]
//   Criterion :=  (optional ! rune)(optional . rune)<Name>
//     where "!" specifies a negation of the criteria and "." specifies a tag as opposed to a name
//   Name := <any string>
//     excluding whitespace, '.', and '!'
//
// Selectors return all items that satisfy all of the criteria. That is, they return the intersection
// of each individual criterion.
//
// For example:
//   "red" would return the item named "red"
//   ".primary" would return all items with the tag "primary"
//   "!.primary" would return all items that are NOT tagged "primary"
//   ".cool !blue" would return all items that are tagged "cool" and NOT named "blue"

const (
	SelectAll             = "*"
	InvalidCriterionRunes = "!."
)

// Selector holds the information necessary to build a set of elements
// based on name and tag combinations.
type Selector []selectCriterion

// String returns a readable representation of the Selector.
func (s Selector) String() string {
	buf := bytes.Buffer{}
	for i, sc := range s {
		if i > 0 {
			buf.WriteRune(' ')
		}
		buf.WriteString(sc.String())
	}
	return buf.String()
}

// selectCriterions are intersected to form the results of a selector.
type selectCriterion struct {
	name string

	// modifiers
	tagged  bool
	negated bool
}

// String returns a readable representation of the criterion.
func (sc selectCriterion) String() string {
	buf := bytes.Buffer{}
	if sc.negated {
		buf.WriteRune('!')
	}
	if sc.tagged {
		buf.WriteRune('.')
	}
	buf.WriteString(sc.name)
	return buf.String()
}

// Validate returns nil if the selectCriterion is valid,
// or an error describing why it is invalid.
func (sc selectCriterion) Validate() error {
	if sc.name == "" {
		return fmt.Errorf("name is empty")
	}
	if i := strings.IndexAny(sc.name, InvalidCriterionRunes); i != -1 {
		return fmt.Errorf("name contains invalid character '%v'", sc.name[i])
	}
	if sc.name == SelectAll {
		if sc.tagged {
			return fmt.Errorf("cannot use '.' with special name 'v'", SelectAll)
		}
		if sc.negated {
			return fmt.Errorf("cannot use '!' with special name 'v'", SelectAll)
		}
	}
	return nil
}

// ParseSelector reads in a set of selection criteria defined as a string.
// This function only parses; it does not evaluate.
// Returns nil on an empty selection string.
func ParseSelector(s string) Selector {
	var criteria []selectCriterion
	// read the white-space delimited criteria
	critStrings := strings.Fields(s)
	for _, c := range critStrings {
		criteria = append(criteria, stringToCriterion(c))
	}
	return criteria
}

// stringToCriterion parses out a single criterion.
// This helper assumes that s != "".
func stringToCriterion(s string) selectCriterion {
	sc := selectCriterion{}
	if len(s) > 0 && s[0] == '!' { // negation
		sc.negated = true
		s = s[1:]
	}
	if len(s) > 0 && s[0] == '.' { // tags
		sc.tagged = true
		s = s[1:]
	}
	sc.name = s
	return sc
}

// Task Selector Logic

// taskSelectorEvaluator expands tags used in build variant definitions.
type taskSelectorEvaluator struct {
	tasks  []ProjectTask
	byName map[string]*ProjectTask
	byTag  map[string][]*ProjectTask
}

// NewTaskSelectorEvaluator returns a new taskSelectorEvaluator.
func NewTaskSelectorEvaluator(tasks []ProjectTask) *taskSelectorEvaluator {
	// cache everything
	byName := map[string]*ProjectTask{}
	byTag := map[string][]*ProjectTask{}
	for i, t := range tasks {
		byName[t.Name] = &tasks[i]
		for _, tag := range t.Tags {
			byTag[tag] = append(byTag[tag], &tasks[i])
		}
	}
	return &taskSelectorEvaluator{
		tasks:  tasks,
		byName: byName,
		byTag:  byTag,
	}
}

// EvaluateTasks expands the selectors in the given BuildVariantTask definitions, removing duplicates.
func (tse *taskSelectorEvaluator) EvaluateTasks(bvTasks []BuildVariantTask) ([]BuildVariantTask, error) {
	// Evaluate all selectors while merging, avoiding duplicates. We use a slice
	// along a map so that we can maintain some basic ordering for better UX.
	newTasks := []BuildVariantTask{}
	newTasksByName := map[string]BuildVariantTask{}
	// TODO accumulate a list of validation errors for each bvt
	// once the validator package is broken up
	for _, bvt := range bvTasks {
		if bvt.Name == "" {
			return nil, fmt.Errorf("task has empty name")
		}
		names, err := tse.evalSelector(ParseSelector(bvt.Name))
		if err != nil {
			return nil, err
		}
		// create new BuildVariantTask definitions, throwing away duplicates
		for _, name := range names {
			// create a newTask by copying the BuildVariantTask that selected it,
			// so we can preserve the other fields (Distros, Priority, etc).
			newTask := bvt
			newTask.Name = name
			// dive into the variant-specific task dependencies
			newTask.DependsOn, err = tse.EvaluateDeps(newTask.DependsOn)
			if err != nil {
				return nil, fmt.Errorf("error expanding dependency for task '%v': %v", name, err)
			}
			// add the new task if it doesn't already exists (we must avoid duplicates)
			if oldTask, ok := newTasksByName[name]; !ok {
				// not a duplicate--add it!
				newTasks = append(newTasks, newTask)
				newTasksByName[name] = newTask
			} else {
				// task already in the new list, so we check to make sure the definitions match.
				if !reflect.DeepEqual(newTask, oldTask) {
					return nil, fmt.Errorf(
						"conflicting definitions of task '%v': %v != %v", name, newTask, oldTask)
				}
			}
		}
	}
	return newTasks, nil
}

// EvaluateDeps expands selectors in the given dependency definitions.
func (tse *taskSelectorEvaluator) EvaluateDeps(deps []TaskDependency) ([]TaskDependency, error) {
	// This is almost an exact copy of EvaluateTasks.
	newDeps := []TaskDependency{}
	newDepsByName := map[string]TaskDependency{}
	// TODO accumulate a list of validation errors for each dep
	// once the validator package is broken up
	for _, d := range deps {
		if d.Name == "" {
			return nil, fmt.Errorf("dependency has empty name")
		}
		names, err := tse.evalSelector(ParseSelector(d.Name))
		if err != nil {
			return nil, err
		}
		// create new BuildVariantTask definitions, throwing away duplicates
		for _, name := range names {
			// create a newDep by copying the dep that selected it,
			// so we can preserve the "Variant" and "Status" field.
			newDep := d
			newDep.Name = name
			// add the new dep if it doesn't already exists (we must avoid duplicates)
			if oldDep, ok := newDepsByName[name]; !ok {
				// not a duplicate--add it!
				newDeps = append(newDeps, newDep)
				newDepsByName[name] = newDep
			} else {
				// dep already in the new list, so we check to make sure the definitions match.
				if !reflect.DeepEqual(newDep, oldDep) {
					return nil, fmt.Errorf(
						"conflicting definitions of dependency '%v': %v != %v", name, newDep, oldDep)
				}
			}
		}
	}
	return newDeps, nil
}

// evalSelector returns all task names that fulfil a selector. This is done
// by evaluating each criterion individually and taking the intersection.
func (tse *taskSelectorEvaluator) evalSelector(s Selector) ([]string, error) {
	// keep a slice of results per criterion
	results := []string{}
	if len(s) == 0 {
		return nil, fmt.Errorf("cannot evaluate selector with no criteria")
	}
	for i, sc := range s {
		taskNames, err := tse.evalCriterion(sc)
		if err != nil {
			return nil, fmt.Errorf("error evaluating '%v' selector: %v", s, err)
		}
		if i == 0 {
			results = taskNames
		} else {
			// intersect all evaluated criteria
			results = util.StringSliceIntersection(results, taskNames)
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("no tasks satisfy selector '%v'", s)
	}
	return results, nil
}

// evalCriterion returns all task names that fulfil a single selection criterion.
func (tse *taskSelectorEvaluator) evalCriterion(sc selectCriterion) ([]string, error) {
	switch {
	case sc.Validate() != nil:
		return nil, fmt.Errorf("criterion '%v' is invalid: %v", sc, sc.Validate())

	case sc.name == SelectAll: // special "All Tasks" case
		names := []string{}
		for _, task := range tse.tasks {
			names = append(names, task.Name)
		}
		return names, nil

	case !sc.tagged && !sc.negated: // just a regular name
		task := tse.byName[sc.name]
		if task == nil {
			return nil, fmt.Errorf("no task named '%v'", sc.name)
		}
		return []string{task.Name}, nil

	case sc.tagged && !sc.negated: // expand a tag
		tasks := tse.byTag[sc.name]
		if len(tasks) == 0 {
			return nil, fmt.Errorf("no tasks have the tag '%v'", sc.name)
		}
		names := []string{}
		for _, task := range tasks {
			names = append(names, task.Name)
		}
		return names, nil

	case !sc.tagged && sc.negated: // everything *but* a specific task
		if tse.byName[sc.name] == nil {
			// we want to treat this as an error for better usability
			return nil, fmt.Errorf("no task named '%v'", sc.name)
		}
		names := []string{}
		for _, task := range tse.tasks {
			if task.Name != sc.name {
				names = append(names, task.Name)
			}
		}
		return names, nil

	case sc.tagged && sc.negated: // everything *but* a tag
		tasks := tse.byTag[sc.name]
		if len(tasks) == 0 {
			// we want to treat this as an error for better usability
			return nil, fmt.Errorf("no tasks have the tag '%v'", sc.name)
		}
		// compare tasks by address to avoid the ones with a negated tag
		illegalTasks := map[*ProjectTask]bool{}
		for _, taskPtr := range tasks {
			illegalTasks[taskPtr] = true
		}
		names := []string{}
		for _, taskPtr := range tse.byName {
			if !illegalTasks[taskPtr] {
				names = append(names, taskPtr.Name)
			}
		}
		return names, nil

	default:
		// protection for if we edit this switch block later
		panic("this should not be reachable")
	}
}
