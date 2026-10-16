package model

import (
	"fmt"
	"slices"
	"testing"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

// We have to define a wrapper for the dependencies,
// since you can't marshal BSON straight into a slice.
type depTask struct {
	DependsOn []task.Dependency `bson:"depends_on"`
}

func TestDependencyBSON(t *testing.T) {
	Convey("With BSON bytes", t, func() {
		Convey("representing legacy dependency format (i.e. just strings)", func() {
			bytes, err := mgobson.Marshal(map[string]any{
				"depends_on": []string{"t1", "t2", "t3"},
			})
			require.NoError(t, err, "failed to marshal test BSON")

			Convey("unmarshalling the BSON into a Dependency slice should succeed", func() {
				var deps depTask
				So(mgobson.Unmarshal(bytes, &deps), ShouldBeNil)
				So(len(deps.DependsOn), ShouldEqual, 3)

				Convey("with the proper tasks", func() {
					So(deps.DependsOn[0].TaskId, ShouldEqual, "t1")
					So(deps.DependsOn[0].Status, ShouldEqual, evergreen.TaskSucceeded)
					So(deps.DependsOn[1].TaskId, ShouldEqual, "t2")
					So(deps.DependsOn[1].Status, ShouldEqual, evergreen.TaskSucceeded)
					So(deps.DependsOn[2].TaskId, ShouldEqual, "t3")
					So(deps.DependsOn[2].Status, ShouldEqual, evergreen.TaskSucceeded)
				})
			})
		})

		Convey("representing the current dependency format", func() {
			inputDeps := depTask{[]task.Dependency{
				{TaskId: "t1", Status: evergreen.TaskSucceeded},
				{TaskId: "t2", Status: "*"},
				{TaskId: "t3", Status: evergreen.TaskFailed},
			}}
			bytes, err := bson.Marshal(inputDeps)
			require.NoError(t, err, "failed to marshal test BSON")

			Convey("unmarshalling the BSON into a Dependency slice should succeed", func() {
				var deps depTask
				So(bson.Unmarshal(bytes, &deps), ShouldBeNil)

				Convey("with the proper tasks", func() {
					So(deps.DependsOn, ShouldResemble, inputDeps.DependsOn)
				})
			})
		})
	})
}

func TestIncludeDependenciesForTaskGroups(t *testing.T) {
	testCases := map[string]func(t *testing.T, p *Project){
		"SingleHostTaskGroup/SchedulesPreviousTasks": func(t *testing.T, p *Project) {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "tg1t2"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 2)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t1"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t2"})
		},
		"SingleHostTaskGroup/SchedulesAllTasks": func(t *testing.T, p *Project) {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "tg1t4"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 3)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t1"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t2"})
			assert.NotContains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t3"}) // tg1t3 is diabled.
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t4"})
		},
		"SingleHostTaskGroup/FirstTaskSchedulesNoOtherTasks": func(t *testing.T, p *Project) {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "tg1t1"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 1)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t1"})
		},
		"SingleHostTaskGroup/TaskGroupTaskAsDependencyIsIncluded": func(t *testing.T, p *Project) {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "t2"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 3)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "t2"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t1"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t2"})

			// The variant selector should not affect the result.
			pairs, err = IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "t3"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 3)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "t3"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t1"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg1t2"})

			// The task group being defined by tg4 should not affect the result.
			pairs, err = IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "tg4"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 2)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg4t1"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg4t2"})
		},
		"SingleHostTaskGroup/InvalidVariantShouldBeIgnored": func(t *testing.T, p *Project) {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "t4"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 1)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "t4"})
		},
		"SingleHostTaskGroup/WithADependencyOnATaskGroup": func(t *testing.T, p *Project) {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "t7"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 3)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "t7"})
			assert.Contains(t, pairs, TVPair{Variant: "v2", TaskName: "tg3t1"})
			assert.Contains(t, pairs, TVPair{Variant: "v2", TaskName: "tg3t2"})
		},
		"SingleHostTaskGroup/ScheduleTaskGroup": func(t *testing.T, p *Project) {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "tg4"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 2)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg4t1"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg4t2"})
		},
		"MultiHostTaskGroup/ShouldNotSchedulePreviousTasks": func(t *testing.T, p *Project) {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "t5"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 2)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "t5"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg2t2"})
		},
		"MultiHostTaskGroup/AllVariantsSelectorWorks": func(t *testing.T, p *Project) {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "t6"}}, evergreen.PatchVersionRequester, "", nil)
			require.NoError(t, err)
			assert.Len(t, pairs, 2)
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "t6"})
			assert.Contains(t, pairs, TVPair{Variant: "v1", TaskName: "tg2t1"})
		},
	}
	for name, tC := range testCases {
		t.Run(name, func(t *testing.T) {
			parserProject := &ParserProject{
				Tasks: []parserTask{
					{Name: "t1"},
					{Name: "t2", DependsOn: parserDependencies{{TaskSelector: taskSelector{
						Name: "tg1t2",
					}}}},
					{Name: "t3", DependsOn: parserDependencies{{TaskSelector: taskSelector{
						Name:    "tg1t2",
						Variant: &variantSelector{StringSelector: "v1"},
					}}}},
					{Name: "t4", DependsOn: parserDependencies{{TaskSelector: taskSelector{
						Name:    "tg1t2",
						Variant: &variantSelector{StringSelector: "v2"},
					}}}},
					{Name: "t5", DependsOn: parserDependencies{{TaskSelector: taskSelector{
						Name: "tg2t2",
					}}}},
					{Name: "t6", DependsOn: parserDependencies{{TaskSelector: taskSelector{
						Name:    "tg2t1",
						Variant: &variantSelector{StringSelector: AllVariants},
					}}}},
					{Name: "t7", DependsOn: parserDependencies{{TaskSelector: taskSelector{
						Name:    "tg3t2",
						Variant: &variantSelector{StringSelector: AllVariants},
					}}}},
					{Name: "tg1t1"},
					{Name: "tg1t2"},
					{Name: "tg1t3", Disable: utility.TruePtr()},
					{Name: "tg1t4"},
					{Name: "tg2t1"},
					{Name: "tg2t2"},
					{Name: "tg2t3"},
					{Name: "tg3t1"},
					{Name: "tg3t2"},
					{Name: "tg4t1"},
					{Name: "tg4t2"},
				},
				BuildVariants: []parserBV{
					{Name: "v1", Tasks: []parserBVTaskUnit{
						{Name: "t1"},
						{Name: "t2"},
						{Name: "t3"},
						{Name: "t4"},
						{Name: "t5"},
						{Name: "t6"},
						{Name: "t7"},
						{Name: "tg1"},
						{Name: "tg2"},
						{Name: "tg4"},
					}},
					{Name: "v2", Tasks: []parserBVTaskUnit{
						{Name: "t1"},
						{Name: "tg3"},
					}},
				},
				TaskGroups: []parserTaskGroup{
					{Name: "tg1", Tasks: []string{"tg1t1", "tg1t2", "tg1t3", "tg1t4"}, MaxHosts: 1},
					{Name: "tg2", Tasks: []string{"tg2t1", "tg2t2", "tg2t3"}, MaxHosts: 2},
					{Name: "tg3", Tasks: []string{"tg3t1", "tg3t2"}, MaxHosts: 1},
					{Name: "tg4", Tasks: []string{"tg4t1", "tg4t2"}, MaxHosts: 1},
				},
			}
			p, err := TranslateProject(t.Context(), parserProject)
			require.NoError(t, err)
			require.NotNil(t, p)
			tC(t, p)
		})
	}
}

