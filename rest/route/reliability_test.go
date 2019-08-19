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

// func getURL(projectID string, tasks []string) string {
func getURL(projectID string, parameters map[string]interface{}) string {
	url := fmt.Sprintf("https://example.net/api/rest/v2/projects/%s/logs/task_reliability", projectID)
	params := []string{}
	for key, value := range parameters {
		switch value.(type) {
		// case int:
		// 	params = append(params, fmt.Sprintf("%s=%v", key, value))
		case []string:
			params = append(params, fmt.Sprintf("%s=%s", key, strings.Join(value.([]string), ",")))
			// params = fmt.Sprintf("%s%c%s=%s", params, sep, key, strings.Join(value.([]string), ","))
		// case string:
		// 	params = append(params, fmt.Sprintf("%s=%s", key, value))
		// 	// params = fmt.Sprintf("%s%c%s=%s", params, sep, key, value)
		default:
			params = append(params, fmt.Sprintf("%s=%v", key, value))
		}
	}
	if len(params) > 0 {
		url = fmt.Sprintf("%s?%s", url, strings.Join(params, "&"))
	}
	return url
}

func TestParseNoTasks(t *testing.T) {
	assert := assert.New(t)
	values := url.Values{}
	handler := taskReliabilityHandler{}

	err := handler.parseTaskReliabilityFilter(values)
	assert.NotNil(err)

	resp := err.(gimlet.ErrorResponse)
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("Missing Tasks values", resp.Message)
}

func TestParseTooManyTasks(t *testing.T) {
	assert := assert.New(t)
	handler := taskReliabilityHandler{}

	values := url.Values{
		"tasks": make([]string, reliabilityAPIMaxNumTasksLimit+1, reliabilityAPIMaxNumTasksLimit+1),
	}

	err := handler.parseTaskReliabilityFilter(values)
	assert.NotNil(err)

	resp := err.(gimlet.ErrorResponse)
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("Too many Tasks values", resp.Message)
}

func TestParseInvalidAfterDate(t *testing.T) {
	assert := assert.New(t)
	handler := taskReliabilityHandler{}

	values := url.Values{
		"tasks":      []string{"aggregation_expression_multiversion_fuzzer"},
		"after_date": []string{"invalid date"},
	}

	err := handler.parseTaskReliabilityFilter(values)
	assert.NotNil(err)

	resp := err.(gimlet.ErrorResponse)
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("Invalid after_date value", resp.Message)

}

func TestParseInvalidBeforeDate(t *testing.T) {
	assert := assert.New(t)
	handler := taskReliabilityHandler{}

	values := url.Values{
		"tasks":       []string{"aggregation_expression_multiversion_fuzzer"},
		"before_date": []string{"invalid date"},
	}

	err := handler.parseTaskReliabilityFilter(values)
	assert.NotNil(err)

	resp := err.(gimlet.ErrorResponse)
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("Invalid before_date value", resp.Message)

}

func TestParseInvalidSort(t *testing.T) {
	assert := assert.New(t)
	handler := taskReliabilityHandler{}

	values := url.Values{
		"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
		"sort":  []string{"invalid sort"},
	}

	err := handler.parseTaskReliabilityFilter(values)
	assert.NotNil(err)

	resp := err.(gimlet.ErrorResponse)
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("Invalid sort value", resp.Message)

}

func TestParseInvalidSignificance(t *testing.T) {
	assert := assert.New(t)
	handler := taskReliabilityHandler{}

	values := url.Values{
		"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
		"significance": []string{"-1.0"},
	}

	err := handler.parseTaskReliabilityFilter(values)
	assert.NotNil(err)

	resp := err.(gimlet.ErrorResponse)
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("Invalid Significance value", resp.Message)

	values = url.Values{
		"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
		"significance": []string{"2.0"},
	}

	err = handler.parseTaskReliabilityFilter(values)
	assert.NotNil(err)

	resp = err.(gimlet.ErrorResponse)
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("Invalid Significance value", resp.Message)

}

func TestParseInvalidStartAt(t *testing.T) {
	assert := assert.New(t)
	handler := taskReliabilityHandler{}

	values := url.Values{
		"tasks":        []string{"aggregation_expression_multiversion_fuzzer"},
		"significance": []string{"-1.0"},
	}

	err := handler.parseTaskReliabilityFilter(values)
	assert.NotNil(err)

	resp := err.(gimlet.ErrorResponse)
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("Invalid Significance value", resp.Message)

	values = url.Values{
		"tasks":    []string{"aggregation_expression_multiversion_fuzzer"},
		"start_at": []string{"2.0"},
	}

	err = handler.parseTaskReliabilityFilter(values)
	assert.NotNil(err)

	resp = err.(gimlet.ErrorResponse)
	assert.Equal(http.StatusBadRequest, resp.StatusCode)
	assert.Equal("Invalid Significance value", resp.Message)

}

