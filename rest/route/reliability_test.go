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
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	projectID = "mongodb-mongo-master"
)

func configureTaskReliability(disabled bool) error {
	var err error
	flags := &evergreen.ServiceFlags{}
	err = flags.Get(evergreen.GetEnvironment())
	if err == nil {
		flags.TaskReliabilityDisabled = disabled
		err = flags.Set()
	}
	return err
}

func disableTaskReliability() error {
	return configureTaskReliability(true)
}

func enableTaskReliability() error {
	return configureTaskReliability(false)
}

func setupTest(t *testing.T) error {
	return enableTaskReliability()
}

func truncatedTime(deltaHours time.Duration) time.Time {
	return time.Now().UTC().Add(deltaHours).Truncate(24 * time.Hour)
}

func getURL(projectID string, parameters map[string]interface{}) string {
	url := fmt.Sprintf("https://example.net/api/rest/v2/projects/%s/logs/task_reliability", projectID)
	params := []string{}
	for key, value := range parameters {
		switch value.(type) {
		case []string:
			params = append(params, fmt.Sprintf("%s=%s", key, strings.Join(value.([]string), ",")))
		default:
			params = append(params, fmt.Sprintf("%s=%v", key, value))
		}
	}
	if len(params) > 0 {
		url = fmt.Sprintf("%s?%s", url, strings.Join(params, "&"))
	}
	return url
}

