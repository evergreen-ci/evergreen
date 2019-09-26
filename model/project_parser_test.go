package model

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/mongodb/grip"
	"github.com/pkg/errors"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	yaml "gopkg.in/yaml.v2"

	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

// ShouldContainResembling tests whether a slice contains an element that DeepEquals
// the expected input.
func ShouldContainResembling(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 || expected == nil {
		return "ShouldContainResembling takes 1 argument"
	}

	v := reflect.ValueOf(actual)
	if v.Kind() != reflect.Slice {
		return fmt.Sprintf("Cannot call ExpectedContains on a non-expected %#v of kind %#v", expected, v.Kind().String())
	}
	for i := 0; i < v.Len(); i++ {
		if reflect.DeepEqual(v.Index(i).Interface(), expected[0]) {
			return ""
		}
	}

	return fmt.Sprintf("%#v does not contain %#v", actual, expected[0])
}

func TestCreateIntermediateProjectDependencies(t *testing.T) {
	Convey("Testing different project files", t, func() {
		Convey("a simple project file should parse", func() {
			simple := `
tasks:
- name: "compile"
- name: task0
- name: task1
  patchable: false
  tags: ["tag1", "tag2"]
  depends_on:
  - compile
  - name: "task0"
    status: "failed"
    patch_optional: true
`
			p, errs := createIntermediateProject([]byte(simple))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			So(p.Tasks[2].DependsOn[0].TaskSelector.Name, ShouldEqual, "compile")
			So(p.Tasks[2].DependsOn[0].PatchOptional, ShouldEqual, false)
			So(p.Tasks[2].DependsOn[1].TaskSelector.Name, ShouldEqual, "task0")
			So(p.Tasks[2].DependsOn[1].Status, ShouldEqual, "failed")
			So(p.Tasks[2].DependsOn[1].PatchOptional, ShouldEqual, true)
		})
		Convey("a file with a single dependency should parse", func() {
			single := `
tasks:
- name: "compile"
- name: task0
- name: task1
  depends_on: task0
`
			p, errs := createIntermediateProject([]byte(single))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			So(p.Tasks[2].DependsOn[0].TaskSelector.Name, ShouldEqual, "task0")
		})
		Convey("a file with a nameless dependency should error", func() {
			Convey("with a single dep", func() {
				nameless := `
tasks:
- name: "compile"
  depends_on: ""
`
				p, errs := createIntermediateProject([]byte(nameless))
				So(p, ShouldBeNil)
				So(len(errs), ShouldEqual, 1)
			})
			Convey("or multiple", func() {
				nameless := `
tasks:
- name: "compile"
  depends_on:
  - name: "task1"
  - status: "failed" #this has no task attached
`
				p, errs := createIntermediateProject([]byte(nameless))
				So(p, ShouldBeNil)
				So(len(errs), ShouldEqual, 1)
			})
			Convey("but an unused depends_on field should not error", func() {
				nameless := `
tasks:
- name: "compile"
`
				p, errs := createIntermediateProject([]byte(nameless))
				So(p, ShouldNotBeNil)
				So(len(errs), ShouldEqual, 0)
			})
		})
	})
}

func TestCreateIntermediateProjectRequirements(t *testing.T) {
	Convey("Testing different project files", t, func() {
		Convey("a simple project file should parse", func() {
			simple := `
tasks:
- name: task0
- name: task1
  requires:
  - name: "task0"
    variant: "v1"
  - "task2"
`
			p, errs := createIntermediateProject([]byte(simple))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			So(p.Tasks[1].Requires[0].Name, ShouldEqual, "task0")
			So(p.Tasks[1].Requires[0].Variant.StringSelector, ShouldEqual, "v1")
			So(p.Tasks[1].Requires[1].Name, ShouldEqual, "task2")
			So(p.Tasks[1].Requires[1].Variant, ShouldBeNil)
		})
		Convey("a single requirement should parse", func() {
			simple := `
tasks:
- name: task1
  requires:
    name: "task0"
    variant: "v1"
`
			p, errs := createIntermediateProject([]byte(simple))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			So(p.Tasks[0].Requires[0].Name, ShouldEqual, "task0")
			So(p.Tasks[0].Requires[0].Variant.StringSelector, ShouldEqual, "v1")
		})
		Convey("a single requirement with a matrix selector should parse", func() {
			simple := `
tasks:
- name: task1
  requires:
    name: "task0"
    variant:
     cool: "shoes"
     colors:
      - red
      - green
      - blue
`
			p, errs := createIntermediateProject([]byte(simple))
			So(errs, ShouldBeNil)
			So(p, ShouldNotBeNil)
			So(p.Tasks[0].Requires[0].Name, ShouldEqual, "task0")
			So(p.Tasks[0].Requires[0].Variant.StringSelector, ShouldEqual, "")
			So(p.Tasks[0].Requires[0].Variant.MatrixSelector, ShouldResemble, matrixDefinition{
				"cool": []string{"shoes"}, "colors": []string{"red", "green", "blue"},
			})
		})
	})
}

