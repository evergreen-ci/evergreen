package model

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

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
	"gopkg.in/yaml.v3"
)

// ShouldContainResembling tests whether a slice contains an element that DeepEquals
// the expected input.
func ShouldContainResembling(actual any, expected ...any) string {
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
				Name: "patch_requesters_allowed",
				AllowedRequesters: []evergreen.UserRequester{
					evergreen.PatchVersionUserRequester,
					evergreen.GithubPRUserRequester,
					evergreen.GithubMergeUserRequester,
				},
				Tasks: parserBVTaskUnits{
					{
						Name: "a_task_with_no_special_configuration",
					},
					{
						Name: "a_task_with_allowed_requesters",
					},
					{
						Name:              "a_task_with_build_variant_task_configuration",
						AllowedRequesters: []evergreen.UserRequester{evergreen.RepotrackerVersionUserRequester, evergreen.GitTagUserRequester},
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
			{
				Name: "bv_with_check_run",
				Tasks: parserBVTaskUnits{
					{
						Name: "a_task_with_no_special_configuration",
						CreateCheckRun: &CheckRun{
							PathToOutputs: "path",
						},
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
			{Name: "a_task_with_allowed_requesters", AllowedRequesters: []evergreen.UserRequester{evergreen.AdHocUserRequester}},
		},
		TaskGroups: []parserTaskGroup{{
			Name:  "my_tg",
			Tasks: []string{"tg_task"},
		}},
	}
	out, err := TranslateProject(parserProject)
	assert.NoError(t, err)
	assert.NotNil(t, out)
	require.Len(t, out.Tasks, 6)
	require.Len(t, out.BuildVariants, 8)

	for _, bv := range out.BuildVariants {
		for _, bvtu := range bv.Tasks {
			assert.Equal(t, bv.Name, bvtu.Variant, "build variant task unit's variant should be properly linked")
		}
	}

	assert.Equal(t, "bv0", out.BuildVariants[0].Name)
	require.Len(t, out.BuildVariants[0].Tasks, 3)
	assert.Equal(t, "my_task", out.BuildVariants[0].Tasks[0].Name)
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
	assert.True(t, bvt.IsGroup)
	assert.False(t, bvt.IsPartOfGroup)
	assert.Zero(t, bvt.GroupName)

	bvt = out.FindTaskForVariant("tg_task", "bv0")
	assert.Equal(t, "my_tg", bvt.Name, "task within a task group retains its task group name in resulting build variant task unit")
	assert.NotNil(t, bvt)
	assert.True(t, utility.FromBoolPtr(bvt.PatchOnly))
	assert.Contains(t, bvt.RunOn, "my_distro")
	assert.True(t, bvt.IsGroup)
	assert.False(t, bvt.IsPartOfGroup)
	assert.Zero(t, bvt.GroupName)

	patchOnlyBV := out.BuildVariants[1]
	assert.Equal(t, "patch_only_bv", patchOnlyBV.Name)
	require.Len(t, patchOnlyBV.Tasks, 4)
	assert.Equal(t, "your_task", patchOnlyBV.Tasks[0].Name)
	assert.True(t, utility.FromBoolPtr(patchOnlyBV.Tasks[0].Stepback))
	assert.False(t, utility.FromBoolPtr(patchOnlyBV.Tasks[0].PatchOnly))
	assert.False(t, utility.FromBoolPtr(patchOnlyBV.Tasks[0].GitTagOnly))
	assert.Equal(t, "my_task", patchOnlyBV.Tasks[1].Name)
	assert.True(t, utility.FromBoolPtr(patchOnlyBV.Tasks[1].PatchOnly))
	assert.Equal(t, "a_task_with_no_special_configuration", patchOnlyBV.Tasks[2].Name)
	assert.False(t, utility.FromBoolPtr(patchOnlyBV.Tasks[2].PatchOnly))
	assert.Equal(t, "a_task_with_build_variant_task_configuration", patchOnlyBV.Tasks[3].Name)
	assert.True(t, utility.FromBoolPtr(patchOnlyBV.Tasks[3].PatchOnly))

	unpatchableBV := out.BuildVariants[2]
	assert.Equal(t, "unpatchable_bv", unpatchableBV.Name)
	require.Len(t, unpatchableBV.Tasks, 2)
	assert.Equal(t, "a_task_with_no_special_configuration", unpatchableBV.Tasks[0].Name)
	assert.False(t, utility.FromBoolPtr(unpatchableBV.Tasks[0].Patchable))
	assert.Equal(t, "a_task_with_build_variant_task_configuration", unpatchableBV.Tasks[1].Name)
	assert.True(t, utility.FromBoolPtr(unpatchableBV.Tasks[1].Patchable))

	allowForGitTagBV := out.BuildVariants[3]
	assert.Equal(t, "allow_for_git_tag_bv", allowForGitTagBV.Name)
	require.Len(t, allowForGitTagBV.Tasks, 2)
	assert.Equal(t, "a_task_with_no_special_configuration", allowForGitTagBV.Tasks[0].Name)
	assert.False(t, utility.FromBoolTPtr(allowForGitTagBV.Tasks[0].AllowForGitTag))
	assert.Equal(t, "a_task_with_build_variant_task_configuration", allowForGitTagBV.Tasks[1].Name)
	assert.True(t, utility.FromBoolTPtr(allowForGitTagBV.Tasks[1].AllowForGitTag))

	gitTagOnlyBV := out.BuildVariants[4]
	assert.Equal(t, "git_tag_only_bv", gitTagOnlyBV.Name)
	require.Len(t, gitTagOnlyBV.Tasks, 2)
	assert.Equal(t, "a_task_with_no_special_configuration", gitTagOnlyBV.Tasks[0].Name)
	assert.True(t, utility.FromBoolPtr(gitTagOnlyBV.Tasks[0].GitTagOnly))
	assert.Equal(t, "a_task_with_build_variant_task_configuration", gitTagOnlyBV.Tasks[1].Name)
	assert.False(t, utility.FromBoolPtr(gitTagOnlyBV.Tasks[1].GitTagOnly))

	patchRequestersAllowedBV := out.BuildVariants[5]
	assert.Equal(t, "patch_requesters_allowed", patchRequestersAllowedBV.Name)
	assert.Equal(t, "a_task_with_no_special_configuration", gitTagOnlyBV.Tasks[0].Name)
	assert.ElementsMatch(t, []evergreen.UserRequester{
		evergreen.PatchVersionUserRequester,
		evergreen.GithubPRUserRequester,
		evergreen.GithubMergeUserRequester,
	}, patchRequestersAllowedBV.Tasks[0].AllowedRequesters)
	assert.Equal(t, "a_task_with_allowed_requesters", patchRequestersAllowedBV.Tasks[1].Name)
	assert.Equal(t, []evergreen.UserRequester{evergreen.AdHocUserRequester}, patchRequestersAllowedBV.Tasks[1].AllowedRequesters)
	assert.Equal(t, "a_task_with_build_variant_task_configuration", patchRequestersAllowedBV.Tasks[2].Name)
	assert.Equal(t, []evergreen.UserRequester{evergreen.RepotrackerVersionUserRequester, evergreen.GitTagUserRequester}, patchRequestersAllowedBV.Tasks[2].AllowedRequesters)

	disabledBV := out.BuildVariants[6]
	assert.Equal(t, "disabled_bv", disabledBV.Name)
	require.Len(t, disabledBV.Tasks, 2)
	assert.Equal(t, "your_task", disabledBV.Tasks[0].Name)
	assert.False(t, utility.FromBoolPtr(disabledBV.Tasks[0].GitTagOnly))
	assert.True(t, utility.FromBoolPtr(disabledBV.Tasks[0].Stepback))
	assert.True(t, utility.FromBoolPtr(disabledBV.Tasks[0].Disable))
	assert.Equal(t, "my_task", disabledBV.Tasks[1].Name)
	assert.True(t, utility.FromBoolPtr(disabledBV.Tasks[1].PatchOnly))
	assert.False(t, utility.FromBoolPtr(disabledBV.Tasks[1].Disable))

	checkRunBV := out.BuildVariants[7]
	assert.Equal(t, "bv_with_check_run", checkRunBV.Name)
	require.Len(t, checkRunBV.Tasks, 1)
	assert.NotNil(t, checkRunBV.Tasks[0].CreateCheckRun)
	assert.Equal(t, "path", checkRunBV.Tasks[0].CreateCheckRun.PathToOutputs)
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
					{Name: "t1"},
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
			So(bvts[1].DependsOn[0].Name, ShouldEqual, "t3")
		})
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
			tgse := newTaskGroupSelectorEvaluator([]parserTaskGroup{})
			Convey("should evaluate valid tasks pointers properly", func() {
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{{Name: "white"}},
					taskDefs,
					[]BuildVariantTaskUnit{{Name: "white"}}, nil, nil)
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{{Name: "red", Priority: 50}, {Name: ".secondary"}},
					taskDefs,
					[]BuildVariantTaskUnit{{Name: "red", Priority: 50}, {Name: "orange"}, {Name: "purple"}, {Name: "green"}}, nil, nil)
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: ".warm .secondary", Distros: []string{"d1"}}},
					taskDefs,
					[]BuildVariantTaskUnit{{Name: "orange", RunOn: []string{"d1"}}}, nil, nil)
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{
						{Name: "orange", Distros: []string{"d1"}},
						{Name: "!.warm .secondary", Distros: []string{"d1"}}},
					taskDefs,
					[]BuildVariantTaskUnit{
						{Name: "orange", RunOn: []string{"d1"}},
						{Name: "purple", RunOn: []string{"d1"}},
						{Name: "green", RunOn: []string{"d1"}}}, nil, nil)
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{{Name: "*"}},
					taskDefs,
					[]BuildVariantTaskUnit{
						{Name: "red"}, {Name: "blue"}, {Name: "yellow"},
						{Name: "orange"}, {Name: "purple"}, {Name: "green"},
						{Name: "brown"}, {Name: "white"}, {Name: "black"},
					}, nil, nil)
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{
						{Name: "red", Priority: 10},
						{Name: "!.warm .secondary", Priority: 10}},
					taskDefs,
					[]BuildVariantTaskUnit{
						{Name: "red", Priority: 10},
						{Name: "purple", Priority: 10},
						{Name: "green", Priority: 10}}, nil, nil)
			})
			Convey("should ignore selectors that do not select any tasks if another does select a task", func() {
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{{Name: ".warm .cool"}, {Name: "white"}},
					taskDefs,
					[]BuildVariantTaskUnit{{Name: "white"}}, []string{".warm .cool"}, nil)
			})
			Convey("should error when all selectors combined do not select any tasks", func() {
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{{Name: ".warm .cool"}},
					taskDefs,
					nil, []string{".warm .cool"}, nil)
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{{Name: ".warm .cool"}, {Name: ".secondary .primary"}},
					taskDefs,
					nil, []string{".warm .cool", ".secondary .primary"}, nil)
			})
			Convey("should warn when some selectors do not exist", func() {
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{{Name: ".warm .doesnt-exist"}, {Name: "white"}},
					taskDefs,
					[]BuildVariantTaskUnit{{Name: "white"}}, []string{".warm .doesnt-exist"}, []string{".doesnt-exist"})
				parserTaskSelectorTaskEval(tse, tgse,
					parserBVTaskUnits{{Name: ".warm !.doesnt-exist"}, {Name: "white"}},
					taskDefs,
					[]BuildVariantTaskUnit{{Name: "red"}, {Name: "orange"}, {Name: "yellow"}, {Name: "white"}}, nil, []string{"!.doesnt-exist"})
			})
		})
	})
}