func TestIncludeDependencies(t *testing.T) {
	Convey("With a project task config with cross-variant dependencies", t, func() {
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "t1"},
				{Name: "t2", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "t1"}}}},
				{Name: "t3"},
				{Name: "t4", Patchable: utility.FalsePtr()},
				{Name: "t5", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "t4"}}}},
				{Name: "t6", DependsOn: parserDependencies{{TaskSelector: taskSelector{
					Name:    "tgt1",
					Variant: &variantSelector{StringSelector: AllVariants},
				}}}},
				{Name: "t7", DependsOn: parserDependencies{{TaskSelector: taskSelector{
					Name: "tgt2",
				}}}},
				{Name: "tgt1"},
				{Name: "tgt2"},
			},
			BuildVariants: []parserBV{
				{Name: "v1", Tasks: []parserBVTaskUnit{
					{Name: "t1"},
					{Name: "t2"},
					{Name: "tgt1"},
				}},
				{Name: "v2", Tasks: []parserBVTaskUnit{
					{Name: "t3", DependsOn: parserDependencies{
						{TaskSelector: taskSelector{Name: "t2", Variant: &variantSelector{StringSelector: "v1"}}},
					}},
					{Name: "t4"},
					{Name: "t5"},
					{Name: "t6"},
					{Name: "t7"},
					{Name: "tgt2"},
				}},
			},
			TaskGroups: []parserTaskGroup{
				{Name: "tg1", Tasks: []string{"tgt1", "tgt2"}},
			},
		}
		p, err := TranslateProject(t.Context(), parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		Convey("a patch against v1/t1 should remain unchanged", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "t1"}}, evergreen.PatchVersionRequester, "", nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 1)
			So(pairs[0], ShouldResemble, TVPair{Variant: "v1", TaskName: "t1"})
		})

		Convey("a patch against v1/t2 should add t1", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "t2"}}, evergreen.PatchVersionRequester, "", nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 2)
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t2"})
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t1"})
		})

		Convey("a patch against v2/t3 should add t1,t2, and v1", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v2", TaskName: "t3"}}, evergreen.PatchVersionRequester, "", nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 3)
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t2"})
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t1"})
			So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "t3"})
		})

		Convey("a patch against v2/t5 should be pruned, since its dependency is not patchable", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v2", TaskName: "t5"}}, evergreen.PatchVersionRequester, "", nil)
			So(err, ShouldNotBeNil)
			So(len(pairs), ShouldEqual, 0)

			pairs, err = IncludeDependencies(p, []TVPair{{Variant: "v2", TaskName: "t5"}}, evergreen.RepotrackerVersionRequester, "", nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 2)
		})

		Convey("a patch that has a dependency on a task group task with all variants should include that one task", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v2", TaskName: "t6"}}, evergreen.PatchVersionRequester, "", nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 2)
			expectedPairs := []TVPair{
				{Variant: "v2", TaskName: "t6"},
				{Variant: "v1", TaskName: "tgt1"},
			}
			So(expectedPairs, ShouldContainResembling, pairs[0])
			So(expectedPairs, ShouldContainResembling, pairs[1])
		})
		Convey("a patch that has a dependency on a task group task within the same variant should include that one task", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{Variant: "v2", TaskName: "t7"}}, evergreen.PatchVersionRequester, "", nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 2)
			expectedPairs := []TVPair{
				{Variant: "v2", TaskName: "t7"},
				{Variant: "v2", TaskName: "tgt2"},
			}
			So(expectedPairs, ShouldContainResembling, pairs[0])
			So(expectedPairs, ShouldContainResembling, pairs[1])
		})
	})

	Convey("With a project task config with * selectors", t, func() {
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "t1"},
				{Name: "t2"},
				{Name: "t3", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: AllDependencies}}}},
				{Name: "t4", DependsOn: parserDependencies{
					{TaskSelector: taskSelector{
						Name: "t3", Variant: &variantSelector{StringSelector: AllVariants},
					}},
				}},
				{Name: "t5", DependsOn: parserDependencies{
					{TaskSelector: taskSelector{
						Name: AllDependencies, Variant: &variantSelector{StringSelector: AllVariants},
					}},
				}},
			},
			BuildVariants: []parserBV{
				{Name: "v1", Tasks: []parserBVTaskUnit{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}},
				{Name: "v2", Tasks: []parserBVTaskUnit{{Name: "t1"}, {Name: "t2"}, {Name: "t3"}}},
				{Name: "v3", Tasks: []parserBVTaskUnit{{Name: "t4"}}},
				{Name: "v4", Tasks: []parserBVTaskUnit{{Name: "t5"}}},
			},
		}
		p, err := TranslateProject(t.Context(), parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		Convey("a patch against v1/t3 should include t2 and t1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "t3"}}, evergreen.PatchVersionRequester, "", nil)
			So(len(pairs), ShouldEqual, 3)
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t2"})
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t1"})
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t3"})
		})

		Convey("a patch against v3/t4 should include v1, v2, t3, t2, and t1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{Variant: "v3", TaskName: "t4"}}, evergreen.PatchVersionRequester, "", nil)
			So(len(pairs), ShouldEqual, 7)

			So(pairs, shouldContainPair, TVPair{Variant: "v3", TaskName: "t4"})
			// requires t3 on the other variants
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t3"})
			So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "t3"})

			// t3 requires all the others
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t2"})
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t1"})
			So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "t2"})
			So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "t1"})
		})

		Convey("a patch against v4/t5 should include v1, v2, v3, t4, t3, t2, and t1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{Variant: "v4", TaskName: "t5"}}, evergreen.PatchVersionRequester, "", nil)
			So(len(pairs), ShouldEqual, 8)
			So(pairs, shouldContainPair, TVPair{Variant: "v4", TaskName: "t5"})
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t1"})
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t2"})
			So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "t3"})
			So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "t1"})
			So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "t2"})
			So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "t3"})
			So(pairs, shouldContainPair, TVPair{Variant: "v3", TaskName: "t4"})
		})
	})

	Convey("With a project task config with cyclical requirements", t, func() {
		all := []parserBVTaskUnit{{Name: "1"}, {Name: "2"}, {Name: "3"}}
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "1", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "2"}}, {TaskSelector: taskSelector{Name: "3"}}}},
				{Name: "2", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "1"}}, {TaskSelector: taskSelector{Name: "3"}}}},
				{Name: "3", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "2"}}, {TaskSelector: taskSelector{Name: "1"}}}},
			},
			BuildVariants: []parserBV{
				{Name: "v1", Tasks: all},
				{Name: "v2", Tasks: all},
			},
		}

		p, err := TranslateProject(t.Context(), parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		Convey("all tasks should be scheduled no matter which is initially added", func() {
			Convey("for '1'", func() {
				pairs, _ := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "1"}}, evergreen.PatchVersionRequester, "", nil)
				So(len(pairs), ShouldEqual, 3)
				So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "1"})
				So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "2"})
				So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "3"})
			})
			Convey("for '2'", func() {
				pairs, _ := IncludeDependencies(p, []TVPair{{Variant: "v1", TaskName: "2"}, {"v2", "2"}}, evergreen.PatchVersionRequester, "", nil)
				So(len(pairs), ShouldEqual, 6)
				So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "1"})
				So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "2"})
				So(pairs, shouldContainPair, TVPair{Variant: "v1", TaskName: "3"})
				So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "1"})
				So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "2"})
				So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "3"})
			})
			Convey("for '3'", func() {
				pairs, _ := IncludeDependencies(p, []TVPair{{Variant: "v2", TaskName: "3"}}, evergreen.PatchVersionRequester, "", nil)
				So(len(pairs), ShouldEqual, 3)
				So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "1"})
				So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "2"})
				So(pairs, shouldContainPair, TVPair{Variant: "v2", TaskName: "3"})
			})
		})
	})
	Convey("With a task that depends on task groups", t, func() {
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "a", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "*", Variant: &variantSelector{StringSelector: "*"}}}}},
				{Name: "b"},
			},
			TaskGroups: []parserTaskGroup{
				{Name: "task-group", Tasks: []string{"b"}},
			},
			BuildVariants: []parserBV{
				{Name: "variant-with-group", Tasks: []parserBVTaskUnit{{Name: "task-group"}}},
				{Name: "initial-variant", Tasks: []parserBVTaskUnit{{Name: "a"}}},
			},
		}
		p, err := TranslateProject(t.Context(), parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		initDep := TVPair{TaskName: "a", Variant: "initial-variant"}
		pairs, _ := IncludeDependencies(p, []TVPair{initDep}, evergreen.PatchVersionRequester, "", nil)
		So(pairs, ShouldHaveLength, 2)
		So(initDep, ShouldBeIn, pairs)
		So(TVPair{TaskName: "b", Variant: "variant-with-group"}, ShouldBeIn, pairs)
	})
	Convey("With a disabled task", t, func() {
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "a", Disable: utility.TruePtr()},
			},
			BuildVariants: []parserBV{
				{Name: "bv", Tasks: []parserBVTaskUnit{{Name: "a"}}},
			},
		}
		p, err := TranslateProject(t.Context(), parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		pair := TVPair{TaskName: "a", Variant: "bv"}
		pairs, _ := IncludeDependencies(p, []TVPair{pair}, evergreen.PatchVersionRequester, "", nil)
		So(pairs, ShouldBeEmpty)
	})
	Convey("With a task group that has a subset of its tasks disabled", t, func() {
		parserProject := &ParserProject{
			Tasks: []parserTask{
				{Name: "a"},
				{Name: "b"},
				{Name: "c", Disable: utility.TruePtr()},
			},
			TaskGroups: []parserTaskGroup{
				{Name: "task-group", MaxHosts: 2, Tasks: []string{"a", "b", "c"}},
			},
			BuildVariants: []parserBV{
				{Name: "bv-with-group", Tasks: []parserBVTaskUnit{{Name: "task-group"}}},
			},
		}
		p, err := TranslateProject(t.Context(), parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		tgTaskPair := TVPair{TaskName: "task-group", Variant: "bv-with-group"}
		pairs, _ := IncludeDependencies(p, []TVPair{tgTaskPair}, evergreen.PatchVersionRequester, "", nil)
		So(pairs, ShouldHaveLength, 2)
		So(pairs, ShouldNotContain, tgTaskPair)
		So(pairs, ShouldNotContain, TVPair{TaskName: "c", Variant: "bv-with-group"})
		So(pairs, ShouldContain, TVPair{TaskName: "a", Variant: "bv-with-group"})
		So(pairs, ShouldContain, TVPair{TaskName: "b", Variant: "bv-with-group"})
	})
}

