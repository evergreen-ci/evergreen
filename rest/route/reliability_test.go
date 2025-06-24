package route

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	projectID = "mongodb-mongo-master"
)

func configureTaskReliability(disabled bool) error {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	var err error
	flags := &evergreen.ServiceFlags{}
	err = flags.Get(ctx)
	if err == nil {
		flags.TaskReliabilityDisabled = disabled
		err = flags.Set(ctx)
	}
	return err
}

func disableTaskReliability() error {
	return configureTaskReliability(true)
}

func enableTaskReliability() error {
	return configureTaskReliability(false)
}

func setupTest(_ *testing.T) error {
	return enableTaskReliability()
}

func truncatedTime(deltaHours time.Duration) time.Time {
	return time.Now().UTC().Add(deltaHours).Truncate(24 * time.Hour)
}

func getURL(projectID string, parameters map[string]any) string {
	url := fmt.Sprintf("https://example.net/api/rest/v2/projects/%s/logs/task_reliability", projectID)
	params := []string{}
	for key, value := range parameters {
		switch v := value.(type) {
		case []string:
			params = append(params, fmt.Sprintf("%s=%s", key, strings.Join(v, ",")))
		default:
			params = append(params, fmt.Sprintf("%s=%v", key, value))
		}
	}
	if len(params) > 0 {
		url = fmt.Sprintf("%s?%s", url, strings.Join(params, "&"))
	}
	return url
}

func TestReliabilityParseParameters(t *testing.T) {

	groupContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	for opName, opTests := range map[string]func(context.Context, *testing.T, evergreen.Environment){
		"Tasks": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler taskReliabilityHandler){
				"invalid: No Tasks": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{}

					err = handler.parseTaskReliabilityFilter(values)
					require.Error(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
					assert.Equal(t, "must specify at least one task", resp.Message)
				},
				"invalid: Too Many Tasks": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks": make([]string, reliabilityAPIMaxNumTasksLimit+1),
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Error(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
					assert.Equal(t, fmt.Sprintf("cannot request more than %d tasks", reliabilityAPIMaxNumTasksLimit), resp.Message)
				},
				"valid": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, func() {
						testCase(paginationContext, t, handler)
					})
				})
			}
		},
		"Dates": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler taskReliabilityHandler){
				"invalid: after_date": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					assert := assert.New(t)
					values := url.Values{
						"tasks":      []string{"aggregation_expression_multiversion_fuzzer"},
						"after_date": []string{"invalid date"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.Error(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(http.StatusBadRequest, resp.StatusCode)
					assert.Equal("invalid 'after' date", resp.Message)
				},
				"invalid: before_date": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					assert := assert.New(t)
					values := url.Values{
						"tasks":       []string{"aggregation_expression_multiversion_fuzzer"},
						"before_date": []string{"before_date date"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.Error(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(http.StatusBadRequest, resp.StatusCode)
					assert.Equal("invalid 'before' date", resp.Message)
				},
				"valid": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					values := url.Values{
						"tasks":       []string{"aggregation_expression_multiversion_fuzzer"},
						"before_date": []string{"2019-08-21"},
						"after_date":  []string{"2019-08-20"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, func() {
						testCase(paginationContext, t, handler)
					})
				})
			}
		},
		"Sort": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler taskReliabilityHandler){
				"invalid": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					assert := assert.New(t)
					values := url.Values{
						"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
						"sort":  []string{"invalid sort"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.Error(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(http.StatusBadRequest, resp.StatusCode)
					assert.Equal("invalid sort", resp.Message)
				},
				"valid: earliest": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					values := url.Values{
						"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
						"sort":  []string{"earliest"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
				},
				"valid: latest": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					values := url.Values{
						"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
						"sort":  []string{"latest"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, func() {
						testCase(paginationContext, t, handler)
					})
				})
			}
		},
		"Significance": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler taskReliabilityHandler){
				"invalid: Less Than zero": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
						"significance": []string{"-1.0"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Error(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
					assert.Equal(t, "invalid significance value", resp.Message)
				},
				"invalid: greater Than one": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
						"significance": []string{"2.0"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Error(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
					assert.Equal(t, "invalid significance value", resp.Message)
				},
				"valid: 0.05": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
						"significance": []string{"0.05"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
				},
				"valid: 0": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
						"significance": []string{"0"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
				},
				"valid: 1": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
						"significance": []string{"1"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, func() {
						testCase(paginationContext, t, handler)
					})
				})
			}
		},
		"StartAt": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler taskReliabilityHandler){
				"invalid": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":    []string{"aggregation_expression_multiversion_fuzzer"},
						"start_at": []string{"2.0"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Error(t, err)

					resp := err.(gimlet.ErrorResponse)
					require.Equal(t, http.StatusBadRequest, resp.StatusCode)
					require.Equal(t, "invalid 'start at' value", resp.Message)
				},
				"valid: blank": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":    []string{"aggregation_expression_multiversion_fuzzer"},
						"start_at": []string{""},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
				},
				"valid": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":    []string{"aggregation_expression_multiversion_fuzzer"},
						"start_at": []string{"2019-08-21|||"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, func() {
						testCase(paginationContext, t, handler)
					})
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			env, err := setupEnv(groupContext)
			require.NoError(t, err)

			testContext, cancel := context.WithTimeout(groupContext, 5*time.Second)
			defer cancel()

			env.Settings().DomainName = "test"
			opTests(testContext, t, env)
		})
	}
}