func parserTaskSelectorTaskEval(tse *taskSelectorEvaluator, tsge *tagSelectorEvaluator, tasks parserBVTaskUnits, taskDefs []parserTask, expected []BuildVariantTaskUnit, expectedEmptySelectors, expectedUnmatchedCriteria []string) {
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
		taskUnit, unmatchedSelectors, unmatchedCriteria, errs := evaluateBVTasks(tse, tsge, vse, pbv, taskDefs)
		if expected != nil {
			So(errs, ShouldBeNil)
		} else {
			So(errs, ShouldNotBeNil)
		}
		So(len(taskUnit), ShouldEqual, len(expected))
		for _, e := range expected {
			exists := false
			for _, t := range taskUnit {
				if t.Name == e.Name && t.Priority == e.Priority && len(t.DependsOn) == len(e.DependsOn) {
					exists = true
					break
				}
				So(t.Variant, ShouldEqual, pbv.Name)
			}
			So(exists, ShouldBeTrue)
		}
		So(len(unmatchedSelectors), ShouldEqual, len(expectedEmptySelectors))
		for _, expectedEmptySelector := range expectedEmptySelectors {
			exists := false
			for _, emptySelector := range unmatchedSelectors {
				if emptySelector == expectedEmptySelector {
					exists = true
					break
				}
			}
			So(exists, ShouldBeTrue)
		}
		So(len(unmatchedCriteria), ShouldEqual, len(expectedUnmatchedCriteria))
		for _, expectedUnmatchedTag := range expectedUnmatchedCriteria {
			exists := false
			for _, unmatchedTag := range unmatchedCriteria {
				if unmatchedTag == expectedUnmatchedTag {
					exists = true
					break
				}
			}
			So(exists, ShouldBeTrue)
		}
	})
}