func TestInactiveGeneratedRootMarksDepsInactive(t *testing.T) {
	t.Run("ExplicitDeactivateWithBVCronPropagatesInactivity", func(t *testing.T) {
		p := &Project{
			Tasks: []ProjectTask{{Name: "my_task"}},
			BuildVariants: []BuildVariant{{
				Name:          "bv1",
				CronBatchTime: "0 * * * *",
				Tasks:         []BuildVariantTaskUnit{{Name: "my_task", Activate: utility.FalsePtr()}},
			}},
		}
		di := &dependencyIncluder{Project: p}
		result := di.inactiveGeneratedRootMarksDepsInactive(TVPair{Variant: "bv1", TaskName: "my_task"})
		assert.True(t, result)
	})

	t.Run("BVCronWithoutExplicitDeactivateDoesNotPropagate", func(t *testing.T) {
		p := &Project{
			Tasks: []ProjectTask{{Name: "my_task"}},
			BuildVariants: []BuildVariant{{
				Name:          "bv1",
				CronBatchTime: "0 * * * *",
				Tasks:         []BuildVariantTaskUnit{{Name: "my_task"}},
			}},
		}
		di := &dependencyIncluder{Project: p}
		result := di.inactiveGeneratedRootMarksDepsInactive(TVPair{Variant: "bv1", TaskName: "my_task"})
		assert.False(t, result)
	})

	t.Run("ExplicitDeactivateWithoutCronPropagatesInactivity", func(t *testing.T) {
		p := &Project{
			Tasks: []ProjectTask{{Name: "my_task"}},
			BuildVariants: []BuildVariant{{
				Name:  "bv1",
				Tasks: []BuildVariantTaskUnit{{Name: "my_task", Activate: utility.FalsePtr()}},
			}},
		}
		di := &dependencyIncluder{Project: p}
		result := di.inactiveGeneratedRootMarksDepsInactive(TVPair{Variant: "bv1", TaskName: "my_task"})
		assert.True(t, result)
	})
}

