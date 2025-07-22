package reliability

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func setupEnv(ctx context.Context) (*mock.Environment, error) {
	env := &mock.Environment{}

	if err := env.Configure(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	return env, nil
}

func withSetupAndTeardown(t *testing.T, fn func()) {
	require.NoError(t, db.ClearCollections(taskstats.DailyTaskStatsCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(taskstats.DailyTaskStatsCollection))
	}()

	fn()
}

func TestPipeline(t *testing.T) {
	after := time.Date(2018, 8, 12, 0, 0, 0, 0, time.UTC)
	before := time.Date(2018, 8, 13, 0, 0, 0, 0, time.UTC)
	requesters := []string{
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	task := "lint"
	variant := "enterprise-rhel-62-64-bit"
	distro := "rhel62-small"

	simpleGroup := bson.M{
		"$group": bson.M{
			"_id":                    bson.M{"date": "$_id.date"},
			"num_failed":             bson.M{"$sum": "$num_failed"},
			"num_setup_failed":       bson.M{"$sum": "$num_setup_failed"},
			"num_success":            bson.M{"$sum": "$num_success"},
			"num_system_failed":      bson.M{"$sum": "$num_system_failed"},
			"num_test_failed":        bson.M{"$sum": "$num_test_failed"},
			"num_timeout":            bson.M{"$sum": "$num_timeout"},
			"total_duration_success": bson.M{"$sum": bson.M{"$multiply": taskstats.Array{"$num_success", "$avg_duration_success"}}},
		},
	}

	withCancelledContext := func(ctx context.Context, fn func(context.Context)) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		fn(ctx)
	}

	for opName, opTests := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment){
		// Test DateBoundary creation for the task reliability pipeline.
		"DateBoundaries": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter){
				"Simple": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []time.Time{before.Add(24 * time.Hour), before, after}
						boundaries := filter.dateBoundaries()
						assert.Equal(t, len(expected), len(boundaries))
						assert.Equal(t, expected, boundaries)
					})
				},
				"Before": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.BeforeDate = before.Add(24 * time.Hour)
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []time.Time{before.Add(2 * 24 * time.Hour), before.Add(24 * time.Hour), before, after}
						boundaries := filter.dateBoundaries()
						assert.Equal(t, len(expected), len(boundaries))
						assert.Equal(t, expected, boundaries)
					})
				},
				"BeforeAndAfter": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.BeforeDate = before.Add(24 * time.Hour)
					filter.StatsFilter.AfterDate = after.Add(-24 * time.Hour)
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []time.Time{before.Add(2 * 24 * time.Hour), before.Add(24 * time.Hour), before, after, after.Add(-24 * time.Hour)}
						boundaries := filter.dateBoundaries()
						assert.Equal(t, len(expected), len(boundaries))
						assert.Equal(t, expected, boundaries)
					})
				},
				"After": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.AfterDate = after.Add(-24 * time.Hour)
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []time.Time{before.Add(24 * time.Hour), before, after, after.Add(-24 * time.Hour)}
						boundaries := filter.dateBoundaries()
						assert.Equal(t, len(expected), len(boundaries))
						assert.Equal(t, expected, boundaries)
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, func() {
						filter := TaskReliabilityFilter{
							StatsFilter: taskstats.StatsFilter{
								Project:    project,
								Requesters: requesters,
								AfterDate:  after,
								BeforeDate: before,
							},
						}
						testCase(ctx, t, &filter)
					})
				})
			}
		},
		// Test BuildTaskPaginationOrBranches creation for the task reliability pipeline.
		"BuildTaskPaginationOrBranches": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			after = time.Date(2018, 7, 12, 0, 0, 0, 0, time.UTC)
			before = time.Date(2018, 9, 13, 0, 0, 0, 0, time.UTC)

			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter){
				"Sort Latest, GroupBy task, Num Days 1": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []bson.M{
							{"_id.date": bson.M{"$lt": filter.StatsFilter.StartAt.Date}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.task_name": bson.M{"$gt": task}},
						}
						branches := filter.buildTaskPaginationOrBranches()
						assert.Equal(t, expected, branches)
					})
				},
				"Sort Earliest, GroupBy task, Num Days 1": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.Sort = taskstats.SortEarliestFirst
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []bson.M{
							{"_id.date": bson.M{"$gt": filter.StatsFilter.StartAt.Date}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.task_name": bson.M{"$gt": task}},
						}
						branches := filter.buildTaskPaginationOrBranches()
						assert.Equal(t, expected, branches)
					})
				},
				"Sort Latest, GroupBy task, Num Days 28": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					groupNumDays := 28
					startAtDate := after.Add(28 * 24 * time.Hour)
					filter.StatsFilter.GroupNumDays = groupNumDays
					filter.StatsFilter.StartAt.Date = startAtDate

					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []bson.M{
							{"_id.date": bson.M{"$lte": startAtDate}},
							{"_id.date": bson.M{"$lte": startAtDate, "$gt": startAtDate}, "_id.task_name": bson.M{"$gt": task}},
						}
						branches := filter.buildTaskPaginationOrBranches()
						assert.Equal(t, expected, branches)
					})
				},
				// Variant
				"Sort Latest, GroupBy variant, Num Days 1": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.GroupBy = taskstats.GroupByVariant
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []bson.M{
							{"_id.date": bson.M{"$lt": filter.StatsFilter.StartAt.Date}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": bson.M{"$gt": variant}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": variant, "_id.task_name": bson.M{"$gt": task}},
						}
						branches := filter.buildTaskPaginationOrBranches()
						assert.Equal(t, expected, branches)
					})
				},
				"Sort Earliest, GroupBy variant, Num Days 1": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.Sort = taskstats.SortEarliestFirst
					filter.StatsFilter.GroupBy = taskstats.GroupByVariant
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []bson.M{
							{"_id.date": bson.M{"$gt": filter.StatsFilter.StartAt.Date}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": bson.M{"$gt": variant}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": variant, "_id.task_name": bson.M{"$gt": task}},
						}
						branches := filter.buildTaskPaginationOrBranches()
						assert.Equal(t, expected, branches)
					})
				},
				"Sort Latest, GroupBy variant, Num Days 28": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					groupNumDays := 28
					startAtDate := after.Add(28 * 24 * time.Hour)
					filter.StatsFilter.GroupNumDays = groupNumDays
					filter.StatsFilter.StartAt.Date = startAtDate
					filter.StatsFilter.GroupBy = taskstats.GroupByVariant

					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []bson.M{
							{"_id.date": bson.M{"$lte": filter.StatsFilter.StartAt.Date}},
							{"_id.date": bson.M{"$lte": filter.StatsFilter.StartAt.Date, "$gt": filter.StatsFilter.StartAt.Date}, "_id.variant": bson.M{"$gt": variant}},
							{"_id.date": bson.M{"$lte": filter.StatsFilter.StartAt.Date, "$gt": filter.StatsFilter.StartAt.Date}, "_id.variant": variant, "_id.task_name": bson.M{"$gt": task}},
						}
						branches := filter.buildTaskPaginationOrBranches()
						assert.Equal(t, expected, branches)
					})
				},
				// Distro
				"Sort Latest, GroupBy distro, Num Days 1": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.GroupBy = taskstats.GroupByDistro
					filter.StatsFilter.StartAt.Date = after.Add(24 * time.Hour)

					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []bson.M{
							{"_id.date": bson.M{"$lt": filter.StatsFilter.StartAt.Date}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": bson.M{"$gt": variant}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": variant, "_id.task_name": bson.M{"$gt": task}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": variant, "_id.task_name": task, "_id.distro": bson.M{"$gt": distro}},
						}
						branches := filter.buildTaskPaginationOrBranches()
						assert.Equal(t, expected, branches)
					})
				},
				"Sort Earliest, GroupBy distro, Num Days 1": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.GroupBy = taskstats.GroupByDistro
					filter.StatsFilter.Sort = taskstats.SortEarliestFirst
					filter.StatsFilter.StartAt.Date = after.Add(24 * time.Hour)
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []bson.M{
							{"_id.date": bson.M{"$gt": filter.StatsFilter.StartAt.Date}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": bson.M{"$gt": variant}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": variant, "_id.task_name": bson.M{"$gt": task}},
							{"_id.date": filter.StatsFilter.StartAt.Date, "_id.variant": variant, "_id.task_name": task, "_id.distro": bson.M{"$gt": distro}},
						}
						branches := filter.buildTaskPaginationOrBranches()
						assert.Equal(t, expected, branches)
					})
				},
				"Sort Latest, GroupBy distro, Num Days 28": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.GroupBy = taskstats.GroupByDistro
					groupNumDays := 28
					startAtDate := after.Add(28 * 24 * time.Hour)
					filter.StatsFilter.GroupNumDays = groupNumDays
					filter.StatsFilter.StartAt.Date = startAtDate
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []bson.M{
							{"_id.date": bson.M{"$lte": filter.StatsFilter.StartAt.Date}},
							{"_id.date": bson.M{"$lte": filter.StatsFilter.StartAt.Date, "$gt": filter.StatsFilter.StartAt.Date}, "_id.variant": bson.M{"$gt": variant}},
							{"_id.date": bson.M{"$lte": filter.StatsFilter.StartAt.Date, "$gt": filter.StatsFilter.StartAt.Date}, "_id.variant": variant, "_id.task_name": bson.M{"$gt": task}},
							{"_id.date": bson.M{"$lte": filter.StatsFilter.StartAt.Date, "$gt": filter.StatsFilter.StartAt.Date}, "_id.variant": variant, "_id.task_name": task, "_id.distro": bson.M{"$gt": distro}},
						}
						branches := filter.buildTaskPaginationOrBranches()
						assert.Equal(t, expected, branches)
					})
				},
				"Before": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					after = time.Date(2018, 9, 12, 0, 0, 0, 0, time.UTC)
					before = time.Date(2018, 9, 13, 0, 0, 0, 0, time.UTC)
					filter.StatsFilter.AfterDate = after
					filter.StatsFilter.BeforeDate = before.Add(24 * time.Hour)
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []time.Time{before.Add(2 * 24 * time.Hour), before.Add(24 * time.Hour), before, after}
						boundaries := filter.dateBoundaries()
						assert.Equal(t, len(expected), len(boundaries))
						assert.Equal(t, expected, boundaries)
					})
				},
				"BeforeAndAfter": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					before = after.Add(24 * time.Hour)

					filter.StatsFilter.BeforeDate = before.Add(24 * time.Hour)
					filter.StatsFilter.AfterDate = after.Add(-24 * time.Hour)
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []time.Time{before.Add(2 * 24 * time.Hour), before.Add(24 * time.Hour), before, after, after.Add(-24 * time.Hour)}
						boundaries := filter.dateBoundaries()
						assert.Equal(t, len(expected), len(boundaries))
						assert.Equal(t, expected, boundaries)
					})
				},
				"After": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					before = after.Add(24 * time.Hour)

					filter.StatsFilter.AfterDate = after.Add(-24 * time.Hour)
					filter.StatsFilter.BeforeDate = before
					withCancelledContext(ctx, func(ctx context.Context) {
						expected := []time.Time{before.Add(24 * time.Hour), before, after, after.Add(-24 * time.Hour)}
						boundaries := filter.dateBoundaries()
						assert.Equal(t, len(expected), len(boundaries))
						assert.Equal(t, expected, boundaries)
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, func() {
						filter := TaskReliabilityFilter{
							StatsFilter: taskstats.StatsFilter{
								Project:      project,
								Requesters:   requesters,
								GroupNumDays: 1,
								GroupBy:      taskstats.GroupByTask,
								Sort:         taskstats.SortLatestFirst,
								AfterDate:    after,
								BeforeDate:   before,
								StartAt: &taskstats.StartAt{
									Date:         after.Add(24 * time.Hour),
									Task:         task,
									BuildVariant: variant,
									Distro:       distro,
								},
							},
						}
						testCase(ctx, t, &filter)
					})
				})
			}
		},
		// Test BuildMatchStageForTask.
		"BuildMatchStageForTask": func(ctx context.Context, t *testing.T, env evergreen.Environment) {

			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter, expected bson.M){
				"Simple": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter, expected bson.M) {
					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.buildMatchStageForTask()
						assert.Equal(t, expected, stage)
					})
				},
				"Task": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter, expected bson.M) {
					filter.Tasks = []string{task}
					expected["$match"].(bson.M)["_id.task_name"] = filter.Tasks[0]
					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.buildMatchStageForTask()
						assert.Equal(t, expected, stage)
					})
				},
				"Tasks": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter, expected bson.M) {
					filter.Tasks = []string{task, "compile"}
					expected["$match"].(bson.M)["_id.task_name"] = bson.M{"$in": filter.Tasks}
					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.buildMatchStageForTask()
						assert.Equal(t, expected, stage)
					})
				},
				"Variant": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter, expected bson.M) {
					filter.BuildVariants = []string{variant}
					expected["$match"].(bson.M)["_id.variant"] = filter.BuildVariants[0]
					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.buildMatchStageForTask()
						assert.Equal(t, expected, stage)
					})
				},
				"Variants": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter, expected bson.M) {
					filter.BuildVariants = []string{variant, "rhel76-build"}
					expected["$match"].(bson.M)["_id.variant"] = bson.M{"$in": filter.BuildVariants}
					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.buildMatchStageForTask()
						assert.Equal(t, expected, stage)
					})
				},
				"Distro": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter, expected bson.M) {
					filter.Distros = []string{distro}
					expected["$match"].(bson.M)["_id.distro"] = filter.Distros[0]
					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.buildMatchStageForTask()
						assert.Equal(t, expected, stage)
					})
				},
				"Distros": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter, expected bson.M) {
					filter.Distros = []string{distro, "debian71-build"}
					expected["$match"].(bson.M)["_id.distro"] = bson.M{"$in": filter.Distros}
					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.buildMatchStageForTask()
						assert.Equal(t, expected, stage)
					})
				},
				"Pagination": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter, expected bson.M) {
					startAt := after.Add(24 * time.Hour)
					filter.StatsFilter.StartAt = &taskstats.StartAt{
						Date:         startAt,
						Task:         task,
						BuildVariant: variant,
						Distro:       distro,
					}
					expected["$match"].(bson.M)["$or"] = []bson.M{
						{"_id.date": bson.M{"$gt": startAt}},
						{"_id.date": startAt, "_id.variant": bson.M{"$gt": variant}},
						{"_id.date": startAt, "_id.variant": variant, "_id.task_name": bson.M{"$gt": task}},
						{"_id.date": startAt, "_id.variant": variant, "_id.task_name": task, "_id.distro": bson.M{"$gt": distro}},
					}
					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.buildMatchStageForTask()
						assert.Equal(t, expected, stage)
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, func() {
						filter := TaskReliabilityFilter{
							StatsFilter: taskstats.StatsFilter{
								Project:    project,
								Requesters: requesters,
								AfterDate:  after,
								BeforeDate: before,
								GroupBy:    taskstats.GroupByDistro,
							},
						}
						match := bson.M{
							"$match": bson.M{
								"_id.project":   project,
								"_id.date":      bson.M{"$gte": after, "$lt": before.Add(24 * time.Hour)},
								"_id.requester": bson.M{"$in": requesters},
							},
						}
						testCase(ctx, t, &filter, match)
					})
				})
			}
		},
		"BuildTaskStatsQueryGroupStage": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter){
				"Simple": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {

					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.BuildTaskStatsQueryGroupStage()
						assert.Equal(t, stage, simpleGroup)
					})
				},
				"WithAfterBefore": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.AfterDate = after
						filter.StatsFilter.BeforeDate = before
						filter.StatsFilter.GroupNumDays = 2
						stage := filter.BuildTaskStatsQueryGroupStage()
						assert.Equal(t, bson.M{
							"$group": bson.M{
								"_id": bson.M{
									"date": bson.M{
										"$switch": bson.M{
											"branches": []bson.M{
												{
													"case": bson.M{
														"$and": taskstats.Array{
															bson.M{
																"$lt": taskstats.Array{
																	"$_id.date",
																	before.Add(24 * time.Hour),
																},
															},
															bson.M{
																"$gte": taskstats.Array{
																	"$_id.date",
																	after,
																},
															},
														},
													},
													"then": after,
												},
											},
										},
									},
								},
								"num_failed":             bson.M{"$sum": "$num_failed"},
								"num_setup_failed":       bson.M{"$sum": "$num_setup_failed"},
								"num_success":            bson.M{"$sum": "$num_success"},
								"num_system_failed":      bson.M{"$sum": "$num_system_failed"},
								"num_test_failed":        bson.M{"$sum": "$num_test_failed"},
								"num_timeout":            bson.M{"$sum": "$num_timeout"},
								"total_duration_success": bson.M{"$sum": bson.M{"$multiply": taskstats.Array{"$num_success", "$avg_duration_success"}}},
							},
						}, stage)
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, func() {
						filter := TaskReliabilityFilter{
							StatsFilter: taskstats.StatsFilter{
								Project:    project,
								Requesters: requesters},
						}
						testCase(ctx, t, &filter)
					})
				})
			}
		},
		"TaskReliabilityQueryPipeline": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			match := bson.M{
				"$match": bson.M{
					"_id.project":   project,
					"_id.date":      bson.M{"$gte": after, "$lt": before.Add(24 * time.Hour)},
					"_id.requester": bson.M{"$in": requesters},
				},
			}
			projection := bson.M{
				"$project": bson.M{
					"avg_duration_success": bson.M{
						"$cond": bson.M{
							"else": nil,
							"if": bson.M{
								"$ne": taskstats.Array{
									"$num_success",
									0,
								},
							},
							"then": bson.M{
								"$divide": taskstats.Array{
									"$total_duration_success",
									"$num_success",
								},
							},
						},
					},
					"date":              "$_id.date",
					"distro":            "$_id.distro",
					"num_failed":        1,
					"num_setup_failed":  1,
					"num_success":       1,
					"num_system_failed": 1,
					"num_test_failed":   1,
					"num_timeout":       1,
					"num_total":         bson.M{"$add": taskstats.Array{"$num_success", "$num_failed"}},
					"task_name":         "$_id.task_name",
					"variant":           "$_id.variant",
				},
			}
			sort := bson.M{
				"$sort": bson.D{
					{Key: "date", Value: 1},
					{Key: "variant", Value: 1},
					{Key: "task_name", Value: 1},
					{Key: "distro", Value: 1},
				},
			}
			limit := bson.M{"$limit": 1}
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter){
				"Simple": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.AfterDate = after
					filter.StatsFilter.BeforeDate = before
					filter.StatsFilter.GroupNumDays = 1
					filter.StatsFilter.Limit = 1

					withCancelledContext(ctx, func(ctx context.Context) {
						pipeline := filter.taskReliabilityQueryPipeline()
						assert.Equal(t, []bson.M{
							match,
							simpleGroup,
							projection,
							sort,
							limit,
						}, pipeline)
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, func() {
						filter := TaskReliabilityFilter{
							StatsFilter: taskstats.StatsFilter{
								Project:    project,
								Requesters: requesters},
						}
						testCase(ctx, t, &filter)
					})
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			env, err := setupEnv(ctx)
			require.NoError(t, err)

			tctx, cancel := context.WithTimeout(ctx, 5*time.Second)
			defer cancel()

			env.Settings().DomainName = "test"
			opTests(tctx, t, env)
		})
	}
}
