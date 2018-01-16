package model

import (
	"fmt"
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/mongodb/grip"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
)

func TestMakePatchedConfig(t *testing.T) {
	Convey("With calling MakePatchedConfig with a config and remote configuration path", t, func() {
		cwd := testutil.GetDirectoryOfFile()

		Convey("the config should be patched correctly", func() {
			remoteConfigPath := filepath.Join("config", "evergreen.yml")
			fileBytes, err := ioutil.ReadFile(filepath.Join(cwd, "testdata", "patch.diff"))
			So(err, ShouldBeNil)
			// update patch with remove config path variable
			diffString := fmt.Sprintf(string(fileBytes),
				remoteConfigPath, remoteConfigPath, remoteConfigPath, remoteConfigPath)
			// the patch adds a new task
			p := &patch.Patch{
				Patches: []patch.ModulePatch{{
					Githash: "revision",
					PatchSet: patch.PatchSet{
						Patch: diffString,
						Summary: []patch.Summary{{
							Name:      remoteConfigPath,
							Additions: 3,
							Deletions: 3,
						}},
					},
				}},
			}
			projectBytes, err := ioutil.ReadFile(filepath.Join(cwd, "testdata", "project.config"))
			So(err, ShouldBeNil)
			project, err := MakePatchedConfig(p, remoteConfigPath, string(projectBytes))
			So(err, ShouldBeNil)
			So(project, ShouldNotBeNil)
			So(len(project.Tasks), ShouldEqual, 2)
		})
		Convey("an empty base config should be patched correctly", func() {
			remoteConfigPath := filepath.Join("model", "testdata", "project2.config")
			fileBytes, err := ioutil.ReadFile(filepath.Join(cwd, "testdata", "project.diff"))
			So(err, ShouldBeNil)
			p := &patch.Patch{
				Patches: []patch.ModulePatch{{
					Githash: "revision",
					PatchSet: patch.PatchSet{
						Patch:   string(fileBytes),
						Summary: []patch.Summary{{Name: remoteConfigPath}},
					},
				}},
			}

			project, err := MakePatchedConfig(p, remoteConfigPath, "")
			So(err, ShouldBeNil)
			So(project, ShouldNotBeNil)
			So(len(project.Tasks), ShouldEqual, 1)
			So(project.Tasks[0].Name, ShouldEqual, "hello")

			Reset(func() {
				grip.Warning(os.Remove(remoteConfigPath))
			})
		})
	})
}

// shouldContainPair returns a blank string if its arguments resemble each other, and returns a
// list of pretty-printed diffs between the objects if they do not match.
func shouldContainPair(actual interface{}, expected ...interface{}) string {
	actualPairsList, ok := actual.([]TVPair)

	if !ok {
		return fmt.Sprintf("Assertion requires a list of TVPair objects")
	}

	if len(expected) != 1 {
		return fmt.Sprintf("Assertion requires 1 expected value, you provided %v", len(expected))
	}

	expectedPair, ok := expected[0].(TVPair)
	if !ok {
		return fmt.Sprintf("Assertion requires expected value to be an instance of TVPair")
	}

	for _, ap := range actualPairsList {
		if ap.Variant == expectedPair.Variant && ap.TaskName == expectedPair.TaskName {
			return ""
		}
	}
	return fmt.Sprintf("Expected list to contain pair '%v', but it didn't", expectedPair)
}

