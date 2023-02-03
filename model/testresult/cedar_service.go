package testresult

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/testresults"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

type cedarService struct {
	baseURL string
}

// NewCedarTestResultsService returns a Cedar backed test results service
// implementation.
func NewCedarTestResultsService(env evergreen.Environment) testResultsService {
	cedarSettings := env.Settings().Cedar
	httpScheme := "https"
	if cedarSettings.Insecure {
		httpScheme = "http"
	}

	return &cedarService{baseURL: fmt.Sprintf("%s://%s", httpScheme, cedarSettings.BaseURL)}
}

func (s *cedarService) GetMergedTaskTestResults(ctx context.Context, taskOpts []TaskOptions, filterOpts *FilterOptions) (TaskTestResults, error) {
	data, status, err := testresults.Get(ctx, s.convertFilterOpts(taskOpts[0], filterOpts))
	if err != nil {
		return TaskTestResults{}, errors.Wrap(err, "getting test results from Cedar")
	}
	if status == http.StatusNotFound {
		return TaskTestResults{}, nil
	}
	if status != http.StatusOK {
		return TaskTestResults{}, errors.Errorf("getting test results from Cedar returned HTTP status '%d'", status)
	}

	var testResults TaskTestResults
	if err := json.Unmarshal(data, &testResults); err != nil {
		return TaskTestResults{}, errors.Wrap(err, "unmarshalling test results from Cedar")
	}

	return testResults, nil
}

func (s *cedarService) GetMergedTaskTestResultsStats(ctx context.Context, taskOpts []TaskOptions) (TaskTestResultsStats, error) {
	opts := s.convertFilterOpts(taskOpts[0], nil)
	opts.Stats = true
	data, status, err := testresults.Get(ctx, opts)
	if err != nil {
		return TaskTestResultsStats{}, errors.Wrap(err, "getting test results stats from Cedar")
	}
	if status == http.StatusNotFound {
		return TaskTestResultsStats{}, nil
	}
	if status != http.StatusOK {
		return TaskTestResultsStats{}, errors.Errorf("getting test results stats from Cedar returned HTTP status '%d'", status)
	}

	var stats TaskTestResultsStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return TaskTestResultsStats{}, errors.Wrap(err, "unmarshalling test results stats from Cedar")
	}

	return stats, nil
}

func (s *cedarService) GetFailedTestSamples(ctx context.Context, taskOpts []TaskOptions, regexFilters []string) ([]TaskTestResultsFailedSample, error) {
	opts := testresults.GetFailedSampleOptions{
		Cedar: timber.GetOptions{
			BaseURL: s.baseURL,
		},
		SampleOptions: testresults.FailedTestSampleOptions{
			RegexFilters: regexFilters,
		},
	}
	for i, t := range taskOpts {
		opts.SampleOptions.Tasks[i] = testresults.TaskInfo{
			TaskID:    t.TaskID,
			Execution: t.Execution,
		}
	}

	data, err := testresults.GetFailedSamples(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "getting failed test samples from Cedar")
	}

	var samples []TaskTestResultsFailedSample
	if err := json.Unmarshal(data, &samples); err != nil {
		return nil, errors.Wrap(err, "unmarshalling failed test samples from Cedar")
	}

	return samples, nil
}

func (s *cedarService) convertFilterOpts(taskOpts TaskOptions, filterOpts *FilterOptions) testresults.GetOptions {
	if filterOpts == nil {
		filterOpts = &FilterOptions{}
	}

	return testresults.GetOptions{
		Cedar: timber.GetOptions{
			BaseURL: s.baseURL,
		},
		TaskID:       taskOpts.TaskID,
		Execution:    utility.ToIntPtr(taskOpts.Execution),
		TestName:     filterOpts.TestName,
		Statuses:     filterOpts.Statuses,
		GroupID:      filterOpts.GroupID,
		SortBy:       filterOpts.SortBy,
		SortOrderDSC: filterOpts.SortOrderDSC,
		BaseTaskID:   filterOpts.BaseTaskID,
		Limit:        filterOpts.Limit,
		Page:         filterOpts.Page,
	}
}