func TestCreateIntermediateProjectBuildVariants(t *testing.T) {
	Convey("Testing different project files", t, func() {
		Convey("a file with multiple BVTs should parse", func() {
			simple := `
buildvariants:
- name: "v1"
  stepback: true
  batchtime: 123
  modules: ["wow","cool"]
  run_on:
  - "windows2000"
  tasks:
  - name: "t1"
  - name: "t2"
    patch_only: true
    depends_on:
    - name: "t3"
      variant: "v0"
    requires:
    - name: "t4"
    stepback: false
    priority: 77
`
			p, errs := createIntermediateProject([]byte(simple))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			bv := p.BuildVariants[0]
			So(bv.Name, ShouldEqual, "v1")
			So(*bv.Stepback, ShouldBeTrue)
			So(bv.RunOn[0], ShouldEqual, "windows2000")
			So(len(bv.Modules), ShouldEqual, 2)
			So(bv.Tasks[0].Name, ShouldEqual, "t1")
			So(bv.Tasks[1].Name, ShouldEqual, "t2")
			So(*bv.Tasks[1].PatchOnly, ShouldBeTrue)
			So(bv.Tasks[1].DependsOn[0].TaskSelector, ShouldResemble,
				taskSelector{Name: "t3", Variant: &variantSelector{StringSelector: "v0"}})
			So(bv.Tasks[1].Requires[0], ShouldResemble, taskSelector{Name: "t4"})
			So(*bv.Tasks[1].Stepback, ShouldBeFalse)
			So(bv.Tasks[1].Priority, ShouldEqual, 77)
		})
		Convey("a file with oneline BVTs should parse", func() {
			simple := `
buildvariants:
- name: "v1"
  tasks:
  - "t1"
  - name: "t2"
    depends_on: "t3"
    requires: "t4"
`
			p, errs := createIntermediateProject([]byte(simple))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			bv := p.BuildVariants[0]
			So(bv.Name, ShouldEqual, "v1")
			So(bv.Tasks[0].Name, ShouldEqual, "t1")
			So(bv.Tasks[1].Name, ShouldEqual, "t2")
			So(bv.Tasks[1].DependsOn[0].TaskSelector, ShouldResemble, taskSelector{Name: "t3"})
			So(bv.Tasks[1].Requires[0], ShouldResemble, taskSelector{Name: "t4"})
		})
		Convey("a file with single BVTs should parse", func() {
			simple := `
buildvariants:
- name: "v1"
  tasks: "*"
- name: "v2"
  tasks:
    name: "t1"
`
			p, errs := createIntermediateProject([]byte(simple))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			So(len(p.BuildVariants), ShouldEqual, 2)
			bv1 := p.BuildVariants[0]
			bv2 := p.BuildVariants[1]
			So(bv1.Name, ShouldEqual, "v1")
			So(bv2.Name, ShouldEqual, "v2")
			So(len(bv1.Tasks), ShouldEqual, 1)
			So(bv1.Tasks[0].Name, ShouldEqual, "*")
			So(len(bv2.Tasks), ShouldEqual, 1)
			So(bv2.Tasks[0].Name, ShouldEqual, "t1")
		})
		Convey("a file with single run_on, tags, and ignore fields should parse ", func() {
			single := `
ignore: "*.md"
tasks:
- name: "t1"
  tags: wow
buildvariants:
- name: "v1"
  run_on: "distro1"
  tasks: "*"
`
			p, errs := createIntermediateProject([]byte(single))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			So(len(p.Ignore), ShouldEqual, 1)
			So(p.Ignore[0], ShouldEqual, "*.md")
			So(len(p.Tasks[0].Tags), ShouldEqual, 1)
			So(p.Tasks[0].Tags[0], ShouldEqual, "wow")
			So(len(p.BuildVariants), ShouldEqual, 1)
			bv1 := p.BuildVariants[0]
			So(bv1.Name, ShouldEqual, "v1")
			So(len(bv1.RunOn), ShouldEqual, 1)
			So(bv1.RunOn[0], ShouldEqual, "distro1")
		})
		Convey("a file that uses run_on for BVTasks should parse", func() {
			single := `
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
    run_on: "test"
`
			p, errs := createIntermediateProject([]byte(single))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			So(p.BuildVariants[0].Tasks[0].Distros[0], ShouldEqual, "test")
			So(p.BuildVariants[0].Tasks[0].RunOn, ShouldBeNil)
		})
		Convey("a file that uses run_on AND distros for BVTasks should not parse", func() {
			single := `
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
    run_on: "test"
    distros: "asdasdasd"
`
			p, errs := createIntermediateProject([]byte(single))
			So(p, ShouldBeNil)
			So(len(errs), ShouldEqual, 1)
		})
		Convey("a file with a commit queue merge task should parse", func() {
			single := `
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
    commit_queue_merge: true
`
			p, errs := createIntermediateProject([]byte(single))
			So(p, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			bv := p.BuildVariants[0]
			So(bv.Name, ShouldEqual, "v1")
			So(len(bv.Tasks), ShouldEqual, 1)
			So(bv.Tasks[0].Name, ShouldEqual, "t1")
			So(bv.Tasks[0].CommitQueueMerge, ShouldBeTrue)
		})
	})
}

func TestTranslateDependsOn(t *testing.T) {
	Convey("With an intermediate parseProject", t, func() {
		pp := &parserProject{}
		Convey("a tag-free dependency config should be unchanged", func() {
			pp.BuildVariants = []parserBV{
				{Name: "v1"},
			}
			pp.Tasks = []parserTask{
				{Name: "t1"},
				{Name: "t2"},
				{Name: "t3", DependsOn: parserDependencies{
					{TaskSelector: taskSelector{Name: "t1"}},
					{TaskSelector: taskSelector{
						Name: "t2", Variant: &variantSelector{StringSelector: "v1"}}}},
				},
			}
			out, errs := translateProject(pp)
			So(out, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			deps := out.Tasks[2].DependsOn
			So(deps[0].Name, ShouldEqual, "t1")
			So(deps[1].Name, ShouldEqual, "t2")
			So(deps[1].Variant, ShouldEqual, "v1")
		})
		Convey("a dependency with tag selectors should evaluate", func() {
			pp.BuildVariants = []parserBV{
				{Name: "v1", Tags: []string{"cool"}},
				{Name: "v2", Tags: []string{"cool"}},
			}
			pp.Tasks = []parserTask{
				{Name: "t1", Tags: []string{"a", "b"}},
				{Name: "t2", Tags: []string{"a", "c"}, DependsOn: parserDependencies{
					{TaskSelector: taskSelector{Name: "*"}}}},
				{Name: "t3", DependsOn: parserDependencies{
					{TaskSelector: taskSelector{
						Name: ".b", Variant: &variantSelector{StringSelector: ".cool !v2"}}},
					{TaskSelector: taskSelector{
						Name: ".a !.b", Variant: &variantSelector{StringSelector: ".cool"}}}},
				},
			}
			out, errs := translateProject(pp)
			So(out, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			So(out.Tasks[1].DependsOn[0].Name, ShouldEqual, "*")
			deps := out.Tasks[2].DependsOn
			So(deps[0].Name, ShouldEqual, "t1")
			So(deps[0].Variant, ShouldEqual, "v1")
			So(deps[1].Name, ShouldEqual, "t2")
			So(deps[1].Variant, ShouldEqual, "v1")
			So(deps[2].Name, ShouldEqual, "t2")
			So(deps[2].Variant, ShouldEqual, "v2")
		})
		Convey("a dependency with erroneous selectors should fail", func() {
			pp.BuildVariants = []parserBV{
				{Name: "v1"},
			}
			pp.Tasks = []parserTask{
				{Name: "t1", Tags: []string{"a", "b"}},
				{Name: "t2", Tags: []string{"a", "c"}},
				{Name: "t3", DependsOn: parserDependencies{
					{TaskSelector: taskSelector{Name: ".cool"}},
					{TaskSelector: taskSelector{Name: "!!.cool"}},                                                  //[1] illegal selector
					{TaskSelector: taskSelector{Name: "!.c !.b", Variant: &variantSelector{StringSelector: "v1"}}}, //[2] no matching tasks
					{TaskSelector: taskSelector{Name: "t1", Variant: &variantSelector{StringSelector: ".nope"}}},   //[3] no matching variants
					{TaskSelector: taskSelector{Name: "t1"}, Status: "*"},                                          // valid, but:
					{TaskSelector: taskSelector{Name: ".b"}},                                                       //[4] conflicts with above
				}},
			}
			out, errs := translateProject(pp)
			So(out, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 6)
		})
	})
}

func TestTranslateRequires(t *testing.T) {
	Convey("With an intermediate parseProject", t, func() {
		pp := &parserProject{}
		Convey("a task with valid requirements should succeed", func() {
			pp.BuildVariants = []parserBV{
				{Name: "v1"},
			}
			pp.Tasks = []parserTask{
				{Name: "t1"},
				{Name: "t2"},
				{Name: "t3", Requires: taskSelectors{
					{Name: "t1"},
					{Name: "t2", Variant: &variantSelector{StringSelector: "v1"}},
				}},
			}
			out, errs := translateProject(pp)
			So(out, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			reqs := out.Tasks[2].Requires
			So(reqs[0].Name, ShouldEqual, "t1")
			So(reqs[1].Name, ShouldEqual, "t2")
			So(reqs[1].Variant, ShouldEqual, "v1")
		})
		Convey("a task with erroneous requirements should fail", func() {
			pp.BuildVariants = []parserBV{
				{Name: "v1"},
			}
			pp.Tasks = []parserTask{
				{Name: "t1"},
				{Name: "t2", Tags: []string{"taggy"}},
				{Name: "t3", Requires: taskSelectors{
					{Name: "!!!!!"}, //illegal selector
					{Name: ".taggy !t2", Variant: &variantSelector{StringSelector: "v1"}}, //nothing returned
					{Name: "t1", Variant: &variantSelector{StringSelector: "!v1"}},        //no variants returned
					{Name: "t1 t2"}, //nothing returned
				}},
			}
			out, errs := translateProject(pp)
			So(out, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 7)
		})
	})
}

func TestTranslateBuildVariants(t *testing.T) {
	Convey("With an intermediate parseProject", t, func() {
		pp := &parserProject{}
		Convey("a project with valid variant tasks should succeed", func() {
			pp.Tasks = []parserTask{
				{Name: "t1"},
				{Name: "t2", Tags: []string{"a", "z"}},
				{Name: "t3", Tags: []string{"a", "b"}},
			}
			pp.BuildVariants = []parserBV{{
				Name: "v1",
				Tasks: parserBVTaskUnits{
					{Name: "t1", CommitQueueMerge: true},
					{Name: ".z", DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: ".b"}}}},
					{Name: "* !t1 !t2", Requires: taskSelectors{{Name: "!.a"}}},
				},
			}}

			out, errs := translateProject(pp)
			So(out, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 0)
			bvts := out.BuildVariants[0].Tasks
			So(bvts[0].Name, ShouldEqual, "t1")
			So(bvts[1].Name, ShouldEqual, "t2")
			So(bvts[2].Name, ShouldEqual, "t3")
			So(bvts[0].CommitQueueMerge, ShouldBeTrue)
			So(bvts[1].DependsOn[0].Name, ShouldEqual, "t3")
			So(bvts[2].Requires[0].Name, ShouldEqual, "t1")
		})
		Convey("a bvtask with erroneous requirements should fail", func() {
			pp.Tasks = []parserTask{
				{Name: "t1"},
			}
			pp.BuildVariants = []parserBV{{
				Name: "v1",
				Tasks: parserBVTaskUnits{
					{Name: "t1", Requires: taskSelectors{{Name: ".b"}}},
				},
			}}
			out, errs := translateProject(pp)
			So(out, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 2)
		})
	})
}

func parserTaskSelectorTaskEval(tse *taskSelectorEvaluator, tasks parserBVTaskUnits, expected []BuildVariantTaskUnit) {
	names := []string{}
	exp := []string{}
	for _, t := range tasks {
		names = append(names, t.Name)
	}
	for _, e := range expected {
		exp = append(exp, e.Name)
	}
	vse := NewVariantSelectorEvaluator([]parserBV{}, nil)
	Convey(fmt.Sprintf("tasks [%v] should evaluate to [%v]",
		strings.Join(names, ", "), strings.Join(exp, ", ")), func() {
		pbv := parserBV{Tasks: tasks}
		ts, errs := evaluateBVTasks(tse, nil, vse, pbv)
		if expected != nil {
			So(errs, ShouldBeNil)
		} else {
			So(errs, ShouldNotBeNil)
		}
		So(len(ts), ShouldEqual, len(expected))
		for _, e := range expected {
			exists := false
			for _, t := range ts {
				if t.Name == e.Name && t.Priority == e.Priority && len(t.DependsOn) == len(e.DependsOn) {
					exists = true
				}
			}
			So(exists, ShouldBeTrue)
		}
	})
}

func TestParserTaskSelectorEvaluation(t *testing.T) {
	Convey("With a colorful set of ProjectTasks", t, func() {
		taskDefs := []parserTask{
			{Name: "red", Tags: []string{"primary", "warm"}},
			{Name: "orange", Tags: []string{"secondary", "warm"}},
			{Name: "yellow", Tags: []string{"primary", "warm"}},
			{Name: "green", Tags: []string{"secondary", "cool"}},
			{Name: "blue", Tags: []string{"primary", "cool"}},
			{Name: "purple", Tags: []string{"secondary", "cool"}},
			{Name: "brown", Tags: []string{"tertiary"}},
			{Name: "black", Tags: []string{"special"}},
			{Name: "white", Tags: []string{"special"}},
		}

		Convey("a project parser", func() {
			tse := NewParserTaskSelectorEvaluator(taskDefs)
			Convey("should evaluate valid tasks pointers properly", func() {
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{{Name: "white"}},
					[]BuildVariantTaskUnit{{Name: "white"}})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{{Name: "red", Priority: 500}, {Name: ".secondary"}},
					[]BuildVariantTaskUnit{{Name: "red", Priority: 500}, {Name: "orange"}, {Name: "purple"}, {Name: "green"}})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: ".warm .secondary", Distros: []string{"d1"}}},
					[]BuildVariantTaskUnit{{Name: "orange", Distros: []string{"d1"}}})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: "!.warm .secondary", Distros: []string{"d1"}}},
					[]BuildVariantTaskUnit{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: "purple", Distros: []string{"d1"}},
						{Name: "green", Distros: []string{"d1"}}})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{{Name: "*"}},
					[]BuildVariantTaskUnit{
						{Name: "red"}, {Name: "blue"}, {Name: "yellow"},
						{Name: "orange"}, {Name: "purple"}, {Name: "green"},
						{Name: "brown"}, {Name: "white"}, {Name: "black"},
					})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{
						{Name: "red", Priority: 100},
						{Name: "!.warm .secondary", Priority: 100}},
					[]BuildVariantTaskUnit{
						{Name: "red", Priority: 100},
						{Name: "purple", Priority: 100},
						{Name: "green", Priority: 100}})
			})
		})
	})
}