func TestParseValid(t *testing.T) {
	assert := assert.New(t)
	handler := taskReliabilityHandler{}

	// Defaults
	values := url.Values{
		"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
	}

	err := handler.parseTaskReliabilityFilter(values)
	assert.NoError(err)
	assert.Equal(values["tasks"], handler.filter.Tasks)
	assert.Equal(handler.filter.Sort, stats.SortLatestFirst)
	assert.Equal(handler.filter.Significance, reliability.DefaultSignificance)

	assert.Equal(handler.filter.BeforeDate, truncatedTime(0))
	assert.Equal(handler.filter.AfterDate, truncatedTime(-20*dayInHours))

	values = url.Values{
		"requesters":   []string{statsAPIRequesterMainline, statsAPIRequesterPatch},
		"after_date":   []string{"1998-07-12"},
		"before_date":  []string{"2018-07-15"},
		"tasks":        []string{"aggregation_expression_multiversion_fuzzer", "compile"},
		"variants":     []string{"enterprise-rhel-62-64-bit,enterprise-windows", "enterprise-rhel-80-64-bit"},
		"significance": []string{"0.1"},
	}

	err = handler.parseTaskReliabilityFilter(values)
	assert.NoError(err)

	assert.Equal([]string{
		evergreen.RepotrackerVersionRequester,
		evergreen.PatchVersionRequester,
		evergreen.GithubPRRequester,
		evergreen.MergeTestRequester,
	}, handler.filter.Requesters)
	assert.Equal(time.Date(1998, 7, 12, 0, 0, 0, 0, time.UTC), handler.filter.AfterDate)
	assert.Equal(time.Date(2018, 7, 15, 0, 0, 0, 0, time.UTC), handler.filter.BeforeDate)
	assert.Equal(values["tasks"], handler.filter.Tasks)
	assert.Equal([]string{"enterprise-rhel-62-64-bit", "enterprise-windows", "enterprise-rhel-80-64-bit"}, handler.filter.BuildVariants)
	assert.Nil(handler.filter.Distros)
	assert.Nil(handler.filter.StartAt)
	assert.Equal(stats.GroupByDistro, handler.filter.GroupBy)          // default value
	assert.Equal(stats.SortLatestFirst, handler.filter.Sort)           // default value
	assert.Equal(reliabilityAPIMaxNumTasksLimit, handler.filter.Limit) // default value
	assert.Equal(handler.filter.Significance, 0.1)

}

func TestParse(t *testing.T) {
	assert := assert.New(t)

	// parameters := map[string]interface{}{
	// 	"tasks":         "aggregation_expression_multiversion_fuzzer",
	// 	"after_date":    "2019-01-02",
	// 	"group_by_days": "10",
	// }
	// url := getURL(projectID, parameters) // []string{"aggregation_expression_multiversion_fuzzer"})
	url := getURL(projectID, map[string]interface{}{
		"tasks":         "aggregation_expression_multiversion_fuzzer",
		"after_date":    "2019-01-02",
		"group_by_days": "10",
	})
	request, err := http.NewRequest("GET", url, bytes.NewReader(nil))
	assert.NoError(err)

	options := map[string]string{"project_id": projectID}
	request = gimlet.SetURLVars(request, options)

	sc := &data.MockConnector{
		MockStatsConnector: data.MockStatsConnector{},
		URL:                url,
	}
	handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
	err = handler.Parse(context.Background(), request)
	assert.NoError(err)
}

func TestDisabledRunTestHandler(t *testing.T) {
	assert := assert.New(t)
	err := setupTest(t)
	assert.NoError(err)
	sc := &data.MockConnector{
		MockStatsConnector: data.MockStatsConnector{},
		URL:                "https://example.net/test",
	}

	err = disableTaskReliability()
	assert.NoError(err)

	handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)

	resp := handler.Run(context.Background())

	assert.NotNil(resp)
	assert.Equal(http.StatusServiceUnavailable, resp.Status())
	assert.Nil(resp.Pages())
}

func TestRunNoSuchTask(t *testing.T) {
	assert := assert.New(t)
	err := setupTest(t)
	assert.NoError(err)
	// url := getURL(projectID, []string{"no_such_task"})
	url := getURL(projectID, map[string]interface{}{
		"tasks":         "no_such_task",
		"after_date":    "2019-01-02",
		"group_by_days": "10",
	})

	sc := &data.MockConnector{
		MockTaskReliabilityConnector: data.MockTaskReliabilityConnector{},
		URL:                          url,
	}
	handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
	assert.NoError(err)

	handler.filter = reliability.TaskReliabilityFilter{
		StatsFilter: stats.StatsFilter{
			Limit: 1,
		},
	}

	// code subtracts 1
	handler.filter.Limit++

	resp := handler.Run(context.Background())

	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	data := resp.Data().([]interface{})
	assert.Equal(0, len(data))
	assert.Nil(resp.Pages())
}

