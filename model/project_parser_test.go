package model

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/util"
	. "github.com/smartystreets/goconvey/convey"
)

// ShouldContainResembling tests whether a slice contains an element that DeepEquals
// the expected input.
func ShouldContainResembling(actual interface{}, expected ...interface{}) string {
	if len(expected) != 1 {
		return "ShouldContainResembling takes 1 argument"
	}
	if !util.SliceContains(actual, expected[0]) {
		return fmt.Sprintf("%#v does not contain %#v", actual, expected[0])
	}
	return ""
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
			So(p.Tasks[2].DependsOn[0].Name, ShouldEqual, "compile")
			So(p.Tasks[2].DependsOn[0].PatchOptional, ShouldEqual, false)
			So(p.Tasks[2].DependsOn[1].Name, ShouldEqual, "task0")
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
			So(p.Tasks[2].DependsOn[0].Name, ShouldEqual, "task0")
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
			So(bv.Tasks[1].DependsOn[0].taskSelector, ShouldResemble,
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
			So(bv.Tasks[1].DependsOn[0].taskSelector, ShouldResemble, taskSelector{Name: "t3"})
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
					{taskSelector: taskSelector{Name: "t1"}},
					{taskSelector: taskSelector{
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
					{taskSelector: taskSelector{Name: "*"}}}},
				{Name: "t3", DependsOn: parserDependencies{
					{taskSelector: taskSelector{
						Name: ".b", Variant: &variantSelector{stringSelector: ".cool !v2"}}},
					{taskSelector: taskSelector{
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
					{taskSelector: taskSelector{Name: ".cool"}},
					{taskSelector: taskSelector{Name: "!!.cool"}},                                                  //[1] illegal selector
					{taskSelector: taskSelector{Name: "!.c !.b", Variant: &variantSelector{stringSelector: "v1"}}}, //[2] no matching tasks
					{taskSelector: taskSelector{Name: "t1", Variant: &variantSelector{stringSelector: ".nope"}}},   //[3] no matching variants
					{taskSelector: taskSelector{Name: "t1"}, Status: "*"},                                          // valid, but:
					{taskSelector: taskSelector{Name: ".b"}},                                                       //[4] conflicts with above
				}},
			}
			out, errs := translateProject(pp)
			So(out, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 4)
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
			So(len(errs), ShouldEqual, 4)
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
				Tasks: parserBVTasks{
					{Name: "t1"},
					{Name: ".z", DependsOn: parserDependencies{
						{taskSelector: taskSelector{Name: ".b"}}}},
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
				Tasks: parserBVTasks{
					{Name: "t1", Requires: taskSelectors{{Name: ".b"}}},
				},
			}}
			out, errs := translateProject(pp)
			So(out, ShouldNotBeNil)
			So(len(errs), ShouldEqual, 1)
		})
	})
}

func parserTaskSelectorTaskEval(tse *taskSelectorEvaluator, tasks parserBVTasks, expected []BuildVariantTask) {
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
		ts, errs := evaluateBVTasks(tse, vse, tasks)
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
					parserBVTasks{{Name: "white"}},
					[]BuildVariantTask{{Name: "white"}})
				parserTaskSelectorTaskEval(tse,
					parserBVTasks{{Name: "red", Priority: 500}, {Name: ".secondary"}},
					[]BuildVariantTask{{Name: "red", Priority: 500}, {Name: "orange"}, {Name: "purple"}, {Name: "green"}})
				parserTaskSelectorTaskEval(tse,
					parserBVTasks{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: ".warm .secondary", Distros: []string{"d1"}}},
					[]BuildVariantTask{{Name: "orange", Distros: []string{"d1"}}})
				parserTaskSelectorTaskEval(tse,
					parserBVTasks{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: "!.warm .secondary", Distros: []string{"d1"}}},
					[]BuildVariantTask{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: "purple", Distros: []string{"d1"}},
						{Name: "green", Distros: []string{"d1"}}})
				parserTaskSelectorTaskEval(tse,
					parserBVTasks{{Name: "*"}},
					[]BuildVariantTask{
						{Name: "red"}, {Name: "blue"}, {Name: "yellow"},
						{Name: "orange"}, {Name: "purple"}, {Name: "green"},
						{Name: "brown"}, {Name: "white"}, {Name: "black"},
					})
				parserTaskSelectorTaskEval(tse,
					parserBVTasks{
						{Name: "red", Priority: 100},
						{Name: "!.warm .secondary", Priority: 100}},
					[]BuildVariantTask{
						{Name: "red", Priority: 100},
						{Name: "purple", Priority: 100},
						{Name: "green", Priority: 100}})
			})
		})
	})
}