func TestIncludeDependenciesWithGeneratedSharedPrerequisitesPreservesActivation(t *testing.T) {
	for _, rootOrder := range []struct {
		name  string
		roots []string
	}{
		{name: "ActiveRootFirst", roots: []string{"active", "inactive1", "inactive2"}},
		{name: "InactiveRootsFirst", roots: []string{"inactive1", "inactive2", "active"}},
	} {
		t.Run(rootOrder.name, func(t *testing.T) {
			pp := &ParserProject{
				Tasks: []parserTask{
					{Name: "active", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "shared", Variant: &variantSelector{StringSelector: "deps"}}}}},
					{Name: "inactive1", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "shared", Variant: &variantSelector{StringSelector: "deps"}}}}},
					{Name: "inactive2", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "shared", Variant: &variantSelector{StringSelector: "deps"}}}}},
					{Name: "shared", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "leaf"}}}},
					{Name: "leaf", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "shared"}}}},
				},
				BuildVariants: []parserBV{
					{Name: "roots", Tasks: []parserBVTaskUnit{{Name: "active"}, {Name: "inactive1", Activate: utility.FalsePtr()}, {Name: "inactive2", Activate: utility.FalsePtr()}}},
					{Name: "deps", Tasks: []parserBVTaskUnit{{Name: "shared"}, {Name: "leaf"}}},
				},
			}
			p, err := TranslateProject(t.Context(), pp)
			require.NoError(t, err)
			activationInfo := newSpecificActivationInfo()
			activationInfo.activationTasks["roots"] = []string{"inactive1", "inactive2"}
			var roots []TVPair
			for _, name := range rootOrder.roots {
				roots = append(roots, TVPair{Variant: "roots", TaskName: name})
			}
			pairs, err := IncludeDependenciesWithGenerated(p, roots, evergreen.RepotrackerVersionRequester, "", &activationInfo, pp.BuildVariants[:1])
			require.NoError(t, err)
			assert.ElementsMatch(t, append(slices.Clone(roots), TVPair{Variant: "deps", TaskName: "shared"}, TVPair{Variant: "deps", TaskName: "leaf"}), pairs)
			assert.ElementsMatch(t, []string{"shared", "leaf"}, activationInfo.activationTasks["deps"])
			assert.ElementsMatch(t, []string{"inactive1", "inactive2"}, activationInfo.activationTasks["roots"])
		})
	}
}

