package model

import (
	"bytes"
	"encoding/json"
	"fmt"

	"github.com/evergreen-ci/evergreen/command"
	"github.com/evergreen-ci/evergreen/util"
)

// This file contains the code for matrix generation.
// Project matrices are a shortcut for defining many variants
// by combining multiple axes. A full explanation of the matrix format
// is available at #TODO GitHub wiki documentation (EVG-1175).
//
// On a high level, matrix variant construction takes the following steps:
//   1. Matrix definitions are read as part of a project's `buildvariants` field
//  and moved into a separate "matrices" slice.
//   2. A tag selector evaluator is constructed for evaluating axis selectors
//   3. The matrix and axis definitions are passed to buildMatrixVariants, which
//  creates all combinations of matrix cells and removes excluded ones.
//   4. During the generation of a single cell, we merge all axis values for the cell
//  together to create a fully filled-in variant. Matrix rules concerning non-task settings
//  are evaluated as well. Rules `add_tasks` and `remove_tasks` are stored in the variant
//  for later evaluation.
//   5. Created variants are appended back to the project's list of buildvariants.
//   6. During evaluateBuildVariants in project_parser.go, rules are executed.

// matrix defines a set of variants programmatically by
// combining a series of axis values and rules.
type matrix struct {
	Id          string            `yaml:"matrix_name"`
	Spec        matrixDefinition  `yaml:"matrix_spec"`
	Exclude     matrixDefinitions `yaml:"exclude_spec"`
	DisplayName string            `yaml:"display_name"`
	Tags        parserStringSlice `yaml:"tags"`
	Modules     parserStringSlice `yaml:"modules"`
	BatchTime   *int              `yaml:"batchtime"`
	Stepback    *bool             `yaml:"stepback"`
	RunOn       parserStringSlice `yaml:"run_on"`
	Tasks       parserBVTasks     `yaml:"tasks"`
	Rules       []matrixRule      `yaml:"rules"`
}

// matrixAxis represents one axis of a matrix definition.
type matrixAxis struct {
	Id          string      `yaml:"id"`
	DisplayName string      `yaml:"display_name"`
	Values      []axisValue `yaml:"values"`
}

// find returns the axisValue with the given name.
func (ma matrixAxis) find(id string) (axisValue, error) {
	for _, v := range ma.Values {
		if v.Id == id {
			return v, nil
		}
	}
	return axisValue{}, fmt.Errorf("axis '%v' does not contain value '%v'", ma.Id, id)
}

// axisValues make up the "points" along a matrix axis. Values are
// combined during matrix evaluation to produce new variants.
type axisValue struct {
	Id          string             `yaml:"id"`
	DisplayName string             `yaml:"display_name"`
	Variables   command.Expansions `yaml:"variables"`
	RunOn       parserStringSlice  `yaml:"run_on"`
	Tags        parserStringSlice  `yaml:"tags"`
	Modules     parserStringSlice  `yaml:"modules"`
	BatchTime   *int               `yaml:"batchtime"`
	Stepback    *bool              `yaml:"stepback"`
}

// helper methods for tag selectors
func (av *axisValue) name() string   { return av.Id }
func (av *axisValue) tags() []string { return av.Tags }

// matrixValue represents a "cell" of a matrix
type matrixValue map[string]string

// String returns the matrixValue in simple JSON format
func (mv matrixValue) String() string {
	asJSON, err := json.Marshal(&mv)
	if err != nil {
		return fmt.Sprintf("%#v", mv)
	}
	return string(asJSON)
}

// matrixDefinition is a map of axis name -> axis value representing
// an n-dimensional matrix configuration.
type matrixDefinition map[string]parserStringSlice

// String returns the matrixDefinition in simple JSON format
func (mdef matrixDefinition) String() string {
	asJSON, err := json.Marshal(&mdef)
	if err != nil {
		return fmt.Sprintf("%#v", mdef)
	}
	return string(asJSON)
}

