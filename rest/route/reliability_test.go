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
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/assert"
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

func getURL(projectID string, tasks []string) string {
	url := fmt.Sprintf("https://example.net/api/rest/v2/projects/%s/logs/task_reliability", projectID)
	sep := '?'
	if len(tasks) > 0 {
		url = fmt.Sprintf("%s%ctasks=%s", url, sep, strings.Join(tasks, ","))
		sep = '&'
	}
	sep = '&'
	url = fmt.Sprintf("%s%cafter_date=2019-01-02&group_by_days=10", url, sep)
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
	values := url.Values{}
	handler := taskReliabilityHandler{}

	values = url.Values{
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
	values := url.Values{}
	handler := taskReliabilityHandler{}

	values = url.Values{
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
	values := url.Values{}
	handler := taskReliabilityHandler{}

	values = url.Values{
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
	values := url.Values{}
	handler := taskReliabilityHandler{}

	values = url.Values{
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
	values := url.Values{}
	handler := taskReliabilityHandler{}

	values = url.Values{
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

func TestParseValid(t *testing.T) {
	assert := assert.New(t)
	values := url.Values{}
	handler := taskReliabilityHandler{}

	// Defaults
	values = url.Values{
		"tasks": []string{"aggregation_expression_multiversion_fuzzer"},
	}

	err := handler.parseTaskReliabilityFilter(values)
	assert.NoError(err)
	assert.Equal(values["tasks"], handler.filter.Tasks)
	assert.Equal(handler.filter.Sort, stats.SortLatestFirst)
	assert.Equal(handler.filter.Significance, reliability.DefaultSignificance)

	assert.Equal(handler.filter.BeforeDate, truncatedTime(dayInHours))
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
	assert.Equal(stats.GroupByDistro, handler.filter.GroupBy)            // default value
	assert.Equal(stats.SortLatestFirst, handler.filter.Sort)             // default value
	assert.Equal(reliabilityAPIMaxNumTasksLimit+1, handler.filter.Limit) // default value
	assert.Equal(handler.filter.Significance, 0.1)

}

func TestParse(t *testing.T) {
	assert := assert.New(t)

	url := getURL(projectID, []string{"aggregation_expression_multiversion_fuzzer"})
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
	url := getURL(projectID, []string{"no_such_task"})

	sc := &data.MockConnector{
		MockTaskReliabilityConnector: data.MockTaskReliabilityConnector{},
		URL:                          url,
	}
	handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
	assert.NoError(err)

	handler.filter = reliability.TaskReliabilityFilter{Limit: 1}

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
	url := getURL(projectID, []string{"aggregation_expression_multiversion_fuzzer"})

	sc := &data.MockConnector{
		MockTaskReliabilityConnector: data.MockTaskReliabilityConnector{},
		URL:                          url,
	}
	handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
	assert.NoError(err)

	// 1 document will be returned
	sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", 100)

	handler.filter = reliability.TaskReliabilityFilter{Limit: 1}

	// code subtracts 1
	handler.filter.Limit++
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
	url := getURL(projectID, []string{"aggregation_expression_multiversion_fuzzer"})

	sc := &data.MockConnector{
		MockTaskReliabilityConnector: data.MockTaskReliabilityConnector{},
		URL:                          url,
	}
	handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
	assert.NoError(err)

	// 1000 documents will be returned
	sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", 1001)

	handler.filter = reliability.TaskReliabilityFilter{Limit: 1000}

	// code subtracts 1
	handler.filter.Limit++
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
	url := getURL(projectID, []string{"aggregation_expression_multiversion_fuzzer"})

	sc := &data.MockConnector{
		MockTaskReliabilityConnector: data.MockTaskReliabilityConnector{},
		URL:                          url,
	}
	handler := makeGetProjectTaskReliability(sc).(*taskReliabilityHandler)
	assert.NoError(err)

	// 100 documents will be returned
	sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", 100)
	handler.filter = reliability.TaskReliabilityFilter{Limit: 101}

	resp := handler.Run(context.Background())

	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	assert.Nil(resp.Pages())

	// 101 documents will be returned
	sc.MockTaskReliabilityConnector.SetTaskReliabilityScores("aggregation_expression_multiversion_fuzzer", 101)
	handler.filter = reliability.TaskReliabilityFilter{Limit: 101}

	resp = handler.Run(context.Background())

	assert.NotNil(resp)
	assert.Equal(http.StatusOK, resp.Status())
	assert.NotNil(resp.Pages())
	lastDoc := sc.MockTaskReliabilityConnector.CachedTaskReliability[100]
	assert.Equal(lastDoc.StartAtKey(), resp.Pages().Next.Key)
}