func TestIncludeDependenciesWithGeneratedPreservesSchedulingFilters(t *testing.T) {
	pp := &ParserProject{
		Tasks: []parserTask{
			{Name: "root", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: AllDependencies, Variant: &variantSelector{StringSelector: "deps"}}}}},
			{Name: "first"},
			{Name: "last", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "leaf"}}}},
			{Name: "leaf"},
			{Name: "disabled", Disable: utility.TruePtr()},
			{Name: "patchOnly", PatchOnly: utility.TruePtr()},
			{Name: "otherBranch", AllowedBranches: []string{"other"}},
		},
		TaskGroups: []parserTaskGroup{{Name: "group", Tasks: []string{"first", "last"}, MaxHosts: 1}},
		BuildVariants: []parserBV{
			{Name: "roots", Tasks: []parserBVTaskUnit{{Name: "root", Activate: utility.FalsePtr()}}},
			{Name: "deps", Tasks: []parserBVTaskUnit{{Name: "group"}, {Name: "leaf"}, {Name: "disabled"}, {Name: "patchOnly"}, {Name: "otherBranch"}}},
		},
	}
	p, err := TranslateProject(t.Context(), pp)
	require.NoError(t, err)
	activationInfo := newSpecificActivationInfo()
	activationInfo.activationTasks["roots"] = []string{"root"}
	pairs, err := IncludeDependenciesWithGenerated(p, []TVPair{{Variant: "roots", TaskName: "root"}}, evergreen.RepotrackerVersionRequester, "main", &activationInfo, pp.BuildVariants[:1])
	require.NoError(t, err)
	assert.ElementsMatch(t, []TVPair{
		{Variant: "deps", TaskName: "first"},
		{Variant: "deps", TaskName: "last"},
		{Variant: "deps", TaskName: "leaf"},
	}, pairs)
	assert.True(t, activationInfo.taskHasSpecificActivation("deps", "leaf"))
}