// allCells returns every value (cell) within the matrix definition.
// IMPORTANT: this logic assume that all selectors have been evaluated
// and no duplicates exist.
func (mdef matrixDefinition) allCells() []matrixValue {
	// this should never happen, we handle empty defs but just for sanity
	if len(mdef) == 0 {
		return nil
	}
	// You can think of the logic below as traversing an n-dimensional matrix,
	// emulating an n-dimensional for-loop using a set of counters (like an old-school
	// golf counter).  We're doing this iteratively to avoid the overhead and sloppy code
	// required to constantly copy and merge maps that using recursion would require.
	type axisCache struct {
		Id    string
		Vals  []string
		Count int
	}
	axes := []axisCache{}
	for axis, values := range mdef {
		if len(values) == 0 {
			panic(fmt.Sprintf("axis '%v' has empty values list", axis))
		}
		axes = append(axes, axisCache{Id: axis, Vals: values})
	}
	carryOne := false
	cells := []matrixValue{}
	for {
		c := matrixValue{}
		for i := range axes {
			if carryOne {
				carryOne = false
				axes[i].Count = (axes[i].Count + 1) % len(axes[i].Vals)
				if axes[i].Count == 0 { // we overflowed--time to carry the one
					carryOne = true
				}
			}
			// set the current axis/value pair for the new cell
			c[axes[i].Id] = axes[i].Vals[axes[i].Count]
		}
		// if carryOne is still true, that means the final bucket overflowed--we've finished.
		if carryOne {
			break
		}
		cells = append(cells, c)
		// add one to the leftmost bucket on the next loop
		carryOne = true
	}
	return cells
}

// evaluatedCopy returns a copy of the definition with its tag selectors evaluated.
func (mdef matrixDefinition) evaluatedCopy(ase *axisSelectorEvaluator) (matrixDefinition, []error) {
	var errs []error
	cpy := matrixDefinition{}
	for axis, vals := range mdef {
		evaluated, evalErrs := evaluateAxisTags(ase, axis, vals)
		if len(evalErrs) > 0 {
			errs = append(errs, evalErrs...)
			continue
		}
		cpy[axis] = evaluated
	}
	return cpy, errs
}

// contains returns whether a value is contained by a definition.
// Note that a value that doesn't contain every matrix axis will still
// be evaluated based on the axes that exist.
func (mdef matrixDefinition) contains(mv matrixValue) bool {
	for k, v := range mv {
		axis, ok := mdef[k]
		if !ok {
			return false
		}
		if !util.SliceContains(axis, v) {
			return false
		}
	}
	return true
}

// matrixDefintinos is a helper type for parsing either a single definition
// or a slice of definitions from YAML.
type matrixDefinitions []matrixDefinition

// UnmarshalYAML allows the YAML parser to read both a single def or
// an array of them into a slice.
func (mds *matrixDefinitions) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var single matrixDefinition
	if err := unmarshal(&single); err == nil {
		*mds = matrixDefinitions{single}
		return nil
	}
	var slice []matrixDefinition
	if err := unmarshal(&slice); err != nil {
		return err
	}
	*mds = slice
	return nil
}

// contain returns true if *any* of the definitions contain the given value.
func (mds matrixDefinitions) contain(v matrixValue) bool {
	for _, m := range mds {
		if m.contains(v) {
			return true
		}
	}
	return false
}

// evaluatedCopies is like evaluatedCopy, but for multiple definitions.
func (mds matrixDefinitions) evaluatedCopies(ase *axisSelectorEvaluator) (matrixDefinitions, []error) {
	var out matrixDefinitions
	var errs []error
	for _, md := range mds {
		evaluated, evalErrs := md.evaluatedCopy(ase)
		errs = append(errs, evalErrs...)
		out = append(out, evaluated)
	}
	return out, errs
}

// evaluateAxisTags returns an evaluated list of axis value ids with tag selectors evaluated.
func evaluateAxisTags(ase *axisSelectorEvaluator, axis string, selectors []string) ([]string, []error) {
	var errs []error
	all := map[string]struct{}{}
	for _, s := range selectors {
		ids, err := ase.evalSelector(axis, ParseSelector(s))
		if err != nil {
			errs = append(errs, err)
			continue
		}
		for _, id := range ids {
			all[id] = struct{}{}
		}
	}
	out := []string{}
	for id, _ := range all {
		out = append(out, id)
	}
	return out, errs
}