func TestCheckRunParsing(t *testing.T) {
	assert := assert.New(t)
	yml := `
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
    create_check_run:
      path_to_outputs: "path"
tasks:
- name: t1
`

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(proj)
	assert.NoError(err)
	require.Len(t, proj.BuildVariants, 1)

	assert.Len(proj.BuildVariants[0].Tasks, 1)
	cr := proj.BuildVariants[0].Tasks[0].CreateCheckRun
	assert.NotNil(cr)
	assert.Equal("path", cr.PathToOutputs)

	ymlWithEmptyString := `
buildvariants:
- name: "v1"
  tasks:
  - name: "t1"
    create_check_run:
      path_to_outputs: ""
tasks:
- name: t1
`

	_, err = LoadProjectInto(ctx, []byte(ymlWithEmptyString), nil, "id", proj)
	assert.NotNil(proj)
	assert.NoError(err)
	require.Len(t, proj.BuildVariants, 1)

	assert.Len(proj.BuildVariants[0].Tasks, 1)
	cr = proj.BuildVariants[0].Tasks[0].CreateCheckRun
	assert.NotNil(cr)
	assert.Equal("", cr.PathToOutputs)
}

func TestLocalModuleIncludes(t *testing.T) {
	assert := assert.New(t)
	yml := `
modules:
- name: "something_different"
  repo: "evergreen"
  owner: "evergreen-ci"
  prefix: "src/third_party"
  branch: "master"
include:
- filename: "include.yml"
  module: "something_different"
buildvariants:
- name: "v1"
  modules:
  - something_different
  tasks:
  - name: "t1"
    create_check_run:
      path_to_outputs: "path"
tasks:
- name: t1
- name: t2
`

	opts := &GetProjectOpts{
		LocalModuleIncludes: []patch.LocalModuleInclude{
			{
				Module:   "something_different",
				FileName: "include.yml",
				FileContent: []byte(`
buildvariants:
- name: "v1"
  tasks:
  - name: "t2"
`),
			},
		},
	}

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), opts, "id", proj)
	assert.NotNil(proj)
	assert.NoError(err)

	require.NotNil(t, proj)
	require.Len(t, proj.BuildVariants, 1)
	assert.Len(proj.BuildVariants[0].Tasks, 2)
}

func TestParseModule(t *testing.T) {
	assert := assert.New(t)
	yml := `
modules:
- name: "something_different"
  repo: "evergreen"
  owner: "evergreen-ci"
  prefix: "src/third_party"
  branch: "master"
buildvariants:
- name: "v1"
  modules:
  - something_different
  tasks:
  - name: "t1"
    create_check_run:
      path_to_outputs: "path"
tasks:
- name: t1
`

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(proj)
	assert.NoError(err)

	modules := proj.Modules
	require.NotNil(t, modules)
	require.Len(t, modules, 1)
	assert.Equal("evergreen-ci", modules[0].Owner)
	assert.Equal("evergreen", modules[0].Repo)
}

