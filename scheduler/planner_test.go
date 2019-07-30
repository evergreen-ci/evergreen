package scheduler

import (
	"fmt"
	"testing"

	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPlanner(t *testing.T) {
	t.Run("Caches", func(t *testing.T) {
		t.Run("StringSet", func(t *testing.T) {
			t.Run("ZeroValue", func(t *testing.T) {
				assert.Len(t, StringSet{}, 0)
				assert.NotNil(t, StringSet{})
			})
			t.Run("CheckNonExistant", func(t *testing.T) {
				set := StringSet{}
				assert.False(t, set.Check("foo"))
			})
			t.Run("CheckExisting", func(t *testing.T) {
				set := StringSet{}
				set.Add("foo")
				assert.Len(t, set, 1)
				assert.True(t, set.Check("foo"))
			})
			t.Run("VisitNewKey", func(t *testing.T) {
				set := StringSet{}
				assert.False(t, set.Check("foo"))
				assert.False(t, set.Visit("foo"))
				assert.True(t, set.Check("foo"))
			})
			t.Run("VistExistingKey", func(t *testing.T) {
				set := StringSet{}
				assert.Len(t, set, 0)
				set.Add("foo")
				assert.Len(t, set, 1)
				assert.True(t, set.Visit("foo"))
				assert.Len(t, set, 1)
			})
		})
		t.Run("UnitCache", func(t *testing.T) {
			t.Run("Zero", func(t *testing.T) {
				cache := UnitCache{}
				assert.Len(t, cache, 0)
			})
			t.Run("AddWhenNoops", func(t *testing.T) {
				cache := UnitCache{}
				cache.AddWhen(false, "foo", &Unit{})
				assert.Len(t, cache, 0)
			})
			t.Run("AddWhenAddsNew", func(t *testing.T) {
				cache := UnitCache{}
				cache.AddWhen(true, "foo", &Unit{})
				assert.Len(t, cache, 1)
			})
			t.Run("AddWhenWithExisting", func(t *testing.T) {
				cache := UnitCache{}
				assert.Len(t, cache, 0)
				cache.AddWhen(true, "foo", &Unit{})
				assert.Len(t, cache, 1)
				cache.AddWhen(true, "foo", &Unit{})
				cache.AddWhen(true, "foo", &Unit{})
				cache.AddWhen(true, "foo", &Unit{})
				cache.AddWhen(true, "foo", &Unit{})
				assert.Len(t, cache, 1)
			})
			t.Run("AddNewMergesOntoExisting", func(t *testing.T) {
				cache := UnitCache{}
				unit := NewUnit(task.Task{Id: "foo"})
				assert.Len(t, unit.tasks, 1)
				cache.AddNew("foo", unit)

				second := NewUnit(task.Task{Id: "bar"})
				cache.AddNew("foo", second)
				assert.Len(t, unit.tasks, 2)
				assert.Len(t, second.tasks, 1)
			})
			t.Run("AddNewWithExisting", func(t *testing.T) {
				cache := UnitCache{}
				assert.Len(t, cache, 0)
				cache.AddNew("foo", &Unit{})
				assert.Len(t, cache, 1)
				cache.AddNew("foo", &Unit{})
				cache.AddNew("foo", &Unit{})
				cache.AddNew("foo", &Unit{})
				cache.AddNew("foo", &Unit{})
				assert.Len(t, cache, 1)
			})
			t.Run("CreateNew", func(t *testing.T) {
				cache := UnitCache{}
				unit := cache.Create("foo", task.Task{Id: "foo"})
				assert.NotNil(t, unit)
				assert.Equal(t, unit, cache["foo"])
				assert.Len(t, cache, 1)
			})
			t.Run("CreateTwice", func(t *testing.T) {
				cache := UnitCache{}
				first := cache.Create("foo", task.Task{Id: "foo"})
				second := cache.Create("foo", task.Task{Id: "foo"})
				assert.Exactly(t, first, second)
				assert.Equal(t, fmt.Sprint(first), fmt.Sprint(second))
			})
			t.Run("ExportPropogatesTasks", func(t *testing.T) {
				cache := UnitCache{}
				one := task.Task{Id: "one"}
				two := task.Task{Id: "two"}
				cache.Create("one", one)
				cache.Create("two", two)
				plan := cache.Export()
				assert.Len(t, plan, 2)
				for _, ts := range plan {
					ts.distro = &distro.Distro{} // keep the test from panicing...
					require.Len(t, ts.tasks, 1)
				}
				for _, ts := range plan.Export() {
					require.True(t, ts.Id == "one" || ts.Id == "two")
				}
			})
			t.Run("ExportDeduplicatesMatchingUnitNames", func(t *testing.T) {
				cache := UnitCache{}
				one := task.Task{Id: "one"}
				cache.Create("one", one)
				cache.Create("two", one)
				plan := cache.Export()
				assert.Len(t, plan, 1)
			})
		})
		t.Run("Unit", func(t *testing.T) {
			t.Run("Constructor", func(t *testing.T) {

			})
			t.Run("AddOverwrites", func(t *testing.T) {

			})
			t.Run("ExportIsSimple", func(t *testing.T) {

			})
			t.Run("HashCaches", func(t *testing.T) {

			})
			t.Run("HashIgnoresOrder", func(t *testing.T) {

			})
			t.Run("RankCachesValue", func(t *testing.T) {
				unit := NewUnit(task.Task{Id: "foo", Priority: 100})
				unit.distro = &distro.Distro{}

				// default priority is 1
				// task has extra priority of 100 (so 101)
				// expected time is 10m
				// multiply expected time times
				// priority,
				// expected to be 1112
				assert.EqualValues(t, 1112, unit.RankValue())
				assert.EqualValues(t, 1112, unit.RankValue())
			})
			t.Run("RankForCommitQueue", func(t *testing.T) {

			})
			t.Run("RankForPatches", func(t *testing.T) {

			})

			// TODO:
			//   - write table tests for rank value
			//   - write table tests for hashing
		})
	})
	t.Run("GenerateTaskPlan", func(t *testing.T) {
		t.Run("ExportDeduplicates", func(t *testing.T) {

		})
		t.Run("VersionsGrouped", func(t *testing.T) {

		})
		t.Run("DependenciesGrouped", func(t *testing.T) {

		})
		t.Run("TaskGroupsGrouped", func(t *testing.T) {

		})
	})
	t.Run("Sorting", func(t *testing.T) {
		t.Run("UnitInternal", func(t *testing.T) {

		})
		t.Run("UnitExternal", func(t *testing.T) {
		})
	})
	t.Run("Integration", func(t *testing.T) {
		// build a list of tasks and a distro and pass them
		// through
		assert.Len(t, PrepareTasksForPlanning(&distro.Distro{}, []task.Task{}), 0)
	})

}