func TestReliabilityParse(t *testing.T) {
	groupContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	for opName, opTests := range map[string]func(context.Context, *testing.T, evergreen.Environment){
		"Parse": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler taskReliabilityHandler){
				"Defaults": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					// Defaults
					values := url.Values{
						"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)
					require.Equal(t, values["tasks"], handler.filter.Tasks)
					require.Equal(t, taskstats.SortLatestFirst, handler.filter.Sort)
					//nolint:testifylint // We expect the float to be exactly equal.
					require.Equal(t, handler.filter.Significance, reliability.DefaultSignificance)

					require.Equal(t, []string{"gitter_request"}, handler.filter.Requesters)
					require.Equal(t, 1, handler.filter.GroupNumDays)
					require.Equal(t, reliability.GroupByTask, handler.filter.GroupBy)
					require.Equal(t, handler.filter.BeforeDate, truncatedTime(0))
					require.Equal(t, handler.filter.AfterDate, truncatedTime(0))
				},
				"All Values": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"requesters":     []string{statsAPIRequesterMainline, statsAPIRequesterPatch},
						"after_date":     []string{"1998-07-12"},
						"before_date":    []string{"2018-07-15"},
						"tasks":          []string{"aggregation_expression_multiversion_fuzzer", "compile"},
						"variants":       []string{"enterprise-rhel-62-64-bit,enterprise-windows", "enterprise-rhel-80-64-bit"},
						"significance":   []string{"0.1"},
						"group_by":       []string{"task_variant_distro"},
						"group_num_days": []string{"28"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)

					require.Equal(t, []string{
						evergreen.RepotrackerVersionRequester,
						evergreen.PatchVersionRequester,
						evergreen.GithubPRRequester,
						evergreen.GithubMergeRequester,
					}, handler.filter.Requesters)
					require.Equal(t, time.Date(1998, 7, 12, 0, 0, 0, 0, time.UTC), handler.filter.AfterDate)
					require.Equal(t, time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC), handler.filter.BeforeDate)
					require.Equal(t, values["tasks"], handler.filter.Tasks)
					require.Equal(t, []string{"enterprise-rhel-62-64-bit", "enterprise-windows", "enterprise-rhel-80-64-bit"}, handler.filter.BuildVariants)
					require.Nil(t, handler.filter.Distros)
					require.Nil(t, handler.filter.StartAt)
					require.Equal(t, 28, handler.filter.GroupNumDays)
					require.Equal(t, reliability.GroupByDistro, handler.filter.GroupBy) // default value
					require.Equal(t, reliability.SortLatestFirst, handler.filter.Sort)  // default value
					require.Equal(t, reliability.MaxQueryLimit, handler.filter.Limit)   // default value
					//nolint:testifylint // We expect the float to be exactly 0.1.
					require.Equal(t, 0.1, handler.filter.Significance)
				},
				"Some Values": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					url := getURL(projectID, map[string]any{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})
					request, err := http.NewRequest("GET", url, bytes.NewReader(nil))
					require.NoError(t, err)

					options := map[string]string{"project_id": projectID}
					request = gimlet.SetURLVars(request, options)

					err = handler.Parse(context.Background(), request)
					require.NoError(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, func() {
						testCase(paginationContext, t, handler)
					})
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			env, err := setupEnv(groupContext)
			require.NoError(t, err)

			testContext, cancel := context.WithTimeout(groupContext, 5*time.Second)
			defer cancel()

			env.Settings().DomainName = "test"
			opTests(testContext, t, env)
		})
	}
}