// buildMatrixVariants takes in a list of axis definitions, an axisSelectorEvaluator, and a slice of
// matrix definitions. It returns a slice of parserBuildVariants constructed according to
// our matrix specification.
func buildMatrixVariants(axes []matrixAxis, ase *axisSelectorEvaluator, matrices []matrix) (
	[]parserBV, []error) {
	var errs []error
	// for each matrix, build out its declarations
	matrixVariants := []parserBV{}
	for i, m := range matrices {
		// for each axis value, iterate through possible inputs
		evaluatedSpec, evalErrs := m.Spec.evaluatedCopy(ase)
		if len(evalErrs) > 0 {
			errs = append(errs, evalErrs...)
			continue
		}
		evaluatedExcludes, evalErrs := m.Exclude.evaluatedCopies(ase)
		if len(evalErrs) > 0 {
			errs = append(errs, evalErrs...)
			continue
		}
		unpruned := evaluatedSpec.allCells()
		pruned := []parserBV{}
		for _, cell := range unpruned {
			// create the variant if it isn't excluded
			if !evaluatedExcludes.contain(cell) {
				v, err := buildMatrixVariant(axes, cell, &matrices[i], ase)
				if err != nil {
					errs = append(errs,
						fmt.Errorf("%v: error building matrix cell %v: %v", m.Id, cell, err))
					continue
				}
				pruned = append(pruned, *v)
			}
		}
		// safety check to make sure the exclude field is actually working
		if len(m.Exclude) > 0 && len(unpruned) == len(pruned) {
			errs = append(errs, fmt.Errorf("%v: exclude field did not exclude anything", m.Id))
		}
		matrixVariants = append(matrixVariants, pruned...)
	}
	return matrixVariants, errs
}

// buildMatrixVariant does the heavy lifting of building a matrix variant based on axis information.
// We do this by iterating over all axes and merging the axis value's settings when applicable. Expansions
// are evaluated during this process. Rules are parsed and added to the resulting parserBV for later
// execution.
func buildMatrixVariant(axes []matrixAxis, mv matrixValue, m *matrix, ase *axisSelectorEvaluator) (*parserBV, error) {
	v := parserBV{
		matrixVal:  mv,
		matrixId:   m.Id,
		Stepback:   m.Stepback,
		BatchTime:  m.BatchTime,
		Modules:    m.Modules,
		RunOn:      m.RunOn,
		Expansions: *command.NewExpansions(mv),
	}
	// we declare a separate expansion map for evaluating the display name
	displayNameExp := command.Expansions{}

	// build up the variant id while iterating through axis values
	idBuf := bytes.Buffer{}
	idBuf.WriteString(m.Id)
	idBuf.WriteString("__")

	// track how many axes we cover, so we know the value is only using real axes
	usedAxes := 0

	// we must iterate over axis definitions to have a consistent ordering for our axis priority
	for _, a := range axes {
		// skip any axes that aren't used in the variant's definition
		if _, ok := mv[a.Id]; !ok {
			continue
		}
		usedAxes++
		axisVal, err := a.find(mv[a.Id])
		if err != nil {
			return nil, err
		}
		if err := v.mergeAxisValue(axisVal); err != nil {
			return nil, fmt.Errorf("processing axis value %v,%v: %v", a.Id, axisVal.Id, err)
		}
		// for display names, fall back to the axis values id so we have *something*
		if axisVal.DisplayName != "" {
			displayNameExp.Put(a.Id, axisVal.DisplayName)
		} else {
			displayNameExp.Put(a.Id, axisVal.Id)
		}

		// append to the variant's name
		idBuf.WriteString(a.Id)
		idBuf.WriteRune('~')
		idBuf.WriteString(axisVal.Id)
		if usedAxes < len(mv) {
			idBuf.WriteRune('_')
		}
	}
	if usedAxes != len(mv) {
		// we could make this error more helpful at the expense of extra complexity
		return nil, fmt.Errorf("cell %v uses undefined axes", mv)
	}
	v.Name = idBuf.String()
	disp, err := displayNameExp.ExpandString(m.DisplayName)
	if err != nil {
		return nil, fmt.Errorf("processing display name: %v", err)
	}
	v.DisplayName = disp

	// add final matrix-level tags and tasks
	if err := v.mergeAxisValue(axisValue{Tags: m.Tags}); err != nil {
		return nil, fmt.Errorf("processing matrix tags: %v", err)
	}
	for _, t := range m.Tasks {
		expTask, err := expandParserBVTask(t, v.Expansions)
		if err != nil {
			return nil, fmt.Errorf("processing task %v: %v", t.Name, err)
		}
		v.Tasks = append(v.Tasks, expTask)
	}

	// evaluate rules for matching matrix values
	for i, rule := range m.Rules {
		r, err := expandRule(rule, v.Expansions)
		if err != nil {
			return nil, fmt.Errorf("processing rule[%v]: %v", i, err)
		}
		matchers, errs := r.If.evaluatedCopies(ase) // we could cache this
		if len(errs) > 0 {
			return nil, fmt.Errorf("evaluating rules for matrix %v: %v", m.Id, errs)
		}
		if matchers.contain(mv) {
			if r.Then.Set != nil {
				if err := v.mergeAxisValue(*r.Then.Set); err != nil {
					return nil, fmt.Errorf("evaluating %v rule %v: %v", m.Id, i, err)
				}
			}
			// we append add/remove task rules internally and execute them
			// during task evaluation, when other tasks are being evaluated.
			if len(r.Then.RemoveTasks) > 0 || len(r.Then.AddTasks) > 0 {
				v.matrixRules = append(v.matrixRules, r.Then)
			}
		}
	}
	return &v, nil
}

