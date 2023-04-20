package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/utility"
	. "github.com/smartystreets/goconvey/convey"
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
			bytes, err := mgobson.Marshal(map[string]interface{}{
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
		p, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		Convey("a patch against v1/t1 should remain unchanged", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{"v1", "t1"}}, evergreen.PatchVersionRequester, nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 1)
			So(pairs[0], ShouldResemble, TVPair{"v1", "t1"})
		})

		Convey("a patch against v1/t2 should add t1", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{"v1", "t2"}}, evergreen.PatchVersionRequester, nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 2)
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
		})

		Convey("a patch against v2/t3 should add t1,t2, and v1", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{"v2", "t3"}}, evergreen.PatchVersionRequester, nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 3)
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v2", "t3"})
		})

		Convey("a patch against v2/t5 should be pruned, since its dependency is not patchable", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{"v2", "t5"}}, evergreen.PatchVersionRequester, nil)
			So(err, ShouldNotBeNil)
			So(len(pairs), ShouldEqual, 0)

			pairs, err = IncludeDependencies(p, []TVPair{{"v2", "t5"}}, evergreen.RepotrackerVersionRequester, nil)
			So(err, ShouldBeNil)
			So(len(pairs), ShouldEqual, 2)
		})

		Convey("a patch that has a dependency on a task group task with all variants should include that one task", func() {
			pairs, err := IncludeDependencies(p, []TVPair{{"v2", "t6"}}, evergreen.PatchVersionRequester, nil)
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
			pairs, err := IncludeDependencies(p, []TVPair{{"v2", "t7"}}, evergreen.PatchVersionRequester, nil)
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
		p, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		Convey("a patch against v1/t3 should include t2 and t1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{"v1", "t3"}}, evergreen.PatchVersionRequester, nil)
			So(len(pairs), ShouldEqual, 3)
			So(pairs, shouldContainPair, TVPair{"v1", "t2"})
			So(pairs, shouldContainPair, TVPair{"v1", "t1"})
			So(pairs, shouldContainPair, TVPair{"v1", "t3"})
		})

		Convey("a patch against v3/t4 should include v1, v2, t3, t2, and t1", func() {
			pairs, _ := IncludeDependencies(p, []TVPair{{"v3", "t4"}}, evergreen.PatchVersionRequester, nil)
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
			pairs, _ := IncludeDependencies(p, []TVPair{{"v4", "t5"}}, evergreen.PatchVersionRequester, nil)
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

		p, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		Convey("all tasks should be scheduled no matter which is initially added", func() {
			Convey("for '1'", func() {
				pairs, _ := IncludeDependencies(p, []TVPair{{"v1", "1"}}, evergreen.PatchVersionRequester, nil)
				So(len(pairs), ShouldEqual, 3)
				So(pairs, shouldContainPair, TVPair{"v1", "1"})
				So(pairs, shouldContainPair, TVPair{"v1", "2"})
				So(pairs, shouldContainPair, TVPair{"v1", "3"})
			})
			Convey("for '2'", func() {
				pairs, _ := IncludeDependencies(p, []TVPair{{"v1", "2"}, {"v2", "2"}}, evergreen.PatchVersionRequester, nil)
				So(len(pairs), ShouldEqual, 6)
				So(pairs, shouldContainPair, TVPair{"v1", "1"})
				So(pairs, shouldContainPair, TVPair{"v1", "2"})
				So(pairs, shouldContainPair, TVPair{"v1", "3"})
				So(pairs, shouldContainPair, TVPair{"v2", "1"})
				So(pairs, shouldContainPair, TVPair{"v2", "2"})
				So(pairs, shouldContainPair, TVPair{"v2", "3"})
			})
			Convey("for '3'", func() {
				pairs, _ := IncludeDependencies(p, []TVPair{{"v2", "3"}}, evergreen.PatchVersionRequester, nil)
				So(len(pairs), ShouldEqual, 3)
				So(pairs, shouldContainPair, TVPair{"v2", "1"})
				So(pairs, shouldContainPair, TVPair{"v2", "2"})
				So(pairs, shouldContainPair, TVPair{"v2", "3"})
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
		p, err := TranslateProject(parserProject)
		So(err, ShouldBeNil)
		So(p, ShouldNotBeNil)

		initDep := TVPair{TaskName: "a", Variant: "initial-variant"}
		pairs, _ := IncludeDependencies(p, []TVPair{initDep}, evergreen.PatchVersionRequester, nil)
		So(pairs, ShouldHaveLength, 2)
		So(initDep, ShouldBeIn, pairs)
	})
}