func TestIncludePatchDependencies(t *testing.T) {
	Convey("With a project task config with cross-variant dependencies", t, func() {
		p := &Project{
			Tasks: []ProjectTask{
				{Name: "t1"},
				{Name: "t2", DependsOn: []TaskUnitDependency{{Name: "t1"}}},
				{Name: "t3"},
				{Name: "t4", Patchable: new(bool)},
				{Name: "t5", DependsOn: []TaskUnitDependency{{Name: "t4"}}},
			},
			BuildVariants: []BuildVariant{
				{Name: "v1", TaskUnits: []BuildVariantTaskUnit{{Name: "t1"}, {Name: "t2"}}},
				{Name: "v2", TaskUnits: []BuildVariantTaskUnit{
					{Name: "t3", DependsOn: []TaskUnitDependency{{Name: "t2", Variant: "v1"}}}}},
			},
		}

		Convey("a patch against v1/t1 should remain unchanged", func() {
			pairs := IncludePatchDependencies(p, []TVPair{{"v1", "t1"}})
			So(len(pairs), ShouldEqual, 1)
			So(pairs[0], ShouldResemble, TVPair{"v1", "t1"})
		})

		Convey("a patch against v1/t2 should add t1", func() {
			pairs := IncludePatchDependencies(p, []TVPair{{"v1", "t2"}})
			So(len(pairs), ShouldEqual, 2)
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
		})

		Convey("a patch against v2/t3 should add t1,t2, and v1", func() {
			pairs := IncludePatchDependencies(p, []TVPair{{"v2", "t3"}})
			So(len(pairs), ShouldEqual, 3)
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v2", "t3"})
		})

		Convey("a patch against v2/t5 should be pruned, since its dependency is not patchable", func() {
			pairs := IncludePatchDependencies(p, []TVPair{{"v2", "t5"}})
			So(len(pairs), ShouldEqual, 0)
		})
	})

	Convey("With a project task config with * selectors", t, func() {
		p := &Project{
			Tasks: []ProjectTask{
				{Name: "t1"},
				{Name: "t2"},
				{Name: "t3", DependsOn: []TaskUnitDependency{{Name: AllDependencies}}},
				{Name: "t4", DependsOn: []TaskUnitDependency{{Name: "t3", Variant: AllVariants}}},
				{Name: "t5", DependsOn: []TaskUnitDependency{{Name: AllDependencies, Variant: AllVariants}}},
			},
			BuildVariants: []BuildVariant{
				{Name: "v1", TaskUnits: []BuildVariantTaskUnit{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}},
				{Name: "v2", TaskUnits: []BuildVariantTaskUnit{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}},
				{Name: "v3", TaskUnits: []BuildVariantTaskUnit{{Name: "t4"}}},
				{Name: "v4", TaskUnits: []BuildVariantTaskUnit{{Name: "t5"}}},
			},
		}

		Convey("a patch against v1/t3 should include t2 and t1", func() {
			pairs := IncludePatchDependencies(p, []TVPair{{"v1", "t3"}})
			So(len(pairs), ShouldEqual, 3)
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v1", "t3"})
		})

		Convey("a patch against v3/t4 should include v1, v2, t3, t2, and t1", func() {
			pairs := IncludePatchDependencies(p, []TVPair{{"v3", "t4"}})
			So(len(pairs), ShouldEqual, 7)

			So(pairs, shouldContainPair, TVPair{"v3", "t4"})
			// requires t3 on the other variants
			So(pairs, shouldContainPair, TVPair{"v1", "t3"})
			So(pairs, shouldContainPair, TVPair{"v2", "t3"})

			// t3 requires all the others
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v2", "t2"})
			So(pairs, shouldContainPair, TVPair{"v2", "t1"})

		})

		Convey("a patch against v4/t5 should include v1, v2, v3, t4, t3, t2, and t1", func() {
			pairs := IncludePatchDependencies(p, []TVPair{{"v4", "t5"}})
			So(len(pairs), ShouldEqual, 8)
			So(pairs, shouldContainPair, TVPair{"v4", "t5"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t3"})
			So(pairs, shouldContainPair, TVPair{"v2", "t1"})
			So(pairs, shouldContainPair, TVPair{"v2", "t2"})
			So(pairs, shouldContainPair, TVPair{"v2", "t3"})
			So(pairs, shouldContainPair, TVPair{"v3", "t4"})
		})
	})

	Convey("With a project task config with required tasks", t, func() {
		all := []BuildVariantTaskUnit{{Name: "1"}, {Name: "2"}, {Name: "3"},
			{Name: "before"}, {Name: "after"}}
		beforeDep := []TaskUnitDependency{{Name: "before"}}
		p := &Project{
			Tasks: []ProjectTask{
				{Name: "before", Requires: []TaskUnitRequirement{{Name: "after"}}},
				{Name: "1", DependsOn: beforeDep},
				{Name: "2", DependsOn: beforeDep},
				{Name: "3", DependsOn: beforeDep},
				{Name: "after", DependsOn: []TaskUnitDependency{
					{Name: "before"},
					{Name: "1", PatchOptional: true},
					{Name: "2", PatchOptional: true},
					{Name: "3", PatchOptional: true},
				}},
			},
			BuildVariants: []BuildVariant{
				{Name: "v1", TaskUnits: all},
				{Name: "v2", TaskUnits: all},
			},
		}

		Convey("scheduling the 'before' task should also schedule 'after'", func() {
			pairs := IncludePatchDependencies(p, []TVPair{{"v1", "before"}})
			So(len(pairs), ShouldEqual, 2)
			So(pairs, shouldContainPair, TVPair{"v1", "before"})
			So(pairs, shouldContainPair, TVPair{"v1", "after"})
		})
		Convey("scheduling the middle tasks should include 'before' and 'after'", func() {
			Convey("for '1'", func() {
				pairs := IncludePatchDependencies(p, []TVPair{{"v1", "1"}})
				So(len(pairs), ShouldEqual, 3)
				So(pairs, shouldContainPair, TVPair{"v1", "before"})
				So(pairs, shouldContainPair, TVPair{"v1", "after"})
				So(pairs, shouldContainPair, TVPair{"v1", "1"})
			})
			Convey("for '1' '2' '3'", func() {
				pairs := IncludePatchDependencies(p, []TVPair{{"v1", "1"}, {"v1", "2"}, {"v1", "3"}})

				So(len(pairs), ShouldEqual, 5)
				So(pairs, shouldContainPair, TVPair{"v1", "before"})
				So(pairs, shouldContainPair, TVPair{"v1", "1"})
				So(pairs, shouldContainPair, TVPair{"v1", "2"})
				So(pairs, shouldContainPair, TVPair{"v1", "3"})
				So(pairs, shouldContainPair, TVPair{"v1", "after"})
			})
		})
	})
	Convey("With a project task config with cyclical requirements", t, func() {
		all := []BuildVariantTaskUnit{{Name: "1"}, {Name: "2"}, {Name: "3"}}
		p := &Project{
			Tasks: []ProjectTask{
				{Name: "1", Requires: []TaskUnitRequirement{{Name: "2"}, {Name: "3"}}},
				{Name: "2", Requires: []TaskUnitRequirement{{Name: "1"}, {Name: "3"}}},
				{Name: "3", Requires: []TaskUnitRequirement{{Name: "2"}, {Name: "1"}}},
			},
			BuildVariants: []BuildVariant{
				{Name: "v1", TaskUnits: all},
				{Name: "v2", TaskUnits: all},
			},
		}
		Convey("all tasks should be scheduled no matter which is initially added", func() {
			Convey("for '1'", func() {
				pairs := IncludePatchDependencies(p, []TVPair{{"v1", "1"}})
				So(len(pairs), ShouldEqual, 3)
				So(pairs, shouldContainPair, TVPair{"v1", "1"})
				So(pairs, shouldContainPair, TVPair{"v1", "2"})
				So(pairs, shouldContainPair, TVPair{"v1", "3"})
			})
			Convey("for '2'", func() {
				pairs := IncludePatchDependencies(p, []TVPair{{"v1", "2"}, {"v2", "2"}})
				So(len(pairs), ShouldEqual, 6)
				So(pairs, shouldContainPair, TVPair{"v1", "1"})
				So(pairs, shouldContainPair, TVPair{"v1", "2"})
				So(pairs, shouldContainPair, TVPair{"v1", "3"})
				So(pairs, shouldContainPair, TVPair{"v2", "1"})
				So(pairs, shouldContainPair, TVPair{"v2", "2"})
				So(pairs, shouldContainPair, TVPair{"v2", "3"})
			})
			Convey("for '3'", func() {
				pairs := IncludePatchDependencies(p, []TVPair{{"v2", "3"}})
				So(len(pairs), ShouldEqual, 3)
				So(pairs, shouldContainPair, TVPair{"v2", "1"})
				So(pairs, shouldContainPair, TVPair{"v2", "2"})
				So(pairs, shouldContainPair, TVPair{"v2", "3"})
			})
		})
	})
	Convey("With a project task config that requires a non-patchable task", t, func() {
		p := &Project{
			Tasks: []ProjectTask{
				{Name: "1", Requires: []TaskUnitRequirement{{Name: "2"}}},
				{Name: "2", Patchable: new(bool)},
			},
			BuildVariants: []BuildVariant{
				{Name: "v1", TaskUnits: []BuildVariantTaskUnit{{Name: "1"}, {Name: "2"}}},
			},
		}
		Convey("the non-patchable task should not be added", func() {
			pairs := IncludePatchDependencies(p, []TVPair{{"v1", "1"}})

			So(len(pairs), ShouldEqual, 0)
		})
	})

}

func TestVariantTasksToTVPairs(t *testing.T) {
	assert := assert.New(t)

	input := []patch.VariantTasks{
		patch.VariantTasks{
			Variant: "variant",
			Tasks:   []string{"task1", "task2", "task3"},
			DisplayTasks: []patch.DisplayTask{
				patch.DisplayTask{
					Name: "displaytask1",
				},
			},
		},
	}
	output := VariantTasksToTVPairs(input)
	assert.Len(output.ExecTasks, 3)
	assert.Len(output.DisplayTasks, 1)

	original := output.TVPairsToVariantTasks()
	assert.Equal(input, original)
}

func TestAddNewPatch(t *testing.T) {
	assert := assert.New(t) //nolint

	testutil.HandleTestingErr(db.ClearCollections(patch.Collection, version.Collection, build.Collection, task.Collection), t, "problem clearing collections")
	p := &patch.Patch{
		Activated: true,
	}
	v := &version.Version{
		Id:         "version",
		Revision:   "1234",
		Requester:  evergreen.PatchVersionRequester,
		CreateTime: time.Now(),
	}
	assert.NoError(p.Insert())
	assert.NoError(v.Insert())

	proj := &Project{
		Identifier: "project",
		BuildVariants: []BuildVariant{
			BuildVariant{
				Name: "variant",
				TaskUnits: []BuildVariantTaskUnit{
					{Name: "task1"}, {Name: "task2"}, {Name: "task3"},
				},
				DisplayTasks: []DisplayTask{
					DisplayTask{
						Name:           "displaytask1",
						ExecutionTasks: []string{"task1", "task2"},
					},
				},
			},
		},
		Tasks: []ProjectTask{
			ProjectTask{Name: "task1"}, ProjectTask{Name: "task2"}, ProjectTask{Name: "task3"},
		},
	}
	tasks := VariantTasksToTVPairs([]patch.VariantTasks{
		patch.VariantTasks{
			Variant: "variant",
			Tasks:   []string{"task1", "task2", "task3"},
			DisplayTasks: []patch.DisplayTask{
				patch.DisplayTask{
					Name: "displaytask1",
				},
			},
		},
	})

	assert.NoError(AddNewBuildsForPatch(p, v, proj, tasks))
	dbBuild, err := build.FindOne(db.Q{})
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Len(dbBuild.Tasks, 2)
	assert.Equal(dbBuild.Tasks[0].DisplayName, "displaytask1")
	assert.Equal(dbBuild.Tasks[1].DisplayName, "task3")

	assert.NoError(AddNewTasksForPatch(p, v, proj, tasks))
	dbTasks, err := task.FindWithDisplayTasks(task.ByBuildId(dbBuild.Id))
	assert.NoError(err)
	assert.NotNil(dbBuild)
	assert.Len(dbTasks, 4)
	assert.Equal(dbTasks[0].DisplayName, "displaytask1")
	assert.Equal(dbTasks[1].DisplayName, "task1")
	assert.Equal(dbTasks[2].DisplayName, "task2")
	assert.Equal(dbTasks[3].DisplayName, "task3")
}