// matrixRule allows users to manipulate arbitrary matrix values using selectors.
type matrixRule struct {
	If   matrixDefinitions `yaml:"if"`
	Then ruleAction        `yaml:"then"`
}

// ruleAction is used to define what work must be done when
// "matrixRule.If" is satisfied.
type ruleAction struct {
	Set         *axisValue        `yaml:"set"`
	RemoveTasks parserStringSlice `yaml:"remove_tasks"`
	AddTasks    parserBVTasks     `yaml:"add_tasks"`
}

// mergeAxisValue overwrites a parserBV's fields based on settings
// in the axis value. Matrix expansions are evaluated as this process occurs.
// Returns any errors evaluating expansions.
func (pbv *parserBV) mergeAxisValue(av axisValue) error {
	// expand the variant's expansions (woah, dude) and update them
	if len(av.Variables) > 0 {
		expanded, err := expandExpansions(av.Variables, pbv.Expansions)
		if err != nil {
			return fmt.Errorf("expanding variables: %v", err)
		}
		pbv.Expansions.Update(expanded)
	}
	// merge tags, removing dupes
	if len(av.Tags) > 0 {
		expanded, err := expandStrings(av.Tags, pbv.Expansions)
		if err != nil {
			return fmt.Errorf("expanding tags: %v", err)
		}
		pbv.Tags = util.UniqueStrings(append(pbv.Tags, expanded...))
	}
	// overwrite run_on
	var err error
	if len(av.RunOn) > 0 {
		pbv.RunOn, err = expandStrings(av.RunOn, pbv.Expansions)
		if err != nil {
			return fmt.Errorf("expanding run_on: %v", err)
		}
	}
	// overwrite modules
	if len(av.Modules) > 0 {
		pbv.Modules, err = expandStrings(av.Modules, pbv.Expansions)
		if err != nil {
			return fmt.Errorf("expanding modules: %v", err)
		}
	}
	if av.Stepback != nil {
		pbv.Stepback = av.Stepback
	}
	if av.BatchTime != nil {
		pbv.BatchTime = av.BatchTime
	}
	return nil
}

// expandStrings expands a slice of strings.
func expandStrings(strings []string, exp command.Expansions) ([]string, error) {
	var expanded []string
	for _, s := range strings {
		newS, err := exp.ExpandString(s)
		if err != nil {
			return nil, err
		}
		expanded = append(expanded, newS)
	}
	return expanded, nil
}

// expandExpansions expands expansion maps.
func expandExpansions(in, exp command.Expansions) (command.Expansions, error) {
	newExp := command.Expansions{}
	for k, v := range in {
		newK, err := exp.ExpandString(k)
		if err != nil {
			return nil, err
		}
		newV, err := exp.ExpandString(v)
		if err != nil {
			return nil, err
		}
		newExp[newK] = newV
	}
	return newExp, nil
}

