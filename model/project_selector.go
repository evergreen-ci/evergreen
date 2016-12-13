package model

import (
	"bytes"
	"fmt"
	"strings"

	"github.com/evergreen-ci/evergreen/util"
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
	if i := strings.IndexAny(sc.name, InvalidCriterionRunes); i == 0 {
		return fmt.Errorf("name starts with invalid character '%v'", sc.name[i])
	}
	if sc.name == SelectAll {
		if sc.tagged {
			return fmt.Errorf("cannot use '.' with special name '%v'", SelectAll)
		}
		if sc.negated {
			return fmt.Errorf("cannot use '!' with special name '%v'", SelectAll)
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

// the tagged interface allows the tagSelectorEvaluator to work for multiple types
type tagged interface {
	name() string
	tags() []string
}

// tagSelectorEvaluator evaluates selectors for arbitrary tagged items
type tagSelectorEvaluator struct {
	items  []tagged
	byName map[string]tagged
	byTag  map[string][]tagged
}

// newTagSelectorEvaluator returns a new taskSelectorEvaluator.
func newTagSelectorEvaluator(selectees []tagged) *tagSelectorEvaluator {
	// cache everything
	byName := map[string]tagged{}
	byTag := map[string][]tagged{}
	items := []tagged{}
	for _, s := range selectees {
		items = append(items, s)
		byName[s.name()] = s
		for _, tag := range s.tags() {
			byTag[tag] = append(byTag[tag], s)
		}
	}
	return &tagSelectorEvaluator{
		items:  items,
		byName: byName,
		byTag:  byTag,
	}
}

// evalSelector returns all names that fulfill a selector. This is done
// by evaluating each criterion individually and taking the intersection.
func (tse *tagSelectorEvaluator) evalSelector(s Selector) ([]string, error) {
	// keep a slice of results per criterion
	results := []string{}
	if len(s) == 0 {
		return nil, fmt.Errorf("cannot evaluate selector with no criteria")
	}
	for i, sc := range s {
		names, err := tse.evalCriterion(sc)
		if err != nil {
			return nil, fmt.Errorf("%v: %v", s, err)
		}
		if i == 0 {
			results = names
		} else {
			// intersect all evaluated criteria
			results = util.StringSliceIntersection(results, names)
		}
	}
	if len(results) == 0 {
		return nil, fmt.Errorf("nothing satisfies selector '%v'", s)
	}
	return results, nil
}

// evalCriterion returns all names that fulfill a single selection criterion.
func (tse *tagSelectorEvaluator) evalCriterion(sc selectCriterion) ([]string, error) {
	switch {
	case sc.Validate() != nil:
		return nil, fmt.Errorf("criterion '%v' is invalid: %v", sc, sc.Validate())

	case sc.name == SelectAll: // special * case
		names := []string{}
		for _, item := range tse.items {
			names = append(names, item.name())
		}
		return names, nil

	case !sc.tagged && !sc.negated: // just a regular name
		item := tse.byName[sc.name]
		if item == nil {
			return nil, fmt.Errorf("nothing named '%v'", sc.name)
		}
		return []string{item.name()}, nil

	case sc.tagged && !sc.negated: // expand a tag
		taggedItems := tse.byTag[sc.name]
		if len(taggedItems) == 0 {
			return nil, fmt.Errorf("nothing has the tag '%v'", sc.name)
		}
		names := []string{}
		for _, item := range taggedItems {
			names = append(names, item.name())
		}
		return names, nil

	case !sc.tagged && sc.negated: // everything *but* a specific item
		if tse.byName[sc.name] == nil {
			// we want to treat this as an error for better usability
			return nil, fmt.Errorf("nothing named '%v'", sc.name)
		}
		names := []string{}
		for _, item := range tse.items {
			if item.name() != sc.name {
				names = append(names, item.name())
			}
		}
		return names, nil

	case sc.tagged && sc.negated: // everything *but* a tag
		items := tse.byTag[sc.name]
		if len(items) == 0 {
			// we want to treat this as an error for better usability
			return nil, fmt.Errorf("nothing has the tag '%v'", sc.name)
		}
		illegalItems := map[string]bool{}
		for _, item := range items {
			illegalItems[item.name()] = true
		}
		names := []string{}
		// build slice of all items that aren't in the tag
		for _, item := range tse.items {
			if !illegalItems[item.name()] {
				names = append(names, item.name())
			}
		}
		return names, nil

	default:
		// protection for if we edit this switch block later
		panic("this should not be reachable")
	}
}

// Task Selector Logic

// taskSelectorEvaluator expands tags used in build variant definitions.
type taskSelectorEvaluator struct {
	tagEval *tagSelectorEvaluator
}

// NewParserTaskSelectorEvaluator returns a new taskSelectorEvaluator.
func NewParserTaskSelectorEvaluator(tasks []parserTask) *taskSelectorEvaluator {
	// convert tasks into interface slice and use the tagSelectorEvaluator
	var selectees []tagged
	for i := range tasks {
		selectees = append(selectees, &tasks[i])
	}
	return &taskSelectorEvaluator{
		tagEval: newTagSelectorEvaluator(selectees),
	}
}

// evalSelector returns all tasks selected by a selector.
func (t *taskSelectorEvaluator) evalSelector(s Selector) ([]string, error) {
	results, err := t.tagEval.evalSelector(s)
	if err != nil {
		return nil, fmt.Errorf("evaluating task selector: %v", err)
	}
	return results, nil
}

// Axis selector logic

// axisSelectorEvaluator expands tags used for selected matrix axis values
type axisSelectorEvaluator struct {
	axisEvals map[string]*tagSelectorEvaluator
}

func NewAxisSelectorEvaluator(axes []matrixAxis) *axisSelectorEvaluator {
	evals := map[string]*tagSelectorEvaluator{}
	// convert axis values into interface slices and use the tagSelectorEvaluator
	for i := range axes {
		var selectees []tagged
		for j := range axes[i].Values {
			selectees = append(selectees, &(axes[i].Values[j]))
		}
		evals[axes[i].Id] = newTagSelectorEvaluator(selectees)
	}
	return &axisSelectorEvaluator{
		axisEvals: evals,
	}
}

// evalSelector returns all variants selected by the selector.
func (ase *axisSelectorEvaluator) evalSelector(axis string, s Selector) ([]string, error) {
	tagEval, ok := ase.axisEvals[axis]
	if !ok {
		return nil, fmt.Errorf("axis '%v' does not exist", axis)
	}
	results, err := tagEval.evalSelector(s)
	if err != nil {
		return nil, fmt.Errorf("evaluating axis '%v' selector: %v", axis, err)
	}
	return results, nil
}

// Variant selector logic

// variantSelectorEvaluator expands tags used in build variant definitions.
type variantSelectorEvaluator struct {
	tagEval  *tagSelectorEvaluator
	axisEval *axisSelectorEvaluator
	variants []parserBV
}

// NewVariantSelectorEvaluator returns a new taskSelectorEvaluator.
func NewVariantSelectorEvaluator(variants []parserBV, ase *axisSelectorEvaluator) *variantSelectorEvaluator {
	// convert variants into interface slice and use the tagSelectorEvaluator
	var selectees []tagged
	for i := range variants {
		selectees = append(selectees, &variants[i])
	}
	return &variantSelectorEvaluator{
		tagEval:  newTagSelectorEvaluator(selectees),
		variants: variants,
		axisEval: ase,
	}
}

// evalSelector returns all variants selected by the selector.
func (v *variantSelectorEvaluator) evalSelector(vs *variantSelector) ([]string, error) {
	if vs == nil {
		return nil, fmt.Errorf("empty selector")
	}
	if vs.matrixSelector != nil {
		evaluatedSelector, errs := vs.matrixSelector.evaluatedCopy(v.axisEval)
		if len(errs) > 0 {
			return nil, fmt.Errorf(
				"errors evaluating variant selector %v: %v", vs.matrixSelector, errs)
		}
		results := []string{}
		// this could be sped up considerably with caching, but I doubt we'll need to
		for _, v := range v.variants {
			if v.matrixVal != nil && evaluatedSelector.contains(v.matrixVal) {
				results = append(results, v.Name)
			}
		}
		if len(results) == 0 {
			return nil, fmt.Errorf("variant selector %v returns no variants", vs.matrixSelector)
		}
		return results, nil
	}
	results, err := v.tagEval.evalSelector(ParseSelector(vs.stringSelector))
	if err != nil {
		return nil, fmt.Errorf("variant tag selector: %v", err)
	}
	return results, nil
}
