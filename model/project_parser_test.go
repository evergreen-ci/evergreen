package model

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"testing"

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
			So(p.Tasks[1].Requires[0].Variant.stringSelector, ShouldEqual, "v1")
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
			So(p.Tasks[0].Requires[0].Variant.stringSelector, ShouldEqual, "v1")
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
			So(p.Tasks[0].Requires[0].Variant.stringSelector, ShouldEqual, "")
			So(p.Tasks[0].Requires[0].Variant.matrixSelector, ShouldResemble, matrixDefinition{
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
				taskSelector{Name: "t3", Variant: &variantSelector{stringSelector: "v0"}})
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
						Name: "t2", Variant: &variantSelector{stringSelector: "v1"}}}},
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
						Name: ".b", Variant: &variantSelector{stringSelector: ".cool !v2"}}},
					{TaskSelector: taskSelector{
						Name: ".a !.b", Variant: &variantSelector{stringSelector: ".cool"}}}},
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
					{TaskSelector: taskSelector{Name: "!.c !.b", Variant: &variantSelector{stringSelector: "v1"}}}, //[2] no matching tasks
					{TaskSelector: taskSelector{Name: "t1", Variant: &variantSelector{stringSelector: ".nope"}}},   //[3] no matching variants
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
					{Name: "t2", Variant: &variantSelector{stringSelector: "v1"}},
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
					{Name: ".taggy !t2", Variant: &variantSelector{stringSelector: "v1"}}, //nothing returned
					{Name: "t1", Variant: &variantSelector{stringSelector: "!v1"}},        //no variants returned
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
					{Name: "t1"},
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