// expandParserBVTask expands strings inside parserBVTs.
func expandParserBVTask(pbvt parserBVTask, exp command.Expansions) (parserBVTask, error) {
	var err error
	newTask := pbvt
	newTask.Name, err = exp.ExpandString(pbvt.Name)
	if err != nil {
		return parserBVTask{}, fmt.Errorf("expanding name: %v", err)
	}
	newTask.RunOn, err = expandStrings(pbvt.RunOn, exp)
	if err != nil {
		return parserBVTask{}, fmt.Errorf("expanding run_on: %v", err)
	}
	newTask.Distros, err = expandStrings(pbvt.Distros, exp)
	if err != nil {
		return parserBVTask{}, fmt.Errorf("expanding distros: %v", err)
	}
	var newDeps parserDependencies
	for i, d := range pbvt.DependsOn {
		newDep := d
		newDep.Status, err = exp.ExpandString(d.Status)
		if err != nil {
			return parserBVTask{}, fmt.Errorf("expanding depends_on[%v].status: %v", i, err)
		}
		newDep.taskSelector, err = expandTaskSelector(d.taskSelector, exp)
		if err != nil {
			return parserBVTask{}, fmt.Errorf("expanding depends_on[%v]: %v", i, err)
		}
		newDeps = append(newDeps, newDep)
	}
	newTask.DependsOn = newDeps
	var newReqs taskSelectors
	for i, r := range pbvt.Requires {
		newReq, err := expandTaskSelector(r, exp)
		if err != nil {
			return parserBVTask{}, fmt.Errorf("expanding requires[%v]: %v", i, err)
		}
		newReqs = append(newReqs, newReq)
	}
	newTask.Requires = newReqs
	return newTask, nil
}

// expandTaskSelector expands strings inside task selectors.
func expandTaskSelector(ts taskSelector, exp command.Expansions) (taskSelector, error) {
	newTS := taskSelector{}
	newName, err := exp.ExpandString(ts.Name)
	if err != nil {
		return newTS, fmt.Errorf("expanding name: %v", err)
	}
	newTS.Name = newName
	if v := ts.Variant; v != nil {
		if len(v.matrixSelector) > 0 {
			newMS, err := expandMatrixDefinition(v.matrixSelector, exp)
			if err != nil {
				return newTS, fmt.Errorf("expanding variant: %v", err)
			}
			newTS.Variant = &variantSelector{
				matrixSelector: newMS,
			}
		} else {
			selector, err := exp.ExpandString(v.stringSelector)
			if err != nil {
				return newTS, fmt.Errorf("expanding variant: %v", err)
			}
			newTS.Variant = &variantSelector{
				stringSelector: selector,
			}
		}
	}
	return newTS, nil
}

// expandMatrixDefinition expands strings inside matrix definitions.
func expandMatrixDefinition(md matrixDefinition, exp command.Expansions) (matrixDefinition, error) {
	var err error
	newMS := matrixDefinition{}
	for axis, vals := range md {
		newMS[axis], err = expandStrings(vals, exp)
		if err != nil {
			return nil, fmt.Errorf("matrix selector: %v", err)
		}
	}
	return newMS, nil
}

// expandRules expands strings inside of rules.
func expandRule(r matrixRule, exp command.Expansions) (matrixRule, error) {
	newR := matrixRule{}
	for _, md := range r.If {
		newIf, err := expandMatrixDefinition(md, exp)
		if err != nil {
			return newR, fmt.Errorf("if: %v", err)
		}
		newR.If = append(newR.If, newIf)
	}
	for _, t := range r.Then.AddTasks {
		newTask, err := expandParserBVTask(t, exp)
		if err != nil {
			return newR, fmt.Errorf("add_tasks: %v", err)
		}
		newR.Then.AddTasks = append(newR.Then.AddTasks, newTask)
	}
	if len(r.Then.RemoveTasks) > 0 {
		var err error
		newR.Then.RemoveTasks, err = expandStrings(r.Then.RemoveTasks, exp)
		if err != nil {
			return newR, fmt.Errorf("remove_tasks: %v", err)
		}
	}
	// r.Then.Set will be taken care of when mergeAxisValue is called
	// so we don't have to do it in this function
	return newR, nil
}