func TestDisplayTaskParsing(t *testing.T) {
	assert := assert.New(t)
	yml := `
buildvariants:
- name: "bv1"
  tasks:
  - name: execTask1
  - name: execTask3
  - name: execTask4
  display_tasks:
  - name: displayTask1
    execution_tasks:
    - execTask1
    - execTask3
- name: "bv2"
  tasks:
  - name: execTask2
  - name: execTask3
tasks:
- name: execTask1
- name: execTask2
- name: execTask3
- name: execTask4
`
	p, errs := createIntermediateProject([]byte(yml))

	// check that display tasks in bv1 parsed correctly
	assert.Len(errs, 0)
	assert.Len(p.BuildVariants[0].DisplayTasks, 1)
	assert.Equal("displayTask1", p.BuildVariants[0].DisplayTasks[0].Name)
	assert.Len(p.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 2)
	assert.Equal("execTask1", p.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Equal("execTask3", p.BuildVariants[0].DisplayTasks[0].ExecutionTasks[1])

	// check that bv2 did not parse any display tasks
	assert.Len(p.BuildVariants[1].DisplayTasks, 0)

	// check that bv1 has the correct execution tasks
	assert.Len(p.BuildVariants[0].Tasks, 3)
	assert.Equal("execTask1", p.BuildVariants[0].Tasks[0].Name)
	assert.Equal("execTask3", p.BuildVariants[0].Tasks[1].Name)
	assert.Equal("execTask4", p.BuildVariants[0].Tasks[2].Name)
}

func TestDisplayTaskValidation(t *testing.T) {
	assert := assert.New(t)

	// check that yml with valid display tasks does not error
	validYml := `
buildvariants:
- name: "bv1"
  tasks:
  - name: execTask1
  - name: execTask3
  - name: execTask4
  display_tasks:
  - name: displayTask1
    execution_tasks:
    - execTask1
    - execTask3
- name: "bv2"
  tasks:
  - name: execTask2
  - name: execTask3
tasks:
- name: execTask1
- name: execTask2
- name: execTask3
- name: execTask4
`

	proj, errs := projectFromYAML([]byte(validYml))
	assert.NotNil(proj)
	assert.Len(errs, 0)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 2)
	assert.Len(proj.BuildVariants[1].DisplayTasks, 0)

	// test that a display task listing a nonexistent task errors
	nonexistentTaskYml := `
buildvariants:
- name: "bv1"
  tasks:
  - name: execTask1
  - name: execTask3
  - name: execTask4
  display_tasks:
  - name: displayTask1
    execution_tasks:
    - execTask1
    - notHere
    - execTask3
- name: "bv2"
  tasks:
  - name: execTask2
  - name: execTask3
tasks:
- name: execTask1
- name: execTask2
- name: execTask3
- name: execTask4
`

	proj, errs = projectFromYAML([]byte(nonexistentTaskYml))
	assert.NotNil(proj)
	assert.Len(errs, 1)
	assert.EqualError(errs[0], "notHere: nothing named 'notHere'")
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 2)
	assert.Len(proj.BuildVariants[1].DisplayTasks, 0)

	// test that a display task with duplicate task errors
	duplicateTaskYml := `
buildvariants:
- name: "bv1"
  tasks:
  - name: execTask1
  - name: execTask3
  - name: execTask4
  display_tasks:
  - name: displayTask1
    execution_tasks:
    - execTask1
    - execTask3
  - name: displayTask2
    execution_tasks:
    - execTask4
    - execTask3
tasks:
- name: execTask1
- name: execTask2
- name: execTask3
- name: execTask4
`

	proj, errs = projectFromYAML([]byte(duplicateTaskYml))
	assert.NotNil(proj)
	assert.Len(errs, 1)
	assert.EqualError(errs[0], "execution task execTask3 is listed in more than 1 display task")
	assert.Len(proj.BuildVariants[0].DisplayTasks, 0)

	// test that a display task can't share a name with an execution task
	conflictYml := `
buildvariants:
- name: "bv1"
  tasks:
  - name: execTask1
  - name: execTask3
  - name: execTask4
  display_tasks:
  - name: execTask1
    execution_tasks:
    - execTask3
tasks:
- name: execTask1
- name: execTask2
- name: execTask3
- name: execTask4
`

	proj, errs = projectFromYAML([]byte(conflictYml))
	assert.NotNil(proj)
	assert.Len(errs, 1)
	assert.EqualError(errs[0], "display task execTask1 cannot have the same name as an execution task")
	assert.Len(proj.BuildVariants[0].DisplayTasks, 0)

	// test that wildcard selectors are resolved correctly
	wildcardYml := `
buildvariants:
- name: "bv1"
  tasks:
  - name: execTask1
  - name: execTask3
  - name: execTask4
  display_tasks:
  - name: displayTask1
    execution_tasks:
    - "*"
- name: "bv2"
  tasks:
  - name: execTask2
  - name: execTask3
tasks:
- name: execTask1
- name: execTask2
- name: execTask3
- name: execTask4
`

	proj, errs = projectFromYAML([]byte(wildcardYml))
	assert.NotNil(proj)
	assert.Len(errs, 0)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 3)

	// test that tag selectors are resolved correctly
	tagYml := `
buildvariants:
- name: "bv1"
  tasks:
  - name: execTask1
  - name: execTask2
  - name: execTask3
  - name: execTask4
  display_tasks:
  - name: displayTaskOdd
    execution_tasks:
    - ".odd"
  - name: displayTaskEven
    execution_tasks:
    - ".even"
tasks:
- name: execTask1
  tags: [ "odd" ]
- name: execTask2
  tags: [ "even" ]
- name: execTask3
  tags: [ "odd" ]
- name: execTask4
  tags: [ "even" ]
`

	proj, errs = projectFromYAML([]byte(tagYml))
	assert.NotNil(proj)
	assert.Len(errs, 0)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 2)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 2)
	assert.Len(proj.BuildVariants[0].DisplayTasks[1].ExecutionTasks, 2)
	assert.Equal("execTask1", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Equal("execTask3", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[1])
	assert.Equal("execTask2", proj.BuildVariants[0].DisplayTasks[1].ExecutionTasks[0])
	assert.Equal("execTask4", proj.BuildVariants[0].DisplayTasks[1].ExecutionTasks[1])
}