func TestReliabilityRun(t *testing.T) {
	groupContext, cancel := context.WithCancel(context.Background())
	defer cancel()
	require.NoError(t, db.ClearCollections(taskstats.DailyTaskStatsCollection, model.ProjectRefCollection))
	proj := model.ProjectRef{
		Id: "project",
	}
	require.NoError(t, proj.Insert(t.Context()))

	for opName, opTests := range map[string]func(context.Context, *testing.T, evergreen.Environment){
		"Run": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler){
				"Disabled": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					err = disableTaskReliability()
					assert.NoError(t, err)

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusServiceUnavailable, resp.Status())
					require.Nil(t, resp.Pages())
				},
				"NoSuchTask": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit:        100,
							Project:      "project",
							Requesters:   []string{"requester"},
							Tasks:        []string{"no_such_task"},
							GroupBy:      "distro",
							GroupNumDays: 1,
							Sort:         taskstats.SortEarliestFirst,
							BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
							AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					data := resp.Data().([]any)
					require.Empty(t, data)
					require.Nil(t, resp.Pages())
				},
				"Limit 1": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					// 100 documents are available but only 1 will be returned
					day := time.Now()
					tasks := []string{}
					for i := 0; i < 100; i++ {
						taskName := fmt.Sprintf("%v%v", "aggregation_expression_multiversion_fuzzer", i)
						tasks = append(tasks, taskName)
						err = db.Insert(t.Context(), taskstats.DailyTaskStatsCollection, mgobson.M{
							"_id": taskstats.DBTaskStatsID{
								Project:      "project",
								Requester:    "requester",
								TaskName:     taskName,
								BuildVariant: "variant",
								Distro:       "distro",
								Date:         utility.GetUTCDay(day),
							},
						})
						require.NoError(t, err)
					}
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit:        1,
							Project:      "project",
							Requesters:   []string{"requester"},
							Tasks:        tasks,
							GroupBy:      "distro",
							GroupNumDays: 1,
							Sort:         taskstats.SortEarliestFirst,
							BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
							AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					data := resp.Data().([]any)
					require.Len(t, data, 1)
					require.NotNil(t, resp.Pages())
				},
				"Limit 1000": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					// limit + 1 documents are available but only limit will be returned
					day := time.Now()
					tasks := []string{}
					for i := 0; i < 1001; i++ {
						taskName := fmt.Sprintf("%v%v", "aggregation_expression_multiversion_fuzzer", i)
						tasks = append(tasks, taskName)
						err = db.Insert(t.Context(), taskstats.DailyTaskStatsCollection, mgobson.M{
							"_id": taskstats.DBTaskStatsID{
								Project:      "project",
								Requester:    "requester",
								TaskName:     taskName,
								BuildVariant: "variant",
								Distro:       "distro",
								Date:         utility.GetUTCDay(day),
							},
						})
						require.NoError(t, err)
					}
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit:        1000,
							Project:      "project",
							Requesters:   []string{"requester"},
							Tasks:        tasks,
							GroupBy:      "distro",
							GroupNumDays: 1,
							Sort:         taskstats.SortEarliestFirst,
							BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
							AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					data := resp.Data().([]any)
					require.Len(t, data, handler.filter.StatsFilter.Limit)
					require.NotNil(t, resp.Pages())
				},
				"StartAt Not Set": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit: 101,
						},
					}

					// limit - 1 documents are available.
					day := time.Now()
					tasks := []string{}
					for i := 0; i < 99; i++ {
						taskName := fmt.Sprintf("%v%v", "aggregation_expression_multiversion_fuzzer", i)
						tasks = append(tasks, taskName)
						err = db.Insert(t.Context(), taskstats.DailyTaskStatsCollection, mgobson.M{
							"_id": taskstats.DBTaskStatsID{
								Project:      "project",
								Requester:    "requester",
								TaskName:     taskName,
								BuildVariant: "variant",
								Distro:       "distro",
								Date:         utility.GetUTCDay(day),
							},
						})
						require.NoError(t, err)
					}
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit:        100,
							Project:      "project",
							Requesters:   []string{"requester"},
							Tasks:        tasks,
							GroupBy:      "distro",
							GroupNumDays: 1,
							Sort:         taskstats.SortEarliestFirst,
							BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
							AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
						},
					}
					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					data := resp.Data().([]any)
					require.Len(t, data, handler.filter.StatsFilter.Limit-1)
					require.Nil(t, resp.Pages())
				},
				"StartAt Set": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit: 100,
						},
					}

					// limit + 1 documents are available.
					day := time.Now()
					tasks := []string{}
					for i := 0; i < 101; i++ {
						taskName := fmt.Sprintf("%v%v", "aggregation_expression_multiversion_fuzzer", i)
						tasks = append(tasks, taskName)
						err = db.Insert(t.Context(), taskstats.DailyTaskStatsCollection, mgobson.M{
							"_id": taskstats.DBTaskStatsID{
								Project:      "project",
								Requester:    "requester",
								TaskName:     taskName,
								BuildVariant: "variant",
								Distro:       "distro",
								Date:         utility.GetUTCDay(day),
							},
						})
						require.NoError(t, err)
					}
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit:        100,
							Project:      "project",
							Requesters:   []string{"requester"},
							Tasks:        tasks,
							GroupBy:      "distro",
							GroupNumDays: 1,
							Sort:         taskstats.SortEarliestFirst,
							BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
							AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
						},
					}
					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					respData := resp.Data().([]any)
					require.Len(t, respData, handler.filter.StatsFilter.Limit)
					require.NotNil(t, resp.Pages())
					docs, err := data.GetTaskReliabilityScores(t.Context(), handler.filter)
					require.NoError(t, err)
					require.Equal(t, docs[handler.filter.StatsFilter.Limit-1].StartAtKey(), resp.Pages().Next.Key)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := makeGetProjectTaskReliability().(*taskReliabilityHandler)
					handler.url = "https://example.net/test"
					withSetupAndTeardown(t, func() {
						testCase(paginationContext, t, handler)
					})
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			env, err := setupEnv(groupContext)
			require.NoError(t, err)

			testContext, cancel := context.WithTimeout(groupContext, 5*time.Second)
			defer cancel()

			env.Settings().DomainName = "test"
			opTests(testContext, t, env)
		})
	}
}

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

