package model

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go/aws/endpoints"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
	yaml "gopkg.in/yaml.v3"
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
			p, err := createIntermediateProject([]byte(simple), false)
			So(p, ShouldNotBeNil)
			So(err, ShouldBeNil)
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
			p, err := createIntermediateProject([]byte(single), false)
			So(p, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(p.Tasks[2].DependsOn[0].TaskSelector.Name, ShouldEqual, "task0")
		})
		Convey("a file with a nameless dependency should error", func() {
			Convey("with a single dep", func() {
				nameless := `
tasks:
- name: "compile"
  depends_on: ""
`
				p, err := createIntermediateProject([]byte(nameless), false)
				So(p, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
			Convey("or multiple", func() {
				nameless := `
tasks:
- name: "compile"
  depends_on:
  - name: "task1"
  - status: "failed" #this has no task attached
`
				p, err := createIntermediateProject([]byte(nameless), false)
				So(p, ShouldBeNil)
				So(err, ShouldNotBeNil)
			})
			Convey("but an unused depends_on field should not error", func() {
				nameless := `
tasks:
- name: "compile"
`
				p, err := createIntermediateProject([]byte(nameless), false)
				So(p, ShouldNotBeNil)
				So(err, ShouldBeNil)
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
    stepback: false
    priority: 77
`
			p, err := createIntermediateProject([]byte(simple), false)
			So(p, ShouldNotBeNil)
			So(err, ShouldBeNil)
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
`
			p, err := createIntermediateProject([]byte(simple), false)
			So(p, ShouldNotBeNil)
			So(err, ShouldBeNil)
			bv := p.BuildVariants[0]
			So(bv.Name, ShouldEqual, "v1")
			So(bv.Tasks[0].Name, ShouldEqual, "t1")
			So(bv.Tasks[1].Name, ShouldEqual, "t2")
			So(bv.Tasks[1].DependsOn[0].TaskSelector, ShouldResemble, taskSelector{Name: "t3"})
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
			p, err := createIntermediateProject([]byte(simple), false)
			So(p, ShouldNotBeNil)
			So(err, ShouldBeNil)
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
			p, err := createIntermediateProject([]byte(single), false)
			So(p, ShouldNotBeNil)
			So(err, ShouldBeNil)
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
			p, err := createIntermediateProject([]byte(single), false)
			So(p, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(p.BuildVariants[0].Tasks[0].RunOn[0], ShouldEqual, "test")
			So(p.BuildVariants[0].Tasks[0].Distros, ShouldBeNil)
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
			p, err := createIntermediateProject([]byte(single), false)
			So(p, ShouldBeNil)
			So(err, ShouldNotBeNil)
		})
		Convey("a file with a commit queue merge task should parse", func() {
			single := `
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
    commit_queue_merge: true
`
			p, err := createIntermediateProject([]byte(single), false)
			So(p, ShouldNotBeNil)
			So(err, ShouldBeNil)
			bv := p.BuildVariants[0]
			So(bv.Name, ShouldEqual, "v1")
			So(len(bv.Tasks), ShouldEqual, 1)
			So(bv.Tasks[0].Name, ShouldEqual, "t1")
			So(bv.Tasks[0].CommitQueueMerge, ShouldBeTrue)
		})
	})
}

func TestIntermediateProjectWithActivate(t *testing.T) {
	yml := `
tasks:
- name: "t1"
buildvariants:
- name: "v1"
  activate: false 
  run_on: "distro1"
  tasks: 
  - name: "t1"
    activate: true
`
	p, err := createIntermediateProject([]byte(yml), false)
	assert.NoError(t, err)
	assert.NotNil(t, p)
	bv := p.BuildVariants[0]
	assert.False(t, utility.FromBoolTPtr(bv.Activate))
	assert.True(t, utility.FromBoolPtr(bv.Tasks[0].Activate))
}

func TestTranslateTasks(t *testing.T) {
	parserProject := &ParserProject{
		BuildVariants: []parserBV{
			{
				Name: "bv0",
				Tasks: parserBVTaskUnits{
					{
						Name:            "my_task",
						ExecTimeoutSecs: 30,
					},
					{
						Name:       "your_task",
						GitTagOnly: utility.TruePtr(),
					},
					{
						Name:            "my_tg",
						RunOn:           []string{"my_distro"},
						ExecTimeoutSecs: 20,
					},
				},
			},
			{
				Name:      "patch_only_bv",
				PatchOnly: utility.FalsePtr(),
				Tasks: parserBVTaskUnits{
					{
						Name: "your_task",
					},
					{
						Name: "my_task",
					},
					{
						Name: "a_task_with_no_special_configuration",
					},
					{
						Name:      "a_task_with_build_variant_task_configuration",
						PatchOnly: utility.TruePtr(),
					},
				},
			},
			{
				Name:      "unpatchable_bv",
				Patchable: utility.FalsePtr(),
				Tasks: parserBVTaskUnits{
					{
						Name: "a_task_with_no_special_configuration",
					},
					{
						Name:      "a_task_with_build_variant_task_configuration",
						Patchable: utility.TruePtr(),
					},
				},
			},
			{
				Name:           "allow_for_git_tag_bv",
				AllowForGitTag: utility.FalsePtr(),
				Tasks: parserBVTaskUnits{
					{
						Name: "a_task_with_no_special_configuration",
					},
					{
						Name:           "a_task_with_build_variant_task_configuration",
						AllowForGitTag: utility.TruePtr(),
					},
				},
			},
			{
				Name:       "git_tag_only_bv",
				GitTagOnly: utility.TruePtr(),
				Tasks: parserBVTaskUnits{
					{
						Name: "a_task_with_no_special_configuration",
					},
					{
						Name:       "a_task_with_build_variant_task_configuration",
						GitTagOnly: utility.FalsePtr(),
					},
				},
			},
			{
				Name:    "disabled_bv",
				Disable: utility.TruePtr(),
				Tasks: parserBVTaskUnits{
					{
						Name: "your_task",
					},
					{
						Name:    "my_task",
						Disable: utility.FalsePtr(),
					},
				},
			},
		},
		Tasks: []parserTask{
			{Name: "my_task", PatchOnly: utility.TruePtr(), ExecTimeoutSecs: 15},
			{Name: "your_task", GitTagOnly: utility.FalsePtr(), Stepback: utility.TruePtr(), RunOn: []string{"a different distro"}},
			{Name: "tg_task", PatchOnly: utility.TruePtr(), RunOn: []string{"a different distro"}},
			{Name: "a_task_with_no_special_configuration"},
			{Name: "a_task_with_build_variant_task_configuration"},
		},
		TaskGroups: []parserTaskGroup{{
			Name:  "my_tg",
			Tasks: []string{"tg_task"},
		}},
	}
	out, err := TranslateProject(parserProject)
	assert.NoError(t, err)
	assert.NotNil(t, out)
	require.Len(t, out.Tasks, 5)
	require.Len(t, out.BuildVariants, 6)

	for _, bv := range out.BuildVariants {
		for _, bvtu := range bv.Tasks {
			assert.Equal(t, bv.Name, bvtu.Variant, "build variant task unit's variant should be properly linked")
		}
	}

	assert.Equal(t, "bv0", out.BuildVariants[0].Name)
	require.Len(t, out.BuildVariants[0].Tasks, 3)
	assert.Equal(t, "my_task", out.BuildVariants[0].Tasks[0].Name)
	assert.Equal(t, 30, out.BuildVariants[0].Tasks[0].ExecTimeoutSecs)
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[0].Tasks[0].PatchOnly))
	assert.Equal(t, "your_task", out.BuildVariants[0].Tasks[1].Name)
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[0].Tasks[1].GitTagOnly))
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[0].Tasks[1].Stepback))

	assert.Contains(t, out.BuildVariants[0].Tasks[1].RunOn, "a different distro")

	assert.Equal(t, "my_tg", out.BuildVariants[0].Tasks[2].Name)
	bvt := out.FindTaskForVariant("my_tg", "bv0")
	require.NotNil(t, bvt)
	assert.Equal(t, "my_tg", bvt.Name)
	assert.Nil(t, bvt.PatchOnly)
	assert.Contains(t, bvt.RunOn, "my_distro")
	assert.Equal(t, 20, bvt.ExecTimeoutSecs)
	assert.True(t, bvt.IsGroup)

	bvt = out.FindTaskForVariant("tg_task", "bv0")
	assert.Equal(t, "my_tg", bvt.Name, "task within a task group retains its task group name in resulting build variant task unit")
	assert.NotNil(t, bvt)
	assert.True(t, utility.FromBoolPtr(bvt.PatchOnly))
	assert.Contains(t, bvt.RunOn, "my_distro")
	assert.True(t, bvt.IsGroup)

	assert.Equal(t, "patch_only_bv", out.BuildVariants[1].Name)
	require.Len(t, out.BuildVariants[1].Tasks, 4)
	assert.Equal(t, "your_task", out.BuildVariants[1].Tasks[0].Name)
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[1].Tasks[0].Stepback))
	assert.False(t, utility.FromBoolPtr(out.BuildVariants[1].Tasks[0].PatchOnly))
	assert.False(t, utility.FromBoolPtr(out.BuildVariants[1].Tasks[0].GitTagOnly))
	assert.Equal(t, "my_task", out.BuildVariants[1].Tasks[1].Name)
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[1].Tasks[1].PatchOnly))
	assert.Equal(t, "a_task_with_no_special_configuration", out.BuildVariants[1].Tasks[2].Name)
	assert.False(t, utility.FromBoolPtr(out.BuildVariants[1].Tasks[2].PatchOnly))
	assert.Equal(t, "a_task_with_build_variant_task_configuration", out.BuildVariants[1].Tasks[3].Name)
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[1].Tasks[3].PatchOnly))

	assert.Equal(t, "unpatchable_bv", out.BuildVariants[2].Name)
	require.Len(t, out.BuildVariants[2].Tasks, 2)
	assert.Equal(t, "a_task_with_no_special_configuration", out.BuildVariants[2].Tasks[0].Name)
	assert.False(t, utility.FromBoolPtr(out.BuildVariants[2].Tasks[0].Patchable))
	assert.Equal(t, "a_task_with_build_variant_task_configuration", out.BuildVariants[2].Tasks[1].Name)
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[2].Tasks[1].Patchable))

	assert.Equal(t, "allow_for_git_tag_bv", out.BuildVariants[3].Name)
	require.Len(t, out.BuildVariants[3].Tasks, 2)
	assert.Equal(t, "a_task_with_no_special_configuration", out.BuildVariants[3].Tasks[0].Name)
	assert.False(t, utility.FromBoolTPtr(out.BuildVariants[3].Tasks[0].AllowForGitTag))
	assert.Equal(t, "a_task_with_build_variant_task_configuration", out.BuildVariants[3].Tasks[1].Name)
	assert.True(t, utility.FromBoolTPtr(out.BuildVariants[3].Tasks[1].AllowForGitTag))

	assert.Equal(t, "git_tag_only_bv", out.BuildVariants[4].Name)
	require.Len(t, out.BuildVariants[4].Tasks, 2)
	assert.Equal(t, "a_task_with_no_special_configuration", out.BuildVariants[4].Tasks[0].Name)
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[4].Tasks[0].GitTagOnly))
	assert.Equal(t, "a_task_with_build_variant_task_configuration", out.BuildVariants[4].Tasks[1].Name)
	assert.False(t, utility.FromBoolPtr(out.BuildVariants[4].Tasks[1].GitTagOnly))

	assert.Equal(t, "disabled_bv", out.BuildVariants[5].Name)
	require.Len(t, out.BuildVariants[5].Tasks, 2)
	assert.Equal(t, "your_task", out.BuildVariants[5].Tasks[0].Name)
	assert.False(t, utility.FromBoolPtr(out.BuildVariants[5].Tasks[0].GitTagOnly))
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[5].Tasks[0].Stepback))
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[5].Tasks[0].Disable))
	assert.Equal(t, "my_task", out.BuildVariants[5].Tasks[1].Name)
	assert.True(t, utility.FromBoolPtr(out.BuildVariants[5].Tasks[1].PatchOnly))
	assert.False(t, utility.FromBoolPtr(out.BuildVariants[5].Tasks[1].Disable))
}