func TestDependencyIncluderReuseReflectsProjectChanges(t *testing.T) {
	pp := &ParserProject{
		Tasks: []parserTask{
			{Name: "root", DependsOn: parserDependencies{{TaskSelector: taskSelector{Name: "shared"}}}},
			{Name: "shared"},
			{Name: "leaf"},
		},
		BuildVariants: []parserBV{{Name: "bv", Tasks: []parserBVTaskUnit{{Name: "root", Activate: utility.FalsePtr()}, {Name: "shared"}, {Name: "leaf"}}}},
	}
	p, err := TranslateProject(t.Context(), pp)
	require.NoError(t, err)
	di := &dependencyIncluder{Project: p, requester: evergreen.RepotrackerVersionRequester}
	activationInfo := newSpecificActivationInfo()
	activationInfo.activationTasks["bv"] = []string{"root"}
	_, err = di.include([]TVPair{{Variant: "bv", TaskName: "root"}}, &activationInfo, pp.BuildVariants)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"root", "shared"}, activationInfo.activationTasks["bv"])

	p.BuildVariants[0].Tasks[1].DependsOn = []TaskUnitDependency{{Name: "leaf"}}
	activationInfo = newSpecificActivationInfo()
	activationInfo.activationTasks["bv"] = []string{"root"}
	_, err = di.include([]TVPair{{Variant: "bv", TaskName: "root"}}, &activationInfo, pp.BuildVariants)
	require.NoError(t, err)
	assert.ElementsMatch(t, []string{"root", "shared", "leaf"}, activationInfo.activationTasks["bv"])
}