func TestRunLimit1(t *testing.T) {
	assert := assert.New(t)
	err := setupTest(t)
	assert.NoError(err)
	// url := getURL(projectID, []string{"aggregation_expression_multiversion_fuzzer"})
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
	assert.NoError(err)

	// 1 document will be returned
	sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", 100)

	handler.filter = reliability.TaskReliabilityFilter{
		StatsFilter: stats.StatsFilter{
			Limit: 1,
		},
	}

	// code subtracts 1
	// handler.filter.Limit++
	resp := handler.Run(context.Background())

	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	data := resp.Data().([]interface{})
	assert.Equal(1, len(data))
	assert.NotNil(resp.Pages())
}

func TestRunLimit1000(t *testing.T) {
	assert := assert.New(t)
	err := setupTest(t)
	assert.NoError(err)
	// url := getURL(projectID, []string{"aggregation_expression_multiversion_fuzzer"})
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
	assert.NoError(err)

	// 1000 documents will be returned
	sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", 1001)

	handler.filter = reliability.TaskReliabilityFilter{
		StatsFilter: stats.StatsFilter{
			Limit: 1000,
		},
	}

	resp := handler.Run(context.Background())

	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	data := resp.Data().([]interface{})
	assert.Equal(1000, len(data))
	assert.NotNil(resp.Pages())
}

func TestRunTestHandler(t *testing.T) {
	assert := assert.New(t)
	err := setupTest(t)
	assert.NoError(err)
	// url := getURL(projectID, []string{"aggregation_expression_multiversion_fuzzer"})
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
	assert.NoError(err)

	// 100 documents will be returned
	sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", 100)
	handler.filter = reliability.TaskReliabilityFilter{
		StatsFilter: stats.StatsFilter{
			Limit: 101,
		},
	}

	resp := handler.Run(context.Background())

	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	assert.Nil(resp.Pages())

	// 101 documents will be returned
	sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", 101)
	handler.filter = reliability.TaskReliabilityFilter{
		StatsFilter: stats.StatsFilter{
			Limit: 101,
		},
	}

	resp = handler.Run(context.Background())

	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	assert.NotNil(resp.Pages())
	lastDoc := sc.MockTaskReliabilityConnector.CachedTaskReliability[100]
	assert.Equal(lastDoc.StartAtKey(), resp.Pages().Next.Key)
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	for opName, opTests := range map[string]func(ctx context.Context, t *testing.T, env evergreen.Environment){
		"Pagination": func(ctx context.Context, t *testing.T, env evergreen.Environment) {
			pageSize := 50
			for testName, testCase := range map[string]func(ctx context.Context, t *testing.T){
				"Less Than One Page": func(ctx context.Context, t *testing.T) {
					assert := assert.New(t)
					err := setupTest(t)
					assert.NoError(err)
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
					assert.NoError(err)

					// 1 page size of documents are available but 2 page sizes requested.
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", pageSize)
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: pageSize * 2,
						},
					}

					resp := handler.Run(context.Background())

					assert.NotNil(resp)
					assert.Equal(http.StatusOK, resp.Status())
					assert.Nil(resp.Pages())
				},
				"Exactly One Page": func(ctx context.Context, t *testing.T) {
					assert := assert.New(t)
					err := setupTest(t)
					assert.NoError(err)
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
					assert.NoError(err)

					// 1 page size of documents will be returned
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", pageSize)
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: pageSize,
						},
					}

					resp := handler.Run(context.Background())

					assert.NotNil(resp)
					assert.Equal(http.StatusOK, resp.Status())
					assert.NotNil(resp.Pages())
					lastDoc := sc.MockTaskReliabilityConnector.CachedTaskReliability[pageSize-1]
					assert.Equal(lastDoc.StartAtKey(), resp.Pages().Next.Key)
				},
				"More Than One Page": func(ctx context.Context, t *testing.T) {
					assert := assert.New(t)
					err := setupTest(t)
					assert.NoError(err)
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
					assert.NoError(err)

					// 2 pages of documents are available.
					sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", pageSize*2)
					handler.filter = reliability.TaskReliabilityFilter{
						StatsFilter: stats.StatsFilter{
							Limit: pageSize,
						},
					}

					resp := handler.Run(context.Background())

					assert.NotNil(resp)
					assert.Equal(http.StatusOK, resp.Status())
					assert.NotNil(resp.Pages())
					lastDoc := sc.MockTaskReliabilityConnector.CachedTaskReliability[pageSize-1]
					assert.Equal(lastDoc.StartAtKey(), resp.Pages().Next.Key)
				},
				"Invalid Start At": func(ctx context.Context, t *testing.T) {
					assert := assert.New(t)
					handler := taskReliabilityHandler{}

					values := url.Values{
						"tasks":    []string{"aggregation_expression_multiversion_fuzzer"},
						"start_at": []string{"2.0"},
					}

					err := handler.parseTaskReliabilityFilter(values)
					assert.NotNil(err)

					resp := err.(gimlet.ErrorResponse)
					assert.Equal(http.StatusBadRequest, resp.StatusCode)
					assert.Equal("Invalid start_at value", resp.Message)
				},
			} {
				t.Run(testName, func(t *testing.T) {
					withSetupAndTeardown(t, env, func() {
						testCase(ctx, t)
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