func TestLoadProjectInto(t *testing.T) {
	assert := assert.New(t)
	confStr := "stepback: true\nidentifier: mci\ncommand_type: test\nignore:\n- '*.md'\n- scripts/*\n- .github/*\npost:\n- func: attach-test-results\n- type: system\n  command: s3.put\n  params:\n    aws_key: ${aws_key}\n    aws_secret: ${aws_secret}\n    bucket: mciuploads\n    content_type: text/html\n    display_name: '(html) coverage:'\n    local_files_include_filter:\n    - gopath/src/github.com/evergreen-ci/evergreen/bin/output.*.coverage.html\n    permissions: public-read\n    remote_file: evergreen/${task_id}/\n- type: system\n  command: s3.put\n  params:\n    aws_key: ${aws_key}\n    aws_secret: ${aws_secret}\n    bucket: mciuploads\n    content_type: text/plain\n    display_name: '(txt) coverage:'\n    local_files_include_filter:\n    - gopath/src/github.com/evergreen-ci/evergreen/bin/output.*.coverage\n    permissions: public-read\n    remote_file: evergreen/${task_id}/\nbuildvariants:\n- name: ubuntu1604\n  display_name: Ubuntu 16.04\n  expansions:\n    disable_coverage: \"yes\"\n    goarch: amd64\n    gobin: /opt/golang/go1.9/bin/go\n    goos: linux\n    goroot: /opt/golang/go1.9\n    mongodb_url: https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-ubuntu1604-4.0.3.tgz\n  run_on:\n  - ubuntu1604-test\n  tasks:\n  - name: dist\n  - name: smoke-test-task\n  - name: smoke-test-endpoints\n  - name: smoke-test-agent-monitor\n  - name: test-auth\n  - name: test-rest-route\n  - name: test-rest-client\n  - name: test-rest-model\n  - name: test-command\n  - name: test-units\n  - name: test-agent\n  - name: test-rest-data\n  - name: test-operations\n  - name: test-db\n  - name: test-cloud\n  - name: test-repotracker\n  - name: test-scheduler\n  - name: test-service\n  - name: test-monitor\n  - name: test-evergreen\n  - name: test-thirdparty\n  - name: test-trigger\n  - name: test-util\n  - name: test-validator\n  - name: test-model\n  - name: test-model-alertrecord\n  - name: test-model-artifact\n  - name: test-model-build\n  - name: test-model-event\n  - name: test-model-host\n  - name: test-model-notification\n  - name: test-model-patch\n  - name: test-model-stats\n  - name: test-model-task\n  - name: test-model-testresult\n  - name: test-model-user\n  - name: test-model-distro\n  - name: test-model-commitqueue\n  - name: test-model-manifest\n  - name: test-plugin\n  - name: test-migrations\n  - name: test-model-grid\n  - name: js-test\n- name: ubuntu1604-docker\n  display_name: Ubuntu 16.04 (Docker)\n  expansions:\n    goarch: amd64\n    gobin: /opt/golang/go1.9/bin/go\n    goos: linux\n    goroot: /opt/golang/go1.9\n    mongodb_url: https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-4.0.3.tgz\n    nodebin: /opt/node/bin\n    test_timeout: 15m\n  run_on:\n  - ubuntu1604-container\n  tasks:\n  - name: dist\n  - name: smoke-test-task\n  - name: smoke-test-endpoints\n  - name: smoke-test-agent-monitor\n  - name: test-auth\n  - name: test-rest-route\n  - name: test-rest-client\n  - name: test-rest-model\n  - name: test-command\n  - name: test-units\n  - name: test-agent\n  - name: test-rest-data\n  - name: test-operations\n  - name: test-db\n  - name: test-cloud\n  - name: test-repotracker\n  - name: test-scheduler\n  - name: test-service\n  - name: test-monitor\n  - name: test-evergreen\n  - name: test-thirdparty\n  - name: test-trigger\n  - name: test-util\n  - name: test-validator\n  - name: test-model\n  - name: test-model-alertrecord\n  - name: test-model-artifact\n  - name: test-model-build\n  - name: test-model-event\n  - name: test-model-host\n  - name: test-model-notification\n  - name: test-model-patch\n  - name: test-model-stats\n  - name: test-model-task\n  - name: test-model-testresult\n  - name: test-model-user\n  - name: test-model-distro\n  - name: test-model-commitqueue\n  - name: test-model-manifest\n  - name: test-plugin\n  - name: test-migrations\n  - name: test-model-grid\n  - name: js-test\n- name: race-detector\n  display_name: Race Detector\n  expansions:\n    mongodb_url: https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-4.0.3.tgz\n    race_detector: \"true\"\n    test_timeout: 15m\n  run_on:\n  - archlinux-test\n  tasks:\n  - name: dist\n  - name: test-auth\n  - name: test-rest-route\n  - name: test-rest-client\n  - name: test-rest-model\n  - name: test-command\n  - name: test-units\n  - name: test-agent\n  - name: test-rest-data\n  - name: test-operations\n  - name: test-db\n  - name: test-cloud\n  - name: test-repotracker\n  - name: test-scheduler\n  - name: test-service\n  - name: test-monitor\n  - name: test-evergreen\n  - name: test-thirdparty\n  - name: test-trigger\n  - name: test-util\n  - name: test-validator\n  - name: test-model\n  - name: test-model-alertrecord\n  - name: test-model-artifact\n  - name: test-model-build\n  - name: test-model-event\n  - name: test-model-host\n  - name: test-model-notification\n  - name: test-model-patch\n  - name: test-model-stats\n  - name: test-model-task\n  - name: test-model-testresult\n  - name: test-model-user\n  - name: test-model-distro\n  - name: test-model-commitqueue\n  - name: test-model-manifest\n  - name: test-plugin\n  - name: test-migrations\n  - name: test-model-grid\n- name: lint\n  display_name: Lint\n  run_on:\n  - archlinux-test\n  tasks:\n  - name: generate-lint\n  - name: lint-group\n- name: coverage\n  display_name: Coverage\n  expansions:\n    mongodb_url: https://fastdl.mongodb.org/linux/mongodb-linux-x86_64-4.0.3.tgz\n    test_timeout: 15m\n  run_on:\n  - archlinux-test\n  tasks:\n  - name: coverage\n    stepback: false\n- name: osx\n  display_name: OSX\n  expansions:\n    disable_coverage: \"yes\"\n    gobin: /opt/golang/go1.9/bin/go\n    goroot: /opt/golang/go1.9\n    mongodb_url: https://fastdl.mongodb.org/osx/mongodb-osx-ssl-x86_64-4.0.3.tgz\n  batchtime: 2880\n  run_on:\n  - macos-1014\n  tasks:\n  - name: dist\n  - name: test-auth\n  - name: test-rest-route\n  - name: test-rest-client\n  - name: test-rest-model\n  - name: test-command\n  - name: test-units\n  - name: test-agent\n  - name: test-rest-data\n  - name: test-operations\n  - name: test-db\n  - name: test-cloud\n  - name: test-repotracker\n  - name: test-scheduler\n  - name: test-service\n  - name: test-monitor\n  - name: test-evergreen\n  - name: test-thirdparty\n  - name: test-trigger\n  - name: test-util\n  - name: test-validator\n  - name: test-model\n  - name: test-model-alertrecord\n  - name: test-model-artifact\n  - name: test-model-build\n  - name: test-model-event\n  - name: test-model-host\n  - name: test-model-notification\n  - name: test-model-patch\n  - name: test-model-stats\n  - name: test-model-task\n  - name: test-model-testresult\n  - name: test-model-user\n  - name: test-model-distro\n  - name: test-model-commitqueue\n  - name: test-model-manifest\n  - name: test-plugin\n  - name: test-migrations\n  - name: test-model-grid\n- name: windows\n  display_name: Windows\n  expansions:\n    archiveExt: .zip\n    disable_coverage: \"yes\"\n    extension: .exe\n    gobin: /cygdrive/c/golang/go1.9/bin/go\n    goroot: c:/golang/go1.9\n    mongodb_url: https://fastdl.mongodb.org/win32/mongodb-win32-x86_64-2008plus-ssl-4.0.3.zip\n  run_on:\n  - windows-64-vs2015-small\n  tasks:\n  - name: test-rest-client\n  - name: test-command\n  - name: test-agent\n  - name: test-thirdparty\n  - name: test-util\n  - name: test-operations\n- name: rhel71-power8\n  display_name: RHEL 7.1 POWER8\n  expansions:\n    disable_coverage: \"yes\"\n    goarch: ppc64le\n    gobin: /opt/golang/go1.9/bin/go\n    goos: linux\n    goroot: /opt/golang/go1.9\n    mongodb_url: https://downloads.mongodb.com/linux/mongodb-linux-ppc64le-enterprise-rhel71-4.0.3.tgz\n    xc_build: \"yes\"\n  batchtime: 2880\n  run_on:\n  - rhel71-power8-test\n  tasks:\n  - name: test-rest-client\n  - name: test-command\n  - name: test-agent\n  - name: test-thirdparty\n  - name: test-util\n- name: rhel72-s390x\n  display_name: RHEL 7.2 zLinux\n  expansions:\n    disable_coverage: \"yes\"\n    goarch: s390x\n    gobin: /opt/golang/go1.9/bin/go\n    goos: linux\n    goroot: /opt/golang/go1.9\n    mongodb_url: https://downloads.mongodb.com/linux/mongodb-linux-s390x-enterprise-rhel72-3.6.4.tgz\n    xc_build: \"yes\"\n  batchtime: 2880\n  run_on:\n  - rhel72-zseries-test\n  tasks:\n  - name: test-rest-client\n  - name: test-command\n  - name: test-agent\n  - name: test-thirdparty\n  - name: test-util\n- name: ubuntu1604-arm64\n  display_name: Ubuntu 16.04 ARM\n  expansions:\n    disable_coverage: \"yes\"\n    goarch: arm64\n    gobin: /opt/golang/go1.9/bin/go\n    goos: linux\n    goroot: /opt/golang/go1.9\n    mongodb_url: https://downloads.mongodb.com/linux/mongodb-linux-arm64-enterprise-ubuntu1604-4.0.3.tgz\n    xc_build: \"yes\"\n  batchtime: 2880\n  run_on:\n  - ubuntu1604-arm64-small\n  tasks:\n  - name: test-rest-client\n  - name: test-command\n  - name: test-agent\n  - name: test-thirdparty\n  - name: test-util\nfunctions:\n  attach-test-results:\n  - type: system\n    command: gotest.parse_files\n    params:\n      files:\n      - gopath/src/github.com/evergreen-ci/evergreen/bin/output.*\n  - type: system\n    command: attach.xunit_results\n    params:\n      files:\n      - gopath/src/github.com/evergreen-ci/evergreen/bin/jstests/*.xml\n  get-project:\n  - type: setup\n    command: git.get_project\n    params:\n      directory: gopath/src/github.com/evergreen-ci/evergreen\n      token: ${github_token}\n  remove-test-results:\n  - type: system\n    command: shell.exec\n    params:\n      script: |\n        set -o xtrace\n        rm gopath/src/github.com/evergreen-ci/evergreen/bin/output.*\n        rm gopath/src/github.com/evergreen-ci/evergreen/bin/jstests/*.xml\n      shell: bash\n  run-make:\n  - command: subprocess.exec\n    params:\n      args:\n      - ${make_args|}\n      - ${target}\n      binary: make\n      env:\n        AWS_KEY: ${aws_key}\n        AWS_SECRET: ${aws_secret}\n        CLIENT_URL: https://s3.amazonaws.com/mciuploads/evergreen/${task_id}/evergreen-ci/evergreen/clients/${goos}_${goarch}/evergreen\n        DEBUG_ENABLED: ${debug}\n        DISABLE_COVERAGE: ${disable_coverage}\n        DOCKER_HOST: ${docker_host}\n        EVERGREEN_ALL: \"true\"\n        GO_BIN_PATH: ${gobin}\n        GOARCH: ${goarch}\n        GOOS: ${goos}\n        GOPATH: ${workdir}/gopath\n        GOROOT: ${goroot}\n        KARMA_REPORTER: junit\n        NODE_BIN_PATH: ${nodebin}\n        RACE_DETECTOR: ${race_detector}\n        SETTINGS_OVERRIDE: creds.yml\n        TEST_TIMEOUT: ${test_timeout}\n        VENDOR_PKG: github.com/${trigger_repo_owner}/${trigger_repo_name}\n        VENDOR_REVISION: ${trigger_revision}\n        XC_BUILD: ${xc_build}\n      working_dir: gopath/src/github.com/evergreen-ci/evergreen\n  setup-credentials:\n  - type: setup\n    command: subprocess.exec\n    params:\n      command: bash scripts/setup-credentials.sh\n      env:\n        AWS_KEY: ${aws_key}\n        AWS_SECRET: ${aws_secret}\n        CROWD_PW: ${crowdpw}\n        CROWD_SERVER: ${crowdserver}\n        CROWD_USER: ${crowduser}\n        GITHUB_TOKEN: ${github_token}\n        JIRA_SERVER: ${jiraserver}\n      silent: true\n      working_dir: gopath/src/github.com/evergreen-ci/evergreen\n  setup-mongodb:\n  - type: setup\n    command: subprocess.exec\n    params:\n      command: make get-mongodb\n      env:\n        DECOMPRESS: ${decompress}\n        MONGODB_URL: ${mongodb_url}\n      working_dir: gopath/src/github.com/evergreen-ci/evergreen/\n  - type: setup\n    command: subprocess.exec\n    params:\n      background: true\n      command: make start-mongod\n      working_dir: gopath/src/github.com/evergreen-ci/evergreen/\n  - type: setup\n    command: subprocess.exec\n    params:\n      command: make check-mongod\n      working_dir: gopath/src/github.com/evergreen-ci/evergreen\n  - type: setup\n    command: subprocess.exec\n    params:\n      command: make init-rs\n      working_dir: gopath/src/github.com/evergreen-ci/evergreen/\ntask_groups:\n- name: lint-group\n  max_hosts: 4\n  setup_group:\n  - type: system\n    command: git.get_project\n    params:\n      directory: gopath/src/github.com/evergreen-ci/evergreen\n  - func: set-up-credentials\n  teardown_task:\n  - func: attach-test-results\n  - func: remove-test-results\n  tasks:\n  - lint-evergreen\n  - lint-agent\n  - lint-apimodels\n  - lint-auth\n  - lint-cloud\n  - lint-command\n  - lint-db\n  - lint-migrations\n  - lint-model\n  - lint-model-alertrecord\n  - lint-model-artifact\n  - lint-model-build\n  - lint-model-commitqueue\n  - lint-model-distro\n  - lint-model-event\n  - lint-model-grid\n  - lint-model-host\n  - lint-model-manifest\n  - lint-model-notification\n  - lint-model-patch\n  - lint-model-stats\n  - lint-model-task\n  - lint-model-testresult\n  - lint-model-user\n  - lint-monitor\n  - lint-operations\n  - lint-plugin\n  - lint-repotracker\n  - lint-rest-client\n  - lint-rest-data\n  - lint-rest-model\n  - lint-rest-route\n  - lint-scheduler\n  - lint-service\n  - lint-service-testutil\n  - lint-thirdparty\n  - lint-trigger\n  - lint-units\n  - lint-util\n  - lint-validator\ntasks:\n- name: coverage\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      make_args: -k\n      target: coverage-html\n  tags:\n  - report\n- name: smoke-test-task\n  commands:\n  - command: timeout.update\n    params:\n      exec_timeout_secs: 900\n      timeout_secs: 900\n  - func: get-project\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: set-var\n  - func: run-make\n    vars:\n      target: set-project-var\n  - func: run-make\n    vars:\n      target: load-smoke-data\n  - command: subprocess.exec\n    params:\n      command: bash scripts/setup-smoke-config.sh ${github_token}\n      silent: true\n      working_dir: gopath/src/github.com/evergreen-ci/evergreen\n  - func: run-make\n    vars:\n      target: set-smoke-vars\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - smoke\n- name: smoke-test-endpoints\n  commands:\n  - command: timeout.update\n    params:\n      exec_timeout_secs: 900\n      timeout_secs: 900\n  - func: get-project\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: set-var\n  - func: run-make\n    vars:\n      target: set-project-var\n  - func: run-make\n    vars:\n      target: load-smoke-data\n  - command: subprocess.exec\n    params:\n      command: bash scripts/setup-smoke-config.sh ${github_token}\n      silent: true\n      working_dir: gopath/src/github.com/evergreen-ci/evergreen\n  - func: run-make\n    vars:\n      target: set-smoke-vars\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - smoke\n- name: smoke-test-agent-monitor\n  commands:\n  - command: timeout.update\n    params:\n      exec_timeout_secs: 900\n      timeout_secs: 900\n  - func: get-project\n  - func: run-make\n    vars:\n      target: cli\n  - type: system\n    command: s3.put\n    params:\n      aws_key: ${aws_key}\n      aws_secret: ${aws_secret}\n      bucket: mciuploads\n      content_type: application/octet-stream\n      display_name: evergreen\n      local_file: gopath/src/github.com/evergreen-ci/evergreen/clients/${goos}_${goarch}/evergreen\n      permissions: public-read\n      remote_file: evergreen/${task_id}/evergreen-ci/evergreen/clients/${goos}_${goarch}/evergreen\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: set-var\n  - func: run-make\n    vars:\n      target: set-project-var\n  - func: run-make\n    vars:\n      target: load-smoke-data\n  - command: subprocess.exec\n    params:\n      command: bash scripts/setup-smoke-config.sh ${github_token}\n      silent: true\n      working_dir: gopath/src/github.com/evergreen-ci/evergreen\n  - func: run-make\n    vars:\n      target: set-smoke-vars\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - smoke\n- name: generate-lint\n  commands:\n  - func: get-project\n  - func: run-make\n    vars:\n      target: ${task_name}\n  - type: system\n    command: s3.put\n    params:\n      aws_key: ${aws_key}\n      aws_secret: ${aws_secret}\n      bucket: mciuploads\n      content_type: application/json\n      display_name: generate-lint.json\n      local_file: gopath/src/github.com/evergreen-ci/evergreen/bin/generate-lint.json\n      permissions: public-read\n      remote_file: evergreen/${build_id}-${build_variant}/bin/generate-lint.json\n  - command: generate.tasks\n    params:\n      files:\n      - gopath/src/github.com/evergreen-ci/evergreen/bin/generate-lint.json\n- name: js-test\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n- name: dist\n  commands:\n  - func: get-project\n  - func: run-make\n    vars:\n      target: ${task_name}\n  - type: system\n    command: s3.put\n    params:\n      aws_key: ${aws_key}\n      aws_secret: ${aws_secret}\n      bucket: mciuploads\n      content_type: application/x-gzip\n      display_name: dist.tar.gz\n      local_file: gopath/src/github.com/evergreen-ci/evergreen/bin/${task_name}.tar.gz\n      optional: true\n      permissions: public-read\n      remote_file: evergreen/${build_id}-${build_variant}/evergreen-${task_name}-${revision}.tar.gz\n- name: test-auth\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - nodb\n  - test\n- name: test-rest-route\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-rest-client\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n  - agent\n- name: test-rest-model\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-command\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - test\n  - db\n  - agent\n- name: test-units\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - test\n  - db\n- name: test-agent\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n  - agent\n- name: test-rest-data\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-operations\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n  - cli\n- name: test-db\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-cloud\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-repotracker\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-scheduler\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-service\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-monitor\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-evergreen\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-thirdparty\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n  - agent\n- name: test-trigger\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-util\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - nodb\n  - test\n  - agent\n- name: test-validator\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-alertrecord\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-artifact\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-build\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-event\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-host\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-notification\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-patch\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-stats\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-task\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-testresult\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-user\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-distro\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-commitqueue\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-manifest\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-plugin\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-migrations\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: test-model-grid\n  commands:\n  - func: get-project\n  - func: setup-credentials\n  - func: setup-mongodb\n  - func: run-make\n    vars:\n      target: revendor\n  - func: run-make\n    vars:\n      target: ${task_name}\n  tags:\n  - db\n  - test\n- name: lint-model-alertrecord\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-alertrecord\n- name: lint-model-patch\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-patch\n- name: lint-plugin\n  commands:\n  - func: run-make\n    vars:\n      target: lint-plugin\n- name: lint-rest-route\n  commands:\n  - func: run-make\n    vars:\n      target: lint-rest-route\n- name: lint-model-stats\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-stats\n- name: lint-repotracker\n  commands:\n  - func: run-make\n    vars:\n      target: lint-repotracker\n- name: lint-rest-client\n  commands:\n  - func: run-make\n    vars:\n      target: lint-rest-client\n- name: lint-validator\n  commands:\n  - func: run-make\n    vars:\n      target: lint-validator\n- name: lint-evergreen\n  commands:\n  - func: run-make\n    vars:\n      target: lint-evergreen\n- name: lint-apimodels\n  commands:\n  - func: run-make\n    vars:\n      target: lint-apimodels\n- name: lint-model-manifest\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-manifest\n- name: lint-scheduler\n  commands:\n  - func: run-make\n    vars:\n      target: lint-scheduler\n- name: lint-trigger\n  commands:\n  - func: run-make\n    vars:\n      target: lint-trigger\n- name: lint-model-testresult\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-testresult\n- name: lint-service-testutil\n  commands:\n  - func: run-make\n    vars:\n      target: lint-service-testutil\n- name: lint-units\n  commands:\n  - func: run-make\n    vars:\n      target: lint-units\n- name: lint-auth\n  commands:\n  - func: run-make\n    vars:\n      target: lint-auth\n- name: lint-command\n  commands:\n  - func: run-make\n    vars:\n      target: lint-command\n- name: lint-db\n  commands:\n  - func: run-make\n    vars:\n      target: lint-db\n- name: lint-model-artifact\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-artifact\n- name: lint-model-event\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-event\n- name: lint-model\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model\n- name: lint-model-commitqueue\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-commitqueue\n- name: lint-model-grid\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-grid\n- name: lint-model-notification\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-notification\n- name: lint-service\n  commands:\n  - func: run-make\n    vars:\n      target: lint-service\n- name: lint-cloud\n  commands:\n  - func: run-make\n    vars:\n      target: lint-cloud\n- name: lint-migrations\n  commands:\n  - func: run-make\n    vars:\n      target: lint-migrations\n- name: lint-model-distro\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-distro\n- name: lint-model-task\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-task\n- name: lint-thirdparty\n  commands:\n  - func: run-make\n    vars:\n      target: lint-thirdparty\n- name: lint-model-build\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-build\n- name: lint-monitor\n  commands:\n  - func: run-make\n    vars:\n      target: lint-monitor\n- name: lint-operations\n  commands:\n  - func: run-make\n    vars:\n      target: lint-operations\n- name: lint-rest-model\n  commands:\n  - func: run-make\n    vars:\n      target: lint-rest-model\n- name: lint-agent\n  commands:\n  - func: run-make\n    vars:\n      target: lint-agent\n- name: lint-model-host\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-host\n- name: lint-model-user\n  commands:\n  - func: run-make\n    vars:\n      target: lint-model-user\n- name: lint-rest-data\n  commands:\n  - func: run-make\n    vars:\n      target: lint-rest-data\n- name: lint-util\n  commands:\n  - func: run-make\n    vars:\n      target: lint-util\nloggers:\n  agent:\n  - type: evergreen\n    log_directory: \"\"\n  system:\n  - type: evergreen\n    log_directory: \"\"\n  task:\n  - type: evergreen\n    log_directory: \"\"\n  - type: file\n    log_directory: \"\"\n"
	proj := &Project{}
	assert.NoError(LoadProjectInto([]byte(confStr), "mci", proj))
	assert.NotNil(proj.FindTaskGroup("lint-group"))
}
