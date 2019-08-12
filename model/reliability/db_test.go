package reliability

import (
	"context"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func setupEnv(ctx context.Context) (*mock.Environment, error) {
	env := &mock.Environment{}

	if err := env.Configure(ctx, "", nil); err != nil {
		return nil, errors.WithStack(err)
	}
	return env, nil
}

func withSetupAndTeardown(t *testing.T, env evergreen.Environment, fn func()) {
	require.NoError(t, db.ClearCollections(stats.DailyTaskStatsCollection))
	defer func() {
		assert.NoError(t, db.ClearCollections(stats.DailyTaskStatsCollection))
	}()

	fn()
}

func TestPipeline(t *testing.T) {
	after := time.Date(2018, 8, 12, 0, 0, 0, 0, time.UTC)
	before := time.Date(2018, 8, 13, 0, 0, 0, 0, time.UTC)
	requesters := []string{
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
		evergreen.MergeTestRequester,
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	withCancelledContext := func(ctx context.Context, fn func(context.Context)) {
		ctx, cancel := context.WithCancel(ctx)
		cancel()
		fn(ctx)
	}

	for opName, opTests := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment){
		"BuildMatchStageForTask": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter){
				"Simple": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {

					boundaries := []time.Time{before, after}
					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.BuildMatchStageForTask(boundaries)
						assert.Contains(t, stage, "$match")
						match := stage["$match"].(bson.M)
						assert.Equal(t, match["_id.date"], bson.M{
							"$gte": after,
							"$lt":  before,
						})
						assert.Equal(t, match["_id.project"], project)
						assert.Equal(t, match["_id.requester"], bson.M{
							"$in": requesters,
						})
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						filter := TaskReliabilityFilter{
							StatsFilter: stats.StatsFilter{
								Project:    project,
								Requesters: requesters},
						}
						testCase(ctx, t, &filter)
					})
				})
			}
		},
		"BuildTaskStatsQueryGroupStage": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter){
				"Simple": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {

					withCancelledContext(ctx, func(ctx context.Context) {
						stage := filter.BuildTaskStatsQueryGroupStage()
						assert.Equal(t, stage, bson.M{
							"$group": bson.M{
								"_id": bson.M{
									"date": "$_id.date",
								},
								"num_failed": bson.M{
									"$sum": "$num_failed",
								},
								"num_setup_failed": bson.M{
									"$sum": "$num_setup_failed",
								},
								"num_success": bson.M{
									"$sum": "$num_success",
								},
								"num_system_failed": bson.M{
									"$sum": "$num_system_failed",
								},
								"num_test_failed": bson.M{
									"$sum": "$num_test_failed",
								},
								"num_timeout": bson.M{
									"$sum": "$num_timeout",
								},
								"total_duration_success": bson.M{
									"$sum": bson.M{
										"$multiply": stats.Array{
											"$num_success",
											"$avg_duration_success",
										},
									},
								},
							},
						})
					})
				},
				"WithAfterBefore": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					withCancelledContext(ctx, func(ctx context.Context) {
						filter.StatsFilter.AfterDate = after
						filter.StatsFilter.BeforeDate = before
						filter.StatsFilter.GroupNumDays = 2
						stage := filter.BuildTaskStatsQueryGroupStage()
						assert.Equal(t, stage, bson.M{
							"$group": bson.M{
								"_id": bson.M{
									"date": bson.M{
										"$switch": bson.M{
											"branches": []bson.M{
												bson.M{
													"case": bson.M{
														"$and": stats.Array{
															bson.M{
																"$lt": stats.Array{
																	"$_id.date",
																	before.Add(24 * time.Hour),
																},
															},
															bson.M{
																"$gte": stats.Array{
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
								"num_failed": bson.M{
									"$sum": "$num_failed",
								},
								"num_setup_failed": bson.M{
									"$sum": "$num_setup_failed",
								},
								"num_success": bson.M{
									"$sum": "$num_success",
								},
								"num_system_failed": bson.M{
									"$sum": "$num_system_failed",
								},
								"num_test_failed": bson.M{
									"$sum": "$num_test_failed",
								},
								"num_timeout": bson.M{
									"$sum": "$num_timeout",
								},
								"total_duration_success": bson.M{
									"$sum": bson.M{
										"$multiply": stats.Array{
											"$num_success",
											"$avg_duration_success",
										},
									},
								},
							},
						})
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						filter := TaskReliabilityFilter{
							StatsFilter: stats.StatsFilter{
								Project:    project,
								Requesters: requesters},
						}
						testCase(ctx, t, &filter)
					})
				})
			}
		},
		"TaskReliabilityQueryPipeline": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter){
				"Simple": func(ctx context.Context, t *testing.T, filter *TaskReliabilityFilter) {
					filter.StatsFilter.AfterDate = after
					filter.StatsFilter.BeforeDate = before
					filter.StatsFilter.GroupNumDays = 1
					filter.StatsFilter.Limit = 1

					withCancelledContext(ctx, func(ctx context.Context) {
						pipeline := filter.TaskReliabilityQueryPipeline()
						assert.Equal(t, pipeline, []bson.M{
							bson.M{
								"$match": bson.M{
									"_id.date": bson.M{
										"$gte": after,
										"$lt":  before.Add(24 * time.Hour),
									},
									"_id.project": "mongodb-mongo-master",
									"_id.requester": bson.M{
										"$in": []string{
											"patch_request",
											"github_pull_request",
											"merge_test",
										},
									},
								},
							},
							bson.M{
								"$group": bson.M{
									"_id": bson.M{
										"date": "$_id.date",
									},
									"num_failed": bson.M{
										"$sum": "$num_failed",
									},
									"num_setup_failed": bson.M{
										"$sum": "$num_setup_failed",
									},
									"num_success": bson.M{
										"$sum": "$num_success",
									},
									"num_system_failed": bson.M{
										"$sum": "$num_system_failed",
									},
									"num_test_failed": bson.M{
										"$sum": "$num_test_failed",
									},
									"num_timeout": bson.M{
										"$sum": "$num_timeout",
									},
									"total_duration_success": bson.M{
										"$sum": bson.M{
											"$multiply": stats.Array{
												"$num_success",
												"$avg_duration_success",
											},
										},
									},
								},
							},
							bson.M{
								"$project": bson.M{
									"avg_duration_success": bson.M{
										"$cond": bson.M{
											"else": nil,
											"if": bson.M{
												"$ne": stats.Array{
													"$num_success",
													0,
												},
											},
											"then": bson.M{
												"$divide": stats.Array{
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
									"num_total": bson.M{
										"$add": stats.Array{
											"$num_success",
											"$num_failed",
										},
									},
									"task_name": "$_id.task_name",
									"variant":   "$_id.variant",
								},
							},
							bson.M{
								"$sort": bson.D{
									{Key: "date", Value: 1},
									{Key: "variant", Value: 1},
									{Key: "task_name", Value: 1},
									{Key: "distro", Value: 1},
								},
							},
							bson.M{
								"$limit": 1,
							},
						})
					})
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						filter := TaskReliabilityFilter{
							StatsFilter: stats.StatsFilter{
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