func TestTranslateDependsOn(t *testing.T) {
	Convey("With an intermediate parseProject", t, func() {
		pp := &ParserProject{}
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
			out, err := TranslateProject(pp)
			So(out, ShouldNotBeNil)
			So(err, ShouldBeNil)
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
			out, err := TranslateProject(pp)
			So(out, ShouldNotBeNil)
			So(err, ShouldBeNil)
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

			out, err := TranslateProject(pp)
			So(out, ShouldNotBeNil)
			So(err, ShouldNotBeNil)
			So(len(strings.Split(err.Error(), "\n")), ShouldEqual, 6)
		})
	})
}

func TestTranslateBuildVariants(t *testing.T) {
	Convey("With an intermediate parseProject", t, func() {
		pp := &ParserProject{}
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
				},
			}}

			out, err := TranslateProject(pp)
			So(out, ShouldNotBeNil)
			So(err, ShouldBeNil)
			So(len(out.BuildVariants), ShouldEqual, 1)
			bvts := out.BuildVariants[0].Tasks
			So(len(bvts), ShouldEqual, 2)
			So(bvts[0].Name, ShouldEqual, "t1")
			So(bvts[1].Name, ShouldEqual, "t2")
			So(bvts[0].CommitQueueMerge, ShouldBeTrue)
			So(bvts[1].DependsOn[0].Name, ShouldEqual, "t3")
		})
	})
}