func TestReliability(t *testing.T) {
	require.NoError(t, db.ClearCollections(taskstats.DailyTaskStatsCollection, model.ProjectRefCollection))
	groupContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	proj := model.ProjectRef{
		Id: "project",
	}
	require.NoError(t, proj.Insert(t.Context()))

	for opName, opTests := range map[string]func(context.Context, *testing.T, evergreen.Environment){
		"Pagination": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			pageSize := 50
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"Less Than One Page": func(ctx context.Context, t *testing.T) {
					err := setupTest(t)
					require.NoError(t, err)
					url := getURL(projectID, map[string]any{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})
					handler := makeGetProjectTaskReliability().(*taskReliabilityHandler)
					require.NoError(t, err)
					handler.url = url

					// 1 page size of documents are available but 2 page sizes requested.
					day := time.Now()
					tasks := []string{}
					for i := 0; i < pageSize; i++ {
						taskName := fmt.Sprintf("%v%v", "aggregation_expression_multiversion_fuzzer", i)
						tasks = append(tasks, taskName)
						err = db.Insert(t.Context(), taskstats.DailyTaskStatsCollection, mgobson.M{
							"_id": taskstats.DBTaskStatsID{
								Project:      "project",
								Requester:    "requester",
								TaskName:     taskName,
								BuildVariant: "variant",
								Distro:       "distro",
								Date:         utility.GetUTCDay(day),
							},
						})
						require.NoError(t, err)
					}
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit:        pageSize * 2,
							Project:      "project",
							Requesters:   []string{"requester"},
							Tasks:        tasks,
							GroupBy:      "distro",
							GroupNumDays: 1,
							Sort:         taskstats.SortEarliestFirst,
							BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
							AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					require.Nil(t, resp.Pages())
				},
				"Exactly One Page": func(ctx context.Context, t *testing.T) {
					err := setupTest(t)
					require.NoError(t, err)
					url := getURL(projectID, map[string]any{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})

					handler := makeGetProjectTaskReliability().(*taskReliabilityHandler)
					handler.url = url
					require.NoError(t, err)

					// 1 page size of documents will be returned
					day := time.Now()
					tasks := []string{}
					for i := 0; i < pageSize; i++ {
						taskName := fmt.Sprintf("%v%v", "aggregation_expression_multiversion_fuzzer", i)
						tasks = append(tasks, taskName)
						err = db.Insert(t.Context(), taskstats.DailyTaskStatsCollection, mgobson.M{
							"_id": taskstats.DBTaskStatsID{
								Project:      "project",
								Requester:    "requester",
								TaskName:     taskName,
								BuildVariant: "variant",
								Distro:       "distro",
								Date:         utility.GetUTCDay(day),
							},
						})
						require.NoError(t, err)
					}
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit:        pageSize,
							Project:      "project",
							Requesters:   []string{"requester"},
							Tasks:        tasks,
							GroupBy:      "distro",
							GroupNumDays: 1,
							Sort:         taskstats.SortEarliestFirst,
							BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
							AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					require.NotNil(t, resp.Pages())
					docs, err := data.GetTaskReliabilityScores(t.Context(), handler.filter)
					require.NoError(t, err)
					require.Equal(t, docs[pageSize-1].StartAtKey(), resp.Pages().Next.Key)
				},
				"More Than One Page": func(ctx context.Context, t *testing.T) {
					err := setupTest(t)
					require.NoError(t, err)
					url := getURL(projectID, map[string]any{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})

					handler := makeGetProjectTaskReliability().(*taskReliabilityHandler)
					handler.url = url
					require.NoError(t, err)

					// 2 pages of documents are available.
					day := time.Now()
					tasks := []string{}
					for i := 0; i < pageSize*2; i++ {
						taskName := fmt.Sprintf("%v%v", "aggregation_expression_multiversion_fuzzer", i)
						tasks = append(tasks, taskName)
						err = db.Insert(t.Context(), taskstats.DailyTaskStatsCollection, mgobson.M{
							"_id": taskstats.DBTaskStatsID{
								Project:      "project",
								Requester:    "requester",
								TaskName:     taskName,
								BuildVariant: "variant",
								Distro:       "distro",
								Date:         utility.GetUTCDay(day),
							},
						})
						require.NoError(t, err)
					}
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: taskstats.StatsFilter{
							Limit:        pageSize,
							Project:      "project",
							Requesters:   []string{"requester"},
							Tasks:        tasks,
							GroupBy:      "distro",
							GroupNumDays: 1,
							Sort:         taskstats.SortEarliestFirst,
							BeforeDate:   utility.GetUTCDay(time.Now().Add(dayInHours)),
							AfterDate:    utility.GetUTCDay(time.Now().Add(-dayInHours)),
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					require.NotNil(t, resp.Pages())
					docs, err := data.GetTaskReliabilityScores(t.Context(), handler.filter)
					require.NoError(t, err)
					require.Equal(t, docs[pageSize-1].StartAtKey(), resp.Pages().Next.Key)
				},
				"Invalid Start At": func(ctx context.Context, t *testing.T) {
					handler := taskReliabilityHandler{}

					values := url.Values{
						"tasks":    []string{"aggregation_expression_multiversion_fuzzer"},
						"start_at": []string{"2.0"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.Error(t, err)

					resp := err.(gimlet.ErrorResponse)
					require.Equal(t, http.StatusBadRequest, resp.StatusCode)
					require.Equal(t, "invalid 'start at' value", resp.Message)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, func() {
						testCase(paginationContext, t)
					})
				})
			}
		},
	} {
		t.Run(opName, func(t *testing.T) {
			env, err := setupEnv(groupContext)
			require.NoError(t, err)

			testContext, cancel := context.WithTimeout(groupContext, 5*time.Second)
			defer cancel()

			env.Settings().DomainName = "test"
			opTests(testContext, t, env)
		})
	}
}
