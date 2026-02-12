package scheduler

import (
	"context"
	"fmt"
	"sort"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestPlanner(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	_, err := evergreen.GetEnvironment().DB().Collection(task.Collection).Indexes().CreateOne(ctx, mongo.IndexModel{Keys: task.TaskHistoricalDataIndex})
	assert.NoError(t, err)

	t.Run("Caches", func(t *testing.T) {
		t.Run("StringSet", func(t *testing.T) {
			t.Run("ZeroValue", func(t *testing.T) {
				assert.Empty(t, StringSet{})
				assert.NotNil(t, StringSet{})
			})
			t.Run("CheckNonExistent", func(t *testing.T) {
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
				assert.Empty(t, set)
				set.Add("foo")
				assert.Len(t, set, 1)
				assert.True(t, set.Visit("foo"))
				assert.Len(t, set, 1)
			})
		})
		t.Run("UnitCache", func(t *testing.T) {
			t.Run("Zero", func(t *testing.T) {
				cache := UnitCache{}
				assert.Empty(t, cache)
			})
			t.Run("AddWhenNoops", func(t *testing.T) {
				cache := UnitCache{}
				cache.AddWhen(false, "foo", task.Task{})
				assert.Empty(t, cache)
			})
			t.Run("AddWhenAddsNew", func(t *testing.T) {
				cache := UnitCache{}
				cache.AddWhen(true, "foo", task.Task{})
				assert.Len(t, cache, 1)
			})
			t.Run("AddWhenWithExisting", func(t *testing.T) {
				cache := UnitCache{}
				assert.Empty(t, cache)
				cache.AddWhen(true, "foo", task.Task{})
				assert.Len(t, cache, 1)
				cache.AddWhen(true, "foo", task.Task{})
				cache.AddWhen(true, "foo", task.Task{})
				cache.AddWhen(true, "foo", task.Task{})
				cache.AddWhen(true, "foo", task.Task{})
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
				assert.Empty(t, cache)
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
			t.Run("ExportSkipsMissingDistroTasks", func(t *testing.T) {
				cache := UnitCache{}
				one := task.Task{Id: "one"}
				cache.Create("one", one)
				assert.Zero(t, cache.Export(ctx).Len())
			})
			t.Run("ExportPropogatesTasks", func(t *testing.T) {
				cache := UnitCache{}
				one := task.Task{Id: "one"}
				two := task.Task{Id: "two"}
				cache.Create("one", one).SetDistro(&distro.Distro{})
				cache.Create("two", two).SetDistro(&distro.Distro{})
				plan := cache.Export(ctx)
				assert.Equal(t, plan.Len(), 2)
				for _, ts := range plan.units {
					ts.SetDistro(&distro.Distro{})
					require.Len(t, ts.tasks, 1)
				}
				for _, ts := range plan.Export(ctx) {
					require.True(t, ts.Id == "one" || ts.Id == "two")
				}
			})
			t.Run("ExportDeduplicatesMatchingUnitNames", func(t *testing.T) {
				cache := UnitCache{}
				one := task.Task{Id: "one"}
				cache.Create("one", one).SetDistro(&distro.Distro{})
				cache.Create("two", one).SetDistro(&distro.Distro{})
				plan := cache.Export(ctx)
				assert.Equal(t, plan.Len(), 1)
			})
		})
		t.Run("Unit", func(t *testing.T) {
			t.Run("NewConstructor", func(t *testing.T) {
				unit := NewUnit(task.Task{})
				assert.NotNil(t, unit.tasks)
				assert.Nil(t, unit.distro)
			})
			t.Run("MakeConstructor", func(t *testing.T) {
				unit := MakeUnit(&distro.Distro{})
				assert.NotNil(t, unit.tasks)
				assert.NotNil(t, unit.distro)
			})
			t.Run("SetDistro", func(t *testing.T) {
				unit := MakeUnit(&distro.Distro{})
				assert.NotNil(t, unit.distro)
				unit.SetDistro(nil)
				assert.NotNil(t, unit.distro)
			})
			t.Run("AddOverwrites", func(t *testing.T) {
				unit := NewUnit(task.Task{Id: "foo", Priority: 100})
				assert.EqualValues(t, 100, unit.tasks["foo"].Priority)
				unit.Add(task.Task{Id: "foo", Priority: 200})
				assert.EqualValues(t, 200, unit.tasks["foo"].Priority)
			})
			t.Run("HashCaches", func(t *testing.T) {
				unit := NewUnit(task.Task{Id: "foo"})
				hash := unit.ID()
				assert.Equal(t, unit.Export(ctx).Len(), 1)
				unit.Add(task.Task{Id: "bar"})
				assert.Equal(t, unit.Export(ctx).Len(), 2)
				newHash := unit.ID()
				assert.Equal(t, hash, newHash)
			})
			t.Run("HashIgnoresOrder", func(t *testing.T) {
				tasks := map[int]task.Task{
					1: {Id: "one"},
					2: {Id: "two"},
					3: {Id: "three"},
				}

				unitOne := NewUnit(task.Task{Id: "four"})
				for _, ts := range tasks {
					unitOne.Add(ts)
				}
				unitTwo := NewUnit(task.Task{Id: "four"})
				for _, ts := range tasks {
					unitTwo.Add(ts)
				}

				assert.Equal(t, unitOne.ID(), unitTwo.ID())
			})
			t.Run("RankExpectedValues", func(t *testing.T) {
				t.Run("SingleTask", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo"})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 180, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("MultipleTasks", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo"})
					unit.SetDistro(&distro.Distro{})
					unit.Add(task.Task{Id: "bar"})
					assert.EqualValues(t, 181, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("MergeQueue", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", Requester: evergreen.GithubMergeRequester})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 2413, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("Patches", func(t *testing.T) {
					t.Run("CLI", func(t *testing.T) {
						unit := NewUnit(task.Task{Id: "foo", Requester: evergreen.PatchVersionRequester})
						unit.SetDistro(&distro.Distro{})
						unit.distro.PlannerSettings.PatchFactor = 10
						assert.EqualValues(t, 22, unit.sortingValueBreakdown(ctx).TotalValue)
						verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
					})
					t.Run("Github", func(t *testing.T) {
						unit := NewUnit(task.Task{Id: "foo", Requester: evergreen.GithubPRRequester})
						unit.SetDistro(&distro.Distro{})
						unit.distro.PlannerSettings.PatchFactor = 10
						assert.EqualValues(t, 22, unit.sortingValueBreakdown(ctx).TotalValue)
						verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
					})
				})
				t.Run("Priority", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", Priority: 10})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 1970, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("TimeInQueuePatch", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", Requester: evergreen.PatchVersionRequester, ActivatedTime: time.Now().Add(-time.Hour)})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 73, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("TimeInQueueMainline", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", Requester: evergreen.RepotrackerVersionRequester, ActivatedTime: time.Now().Add(-time.Hour)})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 178, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("LifeTimePatch", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", Requester: evergreen.PatchVersionRequester, IngestTime: time.Now().Add(-10 * time.Hour)})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 613, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("LifeTimeMainlineNew", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", Requester: evergreen.RepotrackerVersionRequester, IngestTime: time.Now().Add(-10 * time.Minute)})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 179, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("LifeTimeMainlineOld", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", Requester: evergreen.RepotrackerVersionRequester, IngestTime: time.Now().Add(-7 * 24 * time.Hour)})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 12, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("NumDependents", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", NumDependents: 2})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 182, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("NumDependentsWithFactor", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", NumDependents: 2})
					unit.SetDistro(&distro.Distro{})
					unit.distro.PlannerSettings.NumDependentsFactor = 10
					assert.EqualValues(t, 200, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("NumDependentsWithFractionFactor", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", NumDependents: 2})
					unit.SetDistro(&distro.Distro{})
					unit.distro.PlannerSettings.NumDependentsFactor = 0.5
					assert.EqualValues(t, 181, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("GenerateTask", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", GenerateTask: true})
					unit.SetDistro(&distro.Distro{})
					unit.distro.PlannerSettings.GenerateTaskFactor = 10
					assert.EqualValues(t, 1791, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
				t.Run("TaskGroup", func(t *testing.T) {
					unit := NewUnit(task.Task{Id: "foo", TaskGroup: "tg1"})
					unit.Add(task.Task{Id: "bar", TaskGroup: "tg1"})
					unit.Add(task.Task{Id: "baz", TaskGroup: "tg1"})
					unit.SetDistro(&distro.Distro{})
					assert.EqualValues(t, 719, unit.sortingValueBreakdown(ctx).TotalValue)
					verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				})
			})
			t.Run("RankCachesValue", func(t *testing.T) {
				unit := NewUnit(task.Task{Id: "foo", Priority: 100})
				unit.SetDistro(&distro.Distro{})

				assert.EqualValues(t, 18080, unit.sortingValueBreakdown(ctx).TotalValue)
				verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
				unit.Add(task.Task{Id: "bar"})
				assert.EqualValues(t, 18080, unit.sortingValueBreakdown(ctx).TotalValue)
				verifyRankBreakdown(t, unit.sortingValueBreakdown(ctx))
			})
		})
		t.Run("TaskPlan", func(t *testing.T) {
			buildPlan := func(units ...*Unit) TaskPlan {
				d := &distro.Distro{}
				for _, u := range units {
					u.SetDistro(d)
				}
				return TaskPlan{ctx: ctx, units: units}
			}
			t.Run("NoChange", func(t *testing.T) {
				plan := buildPlan(NewUnit(task.Task{Id: "foo"}), NewUnit(task.Task{Id: "bar"}))
				sort.Stable(plan)
				out := plan.Export(ctx)
				assert.Equal(t, "foo", out[0].Id)
				assert.Equal(t, "bar", out[1].Id)
			})
			t.Run("ChangeOrder", func(t *testing.T) {
				plan := buildPlan(NewUnit(task.Task{Id: "foo"}), NewUnit(task.Task{Id: "bar", Priority: 10}))
				sort.Stable(plan)
				out := plan.Export(ctx)
				assert.Equal(t, "bar", out[0].Id)
				assert.Equal(t, "foo", out[1].Id)
			})
			t.Run("Deduplicates", func(t *testing.T) {
				plan := buildPlan(NewUnit(task.Task{Id: "foo"}), NewUnit(task.Task{Id: "foo"}))
				assert.Len(t, plan.Export(ctx), 1)
			})
		})
		t.Run("TaskList", func(t *testing.T) {
			t.Run("NoChange", func(t *testing.T) {
				plan := TaskList{ctx: ctx, tasks: []task.Task{{Id: "second"}, {Id: "first"}}}
				assert.Equal(t, "second", plan.tasks[0].Id)
				assert.Equal(t, "first", plan.tasks[1].Id)
				sort.Sort(plan)
				assert.Equal(t, "second", plan.tasks[0].Id)
				assert.Equal(t, "first", plan.tasks[1].Id)
			})
			t.Run("TaskGroupOrder", func(t *testing.T) {
				plan := TaskList{ctx: ctx, tasks: []task.Task{{Id: "second", TaskGroupOrder: 2}, {Id: "first", TaskGroupOrder: 1}}}
				sort.Sort(plan)
				assert.Equal(t, 1, plan.tasks[0].TaskGroupOrder)
				assert.Equal(t, 2, plan.tasks[1].TaskGroupOrder)

				assert.Equal(t, "first", plan.tasks[0].Id)
				assert.Equal(t, "second", plan.tasks[1].Id)
			})
			t.Run("NumDependents", func(t *testing.T) {
				plan := TaskList{ctx: ctx, tasks: []task.Task{{Id: "second"}, {Id: "first", NumDependents: 2}}}
				sort.Sort(plan)
				assert.Equal(t, "first", plan.tasks[0].Id)
				assert.Equal(t, "second", plan.tasks[1].Id)

			})
			t.Run("Priority", func(t *testing.T) {
				plan := TaskList{ctx: ctx, tasks: []task.Task{{Id: "second"}, {Id: "first", Priority: 100}}}
				sort.Sort(plan)
				assert.Equal(t, "first", plan.tasks[0].Id)
				assert.Equal(t, "second", plan.tasks[1].Id)
			})
			t.Run("ExpectedDuration", func(t *testing.T) {
				plan := TaskList{ctx: ctx, tasks: []task.Task{{Id: "second"}, {Id: "first"}}}
				plan.tasks[1].DurationPrediction.Value = time.Hour
				plan.tasks[1].DurationPrediction.TTL = time.Hour * 24
				plan.tasks[1].DurationPrediction.CollectedAt = time.Now()

				plan.tasks[0].DurationPrediction.Value = time.Minute
				plan.tasks[0].DurationPrediction.TTL = time.Hour * 24
				plan.tasks[0].DurationPrediction.CollectedAt = time.Now()

				sort.Sort(plan)

				assert.Equal(t, "first", plan.tasks[0].Id)
				assert.Equal(t, "second", plan.tasks[1].Id)
			})
		})
	})
	t.Run("PrepareTaskPlan", func(t *testing.T) {
		t.Run("Noop", func(t *testing.T) {
			assert.Zero(t, PrepareTasksForPlanning(ctx, &distro.Distro{}, []task.Task{}).Len())
		})
		t.Run("TaskGroupsGrouped", func(t *testing.T) {
			plan := PrepareTasksForPlanning(ctx, &distro.Distro{}, []task.Task{
				{Id: "one", TaskGroup: "first"},
				{Id: "two", TaskGroup: "first"},
				{Id: "three"},
			})

			assert.Equal(t, plan.Len(), 2)
			assert.Len(t, plan.Export(ctx), 3)
		})
		t.Run("VersionsGrouped", func(t *testing.T) {
			plan := PrepareTasksForPlanning(ctx, &distro.Distro{
				PlannerSettings: distro.PlannerSettings{
					GroupVersions: func() *bool { b := true; return &b }(),
				},
			}, []task.Task{
				{Id: "one", Version: "first"},
				{Id: "two", Version: "first"},
				{Id: "three", Version: "second"},
			})

			assert.Equal(t, plan.Len(), 2)
			assert.Len(t, plan.Export(ctx), 3)
		})
		t.Run("VersionsAndTaskGroupsGrouped", func(t *testing.T) {
			plan := PrepareTasksForPlanning(ctx, &distro.Distro{
				PlannerSettings: distro.PlannerSettings{
					GroupVersions: func() *bool { b := true; return &b }(),
				},
			}, []task.Task{
				{Id: "three", Version: "second"},
				{Id: "four", Version: "second"},
				{Id: "five", Version: "second"},
				{Id: "one", Version: "first", TaskGroup: "one"},
				{Id: "two", Version: "first", TaskGroup: "one"},
				{Id: "extra", Version: "first", Priority: 1},
			})

			assert.Equal(t, plan.Len(), 3)
			tasks := plan.Export(ctx)
			assert.Len(t, tasks, 6)
			assert.Equal(t, "one", tasks[0].TaskGroup)
			assert.Equal(t, "one", tasks[1].TaskGroup)
		})
		t.Run("DependenciesGrouped", func(t *testing.T) {
			plan := PrepareTasksForPlanning(ctx, &distro.Distro{}, []task.Task{
				{Id: "one", DependsOn: []task.Dependency{{TaskId: "two"}}},
				{Id: "three"},
				{Id: "two"},
				{Id: "other", DependsOn: []task.Dependency{{TaskId: "two"}}},
			})

			require.Equal(t, plan.Len(), 4, "keys:%s", plan.Keys())
			tasks := plan.Export(ctx)
			require.Len(t, tasks, 4)
			assert.Equal(t, "three", tasks[3].Id)

			head := []string{tasks[0].Id, tasks[1].Id, tasks[2].Id}
			assert.Contains(t, head, "one")
			assert.Contains(t, head, "two")
			assert.Contains(t, head, "other")

		})
		t.Run("ExternalDependenciesIgnored", func(t *testing.T) {
			plan := PrepareTasksForPlanning(ctx, &distro.Distro{}, []task.Task{
				{Id: "one", DependsOn: []task.Dependency{{TaskId: "missing"}}},
				{Id: "three"},
				{Id: "two", DependsOn: []task.Dependency{{TaskId: "missing"}}},
			})

			assert.Equal(t, plan.Len(), 3)
			assert.Len(t, plan.Export(ctx), 3)
		})
	})
}

func verifyRankBreakdown(t *testing.T, breakdown task.SortingValueBreakdown) {
	totalRankValue := breakdown.RankValueBreakdown.StepbackImpact +
		breakdown.RankValueBreakdown.PatchImpact +
		breakdown.RankValueBreakdown.PatchWaitTimeImpact +
		breakdown.RankValueBreakdown.MainlineWaitTimeImpact +
		breakdown.RankValueBreakdown.EstimatedRuntimeImpact +
		breakdown.RankValueBreakdown.NumDependentsImpact +
		breakdown.RankValueBreakdown.CommitQueueImpact
	totalPriorityValue := breakdown.PriorityBreakdown.InitialPriorityImpact +
		breakdown.PriorityBreakdown.CommitQueueImpact +
		breakdown.PriorityBreakdown.GeneratorTaskImpact +
		breakdown.PriorityBreakdown.TaskGroupImpact
	assert.Equal(t, totalPriorityValue+breakdown.TaskGroupLength+totalRankValue*totalPriorityValue, breakdown.TotalValue)
}