func parserTaskSelectorTaskEval(tse *taskSelectorEvaluator, tasks parserBVTaskUnits, taskDefs []parserTask, expected []BuildVariantTaskUnit) {
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
		pbv := parserBV{Name: "build-variant-wow", Tasks: tasks}
		ts, errs := evaluateBVTasks(tse, nil, vse, pbv, taskDefs)
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
				So(t.Variant, ShouldEqual, pbv.Name)
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
					taskDefs,
					[]BuildVariantTaskUnit{{Name: "white"}})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{{Name: "red", Priority: 500}, {Name: ".secondary"}},
					taskDefs,
					[]BuildVariantTaskUnit{{Name: "red", Priority: 500}, {Name: "orange"}, {Name: "purple"}, {Name: "green"}})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: ".warm .secondary", Distros: []string{"d1"}}},
					taskDefs,
					[]BuildVariantTaskUnit{{Name: "orange", RunOn: []string{"d1"}}})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: "!.warm .secondary", Distros: []string{"d1"}}},
					taskDefs,
					[]BuildVariantTaskUnit{
						{Name: "orange", RunOn: []string{"d1"}},
						{Name: "purple", RunOn: []string{"d1"}},
						{Name: "green", RunOn: []string{"d1"}}})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{{Name: "*"}},
					taskDefs,
					[]BuildVariantTaskUnit{
						{Name: "red"}, {Name: "blue"}, {Name: "yellow"},
						{Name: "orange"}, {Name: "purple"}, {Name: "green"},
						{Name: "brown"}, {Name: "white"}, {Name: "black"},
					})
				parserTaskSelectorTaskEval(tse,
					parserBVTaskUnits{
						{Name: "red", Priority: 100},
						{Name: "!.warm .secondary", Priority: 100}},
					taskDefs,
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
	p, err := createIntermediateProject([]byte(yml), false)

	// check that display tasks in bv1 parsed correctly
	assert.NoError(err)
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

func TestParameterParsing(t *testing.T) {
	yml := `
parameters:
- key: "iter_count"
  value: "3"
  description: "you know it"
- key: buggy
  value: driver
`
	p, err := createIntermediateProject([]byte(yml), false)
	assert.NoError(t, err)
	require.Len(t, p.Parameters, 2)
	assert.Equal(t, "iter_count", p.Parameters[0].Key)
	assert.Equal(t, "3", p.Parameters[0].Value)
	assert.Equal(t, "you know it", p.Parameters[0].Description)
	assert.Equal(t, "buggy", p.Parameters[1].Key)
	assert.Equal(t, "driver", p.Parameters[1].Value)
}

func TestContainerParsing(t *testing.T) {
	yml := `
containers:
- name: "container_1"
  working_dir: "/workdir"
  image: "demo/image:latest"
  resources:
    cpu: 1
    memory_mb: 200
  system:
    cpu_architecture: "arm64"
    operating_system: "windows"
    windows_version: "2019"
- name: "container_2"
  working_dir: "/otherdir"
  image: "sample/image:latest"
  size: "XL"
  system:
    cpu_architecture: "x86_64"
    operating_system: "linux"
`
	p := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", p)
	assert.NoError(t, err)
	require.Len(t, p.Containers, 2)
	assert.Equal(t, "container_1", p.Containers[0].Name)
	assert.Equal(t, "/workdir", p.Containers[0].WorkingDir)
	assert.Equal(t, "demo/image:latest", p.Containers[0].Image)
	assert.Equal(t, 1, p.Containers[0].Resources.CPU)
	assert.Equal(t, 200, p.Containers[0].Resources.MemoryMB)
	assert.Equal(t, "arm64", string(p.Containers[0].System.CPUArchitecture))
	assert.Equal(t, "windows", string(p.Containers[0].System.OperatingSystem))
	assert.Equal(t, "2019", string(p.Containers[0].System.WindowsVersion))

	assert.Equal(t, "container_2", p.Containers[1].Name)
	assert.Equal(t, "/otherdir", p.Containers[1].WorkingDir)
	assert.Equal(t, "sample/image:latest", p.Containers[1].Image)
	assert.Equal(t, "XL", p.Containers[1].Size)
	assert.Equal(t, "linux", string(p.Containers[1].System.OperatingSystem))
	assert.Equal(t, "x86_64", string(p.Containers[1].System.CPUArchitecture))
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

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(validYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
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

	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(nonexistentTaskYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Contains(err.Error(), "notHere: nothing named 'notHere'")
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
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

	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(duplicateTaskYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.NotNil(err)
	assert.Contains(err.Error(), "execution task 'execTask3' is listed in more than 1 display task")
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

	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(conflictYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.NotNil(err)
	assert.Contains(err.Error(), "display task 'execTask1' cannot have the same name as an execution task")
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

	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(wildcardYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 3)

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

	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(tagYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 2)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
	assert.Len(proj.BuildVariants[0].DisplayTasks[1].ExecTasks, 2)
	assert.Equal("execTask1", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[0])
	assert.Equal("execTask3", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[1])
	assert.Equal("execTask2", proj.BuildVariants[0].DisplayTasks[1].ExecTasks[0])
	assert.Equal("execTask4", proj.BuildVariants[0].DisplayTasks[1].ExecTasks[1])
}

func TestLoadProjectIntoStrict(t *testing.T) {
	exampleYml := `
exec_timeout_secs: 100
tasks:
- name: task1
  exec_timeout_secs: 100
  commands:
  - command: shell.exec
    params:
      script: echo test
buildvariants:
- name: "bv"
  display_name: "bv_display"
  run_on: "example_distro"
  not_a_field: true
  tasks:
  - name: task1
`
	proj := Project{}
	ctx := context.Background()
	opts := &GetProjectOpts{
		UnmarshalStrict: true,
	}
	_, err := LoadProjectInto(ctx, []byte(exampleYml), opts, "example_project", &proj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "field not_a_field not found")

	// variables shouldn't error
	yamlWithVariables := `
variables:
tasks:
- name: task1
  commands:
  - command: shell.exec
    params:
      script: echo test
buildvariants:
- name: "bv"
  display_name: "bv_display"
  run_on: "example_distro"
  tasks:
  - name: task1
`
	_, err = LoadProjectInto(ctx, []byte(yamlWithVariables), opts, "example_project", &proj)
	require.NoError(t, err)

	// duplicates should error
	yamlWithDup := `
tasks:
  - name: task1
    commands:
      - command: shell.exec
        params:
          script: echo test
    commands:
      - command: shell.exec
        params:
          script: echo duplicate
buildvariants:
  - name: "bv"
    display_name: "bv_display"
    run_on: "example_distro"
    tasks:
      - name: task1
`
	_, err = LoadProjectInto(ctx, []byte(yamlWithDup), opts, "example_project", &proj)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "already defined")

	// unless not using strict
	_, err = LoadProjectInto(ctx, []byte(yamlWithDup), nil, "example_project", &proj)
	assert.NoError(t, err)
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
	pp, err := createIntermediateProject([]byte(tagYml), false)
	assert.NotNil(pp)
	assert.NoError(err)
	require.Len(pp.BuildVariants[0].DisplayTasks, 2)
	assert.Len(pp.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 1)
	assert.Equal(".odd", pp.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Len(pp.BuildVariants[0].DisplayTasks[1].ExecutionTasks, 1)
	assert.Equal(".even", pp.BuildVariants[0].DisplayTasks[1].ExecutionTasks[0])

	proj, err := TranslateProject(pp)
	assert.NotNil(proj)
	assert.NoError(err)
	// assert parser project hasn't changed
	require.Len(pp.BuildVariants[0].DisplayTasks, 2)
	assert.Len(pp.BuildVariants[0].DisplayTasks[0].ExecutionTasks, 1)
	assert.Equal(".odd", pp.BuildVariants[0].DisplayTasks[0].ExecutionTasks[0])
	assert.Len(pp.BuildVariants[0].DisplayTasks[1].ExecutionTasks, 1)
	assert.Equal(".even", pp.BuildVariants[0].DisplayTasks[1].ExecutionTasks[0])

	//assert project is correct
	require.Len(proj.BuildVariants[0].DisplayTasks, 2)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
	assert.Len(proj.BuildVariants[0].DisplayTasks[1].ExecTasks, 2)
	assert.Equal("execTask1", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[0])
	assert.Equal("execTask3", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[1])
	assert.Equal("execTask2", proj.BuildVariants[0].DisplayTasks[1].ExecTasks[0])
	assert.Equal("execTask4", proj.BuildVariants[0].DisplayTasks[1].ExecTasks[1])
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

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(validYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
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

	// check that yml with inline task groups within its buildvariants correctly parses the group
	inlineYml := `
tasks:
- name: example_task_1
- name: example_task_2
task_groups:
- &example_task_group
  name: example_task_group
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
  - name: inline_task_group
    task_group:
      <<: *example_task_group
      tasks:
      - example_task_1
`
	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(inlineYml), nil, "id", proj)
	assert.Nil(err)
	assert.NotNil(proj)
	assert.Len(proj.BuildVariants[0].Tasks, 1)
	assert.NotNil(proj.BuildVariants[0].Tasks[0].TaskGroup)

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

	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(wrongTaskYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.NotNil(err)
	assert.Contains(err.Error(), `nothing named 'example_task_3'`)

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

	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(orderedYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
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
  display_name: "bv_display"
  tasks:
  - name: even_task_group
  - name: odd_task_group
`

	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(tagYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
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

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(validYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("task_group_1", tg.Name)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
	assert.Equal("task_1", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[0])
	assert.Equal("task_2", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[1])
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

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(validYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("task_group_1", tg.Name)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
	assert.Equal("task_1", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[0])
	assert.Equal("task_2", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[1])
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

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(validYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("task_group_1", tg.Name)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
	assert.Equal("task_1", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[0])
	assert.Equal("task_2", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[1])
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

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(validYml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
	assert.Len(proj.TaskGroups, 1)
	tg := proj.TaskGroups[0]
	assert.Equal("task_group_1", tg.Name)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
	assert.Equal("task_1", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[0])
	assert.Equal("task_2", proj.BuildVariants[0].DisplayTasks[0].ExecTasks[1])
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
  display_name: "bv_display"
  depends_on:
    - name: task_3
  tasks:
  - name: task_1
    depends_on:
      - name: task_4
  - name: task_2
- name: bv_2
  display_name: "bv_display"
  tasks:
    - name: task_3
- name: bv_3
  display_name: "bv_display"
  tasks:
    - name: task_4
    - name: task_5
`

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
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
  display_name: "bv_display"
  tasks:
  - name: task_1
  - name: task_2
    patch_only: false
- name: bv_2
  display_name: "bv_display"
  tasks:
  - name: task_1
    patch_only: false
  - name: task_2
    patch_only: true
- name: bv_3
  display_name: "bv_display"
  tasks:
  - name: task_2
`

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
	assert.Len(proj.BuildVariants, 3)

	assert.Len(proj.BuildVariants[0].Tasks, 2)
	assert.True(*proj.BuildVariants[0].Tasks[0].PatchOnly)
	assert.False(*proj.BuildVariants[0].Tasks[1].PatchOnly)

	assert.Len(proj.BuildVariants[1].Tasks, 2)
	assert.False(*proj.BuildVariants[1].Tasks[0].PatchOnly)
	assert.True(*proj.BuildVariants[1].Tasks[1].PatchOnly)

	assert.Len(proj.BuildVariants[2].Tasks, 1)
	assert.Nil(proj.BuildVariants[2].Tasks[0].PatchOnly)
}

func TestAllowForGitTagTasks(t *testing.T) {
	yml := `
tasks:
- name: task_1
  allow_for_git_tag: true
- name: task_2
buildvariants:
- name: bv_1
  display_name: "bv_display"
  tasks:
  - name: task_1
  - name: task_2
    allow_for_git_tag: false
- name: bv_2
  display_name: "bv_display"
  tasks:
  - name: task_1
    allow_for_git_tag: true
  - name: task_2
    allow_for_git_tag: false
- name: bv_3
  display_name: "bv_display"
  tasks:
  - name: task_2
`

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(t, proj)
	assert.Nil(t, err)
	assert.Len(t, proj.Tasks, 2)
	assert.True(t, *proj.Tasks[0].AllowForGitTag)

	assert.Len(t, proj.BuildVariants, 3)
	assert.Len(t, proj.BuildVariants[0].Tasks, 2)
	assert.True(t, *proj.BuildVariants[0].Tasks[0].AllowForGitTag) // loaded from task
	assert.False(t, *proj.BuildVariants[0].Tasks[1].AllowForGitTag)

	assert.Len(t, proj.BuildVariants[1].Tasks, 2)
	assert.True(t, *proj.BuildVariants[1].Tasks[0].AllowForGitTag)
	assert.False(t, *proj.BuildVariants[1].Tasks[1].AllowForGitTag)

	assert.Len(t, proj.BuildVariants[2].Tasks, 1)
	assert.Nil(t, proj.BuildVariants[2].Tasks[0].AllowForGitTag)
}

func TestGitTagOnlyTasks(t *testing.T) {
	yml := `
tasks:
- name: task_1
  git_tag_only: true
- name: task_2
buildvariants:
- name: bv_1
  display_name: "bv_display"
  tasks:
  - name: task_1
  - name: task_2
    git_tag_only: false
- name: bv_2
  display_name: "bv_display"
  tasks:
  - name: task_1
    git_tag_only: false
  - name: task_2
    git_tag_only: true
- name: bv_3
  display_name: "bv_display"
  tasks:
  - name: task_2
`

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(t, proj)
	assert.Nil(t, err)
	assert.Len(t, proj.BuildVariants, 3)

	assert.Len(t, proj.BuildVariants[0].Tasks, 2)
	assert.True(t, *proj.BuildVariants[0].Tasks[0].GitTagOnly) // loaded from task
	assert.False(t, *proj.BuildVariants[0].Tasks[1].GitTagOnly)

	assert.Len(t, proj.BuildVariants[1].Tasks, 2)
	assert.False(t, *proj.BuildVariants[1].Tasks[0].GitTagOnly)
	assert.True(t, *proj.BuildVariants[1].Tasks[1].GitTagOnly)

	assert.Len(t, proj.BuildVariants[2].Tasks, 1)
	assert.Nil(t, proj.BuildVariants[2].Tasks[0].GitTagOnly)
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

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(proj)
	assert.Nil(err)
	assert.Equal("something", proj.Loggers.Agent[0].Type)
	assert.Equal("idk", proj.Loggers.Agent[0].SplunkToken)
	assert.Equal("somethingElse", proj.Loggers.Agent[1].Type)
	assert.Equal("commandLogger", proj.Tasks[0].Commands[0].Loggers.System[0].Type)
}

func TestAddBuildVariant(t *testing.T) {
	pp := ParserProject{
		Identifier: utility.ToStringPtr("small"),
	}

	pp.AddBuildVariant("name", "my-name", "", nil, []string{"task"})
	require.Len(t, pp.BuildVariants, 1)
	assert.Equal(t, pp.BuildVariants[0].Name, "name")
	assert.Equal(t, pp.BuildVariants[0].DisplayName, "my-name")
	assert.Nil(t, pp.BuildVariants[0].RunOn)
	assert.Len(t, pp.BuildVariants[0].Tasks, 1)
}

func TestParserProjectStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	testutil.ConfigureIntegrationTest(t, env.Settings(), t.Name())

	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ppConf := env.Settings().Providers.AWS.ParserProject
	bucket, err := pail.NewS3BucketWithHTTPClient(c, pail.S3Options{
		Name:        ppConf.Bucket,
		Region:      endpoints.UsEast1RegionID,
		Credentials: pail.CreateAWSCredentials(ppConf.Key, ppConf.Secret, ""),
	})
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, bucket.RemovePrefix(ctx, ppConf.Prefix))
	}()

	for methodName, ppStorageMethod := range map[string]evergreen.ParserProjectStorageMethod{
		"DB": evergreen.ProjectStorageMethodDB,
		"S3": evergreen.ProjectStorageMethodS3,
	} {
		t.Run("StorageMethod"+methodName, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment){
				"FindOneByIDReturnsNilErrorAndResultForNonexistentParserProject": func(ctx context.Context, t *testing.T, env *mock.Environment) {
					ppStorage, err := GetParserProjectStorage(env.Settings(), ppStorageMethod)
					require.NoError(t, err)
					defer ppStorage.Close(ctx)

					pp, err := ppStorage.FindOneByID(ctx, "nonexistent")
					assert.NoError(t, err)
					assert.Zero(t, pp)
				},
				"FindOneByIDWithFieldsReturnsNilErrorAndResultForNonexistentParserProject": func(ctx context.Context, t *testing.T, env *mock.Environment) {
					ppStorage, err := GetParserProjectStorage(env.Settings(), ppStorageMethod)
					require.NoError(t, err)
					defer ppStorage.Close(ctx)

					pp, err := ppStorage.FindOneByIDWithFields(ctx, "nonexistent", ParserProjectBuildVariantsKey)
					assert.NoError(t, err)
					assert.Zero(t, pp)
				},
				"UpsertCreatesNewParserProject": func(ctx context.Context, t *testing.T, env *mock.Environment) {
					pp := &ParserProject{
						Id:    "my-project",
						Owner: utility.ToStringPtr("me"),
					}
					ppStorage, err := GetParserProjectStorage(env.Settings(), ppStorageMethod)
					require.NoError(t, err)
					defer ppStorage.Close(ctx)

					assert.NoError(t, ppStorage.UpsertOne(ctx, pp))

					pp, err = ppStorage.FindOneByID(ctx, pp.Id)
					assert.NoError(t, err)
					require.NotNil(t, pp)
					assert.Equal(t, "me", utility.FromStringPtr(pp.Owner))
				},
				"UpsertUpdatesExistingParserProject": func(ctx context.Context, t *testing.T, env *mock.Environment) {
					pp := &ParserProject{
						Id:    "my-project",
						Owner: utility.ToStringPtr("me"),
					}
					ppStorage, err := GetParserProjectStorage(env.Settings(), ppStorageMethod)
					require.NoError(t, err)
					defer ppStorage.Close(ctx)

					assert.NoError(t, ppStorage.UpsertOne(ctx, pp))
					pp.Owner = utility.ToStringPtr("you")
					assert.NoError(t, ppStorage.UpsertOne(ctx, pp))

					pp, err = ppStorage.FindOneByID(ctx, pp.Id)
					assert.NoError(t, err)
					require.NotNil(t, pp)
					assert.Equal(t, "you", utility.FromStringPtr(pp.Owner))
				},
				"PersistsSimpleYAML": func(ctx context.Context, t *testing.T, env *mock.Environment) {
					simpleYAML := `
loggers:
  agent:
    - type: something
      splunk_token: idk
    - type: somethingElse
parameters:
- key: iter_count
  value: 3
  description: "used for something"
tasks:
- name: task_1
  depends_on:
  - name: embedded_sdk_s3_put
    variant: embedded-sdk-android-arm32
  commands:
  - command: myCommand
    params:
      env:
        key: ${my_value}
        list_key: [1,2,3]
    loggers:
      system:
       - type: commandLogger
functions:
  function-with-updates:
    command: expansions.update
    params:
      updates: 
      - key: ssh_connection_options
        value: -o GSSAPIAuthentication=no -o CheckHostIP=no -o StrictHostKeyChecking=no -o UserKnownHostsFile=/dev/null -o ConnectTimeout=30 -o ConnectionAttempts=20
      - key: ssh_retries
        value: "10"
  run-make:
    command: subprocess.exec
    params:
      working_dir: gopath/src/github.com/evergreen-ci/evergreen
      binary: make
buildvariants:
- matrix_name: "my-matrix"
  matrix_spec: { version: ["4.0", "4.2"], os: "linux" }
  display_name: "${version} ${os} "
  tasks:
    - name: "task_1"
      batchtime: 60
`
					checkProjectPersists(ctx, t, env, []byte(simpleYAML), ppStorageMethod)
				},
				"PersistsEvergreenSelfTestsYAML": func(ctx context.Context, t *testing.T, env *mock.Environment) {
					filepath := filepath.Join(testutil.GetDirectoryOfFile(), "..", "self-tests.yml")
					yml, err := os.ReadFile(filepath)
					assert.NoError(t, err)
					checkProjectPersists(ctx, t, env, yml, ppStorageMethod)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					require.NoError(t, db.ClearCollections(ParserProjectCollection))
					require.NoError(t, bucket.RemovePrefix(ctx, ppConf.Prefix))

					testCase(ctx, t, env)
				})
			}
		})
	}
}

func checkProjectPersists(ctx context.Context, t *testing.T, env evergreen.Environment, yml []byte, ppStorageMethod evergreen.ParserProjectStorageMethod) {
	pp, err := createIntermediateProject(yml, false)
	assert.NoError(t, err)
	pp.Id = "my-project"
	pp.Identifier = utility.ToStringPtr("old-project-identifier")

	ppStorage, err := GetParserProjectStorage(env.Settings(), ppStorageMethod)
	require.NoError(t, err)
	defer ppStorage.Close(ctx)

	yamlToCompare, err := yaml.Marshal(pp)
	assert.NoError(t, err)

	assert.NoError(t, ppStorage.UpsertOne(ctx, pp))

	newPP, err := ppStorage.FindOneByID(ctx, pp.Id)
	assert.NoError(t, err)
	require.NotZero(t, newPP)

	newYaml, err := yaml.Marshal(newPP)
	assert.NoError(t, err)

	assert.True(t, bytes.Equal(newYaml, yamlToCompare))

	// ensure that updating with the re-parsed project doesn't error
	pp, err = createIntermediateProject(newYaml, false)
	assert.NoError(t, err)
	pp.Id = "my-project"
	pp.Identifier = utility.ToStringPtr("new-project-identifier")

	assert.NoError(t, ppStorage.UpsertOne(ctx, pp))

	newPP, err = ppStorage.FindOneByID(ctx, pp.Id)
	assert.NoError(t, err)
	require.NotZero(t, newPP)

	assert.Equal(t, newPP.Identifier, pp.Identifier)

	for i, f := range pp.Functions {
		list := f.List()
		for j := range list {
			assert.NotEmpty(t, list[j].Params)
			assert.EqualValues(t, list[j].Params, newPP.Functions[i].List()[j].Params)
		}
	}
}

func TestParserProjectRoundtrip(t *testing.T) {
	filepath := filepath.Join(testutil.GetDirectoryOfFile(), "..", "self-tests.yml")
	yml, err := os.ReadFile(filepath)
	assert.NoError(t, err)

	original, err := createIntermediateProject(yml, false)
	assert.NoError(t, err)

	// to and from yaml
	yamlBytes, err := yaml.Marshal(original)
	assert.NoError(t, err)
	pp := &ParserProject{}
	assert.NoError(t, yaml.Unmarshal(yamlBytes, pp))

	// to and from BSON
	bsonBytes, err := bson.Marshal(original)
	assert.NoError(t, err)
	bsonPP := &ParserProject{}
	assert.NoError(t, bson.Unmarshal(bsonBytes, bsonPP))

	// ensure bson actually worked
	newBytes, err := yaml.Marshal(bsonPP)
	assert.NoError(t, err)
	assert.True(t, bytes.Equal(yamlBytes, newBytes))
}

func TestMergeUnorderedUnique(t *testing.T) {
	main := &ParserProject{
		Tasks: []parserTask{
			{Name: "my_task", PatchOnly: utility.TruePtr(), ExecTimeoutSecs: 15},
			{Name: "your_task", GitTagOnly: utility.FalsePtr(), Stepback: utility.TruePtr(), RunOn: []string{"a different distro"}},
			{Name: "tg_task", PatchOnly: utility.TruePtr(), RunOn: []string{"a different distro"}},
		},
		TaskGroups: []parserTaskGroup{
			{
				Name:  "my_tg",
				Tasks: []string{"tg_task"},
			},
		},
		Parameters: []ParameterInfo{
			{
				Parameter: patch.Parameter{
					Key:   "key",
					Value: "val",
				},
			},
		},
		Modules: []Module{
			{
				Name: "my_module",
			},
		},
		Containers: []Container{
			{
				Name: "container1",
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"func1": {
				SingleCommand: &PluginCommandConf{
					Command: "single_command",
				},
			},
			"func2": {
				MultiCommand: []PluginCommandConf{
					{
						Command: "multi_command1",
					}, {
						Command: "multi_command2",
					},
				},
			},
		},
	}

	toMerge := &ParserProject{
		Tasks: []parserTask{
			{Name: "add_task"},
		},
		TaskGroups: []parserTaskGroup{
			{
				Name:  "add_group",
				Tasks: []string{"add_tg_task"},
			},
		},
		Parameters: []ParameterInfo{
			{
				Parameter: patch.Parameter{
					Key:   "add_key",
					Value: "add_val",
				},
			},
		},
		Modules: []Module{
			{
				Name: "add_my_module",
			},
		},
		Containers: []Container{
			{
				Name: "container2",
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"add_func1": &YAMLCommandSet{
				SingleCommand: &PluginCommandConf{
					Command: "add_single_command",
				},
			},
			"add_func2": &YAMLCommandSet{
				MultiCommand: []PluginCommandConf{
					{
						Command: "add_multi_command1",
					}, {
						Command: "add_multi_command2",
					},
				},
			},
		},
	}

	err := main.mergeUnorderedUnique(toMerge)
	assert.NoError(t, err)
	assert.Equal(t, len(main.Tasks), 4)
	assert.Equal(t, len(main.TaskGroups), 2)
	assert.Equal(t, len(main.Parameters), 2)
	assert.Equal(t, len(main.Modules), 2)
	assert.Equal(t, len(main.Functions), 4)
	assert.Equal(t, len(main.Containers), 2)
}

func TestMergeUnorderedUniqueFail(t *testing.T) {
	main := &ParserProject{
		Tasks: []parserTask{
			{Name: "my_task", PatchOnly: utility.TruePtr(), ExecTimeoutSecs: 15},
		},
		TaskGroups: []parserTaskGroup{
			{
				Name:  "my_tg",
				Tasks: []string{"tg_task"},
			},
		},
		Parameters: []ParameterInfo{
			{
				Parameter: patch.Parameter{
					Key:   "key",
					Value: "val",
				},
			},
		},
		Modules: []Module{
			{
				Name: "my_module",
			},
		},
		Containers: []Container{
			{
				Name: "my_container",
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"func1": &YAMLCommandSet{
				SingleCommand: &PluginCommandConf{
					Command: "single_command",
				},
			},
			"func2": &YAMLCommandSet{
				MultiCommand: []PluginCommandConf{
					{
						Command: "multi_command1",
					}, {
						Command: "multi_command2",
					},
				},
			},
		},
	}

	fail := &ParserProject{
		Tasks: []parserTask{
			{Name: "my_task", PatchOnly: utility.TruePtr(), ExecTimeoutSecs: 15},
		},
		TaskGroups: []parserTaskGroup{
			{
				Name:  "my_tg",
				Tasks: []string{"tg_task"},
			},
		},
		Parameters: []ParameterInfo{
			{
				Parameter: patch.Parameter{
					Key:   "key",
					Value: "val",
				},
			},
		},
		Modules: []Module{
			{
				Name: "my_module",
			},
		},
		Containers: []Container{
			{
				Name: "my_container",
			},
		},
		Functions: map[string]*YAMLCommandSet{
			"func1": {
				SingleCommand: &PluginCommandConf{
					Command: "single_command",
				},
			},
			"func2": {
				MultiCommand: []PluginCommandConf{
					{
						Command: "multi_command1",
					}, {
						Command: "multi_command2",
					},
				},
			},
		},
	}

	err := main.mergeUnorderedUnique(fail)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "task 'my_task' has been declared already")
	assert.Contains(t, err.Error(), "task group 'my_tg' has been declared already")
	assert.Contains(t, err.Error(), "parameter key 'key' has been declared already")
	assert.Contains(t, err.Error(), "module 'my_module' has been declared already")
	assert.Contains(t, err.Error(), "function 'func1' has been declared already")
	assert.Contains(t, err.Error(), "function 'func2' has been declared already")
	assert.Contains(t, err.Error(), "container 'my_container' has been declared already")
}

func TestMergeUnordered(t *testing.T) {
	main := &ParserProject{
		Ignore: parserStringSlice{
			"a",
		},
		Loggers: &LoggerConfig{
			Agent:  []LogOpts{{Type: BuildloggerLogSender}},
			System: []LogOpts{{Type: BuildloggerLogSender}},
			Task:   []LogOpts{{Type: BuildloggerLogSender}},
		},
	}

	add := &ParserProject{
		Ignore: parserStringSlice{
			"b",
		},
		Loggers: &LoggerConfig{
			Agent:  []LogOpts{{LogDirectory: "a"}},
			System: []LogOpts{{LogDirectory: "a"}},
			Task:   []LogOpts{{LogDirectory: "a"}},
		},
	}
	main.mergeUnordered(add)
	assert.Equal(t, len(main.Ignore), 2)
	assert.Equal(t, len(main.Loggers.Agent), 2)
	assert.Equal(t, len(main.Loggers.System), 2)
	assert.Equal(t, len(main.Loggers.Task), 2)
}

func TestMergeOrderedUnique(t *testing.T) {
	main := &ParserProject{
		Pre: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "pre",
			},
		},
		Post: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "post",
			},
		},
	}

	add := &ParserProject{
		Timeout: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "timeout",
			},
		},
		EarlyTermination: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "early termination",
			},
		},
	}

	err := main.mergeOrderedUnique(add)
	assert.NoError(t, err)
	assert.NotNil(t, main.Pre)
	assert.NotNil(t, main.Post)
	assert.NotNil(t, main.Timeout)
	assert.NotNil(t, main.EarlyTermination)
}

func TestMergeOrderedUniqueFail(t *testing.T) {
	main := &ParserProject{
		Pre: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "pre",
			},
		},
		Post: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "post",
			},
		},
		Timeout: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "timeout",
			},
		},
		EarlyTermination: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "early termination",
			},
		},
	}

	add := &ParserProject{
		Pre: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "add pre",
			},
		},
		Post: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "add post",
			},
		},
		Timeout: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "add timeout",
			},
		},
		EarlyTermination: &YAMLCommandSet{
			SingleCommand: &PluginCommandConf{
				Command: "add early termination",
			},
		},
	}

	err := main.mergeOrderedUnique(add)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pre can only be defined in one YAML")
	assert.Contains(t, err.Error(), "post can only be defined in one YAML")
	assert.Contains(t, err.Error(), "timeout can only be defined in one YAML")
	assert.Contains(t, err.Error(), "early termination can only be defined in one YAML")
}

func TestMergeUnique(t *testing.T) {
	main := &ParserProject{
		Stepback:    utility.ToBoolPtr(true),
		BatchTime:   utility.ToIntPtr(1),
		OomTracker:  utility.ToBoolPtr(true),
		DisplayName: utility.ToStringPtr("name"),
	}

	add := &ParserProject{
		PreTimeoutSecs:    utility.ToIntPtr(1),
		PostTimeoutSecs:   utility.ToIntPtr(1),
		PreErrorFailsTask: utility.ToBoolPtr(true),
		UnsetFunctionVars: utility.ToBoolPtr(true),
		CommandType:       utility.ToStringPtr("type"),
		CallbackTimeout:   utility.ToIntPtr(1),
		ExecTimeoutSecs:   utility.ToIntPtr(1),
	}

	err := main.mergeUnique(add)
	assert.NoError(t, err)
	assert.NotNil(t, main.Stepback)
	assert.NotNil(t, main.BatchTime)
	assert.NotNil(t, main.OomTracker)
	assert.NotNil(t, main.DisplayName)
	assert.NotNil(t, main.PreTimeoutSecs)
	assert.NotNil(t, main.PostTimeoutSecs)
	assert.NotNil(t, main.PreErrorFailsTask)
	assert.NotNil(t, main.UnsetFunctionVars)
	assert.NotNil(t, main.CommandType)
	assert.NotNil(t, main.CallbackTimeout)
	assert.NotNil(t, main.ExecTimeoutSecs)
}

func TestMergeUniqueFail(t *testing.T) {
	main := &ParserProject{
		Stepback:          utility.ToBoolPtr(true),
		BatchTime:         utility.ToIntPtr(1),
		OomTracker:        utility.ToBoolPtr(true),
		PreTimeoutSecs:    utility.ToIntPtr(1),
		PostTimeoutSecs:   utility.ToIntPtr(1),
		PreErrorFailsTask: utility.ToBoolPtr(true),
		UnsetFunctionVars: utility.ToBoolPtr(true),
		DisplayName:       utility.ToStringPtr("name"),
		CommandType:       utility.ToStringPtr("type"),
		CallbackTimeout:   utility.ToIntPtr(1),
		ExecTimeoutSecs:   utility.ToIntPtr(1),
	}

	add := &ParserProject{
		Stepback:          utility.ToBoolPtr(true),
		BatchTime:         utility.ToIntPtr(1),
		OomTracker:        utility.ToBoolPtr(true),
		PreTimeoutSecs:    utility.ToIntPtr(1),
		PostTimeoutSecs:   utility.ToIntPtr(1),
		PreErrorFailsTask: utility.ToBoolPtr(true),
		UnsetFunctionVars: utility.ToBoolPtr(true),
		DisplayName:       utility.ToStringPtr("name"),
		CommandType:       utility.ToStringPtr("type"),
		CallbackTimeout:   utility.ToIntPtr(1),
		ExecTimeoutSecs:   utility.ToIntPtr(1),
	}

	err := main.mergeUnique(add)
	assert.Contains(t, err.Error(), "stepback can only be defined in one YAML")
	assert.Contains(t, err.Error(), "batch time can only be defined in one YAML")
	assert.Contains(t, err.Error(), "OOM tracker can only be defined in one YAML")
	assert.Contains(t, err.Error(), "pre timeout secs can only be defined in one YAML")
	assert.Contains(t, err.Error(), "post timeout secs can only be defined in one YAML")
	assert.Contains(t, err.Error(), "pre error fails task can only be defined in one YAML")
	assert.Contains(t, err.Error(), "display name can only be defined in one YAML")
	assert.Contains(t, err.Error(), "command type can only be defined in one YAML")
	assert.Contains(t, err.Error(), "callback timeout can only be defined in one YAML")
	assert.Contains(t, err.Error(), "exec timeout secs can only be defined in one YAML")
}

func TestMergeBuildVariant(t *testing.T) {
	bvExisting := "a_variant"
	main := &ParserProject{
		BuildVariants: []parserBV{
			parserBV{
				Name:        bvExisting,
				DisplayName: "Defined here",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
				DisplayTasks: []displayTask{
					displayTask{
						Name:           "my_display_task_old_variant",
						ExecutionTasks: []string{"say-bye"},
					},
				},
			},
		},
	}
	bvNew1 := "another_variant"
	bvNew2 := "one_more_variant"
	add := &ParserProject{
		BuildVariants: []parserBV{
			parserBV{
				Name: bvExisting,
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "add this task",
					},
				},
			},
			parserBV{
				Name:      bvNew1,
				BatchTime: &bvBatchTime,
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "example_task_group",
					},
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
				DisplayTasks: []displayTask{
					displayTask{
						Name:           "my_display_task_new_variant",
						ExecutionTasks: []string{"another_task"},
					},
				},
			},
			parserBV{
				Name: bvNew2,
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
			},
		},
	}

	err := main.mergeBuildVariant(add)
	assert.NoError(t, err)
	require.Equal(t, len(main.BuildVariants), 3)
	bvNames := []string{}
	for _, bv := range main.BuildVariants {
		if bv.Name == bvNew1 {
			assert.Equal(t, len(bv.Tasks), 2)
		}
		bvNames = append(bvNames, bv.Name)
	}
	assert.Contains(t, bvNames, bvExisting)
	assert.Contains(t, bvNames, bvNew1)
	assert.Contains(t, bvNames, bvNew2)
}

// Allow build variants to specify tasks before being defined.
func TestMergeExistingBuildVariant(t *testing.T) {
	bvExisting := "a_variant"
	main := &ParserProject{
		BuildVariants: []parserBV{
			parserBV{
				Name: bvExisting,
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
				DisplayTasks: []displayTask{
					displayTask{
						Name:           "my_display_task_old_variant",
						ExecutionTasks: []string{"say-bye"},
					},
				},
			},
		},
	}
	add := &ParserProject{
		BuildVariants: []parserBV{
			parserBV{
				Name:        bvExisting,
				DisplayName: "Defined here",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "add this task",
					},
				},
			},
		},
	}

	err := main.mergeBuildVariant(add)
	assert.NoError(t, err)
	require.Equal(t, len(main.BuildVariants), 1)
	require.Equal(t, len(main.BuildVariants[0].Tasks), 2)
}

func TestMergeBuildVariantFail(t *testing.T) {
	main := &ParserProject{
		BuildVariants: []parserBV{
			parserBV{
				Name:        "a_variant",
				DisplayName: "duplicate",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
				DisplayTasks: []displayTask{
					displayTask{
						Name:           "my_display_task_old_variant",
						ExecutionTasks: []string{"say-bye"},
					},
				},
			},
		},
	}

	add := &ParserProject{
		BuildVariants: []parserBV{
			parserBV{
				Name:        "a_variant",
				DisplayName: "break test",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "add this task",
					},
				},
			},
		},
	}

	err := main.mergeBuildVariant(add)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "build variant 'a_variant' has non-task fields declared already")
}

func TestMergeMatrix(t *testing.T) {
	main := &ParserProject{
		Axes: []matrixAxis{
			{
				Id: "a",
				Values: []axisValue{
					{Id: "0", Tags: []string{"zero"}},
					{Id: "1", Tags: []string{"odd"}},
					{Id: "2", Tags: []string{"even", "prime"}},
					{Id: "3", Tags: []string{"odd", "prime"}},
				},
			},
		},
	}

	add := &ParserProject{}

	err := main.mergeMatrix(add)
	assert.NoError(t, err)
	assert.Equal(t, len(main.Axes), 1)
}

func TestMergeMatrixFail(t *testing.T) {
	main := &ParserProject{
		Axes: []matrixAxis{
			{
				Id: "a",
				Values: []axisValue{
					{Id: "0", Tags: []string{"zero"}},
					{Id: "1", Tags: []string{"odd"}},
					{Id: "2", Tags: []string{"even", "prime"}},
					{Id: "3", Tags: []string{"odd", "prime"}},
				},
			},
		},
	}

	add := &ParserProject{
		Axes: []matrixAxis{
			{
				Id: "b",
				Values: []axisValue{
					{Id: "0", Tags: []string{"zero"}},
					{Id: "1", Tags: []string{"odd"}},
					{Id: "2", Tags: []string{"even", "prime"}},
					{Id: "3", Tags: []string{"odd", "prime"}},
				},
			},
		},
	}

	err := main.mergeMatrix(add)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "matrixes can only be defined in one YAML")
}

func TestMergeMultipleProjectConfigs(t *testing.T) {
	mainYaml := `
include: 
  - filename: small.yml
    module: something_different
post:
  - command: command_number_1
tasks:
  - name: my_task
    commands:
      - func: main_function
functions:
  main_function:
    command: definition_1
modules:
- name: "something_different"
  repo: "git@github.com:foo/bar.git"
  prefix: "src/third_party"
  branch: "master"
ignore:
  - "*.md"
  - "scripts/*"
`
	smallYaml := `
stepback: true
tasks:
  - name: small_task
    commands:
      - func: small_function
functions:
  small_function:
    command: definition_3
ignore:
  - ".github/*"
`

	p1, err := createIntermediateProject([]byte(mainYaml), false)
	assert.NoError(t, err)
	assert.NotNil(t, p1)
	p2, err := createIntermediateProject([]byte(smallYaml), false)
	assert.NoError(t, err)
	assert.NotNil(t, p2)
	err = p1.mergeMultipleParserProjects(p2)
	assert.NoError(t, err)
	assert.NotNil(t, p1)
	assert.Equal(t, len(p1.Functions), 2)
	assert.Equal(t, len(p1.Tasks), 2)
	assert.Equal(t, len(p1.Ignore), 3)
	assert.Equal(t, p1.Stepback, utility.TruePtr())
	assert.NotEqual(t, p1.Post, nil)
}

func TestMergeMultipleProjectConfigsBuildVariant(t *testing.T) {
	mainYaml := `
include: 
  - filename: small.yml
buildvariants:
  - name: bv1
    display_name: bv1_display
    run_on:
      - ubuntu1604-test
    tasks:
      - name: task1
`
	succeed := `
buildvariants:
  - name: bv1
    tasks:
      - name: task2
  - name: bv2
    display_name: bv2_display
    run_on:
      - ubuntu1604-test
    tasks:
      - name: task3
`

	fail := `
buildvariants:
  - name: bv1
    display_name: bv1_display
    tasks:
      - name: task3
`

	p1, err := createIntermediateProject([]byte(mainYaml), false)
	assert.NoError(t, err)
	assert.NotNil(t, p1)
	p2, err := createIntermediateProject([]byte(succeed), false)
	assert.NoError(t, err)
	assert.NotNil(t, p2)
	p3, err := createIntermediateProject([]byte(fail), false)
	assert.NoError(t, err)
	assert.NotNil(t, p3)
	err = p1.mergeMultipleParserProjects(p2)
	assert.NoError(t, err)
	assert.NotNil(t, p1)
	assert.Equal(t, len(p1.BuildVariants), 2)
	if p1.BuildVariants[0].name() == "bv1" {
		assert.Equal(t, len(p1.BuildVariants[0].Tasks), 2)
		assert.Equal(t, len(p1.BuildVariants[1].Tasks), 1)
	} else {
		assert.Equal(t, len(p1.BuildVariants[0].Tasks), 1)
		assert.Equal(t, len(p1.BuildVariants[1].Tasks), 2)
	}
	err = p1.mergeMultipleParserProjects(p3)
	assert.Error(t, err)
}

func TestUpdateForFile(t *testing.T) {
	p := &patch.Patch{
		Id: "p1",
		Patches: []patch.ModulePatch{
			{
				PatchSet: patch.PatchSet{
					Summary: []thirdparty.Summary{
						{
							Name: "small.yml",
						},
					},
				},
			},
		},
	}
	opts := &GetProjectOpts{
		Token:        "token",
		RemotePath:   "main.yml",
		ReadFileFrom: ReadFromPatch,
		PatchOpts: &PatchOpts{
			patch: p,
		},
	}
	opts.UpdateForFile("small.yml")
	assert.Equal(t, opts.ReadFileFrom, ReadFromPatchDiff) // should be changed to patch diff bc it's not a github patch
	p.GithubPatchData = thirdparty.GithubPatch{
		HeadOwner: "me", // indicates this is a github PR patch
	}
	opts.UpdateForFile("small.yml")
	assert.Equal(t, opts.ReadFileFrom, ReadFromPatch) // should be changed to patch bc it is a github patch

	opts.UpdateForFile("nonexistent.yml")
	// should be changed to patch diff because it's not a modified file

}

func TestFindAndTranslateProjectForPatch(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	defer func() {
		assert.NoError(t, db.ClearCollections(ParserProjectCollection, patch.Collection))
	}()

	for tName, tCase := range map[string]func(ctx context.Context, t *testing.T, p *patch.Patch, pp *ParserProject){
		"SucceedsWithUnfinalizedPatch": func(ctx context.Context, t *testing.T, p *patch.Patch, pp *ParserProject) {
			p.ProjectStorageMethod = evergreen.ProjectStorageMethodDB
			require.NoError(t, p.Insert())
			require.NoError(t, pp.Insert())

			project, ppFromDB, err := FindAndTranslateProjectForPatch(ctx, env.Settings(), p)
			require.NoError(t, err)
			require.NotZero(t, pp)
			require.NotZero(t, project)
			assert.Equal(t, utility.FromStringPtr(pp.DisplayName), utility.FromStringPtr(ppFromDB.DisplayName))
			assert.Equal(t, utility.FromStringPtr(pp.DisplayName), project.DisplayName)
		},
		"SucceedsWithFinalizedPatch": func(ctx context.Context, t *testing.T, p *patch.Patch, pp *ParserProject) {
			v := Version{
				Id:                   p.Id.Hex(),
				ProjectStorageMethod: evergreen.ProjectStorageMethodDB,
			}
			require.NoError(t, v.Insert())

			p.Activated = true
			p.Version = v.Id
			require.NoError(t, p.Insert())
			require.NoError(t, pp.Insert())

			project, ppFromDB, err := FindAndTranslateProjectForPatch(ctx, env.Settings(), p)
			require.NoError(t, err)
			require.NotZero(t, ppFromDB)
			require.NotZero(t, project)
			assert.Equal(t, utility.FromStringPtr(pp.DisplayName), utility.FromStringPtr(ppFromDB.DisplayName))
			assert.Equal(t, utility.FromStringPtr(pp.DisplayName), project.DisplayName)
		},
		"SucceedsWithDeprecatedPatchedParserProject": func(ctx context.Context, t *testing.T, p *patch.Patch, pp *ParserProject) {
			yamlPP, err := yaml.Marshal(pp)
			require.NoError(t, err)
			p.PatchedParserProject = string(yamlPP)
			require.NoError(t, p.Insert())

			project, ppFromPatch, err := FindAndTranslateProjectForPatch(ctx, env.Settings(), p)
			require.NoError(t, err)
			require.NotZero(t, ppFromPatch)
			require.NotZero(t, project)
			assert.Equal(t, utility.FromStringPtr(pp.DisplayName), utility.FromStringPtr(ppFromPatch.DisplayName))
			assert.Equal(t, utility.FromStringPtr(pp.DisplayName), project.DisplayName)
		},
		"FailsWithoutStoredParserProject": func(ctx context.Context, t *testing.T, p *patch.Patch, pp *ParserProject) {
			p.ProjectStorageMethod = evergreen.ProjectStorageMethodDB
			require.NoError(t, p.Insert())

			_, _, err := FindAndTranslateProjectForPatch(ctx, env.Settings(), p)
			assert.Error(t, err)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.ClearCollections(ParserProjectCollection, VersionCollection, patch.Collection))
			p := patch.Patch{
				Id: mgobson.NewObjectId(),
			}
			pp := ParserProject{
				Id:          p.Id.Hex(),
				DisplayName: utility.ToStringPtr("display-name"),
			}

			tCase(ctx, t, &p, &pp)
		})
	}
}