func BenchmarkIncludeDependenciesWithGeneratedInactive(b *testing.B) {
	for _, taskCount := range []int{200, 700} {
		b.Run(fmt.Sprintf("Tasks%d", taskCount), func(b *testing.B) {
			pp := &ParserProject{BuildVariants: []parserBV{{Name: "bv", Activate: utility.FalsePtr()}}}
			var roots []TVPair
			for i := range taskCount {
				name := fmt.Sprintf("task%d", i)
				pt := parserTask{Name: name}
				for j := max(0, i-10); j < i; j++ {
					pt.DependsOn = append(pt.DependsOn, parserDependency{TaskSelector: taskSelector{Name: fmt.Sprintf("task%d", j)}})
				}
				pp.Tasks = append(pp.Tasks, pt)
				pp.BuildVariants[0].Tasks = append(pp.BuildVariants[0].Tasks, parserBVTaskUnit{Name: name})
				roots = append(roots, TVPair{Variant: "bv", TaskName: name})
			}
			p, err := TranslateProject(b.Context(), pp)
			require.NoError(b, err)
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				activationInfo := newSpecificActivationInfo()
				activationInfo.activationVariants = []string{"bv"}
				pairs, err := IncludeDependenciesWithGenerated(p, roots, evergreen.RepotrackerVersionRequester, "", &activationInfo, pp.BuildVariants)
				if err != nil || len(pairs) != taskCount {
					b.Fatalf("expected %d tasks, got %d: %v", taskCount, len(pairs), err)
				}
			}
		})
	}
}