func TestTranslateProjectDoesNotModifyParserProject(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)
	// build variant display tasks
	tagYml := `
buildvariants:
- name: "bv1"
  tasks:
  - name: execTask1
  - name: execTask2
  - name: execTask3
  - name: execTask4
  display_tasks:
  - name: displayTaskOdd
    execution_tasks:
    - ".odd"
  - name: displayTaskEven
    execution_tasks:
    - ".even"
tasks:
- name: execTask1
  tags: [ "odd" ]
- name: execTask2
  tags: [ "even" ]
- name: execTask3
  tags: [ "odd" ]
- name: execTask4
  tags: [ "even" ]
`
	pp, errs := createIntermediateProject([]byte(tagYml))
	assert.NotNil(pp)
	assert.Len(errs, 0)
	require.Len(pp.BuildVariants[0].DisplayTasks, 2)
	assert.Len(pp.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 1)
	assert.Equal(".odd", pp.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Len(pp.BuildVariants[0].DisplayTasks[1].ExecutionTasks, 1)
	assert.Equal(".even", pp.BuildVariants[0].DisplayTasks[1].ExecutionTasks[0])

	proj, errs := translateProject(pp)
	assert.NotNil(proj)
	assert.Len(errs, 0)
	// assert parser project hasn't changed
	require.Len(pp.BuildVariants[0].DisplayTasks, 2)
	assert.Len(pp.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 1)
	assert.Equal(".odd", pp.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Len(pp.BuildVariants[0].DisplayTasks[1].ExecutionTasks, 1)
	assert.Equal(".even", pp.BuildVariants[0].DisplayTasks[1].ExecutionTasks[0])

	//assert project is correct
	require.Len(proj.BuildVariants[0].DisplayTasks, 2)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 2)
	assert.Len(proj.BuildVariants[0].DisplayTasks[1].ExecutionTasks, 2)
	assert.Equal("execTask1", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Equal("execTask3", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[1])
	assert.Equal("execTask2", proj.BuildVariants[0].DisplayTasks[1].ExecutionTasks[0])
	assert.Equal("execTask4", proj.BuildVariants[0].DisplayTasks[1].ExecutionTasks[1])
}

func TestTaskGroupParsing(t *testing.T) {
	assert := assert.New(t)

	// check that yml with valid task group does not error and parses correctly
	validYml := `
tasks:
- name: example_task_1
- name: example_task_2
task_groups:
- name: example_task_group
  share_processes: true
  max_hosts: 2
  setup_group_can_fail_task: true
  setup_group_timeout_secs: 10
  setup_group:
  - command: shell.exec
    params:
      script: "echo setup_group"
  teardown_group:
  - command: shell.exec
    params:
      script: "echo teardown_group"
  setup_task:
  - command: shell.exec
    params:
      script: "echo setup_group"
  teardown_task:
  - command: shell.exec
    params:
      script: "echo setup_group"
  tasks:
  - example_task_1
  - example_task_2
buildvariants:
- name: "bv"
  tasks:
  - name: example_task_group
`
	proj, errs := projectFromYAML([]byte(validYml))
	assert.NotNil(proj)
	assert.Empty(errs)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("example_task_group", tg.Name)
	assert.Equal(2, tg.MaxHosts)
	assert.Equal(true, tg.SetupGroupFailTask)
	assert.Equal(10, tg.SetupGroupTimeoutSecs)
	assert.Len(tg.Tasks, 2)
	assert.Len(tg.SetupTask.List(), 1)
	assert.Len(tg.SetupGroup.List(), 1)
	assert.Len(tg.TeardownTask.List(), 1)
	assert.Len(tg.TeardownGroup.List(), 1)
	assert.True(tg.ShareProcs)

	// check that yml with a task group that contains a nonexistent task errors
	wrongTaskYml := `
tasks:
- name: example_task_1
- name: example_task_2
task_groups:
- name: example_task_group
  tasks:
  - example_task_1
  - example_task_3
buildvariants:
- name: "bv"
  tasks:
  - name: example_task_group
`
	proj, errs = projectFromYAML([]byte(wrongTaskYml))
	assert.NotNil(proj)
	assert.Len(errs, 1)
	assert.Contains(errs[0].Error(), `nothing named 'example_task_3'`)

	// check that tasks listed in the task group yml maintain their order
	orderedYml := `
tasks:
- name: 8
- name: 7
- name: 6
- name: 5
- name: 4
- name: 3
- name: 2
- name: 1
task_groups:
- name: example_task_group
  patchable: false
  stepback: false
  tasks:
  - 1
  - 2
  - 3
  - 4
  - 5
  - 6
  - 7
  - 8
buildvariants:
- name: "bv"
  tasks:
  - name: example_task_group
`
	proj, errs = projectFromYAML([]byte(orderedYml))
	assert.NotNil(proj)
	assert.Len(errs, 0)
	for i, t := range proj.TaskGroups[0].Tasks {
		assert.Equal(strconv.Itoa(i+1), t)
	}

	// check that tags select the correct tasks
	tagYml := `
tasks:
- name: 1
  tags: [ "odd" ]
- name: 2
  tags: [ "even" ]
- name: 3
  tags: [ "odd" ]
- name: 4
  tags: [ "even" ]
task_groups:
- name: even_task_group
  tasks:
  - .even
- name: odd_task_group
  tasks:
  - .odd
buildvariants:
- name: bv
  tasks:
  - name: even_task_group
  - name: odd_task_group
`
	proj, errs = projectFromYAML([]byte(tagYml))
	assert.NotNil(proj)
	assert.Len(errs, 0)
	assert.Len(proj.TaskGroups, 2)
	assert.Equal("even_task_group", proj.TaskGroups[0].Name)
	assert.Len(proj.TaskGroups[0].Tasks, 2)
	for _, t := range proj.TaskGroups[0].Tasks {
		v, err := strconv.Atoi(t)
		assert.NoError(err)
		assert.Equal(0, v%2)
	}
}

func TestTaskGroupWithDisplayTask(t *testing.T) {
	assert := assert.New(t)

	validYml := `
tasks:
- name: task_1
- name: task_2
task_groups:
- name: task_group_1
  tasks:
  - task_1
  - task_2
buildvariants:
- name: "bv"
  tasks:
  - name: task_group_1
  display_tasks:
    - name: lint
      execution_tasks:
      - task_1
      - task_2
`
	proj, errs := projectFromYAML([]byte(validYml))
	assert.NotNil(proj)
	assert.Empty(errs)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("task_group_1", tg.Name)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 2)
	assert.Equal("task_1", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Equal("task_2", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[1])
}

func TestTaskGroupWithDisplayTaskWithDisplayTaskTag(t *testing.T) {
	assert := assert.New(t)
	validYml := `
tasks:
- name: task_1
  tags: [ "tag_1" ]
- name: task_2
  tags: [ "tag_1" ]
task_groups:
- name: task_group_1
  tasks:
  - task_1
  - task_2
buildvariants:
- name: "bv"
  tasks:
  - name: task_group_1
  display_tasks:
    - name: display_1
      execution_tasks:
      - ".tag_1"
`
	proj, errs := projectFromYAML([]byte(validYml))
	assert.NotNil(proj)
	assert.Empty(errs)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("task_group_1", tg.Name)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 2)
	assert.Equal("task_1", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Equal("task_2", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[1])
}

func TestTaskGroupWithDisplayTaskWithTaskGroupTag(t *testing.T) {
	assert := assert.New(t)
	validYml := `
tasks:
- name: task_1
  tags: [ "tag_1" ]
- name: task_2
  tags: [ "tag_1" ]
task_groups:
- name: task_group_1
  tasks:
  - ".tag_1"
buildvariants:
- name: "bv"
  tasks:
  - name: task_group_1
  display_tasks:
    - name: display_1
      execution_tasks:
      - task_1
      - task_2
`
	proj, errs := projectFromYAML([]byte(validYml))
	assert.NotNil(proj)
	assert.Empty(errs)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("task_group_1", tg.Name)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 2)
	assert.Equal("task_1", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Equal("task_2", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[1])
}

func TestTaskGroupWithDisplayTaskWithTaskGroupTagAndDisplayTaskTag(t *testing.T) {
	assert := assert.New(t)
	validYml := `
tasks:
- name: task_1
  tags: [ "tag_1" ]
- name: task_2
  tags: [ "tag_1" ]
task_groups:
- name: task_group_1
  tasks:
  - ".tag_1"
buildvariants:
- name: "bv"
  tasks:
  - name: task_group_1
  display_tasks:
    - name: display_1
      execution_tasks:
      - ".tag_1"
`
	proj, errs := projectFromYAML([]byte(validYml))
	assert.NotNil(proj)
	assert.Empty(errs)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("task_group_1", tg.Name)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 2)
	assert.Equal("task_1", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Equal("task_2", proj.BuildVariants[0].DisplayTasks[0].ExecutionTasks[1])
}

func TestBVDependenciesOverrideTaskDependencies(t *testing.T) {
	assert := assert.New(t)
	yml := `
tasks:
- name: task_1
- name: task_2
- name: task_3
- name: task_4
- name: task_5
buildvariants:
- name: bv_1
  depends_on:
    - name: task_3
  tasks:
  - name: task_1
    depends_on:
      - name: task_4
  - name: task_2
- name: bv_2
  tasks:
    - name: task_3
- name: bv_3
  requires:
    - name: task_3
  tasks:
    - name: task_4
    - name: task_5
`
	proj, errs := projectFromYAML([]byte(yml))
	assert.NotNil(proj)
	assert.Empty(errs)
	assert.Len(proj.BuildVariants, 3)

	assert.Equal("bv_1", proj.BuildVariants[0].Name)
	assert.Len(proj.BuildVariants[0].Tasks, 2)
	assert.Equal("task_1", proj.BuildVariants[0].Tasks[0].Name)
	assert.Equal("task_2", proj.BuildVariants[0].Tasks[1].Name)
	assert.Equal("task_4", proj.BuildVariants[0].Tasks[0].DependsOn[0].Name)
	assert.Equal("task_3", proj.BuildVariants[0].Tasks[1].DependsOn[0].Name)

	assert.Equal("bv_2", proj.BuildVariants[1].Name)
	assert.Len(proj.BuildVariants[1].Tasks, 1)
	assert.Equal("task_3", proj.BuildVariants[1].Tasks[0].Name)
	assert.Len(proj.BuildVariants[1].Tasks[0].DependsOn, 0)

	assert.Equal("bv_3", proj.BuildVariants[2].Name)
	assert.Len(proj.BuildVariants[2].Tasks, 2)
	assert.Equal("task_4", proj.BuildVariants[2].Tasks[0].Name)
	assert.Equal("task_5", proj.BuildVariants[2].Tasks[1].Name)
	assert.Equal("task_3", proj.BuildVariants[2].Tasks[0].Requires[0].Name)
	assert.Equal("task_3", proj.BuildVariants[2].Tasks[1].Requires[0].Name)
}

func TestPatchOnlyTasks(t *testing.T) {
	assert := assert.New(t)
	yml := `
tasks:
- name: task_1
  patch_only: true
- name: task_2
buildvariants:
- name: bv_1
  tasks:
  - name: task_1
  - name: task_2
    patch_only: false
- name: bv_2
  tasks:
  - name: task_1
    patch_only: false
  - name: task_2
    patch_only: true
- name: bv_3
  tasks:
  - name: task_2
`

	proj, errs := projectFromYAML([]byte(yml))
	assert.NotNil(proj)
	assert.Empty(errs)
	assert.Len(proj.BuildVariants, 3)

	assert.Len(proj.BuildVariants[0].Tasks, 2)
	assert.Nil(proj.BuildVariants[0].Tasks[0].PatchOnly)
	assert.False(*proj.BuildVariants[0].Tasks[1].PatchOnly)

	assert.Len(proj.BuildVariants[1].Tasks, 2)
	assert.False(*proj.BuildVariants[1].Tasks[0].PatchOnly)
	assert.True(*proj.BuildVariants[1].Tasks[1].PatchOnly)

	assert.Len(proj.BuildVariants[2].Tasks, 1)
	assert.Nil(proj.BuildVariants[2].Tasks[0].PatchOnly)
}

func TestLoggerConfig(t *testing.T) {
	assert := assert.New(t)
	yml := `
loggers:
  agent:
    - type: something
      splunk_token: idk
    - type: somethingElse
tasks:
- name: task_1
  commands:
  - command: myCommand
    loggers:
      system:
        - type: commandLogger
`

	proj, errs := projectFromYAML([]byte(yml))
	assert.NotNil(proj)
	assert.Empty(errs)
	assert.Equal("something", proj.Loggers.Agent[0].Type)
	assert.Equal("idk", proj.Loggers.Agent[0].SplunkToken)
	assert.Equal("somethingElse", proj.Loggers.Agent[1].Type)
	assert.Equal("commandLogger", proj.Tasks[0].Commands[0].Loggers.System[0].Type)
}

func TestParserProjectPersists(t *testing.T) {
	simpleYml := `
loggers:
  agent:
    - type: something
      splunk_token: idk
    - type: somethingElse
tasks:
- name: task_1
  depends_on:
  - name: embedded_sdk_s3_put
    variant: embedded-sdk-android-arm32
  commands:
  - command: myCommand
    params:
      env:
        ${MY_KEY}: my-value
        ${MY_NUMS}: [1,2,3]
    loggers:
      system:
       - type: commandLogger
functions:
  run-make:
    command: subprocess.exec
    params:
      working_dir: gopath/src/github.com/evergreen-ci/evergreen
      binary: make
      env:
        CLIENT_URL: https://s3.amazonaws.com/mciuploads/evergreen/${task_id}/evergreen-ci/evergreen/clients/${goos}_${goarch}/evergreen
`

	for name, test := range map[string]func(t *testing.T){
		"simpleYaml": func(t *testing.T) {
			assert.NoError(t, checkProjectPersists([]byte(simpleYml)))
		},
		"self-tests.yml": func(t *testing.T) {
			filepath := filepath.Join(testutil.GetDirectoryOfFile(), "..", "self-tests.yml")
			yml, err := ioutil.ReadFile(filepath)
			assert.NoError(t, err)
			assert.NoError(t, checkProjectPersists(yml))
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.NoError(t, db.ClearCollections(VersionCollection))
			test(t)
		})
	}
}

func checkProjectPersists(yml []byte) error {
	pp, errs := createIntermediateProject(yml)
	catcher := grip.NewBasicCatcher()
	for _, err := range errs {
		catcher.Add(err)
	}
	if catcher.HasErrors() {
		return catcher.Resolve()
	}
	yamlToCompare, err := yaml.Marshal(pp)
	if err != nil {
		return errors.Wrapf(err, "error marshalling original project")
	}

	v := Version{
		Id:            "my-version",
		ParserProject: pp,
	}
	if err = v.Insert(); err != nil {
		return errors.Wrapf(err, "error inserting version")
	}
	newV, err := VersionFindOneId(v.Id)
	if err != nil {
		return errors.Wrapf(err, "error finding version")
	}
	newYaml, err := yaml.Marshal(newV.ParserProject)
	if err != nil {
		return errors.Wrapf(err, "error marshalling database project")
	}
	if !bytes.Equal(newYaml, yamlToCompare) {
		return errors.New("yamls not equal")
	}
	return nil
}