func TestParseParameters(t *testing.T) {

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
					require.NotNil(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
					assert.Equal(t, "Missing Tasks values", resp.Message)
				},
				"invalid: Too Many Tasks": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks": make([]string, reliabilityAPIMaxNumTasksLimit+1, reliabilityAPIMaxNumTasksLimit+1),
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NotNil(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
					assert.Equal(t, "Too many Tasks values", resp.Message)
				},
				"valid": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Nil(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, env, func() {
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
					require.NotNil(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(http.StatusBadRequest, resp.StatusCode)
					assert.Equal("Invalid after_date value", resp.Message)
				},
				"invalid: before_date": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					assert := assert.New(t)
					values := url.Values{
						"tasks":       []string{"aggregation_expression_multiversion_fuzzer"},
						"before_date": []string{"before_date date"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.NotNil(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(http.StatusBadRequest, resp.StatusCode)
					assert.Equal("Invalid before_date value", resp.Message)
				},
				"valid": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					values := url.Values{
						"tasks":       []string{"aggregation_expression_multiversion_fuzzer"},
						"before_date": []string{"2019-08-21"},
						"after_date":  []string{"2019-08-20"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.Nil(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, env, func() {
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
					require.NotNil(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(http.StatusBadRequest, resp.StatusCode)
					assert.Equal("Invalid sort value", resp.Message)
				},
				"valid: earliest": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					values := url.Values{
						"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
						"sort":  []string{"earliest"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.Nil(t, err)
				},
				"valid: latest": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					values := url.Values{
						"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
						"sort":  []string{"latest"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.Nil(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, env, func() {
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
					require.NotNil(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
					assert.Equal(t, "Invalid Significance value", resp.Message)
				},
				"invalid: greater Than one": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
						"significance": []string{"2.0"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NotNil(t, err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(t, http.StatusBadRequest, resp.StatusCode)
					assert.Equal(t, "Invalid Significance value", resp.Message)
				},
				"valid: 0.05": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
						"significance": []string{"0.05"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Nil(t, err)
				},
				"valid: 0": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
						"significance": []string{"0"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Nil(t, err)
				},
				"valid: 1": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
						"significance": []string{"1"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Nil(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, env, func() {
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
					require.NotNil(t, err)

					resp := err.(gimlet.ErrorResponse)
					require.Equal(t, http.StatusBadRequest, resp.StatusCode)
					require.Equal(t, "Invalid start_at value", resp.Message)
				},
				"valid: blank": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":    []string{"aggregation_expression_multiversion_fuzzer"},
						"start_at": []string{""},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Nil(t, err)
				},
				"valid": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"tasks":    []string{"aggregation_expression_multiversion_fuzzer"},
						"start_at": []string{"2019-08-21||||"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.Nil(t, err)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					handler := taskReliabilityHandler{}
					withSetupAndTeardown(t, env, func() {
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

func TestParse(t *testing.T) {

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
					require.Equal(t, handler.filter.Sort, stats.SortLatestFirst)
					require.Equal(t, handler.filter.Significance, reliability.DefaultSignificance)

					require.Equal(t, handler.filter.BeforeDate, truncatedTime(0))
					require.Equal(t, handler.filter.AfterDate, truncatedTime(-20*dayInHours))
				},
				"All Values": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					values := url.Values{
						"requesters":   []string{statsAPIRequesterMainline, statsAPIRequesterPatch},
						"after_date":   []string{"1998-07-12"},
						"before_date":  []string{"2018-07-15"},
						"tasks":        []string{"aggregation_expression_multiversion_fuzzer", "compile"},
						"variants":     []string{"enterprise-rhel-62-64-bit,enterprise-windows", "enterprise-rhel-80-64-bit"},
						"significance": []string{"0.1"},
					}

					err = handler.parseTaskReliabilityFilter(values)
					require.NoError(t, err)

					require.Equal(t, []string{
						evergreen.RepotrackerVersionRequester,
						evergreen.PatchVersionRequester,
						evergreen.GithubPRRequester,
						evergreen.MergeTestRequester,
					}, handler.filter.Requesters)
					require.Equal(t, time.Date(1998, 7, 12, 0, 0, 0, 0, time.UTC), handler.filter.AfterDate)
					require.Equal(t, time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC), handler.filter.BeforeDate)
					require.Equal(t, values["tasks"], handler.filter.Tasks)
					require.Equal(t, []string{"enterprise-rhel-62-64-bit", "enterprise-windows", "enterprise-rhel-80-64-bit"}, handler.filter.BuildVariants)
					require.Nil(t, handler.filter.Distros)
					require.Nil(t, handler.filter.StartAt)
					require.Equal(t, reliability.GroupByDistro, handler.filter.GroupBy) // default value
					require.Equal(t, reliability.SortLatestFirst, handler.filter.Sort)  // default value
					require.Equal(t, reliability.MaxQueryLimit, handler.filter.Limit)   // default value
					require.Equal(t, handler.filter.Significance, 0.1)
				},
				"Some Values": func(ctx context.Context, t *testing.T, handler taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					url := getURL(projectID, map[string]interface{}{
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
					withSetupAndTeardown(t, env, func() {
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

func TestRun(t *testing.T) {

	groupContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	for opName, opTests := range map[string]func(context.Context, *testing.T, evergreen.Environment){
		"Run": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler){
				"Disabled": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)

					// taskReliabilityHandler.sc.(&data.MockConnector).URL
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
					url := getURL(projectID, map[string]interface{}{
						"tasks":         "no_such_task",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})

					handler.sc.(*data.MockConnector).URL = url
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: 1,
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					data := resp.Data().([]interface{})
					require.Equal(t, 0, len(data))
					require.Nil(t, resp.Pages())
				},
				"Limit 1": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)
					url := getURL(projectID, map[string]interface{}{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})
					sc := handler.sc.(*data.MockConnector)
					sc.URL = url

					// 100 documents are available but only 1 will be returned
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", 100)

					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: 1,
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					data := resp.Data().([]interface{})
					require.Equal(t, 1, len(data))
					require.NotNil(t, resp.Pages())
				},
				"Limit 1000": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)
					url := getURL(projectID, map[string]interface{}{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})
					sc := handler.sc.(*data.MockConnector)
					sc.URL = url

					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: 1000,
						},
					}

					// limit + 1 documents are available but only limit will be returned
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", handler.filter.StatsFilter.Limit+1)

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					data := resp.Data().([]interface{})
					require.Equal(t, handler.filter.StatsFilter.Limit, len(data))
					require.NotNil(t, resp.Pages())
				},
				"StartAt Not Set": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)
					url := getURL(projectID, map[string]interface{}{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})
					sc := handler.sc.(*data.MockConnector)
					sc.URL = url

					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: 101,
						},
					}

					// limit - 1 documents are available.
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", handler.filter.StatsFilter.Limit-1)

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					data := resp.Data().([]interface{})
					require.Equal(t, handler.filter.StatsFilter.Limit-1, len(data))
					require.Nil(t, resp.Pages())
				},
				"StartAt Set": func(ctx context.Context, t *testing.T, handler *taskReliabilityHandler) {
					err := setupTest(t)
					require.NoError(t, err)
					url := getURL(projectID, map[string]interface{}{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})
					sc := handler.sc.(*data.MockConnector)
					sc.URL = url

					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: 100,
						},
					}

					// limit + 1 documents are available.
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", handler.filter.StatsFilter.Limit+1)

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					data := resp.Data().([]interface{})
					require.Equal(t, handler.filter.StatsFilter.Limit, len(data))
					require.NotNil(t, resp.Pages())
					lastDoc := sc.MockTaskReliabilityConnector.CachedTaskReliability[handler.filter.StatsFilter.Limit-1]
					require.Equal(t, lastDoc.StartAtKey(), resp.Pages().Next.Key)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					sc := &data.MockConnector{
						MockStatsConnector: data.MockStatsConnector{},
						URL:                "https://example.net/test",
					}

					handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
					withSetupAndTeardown(t, env, func() {
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

func TestReliability(t *testing.T) {

	groupContext, cancel := context.WithCancel(context.Background())
	defer cancel()

	for opName, opTests := range map[string]func(context.Context, *testing.T, evergreen.Environment){
		"Pagination": func(paginationContext context.Context, t *testing.T, env evergreen.Environment) {
			pageSize := 50
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"Less Than One Page": func(ctx context.Context, t *testing.T) {
					err := setupTest(t)
					require.NoError(t, err)
					url := getURL(projectID, map[string]interface{}{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})

					sc := &data.MockConnector{
						MockTaskReliabilityConnector: data.MockTaskReliabilityConnector{},
						URL:                          url,
					}
					handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
					require.NoError(t, err)

					// 1 page size of documents are available but 2 page sizes requested.
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", pageSize)
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: pageSize * 2,
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
					url := getURL(projectID, map[string]interface{}{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})

					sc := &data.MockConnector{
						MockTaskReliabilityConnector: data.MockTaskReliabilityConnector{},
						URL:                          url,
					}
					handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
					require.NoError(t, err)

					// 1 page size of documents will be returned
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", pageSize)
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: pageSize,
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					require.NotNil(t, resp.Pages())
					lastDoc := sc.MockTaskReliabilityConnector.CachedTaskReliability[pageSize-1]
					require.Equal(t, lastDoc.StartAtKey(), resp.Pages().Next.Key)
				},
				"More Than One Page": func(ctx context.Context, t *testing.T) {
					err := setupTest(t)
					require.NoError(t, err)
					url := getURL(projectID, map[string]interface{}{
						"tasks":         "aggregation_expression_multiversion_fuzzer",
						"after_date":    "2019-01-02",
						"group_by_days": "10",
					})

					sc := &data.MockConnector{
						MockTaskReliabilityConnector: data.MockTaskReliabilityConnector{},
						URL:                          url,
					}
					handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
					require.NoError(t, err)

					// 2 pages of documents are available.
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", pageSize*2)
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: pageSize,
						},
					}

					resp := handler.Run(ctx)

					require.NotNil(t, resp)
					require.Equal(t, http.StatusOK, resp.Status())
					require.NotNil(t, resp.Pages())
					lastDoc := sc.MockTaskReliabilityConnector.CachedTaskReliability[pageSize-1]
					require.Equal(t, lastDoc.StartAtKey(), resp.Pages().Next.Key)
				},
				"Invalid Start At": func(ctx context.Context, t *testing.T) {
					handler := taskReliabilityHandler{}

					values := url.Values{
						"tasks":    []string{"aggregation_expression_multiversion_fuzzer"},
						"start_at": []string{"2.0"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					require.NotNil(t, err)

					resp := err.(gimlet.ErrorResponse)
					require.Equal(t, http.StatusBadRequest, resp.StatusCode)
					require.Equal(t, "Invalid start_at value", resp.Message)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
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