func TestBuildVariantPaths(t *testing.T) {
	assert := assert.New(t)
	yml := `
buildvariants:
- name: "v1"
  paths:
  - "src/**"
  - "etc/**"
  tasks:
  - name: "t1"
tasks:
- name: t1
`

	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(proj)
	assert.NoError(err)

	require.Len(t, proj.BuildVariants, 1)
	assert.Len(proj.BuildVariants[0].Paths, 2)
	assert.Equal("src/**", proj.BuildVariants[0].Paths[0])
	assert.Equal("etc/**", proj.BuildVariants[0].Paths[1])
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
	assert.Empty(p.BuildVariants[1].DisplayTasks)

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
	assert.NoError(err)
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
	assert.Empty(proj.BuildVariants[1].DisplayTasks)

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
	assert.Contains(err.Error(), "contains unmatched criteria: 'notHere'")
	assert.Len(proj.BuildVariants[0].DisplayTasks, 1)
	assert.Len(proj.BuildVariants[0].DisplayTasks[0].ExecTasks, 2)
	assert.Empty(proj.BuildVariants[1].DisplayTasks)

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
	assert.Error(err)
	assert.Contains(err.Error(), "execution task 'execTask3' is listed in more than 1 display task")
	assert.Empty(proj.BuildVariants[0].DisplayTasks)

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
	assert.Error(err)
	assert.Contains(err.Error(), "display task 'execTask1' cannot have the same name as an execution task")
	assert.Empty(proj.BuildVariants[0].DisplayTasks)

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
	assert.NoError(err)
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
	assert.NoError(err)
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
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	checkIsTaskGroupTaskUnit := func(t *testing.T, bvtu BuildVariantTaskUnit) {
		assert.True(t, bvtu.IsGroup)
		assert.False(t, bvtu.IsPartOfGroup)
		assert.Zero(t, bvtu.GroupName)
	}

	t.Run("SucceedsWithRegularTaskGroup", func(t *testing.T) {
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
  setup_task_can_fail_task: true
  setup_task_timeout_secs: 10
  teardown_task_can_fail_task: true
  teardown_task_timeout_secs: 10
  teardown_task_can_fail_task: true
  teardown_group_timeout_secs: 10
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
		_, err := LoadProjectInto(ctx, []byte(validYml), nil, "id", proj)
		require.NotNil(t, proj)
		assert.NoError(t, err)
		require.Len(t, proj.TaskGroups, 1)
		tg := proj.TaskGroups[0]
		assert.Equal(t, "example_task_group", tg.Name)
		assert.Equal(t, 2, tg.MaxHosts)

		assert.Len(t, tg.SetupGroup.List(), 1)
		assert.True(t, tg.SetupGroupCanFailTask)
		assert.Equal(t, 10, tg.SetupGroupTimeoutSecs)

		assert.Len(t, tg.SetupTask.List(), 1)
		assert.True(t, tg.SetupTaskCanFailTask)
		assert.Equal(t, 10, tg.SetupTaskTimeoutSecs)

		assert.Len(t, tg.TeardownTask.List(), 1)
		assert.True(t, tg.TeardownTaskCanFailTask)
		assert.Equal(t, 10, tg.TeardownTaskTimeoutSecs)

		assert.Len(t, tg.TeardownGroup.List(), 1)
		assert.Equal(t, 10, tg.TeardownGroupTimeoutSecs)

		assert.True(t, tg.ShareProcs)

		assert.Len(t, tg.Tasks, 2)

		require.Len(t, proj.BuildVariants, 1)
		require.Len(t, proj.BuildVariants[0].Tasks, 1)
		checkIsTaskGroupTaskUnit(t, proj.BuildVariants[0].Tasks[0])
	})

	t.Run("FailsWithTaskGroupContainingNonexistentTask", func(t *testing.T) {
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

		proj := &Project{}
		_, err := LoadProjectInto(ctx, []byte(wrongTaskYml), nil, "id", proj)
		assert.NotNil(t, proj)
		require.Error(t, err)
		assert.Contains(t, err.Error(), `'example_task_group' has unmatched selector: 'example_task_3'`)
	})

	t.Run("MaintainsTaskGroupTaskOrdering", func(t *testing.T) {
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

		proj := &Project{}
		_, err := LoadProjectInto(ctx, []byte(orderedYml), nil, "id", proj)
		require.NotNil(t, proj)
		assert.NoError(t, err)
		for i, tsk := range proj.TaskGroups[0].Tasks {
			assert.Equal(t, strconv.Itoa(i+1), tsk)
		}
		require.Len(t, proj.BuildVariants, 1)
		require.Len(t, proj.BuildVariants[0].Tasks, 1)
		checkIsTaskGroupTaskUnit(t, proj.BuildVariants[0].Tasks[0])
	})

	t.Run("TagsSelectCorrectTasksForTaskGroup", func(t *testing.T) {
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

		proj := &Project{}
		_, err := LoadProjectInto(ctx, []byte(tagYml), nil, "id", proj)
		require.NotNil(t, proj)
		assert.NoError(t, err)
		require.Len(t, proj.TaskGroups, 2)
		assert.Equal(t, "even_task_group", proj.TaskGroups[0].Name)
		require.Len(t, proj.TaskGroups[0].Tasks, 2)
		for _, tsk := range proj.TaskGroups[0].Tasks {
			v, err := strconv.Atoi(tsk)
			assert.NoError(t, err)
			assert.Zero(t, v%2)
		}
		for _, tsk := range proj.TaskGroups[1].Tasks {
			v, err := strconv.Atoi(tsk)
			assert.NoError(t, err)
			assert.Equal(t, 1, v%2)
		}
		require.Len(t, proj.BuildVariants, 1)
		require.Len(t, proj.BuildVariants[0].Tasks, 2)
		for _, bvtu := range proj.BuildVariants[0].Tasks {
			checkIsTaskGroupTaskUnit(t, bvtu)
		}
	})

	t.Run("MaxHostsForTaskGroup", func(t *testing.T) {
		validMaxHostYml := `
tasks:
- name: example_task_1
- name: example_task_2
- name: example_task_3
- name: example_task_4
task_groups:
- name: example_task_group
  max_hosts: -1
  tasks:
  - example_task_1
  - example_task_2
buildvariants:
- name: bv
  display_name: "bv_display"
  tasks:
  - name: example_task_group
`
		proj := &Project{}
		_, err := LoadProjectInto(ctx, []byte(validMaxHostYml), nil, "id", proj)
		require.NotNil(t, proj)
		assert.NoError(t, err)
		require.Len(t, proj.TaskGroups, 1)
		assert.Equal(t, "example_task_group", proj.TaskGroups[0].Name)
		assert.Len(t, proj.TaskGroups[0].Tasks, proj.TaskGroups[0].MaxHosts)
	})
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
	assert.NoError(err)
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
	assert.NoError(err)
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
	assert.NoError(err)
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
	assert.NoError(err)
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
	assert.NoError(err)
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
	assert.Empty(proj.BuildVariants[1].Tasks[0].DependsOn)

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
	assert.NoError(err)
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
	assert.NoError(t, err)
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
	assert.NoError(t, err)
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

func TestParseOomTracker(t *testing.T) {
	yml := `
tasks:
- name: task_1
  commands:
  - command: myCommand
`
	// Verify that the default is true
	proj := &Project{}
	ctx := context.Background()
	_, err := LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(t, proj)
	assert.NoError(t, err)
	assert.True(t, proj.OomTracker)

	yml = `
oom_tracker: false
tasks:
- name: task_1
  commands:
  - command: myCommand
`
	proj = &Project{}
	_, err = LoadProjectInto(ctx, []byte(yml), nil, "id", proj)
	assert.NotNil(t, proj)
	assert.NoError(t, err)
	assert.False(t, proj.OomTracker)
}

func TestAddBuildVariant(t *testing.T) {
	pp := ParserProject{
		Identifier: utility.ToStringPtr("small"),
	}

	pp.AddBuildVariant("name", "my-name", "", nil, []string{"task"})
	require.Len(t, pp.BuildVariants, 1)
	assert.Equal(t, "name", pp.BuildVariants[0].Name)
	assert.Equal(t, "my-name", pp.BuildVariants[0].DisplayName)
	assert.Nil(t, pp.BuildVariants[0].RunOn)
	assert.Len(t, pp.BuildVariants[0].Tasks, 1)
}

func TestParserProjectStorage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))

	testutil.ConfigureIntegrationTest(t, env.Settings())

	c := utility.GetHTTPClient()
	defer utility.PutHTTPClient(c)

	ppConf := env.Settings().Providers.AWS.ParserProject
	bucket, err := pail.NewS3BucketWithHTTPClient(ctx, c, pail.S3Options{
		Name:   ppConf.Bucket,
		Region: evergreen.DefaultEC2Region,
	})
	require.NoError(t, err)
	defer func() {
		assert.NoError(t, bucket.RemovePrefix(ctx, ppConf.Prefix))
	}()

	defer func() {
		assert.NoError(t, db.ClearCollections(ParserProjectCollection))
	}()

	for methodName, ppStorageMethod := range map[string]evergreen.ParserProjectStorageMethod{
		"DB": evergreen.ProjectStorageMethodDB,
		"S3": evergreen.ProjectStorageMethodS3,
	} {
		t.Run("StorageMethod"+methodName, func(t *testing.T) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, env *mock.Environment){
				"FindOneByIDReturnsNilErrorAndResultForNonexistentParserProject": func(ctx context.Context, t *testing.T, env *mock.Environment) {
					ppStorage, err := GetParserProjectStorage(ctx, env.Settings(), ppStorageMethod)
					require.NoError(t, err)

					pp, err := ppStorage.FindOneByID(ctx, "nonexistent")
					assert.NoError(t, err)
					assert.Zero(t, pp)
				},
				"FindOneByIDWithFieldsReturnsNilErrorAndResultForNonexistentParserProject": func(ctx context.Context, t *testing.T, env *mock.Environment) {
					ppStorage, err := GetParserProjectStorage(ctx, env.Settings(), ppStorageMethod)
					require.NoError(t, err)

					pp, err := ppStorage.FindOneByIDWithFields(ctx, "nonexistent", ParserProjectBuildVariantsKey)
					assert.NoError(t, err)
					assert.Zero(t, pp)
				},
				"UpsertCreatesNewParserProject": func(ctx context.Context, t *testing.T, env *mock.Environment) {
					pp := &ParserProject{
						Id:    "my-project",
						Owner: utility.ToStringPtr("me"),
					}
					ppStorage, err := GetParserProjectStorage(ctx, env.Settings(), ppStorageMethod)
					require.NoError(t, err)

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
					ppStorage, err := GetParserProjectStorage(ctx, env.Settings(), ppStorageMethod)
					require.NoError(t, err)

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

	ppStorage, err := GetParserProjectStorage(ctx, env.Settings(), ppStorageMethod)
	require.NoError(t, err)

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
		Functions: map[string]*YAMLCommandSet{
			"add_func1": {
				SingleCommand: &PluginCommandConf{
					Command: "add_single_command",
				},
			},
			"add_func2": {
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
	assert.Len(t, main.Tasks, 4)
	assert.Len(t, main.TaskGroups, 2)
	assert.Len(t, main.Parameters, 2)
	assert.Len(t, main.Modules, 2)
	assert.Len(t, main.Functions, 4)
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
}

func TestMergeUnordered(t *testing.T) {
	main := &ParserProject{
		Ignore: parserStringSlice{
			"a",
		},
	}

	add := &ParserProject{
		Ignore: parserStringSlice{
			"b",
		},
	}
	main.mergeUnordered(add)
	assert.Len(t, main.Ignore, 2)
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
	}

	err := main.mergeOrderedUnique(add)
	assert.NoError(t, err)
	assert.NotNil(t, main.Pre)
	assert.NotNil(t, main.Post)
	assert.NotNil(t, main.Timeout)
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
	}

	err := main.mergeOrderedUnique(add)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "pre can only be defined in one YAML")
	assert.Contains(t, err.Error(), "post can only be defined in one YAML")
	assert.Contains(t, err.Error(), "timeout can only be defined in one YAML")
}

func TestMergeUnique(t *testing.T) {
	main := &ParserProject{
		Stepback:    utility.ToBoolPtr(true),
		OomTracker:  utility.ToBoolPtr(false),
		DisplayName: utility.ToStringPtr("name"),
	}

	add := &ParserProject{
		PreTimeoutSecs:    utility.ToIntPtr(1),
		PostTimeoutSecs:   utility.ToIntPtr(1),
		PreErrorFailsTask: utility.ToBoolPtr(true),
		CommandType:       utility.ToStringPtr("type"),
		CallbackTimeout:   utility.ToIntPtr(1),
		ExecTimeoutSecs:   utility.ToIntPtr(1),
		TimeoutSecs:       utility.ToIntPtr(1),
	}

	err := main.mergeUnique(add)
	assert.NoError(t, err)
	assert.NotNil(t, main.Stepback)
	assert.NotNil(t, main.OomTracker)
	assert.NotNil(t, main.DisplayName)
	assert.NotNil(t, main.PreTimeoutSecs)
	assert.NotNil(t, main.PostTimeoutSecs)
	assert.NotNil(t, main.PreErrorFailsTask)
	assert.NotNil(t, main.CommandType)
	assert.NotNil(t, main.CallbackTimeout)
	assert.NotNil(t, main.ExecTimeoutSecs)
	assert.NotNil(t, main.TimeoutSecs)
}

func TestMergeUniqueFail(t *testing.T) {
	main := &ParserProject{
		Stepback:          utility.ToBoolPtr(true),
		OomTracker:        utility.ToBoolPtr(true),
		PreTimeoutSecs:    utility.ToIntPtr(1),
		PostTimeoutSecs:   utility.ToIntPtr(1),
		PreErrorFailsTask: utility.ToBoolPtr(true),
		DisplayName:       utility.ToStringPtr("name"),
		CommandType:       utility.ToStringPtr("type"),
		CallbackTimeout:   utility.ToIntPtr(1),
		ExecTimeoutSecs:   utility.ToIntPtr(1),
	}

	add := &ParserProject{
		Stepback:          utility.ToBoolPtr(true),
		OomTracker:        utility.ToBoolPtr(true),
		PreTimeoutSecs:    utility.ToIntPtr(1),
		PostTimeoutSecs:   utility.ToIntPtr(1),
		PreErrorFailsTask: utility.ToBoolPtr(true),
		DisplayName:       utility.ToStringPtr("name"),
		CommandType:       utility.ToStringPtr("type"),
		CallbackTimeout:   utility.ToIntPtr(1),
		ExecTimeoutSecs:   utility.ToIntPtr(1),
		TimeoutSecs:       utility.ToIntPtr(1),
	}

	err := main.mergeUnique(add)
	require.NotNil(t, err)
	assert.Contains(t, err.Error(), "stepback can only be defined in one YAML")
	assert.Contains(t, err.Error(), "OOM tracker can only be defined in one YAML")
	assert.Contains(t, err.Error(), "pre timeout secs can only be defined in one YAML")
	assert.Contains(t, err.Error(), "post timeout secs can only be defined in one YAML")
	assert.Contains(t, err.Error(), "pre error fails task can only be defined in one YAML")
	assert.Contains(t, err.Error(), "display name can only be defined in one YAML")
	assert.Contains(t, err.Error(), "command type can only be defined in one YAML")
	assert.Contains(t, err.Error(), "callback timeout can only be defined in one YAML")
	assert.Contains(t, err.Error(), "exec timeout secs can only be defined in one YAML")
	assert.Contains(t, err.Error(), "timeout secs can only be defined in one YAML")
}

func TestMergeBuildVariant(t *testing.T) {
	bvExisting := "a_variant"
	main := &ParserProject{
		BuildVariants: []parserBV{
			{
				Name:        bvExisting,
				DisplayName: "Defined here",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
				DisplayTasks: []displayTask{
					{
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
			{
				Name: bvExisting,
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name: "add this task",
					},
				},
			},
			{
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
					{
						Name:           "my_display_task_new_variant",
						ExecutionTasks: []string{"another_task"},
					},
				},
			},
			{
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
	require.Len(t, main.BuildVariants, 3)
	bvNames := []string{}
	for _, bv := range main.BuildVariants {
		if bv.Name == bvNew1 {
			assert.Len(t, bv.Tasks, 2)
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
			{
				Name: bvExisting,
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
				DisplayTasks: []displayTask{
					{
						Name:           "my_display_task_old_variant",
						ExecutionTasks: []string{"say-bye"},
					},
				},
			},
		},
	}
	add := &ParserProject{
		BuildVariants: []parserBV{
			{
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
	require.Len(t, main.BuildVariants, 1)
	require.Len(t, main.BuildVariants[0].Tasks, 2)
}

func TestMergeBuildVariantFail(t *testing.T) {
	main := &ParserProject{
		BuildVariants: []parserBV{
			{
				Name:        "a_variant",
				DisplayName: "duplicate",
				Tasks: parserBVTaskUnits{
					parserBVTaskUnit{
						Name:      "say-bye",
						BatchTime: &taskBatchTime,
					},
				},
				DisplayTasks: []displayTask{
					{
						Name:           "my_display_task_old_variant",
						ExecutionTasks: []string{"say-bye"},
					},
				},
			},
		},
	}

	add := &ParserProject{
		BuildVariants: []parserBV{
			{
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
	assert.Len(t, main.Axes, 1)
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
  owner: "foo"
  repo: "bar"
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
	assert.Len(t, p1.Functions, 2)
	assert.Len(t, p1.Tasks, 2)
	assert.Len(t, p1.Ignore, 3)
	assert.Equal(t, p1.Stepback, utility.TruePtr())
	assert.NotNil(t, p1.Post)
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
	assert.Len(t, p1.BuildVariants, 2)
	if p1.BuildVariants[0].name() == "bv1" {
		assert.Len(t, p1.BuildVariants[0].Tasks, 2)
		assert.Len(t, p1.BuildVariants[1].Tasks, 1)
	} else {
		assert.Len(t, p1.BuildVariants[0].Tasks, 1)
		assert.Len(t, p1.BuildVariants[1].Tasks, 2)
	}
	err = p1.mergeMultipleParserProjects(p3)
	assert.Error(t, err)
}

func TestUpdateReadFileFrom(t *testing.T) {
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
		RemotePath:   "main.yml",
		ReadFileFrom: ReadFromPatch,
		PatchOpts: &PatchOpts{
			patch: p,
		},
	}
	opts.UpdateReadFileFrom("small.yml")
	assert.Equal(t, ReadFromPatchDiff, opts.ReadFileFrom) // should be changed to patch diff bc it's not a github patch
	p.GithubPatchData = thirdparty.GithubPatch{
		HeadOwner: "me", // indicates this is a github PR patch
	}
	opts.UpdateReadFileFrom("small.yml")
	assert.Equal(t, ReadFromPatch, opts.ReadFileFrom) // should be changed to patch bc it is a github patch

	opts.UpdateReadFileFrom("nonexistent.yml")
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
			require.NoError(t, p.Insert(t.Context()))
			require.NoError(t, pp.Insert(t.Context()))

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
			require.NoError(t, v.Insert(t.Context()))

			p.Activated = true
			p.Version = v.Id
			require.NoError(t, p.Insert(t.Context()))
			require.NoError(t, pp.Insert(t.Context()))

			project, ppFromDB, err := FindAndTranslateProjectForPatch(ctx, env.Settings(), p)
			require.NoError(t, err)
			require.NotZero(t, ppFromDB)
			require.NotZero(t, project)
			assert.Equal(t, utility.FromStringPtr(pp.DisplayName), utility.FromStringPtr(ppFromDB.DisplayName))
			assert.Equal(t, utility.FromStringPtr(pp.DisplayName), project.DisplayName)
		},
		"FailsWithoutStoredParserProject": func(ctx context.Context, t *testing.T, p *patch.Patch, pp *ParserProject) {
			p.ProjectStorageMethod = evergreen.ProjectStorageMethodDB
			require.NoError(t, p.Insert(t.Context()))

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

func TestMarshalBSON(t *testing.T) {
	pp := ParserProject{
		Identifier: utility.ToStringPtr("small"),
	}

	encoded, err := pp.MarshalBSON()
	require.NoError(t, err)
	require.NotEmpty(t, encoded)

	decoded, err := GetProjectFromBSON(encoded)
	require.NoError(t, err)
	require.NotEmpty(t, decoded)
	assert.Equal(t, utility.FromStringPtr(pp.Identifier), decoded.Identifier)
}

func TestCapParserPriorities(t *testing.T) {
	t.Run("CapsTaskPriorityAboveMax", func(t *testing.T) {
		p := &ParserProject{
			Tasks: []parserTask{
				{Name: "task1", Priority: MaxConfigSetPriority + 100},
				{Name: "task2", Priority: MaxConfigSetPriority + 1},
			},
		}
		capParserPriorities(p)
		assert.Equal(t, int64(MaxConfigSetPriority), p.Tasks[0].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority), p.Tasks[1].Priority)
	})

	t.Run("DoesNotChangeTaskPriorityBelowOrEqualToMax", func(t *testing.T) {
		p := &ParserProject{
			Tasks: []parserTask{
				{Name: "task1", Priority: MaxConfigSetPriority},
				{Name: "task2", Priority: MaxConfigSetPriority - 1},
				{Name: "task3", Priority: 0},
				{Name: "task4", Priority: 10},
			},
		}
		capParserPriorities(p)
		assert.Equal(t, int64(MaxConfigSetPriority), p.Tasks[0].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority-1), p.Tasks[1].Priority)
		assert.Equal(t, int64(0), p.Tasks[2].Priority)
		assert.Equal(t, int64(10), p.Tasks[3].Priority)
	})

	t.Run("CapsBuildVariantTaskPriorityAboveMax", func(t *testing.T) {
		p := &ParserProject{
			BuildVariants: []parserBV{
				{
					Name: "bv1",
					Tasks: []parserBVTaskUnit{
						{Name: "task1", Priority: MaxConfigSetPriority + 200},
						{Name: "task2", Priority: MaxConfigSetPriority + 50},
					},
				},
				{
					Name: "bv2",
					Tasks: []parserBVTaskUnit{
						{Name: "task3", Priority: MaxConfigSetPriority + 1},
					},
				},
			},
		}
		capParserPriorities(p)
		assert.Equal(t, int64(MaxConfigSetPriority), p.BuildVariants[0].Tasks[0].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority), p.BuildVariants[0].Tasks[1].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority), p.BuildVariants[1].Tasks[0].Priority)
	})

	t.Run("DoesNotChangeBuildVariantTaskPriorityBelowOrEqualToMax", func(t *testing.T) {
		p := &ParserProject{
			BuildVariants: []parserBV{
				{
					Name: "bv1",
					Tasks: []parserBVTaskUnit{
						{Name: "task1", Priority: MaxConfigSetPriority},
						{Name: "task2", Priority: MaxConfigSetPriority - 10},
						{Name: "task3", Priority: 0},
					},
				},
			},
		}
		capParserPriorities(p)
		assert.Equal(t, int64(MaxConfigSetPriority), p.BuildVariants[0].Tasks[0].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority-10), p.BuildVariants[0].Tasks[1].Priority)
		assert.Equal(t, int64(0), p.BuildVariants[0].Tasks[2].Priority)
	})

	t.Run("HandlesEmptyProject", func(t *testing.T) {
		p := &ParserProject{
			Tasks:         []parserTask{},
			BuildVariants: []parserBV{},
		}
		// Should not panic
		capParserPriorities(p)
		assert.Len(t, p.Tasks, 0)
		assert.Len(t, p.BuildVariants, 0)
	})

	t.Run("HandlesNilSlices", func(t *testing.T) {
		p := &ParserProject{}
		// Should not panic
		capParserPriorities(p)
		assert.Nil(t, p.Tasks)
		assert.Nil(t, p.BuildVariants)
	})

	t.Run("CapsMixedPriorities", func(t *testing.T) {
		p := &ParserProject{
			Tasks: []parserTask{
				{Name: "task1", Priority: MaxConfigSetPriority + 100},
				{Name: "task2", Priority: MaxConfigSetPriority - 10},
				{Name: "task3", Priority: MaxConfigSetPriority},
			},
			BuildVariants: []parserBV{
				{
					Name: "bv1",
					Tasks: []parserBVTaskUnit{
						{Name: "task1", Priority: MaxConfigSetPriority + 200},
						{Name: "task2", Priority: 25},
						{Name: "task3", Priority: MaxConfigSetPriority},
					},
				},
			},
		}
		capParserPriorities(p)
		// Check tasks
		assert.Equal(t, int64(MaxConfigSetPriority), p.Tasks[0].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority-10), p.Tasks[1].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority), p.Tasks[2].Priority)
		// Check build variant tasks
		assert.Equal(t, int64(MaxConfigSetPriority), p.BuildVariants[0].Tasks[0].Priority)
		assert.Equal(t, int64(25), p.BuildVariants[0].Tasks[1].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority), p.BuildVariants[0].Tasks[2].Priority)
	})

	t.Run("CapsMultipleBuildVariants", func(t *testing.T) {
		p := &ParserProject{
			BuildVariants: []parserBV{
				{
					Name: "bv1",
					Tasks: []parserBVTaskUnit{
						{Name: "task1", Priority: MaxConfigSetPriority + 100},
					},
				},
				{
					Name: "bv2",
					Tasks: []parserBVTaskUnit{
						{Name: "task2", Priority: MaxConfigSetPriority + 200},
					},
				},
				{
					Name: "bv3",
					Tasks: []parserBVTaskUnit{
						{Name: "task3", Priority: MaxConfigSetPriority - 5},
					},
				},
			},
		}
		capParserPriorities(p)
		assert.Equal(t, int64(MaxConfigSetPriority), p.BuildVariants[0].Tasks[0].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority), p.BuildVariants[1].Tasks[0].Priority)
		assert.Equal(t, int64(MaxConfigSetPriority-5), p.BuildVariants[2].Tasks[0].Priority)
	})
}

func TestSetupParallelGitIncludeDirs(t *testing.T) {
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings)

	for tName, tCase := range map[string]func(t *testing.T, modules ModuleList, includes []parserInclude, opts *GetProjectOpts){
		"SucceedsWithGitRestoredIncludeFilesFromRepoAndModules": func(t *testing.T, modules ModuleList, includes []parserInclude, opts *GetProjectOpts) {
			numWorkers := len(includes)
			dirs, err := setupParallelGitIncludeDirs(t.Context(), modules, includes, numWorkers, opts)
			assert.NoError(t, err)
			require.NotZero(t, dirs)
			defer func() {
				assert.NoError(t, dirs.cleanup())
			}()

			assert.Len(t, dirs.clonesForOwnerRepo, len(dirs.worktreesForOwnerRepo), "each git clone should have one set of worktrees")
			for _, dir := range dirs.clonesForOwnerRepo {
				assert.True(t, utility.FileExists(dir))
			}
			for _, worktreeDirs := range dirs.worktreesForOwnerRepo {
				assert.Len(t, worktreeDirs, numWorkers)
				for _, worktreeDir := range worktreeDirs {
					assert.True(t, utility.FileExists(worktreeDir))
				}
			}

			for _, include := range includes {
				var owner, repo, revision string
				if include.Module == "" {
					owner = opts.Ref.Owner
					repo = opts.Ref.Repo
					revision = opts.Revision
				} else {
					mod, err := GetModuleByName(modules, include.Module)
					require.NoError(t, err)
					owner = mod.Owner
					repo = mod.Repo
					revision, err = getRevisionForRemoteModule(t.Context(), *mod, include.Module, *opts)
					require.NoError(t, err)
				}

				worktreeDir := dirs.getWorktreeForOwnerRepoWorker(owner, repo, 0)

				fileContent, err := thirdparty.GetGitHubFileFromGit(t.Context(), owner, repo, revision, include.FileName, worktreeDir)
				require.NoError(t, err)
				require.NotEmpty(t, fileContent)

				comparisonFile, err := thirdparty.GetGithubFile(t.Context(), owner, repo, include.FileName, revision, nil)
				require.NoError(t, err)
				require.NotZero(t, comparisonFile)
				comparisonFileContent, err := base64.StdEncoding.DecodeString(*comparisonFile.Content)
				require.NoError(t, err)
				assert.Equal(t, comparisonFileContent, fileContent, "git restored file for include should exactly match file retrieved from GitHub API")
			}
		},
		"SucceedsWithFewerWorkersThanIncludeFiles": func(t *testing.T, modules ModuleList, includes []parserInclude, opts *GetProjectOpts) {
			const numWorkers = 1
			dirs, err := setupParallelGitIncludeDirs(t.Context(), modules, includes, numWorkers, opts)
			assert.NoError(t, err)
			require.NotZero(t, dirs)

			assert.Len(t, dirs.clonesForOwnerRepo, len(dirs.worktreesForOwnerRepo), "each git clone should have one set of worktrees")
			for _, dir := range dirs.clonesForOwnerRepo {
				assert.True(t, utility.FileExists(dir))
			}
			for _, worktreeDirs := range dirs.worktreesForOwnerRepo {
				assert.Len(t, worktreeDirs, numWorkers)
				for _, worktreeDir := range worktreeDirs {
					assert.True(t, utility.FileExists(worktreeDir))
				}
			}
		},
		"NoopsIfReadingFromLocal": func(t *testing.T, modules ModuleList, includes []parserInclude, opts *GetProjectOpts) {
			opts.ReadFileFrom = ReadFromLocal
			dirs, err := setupParallelGitIncludeDirs(t.Context(), modules, includes, 1, opts)
			require.NoError(t, err)
			assert.Zero(t, dirs)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			modules := ModuleList{
				{
					Name:   "sample",
					Owner:  "evergreen-ci",
					Repo:   "sample",
					Branch: "main",
				},
				{
					Name:   "commit-queue-sandbox",
					Owner:  "evergreen-ci",
					Repo:   "commit-queue-sandbox",
					Branch: "main",
				},
				{
					Name:   "merge-queue-sandbox",
					Owner:  "evergreen-ci",
					Repo:   "github-merge-queue-sandbox",
					Branch: "main",
				},
			}
			includes := []parserInclude{
				{
					FileName: "config_test/evg_settings.yml",
				},
				{
					FileName: "config_test/evg_settings_with_3rd_party_defaults.yml",
				},
				{
					FileName: "evergreen.yml",
					Module:   "sample",
				},
				{
					FileName: "evergreen.yml",
					Module:   "commit-queue-sandbox",
				},
				{
					FileName: "commit-queue-include-yaml.yaml",
					Module:   "merge-queue-sandbox",
				},
			}
			opts := &GetProjectOpts{
				Ref: &ProjectRef{
					Id:         "evergreen",
					Identifier: "evergreen",
					Owner:      "evergreen-ci",
					Repo:       "evergreen",
					Branch:     "main",
					RemotePath: "self-tests.yml",
				},
				ReadFileFrom: ReadFromGithub,
				Revision:     "0503cb25b87dee6a09a74c42c99e314d55edf36b",
			}
			tCase(t, modules, includes, opts)
		})
	}
}
