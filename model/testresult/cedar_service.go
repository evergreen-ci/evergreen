package testresult

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/testresults"
	"github.com/pkg/errors"
)

// cedarService implements the test results service interface for test results
// stored in Cedar.
type cedarService struct {
	baseURL string
}

// newCedarService returns a Cedar backed test results service implementation.
func newCedarService(env evergreen.Environment) *cedarService {
	cedarSettings := env.Settings().Cedar
	httpScheme := "https"
	if cedarSettings.Insecure {
		httpScheme = "http"
	}

	return &cedarService{baseURL: fmt.Sprintf("%s://%s", httpScheme, cedarSettings.BaseURL)}
}

func (s *cedarService) GetMergedTaskTestResults(ctx context.Context, taskOpts []TaskOptions, filterOpts *FilterOptions) (TaskTestResults, error) {
	data, status, err := testresults.Get(ctx, s.convertOpts(taskOpts, filterOpts))
	if err != nil {
		return TaskTestResults{}, errors.Wrap(err, "getting test results from Cedar")
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
	opts := s.convertOpts(taskOpts, nil)
	opts.Stats = true
	data, status, err := testresults.Get(ctx, opts)
	if err != nil {
		return TaskTestResultsStats{}, errors.Wrap(err, "getting test results stats from Cedar")
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

func (s *cedarService) GetMergedFailedTestSample(ctx context.Context, taskOpts []TaskOptions) ([]string, error) {
	opts := s.convertOpts(taskOpts, nil)
	opts.FailedSample = true
	data, status, err := testresults.Get(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "getting failed test sample from Cedar")
	}
	if status != http.StatusOK {
		return nil, errors.Errorf("getting failed test sample from Cedar returned HTTP status '%d'", status)
	}

	var sample []string
	if err := json.Unmarshal(data, &sample); err != nil {
		return nil, errors.Wrap(err, "unmarshalling failed test sample from Cedar")
	}

	return sample, nil
}

func (s *cedarService) GetFailedTestSamples(ctx context.Context, taskOpts []TaskOptions, regexFilters []string) ([]TaskTestResultsFailedSample, error) {
	opts := testresults.GetFailedSampleOptions{
		Cedar: timber.GetOptions{
			BaseURL: s.baseURL,
		},
		SampleOptions: testresults.FailedTestSampleOptions{
			Tasks:        make([]testresults.TaskInfo, len(taskOpts)),
			RegexFilters: regexFilters,
		},
	}
	for i, t := range taskOpts {
		opts.SampleOptions.Tasks[i].TaskID = t.TaskID
		opts.SampleOptions.Tasks[i].Execution = t.Execution
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

func (s *cedarService) convertOpts(taskOpts []TaskOptions, filterOpts *FilterOptions) testresults.GetOptions {
	cedarTaskOpts := make([]testresults.TaskOptions, len(taskOpts))
	for i, task := range taskOpts {
		cedarTaskOpts[i].TaskID = task.TaskID
		cedarTaskOpts[i].Execution = task.Execution
	}

	var cedarFilterOpts *testresults.FilterOptions
	if filterOpts != nil {
		var sort []testresults.SortBy
		for _, sortBy := range filterOpts.Sort {
			sort = append(sort, testresults.SortBy{
				Key:      sortBy.Key,
				OrderDSC: sortBy.OrderDSC,
			})
		}
		var baseTasks []testresults.TaskOptions
		for _, task := range filterOpts.BaseTasks {
			baseTasks = append(baseTasks, testresults.TaskOptions{
				TaskID:    task.TaskID,
				Execution: task.Execution,
			})
		}
		cedarFilterOpts = &testresults.FilterOptions{
			TestName:            filterOpts.TestName,
			ExcludeDisplayNames: filterOpts.ExcludeDisplayNames,
			Statuses:            filterOpts.Statuses,
			GroupID:             filterOpts.GroupID,
			Sort:                sort,
			Limit:               filterOpts.Limit,
			Page:                filterOpts.Page,
			BaseTasks:           baseTasks,
		}
	}

	return testresults.GetOptions{
		Cedar: timber.GetOptions{
			BaseURL: s.baseURL,
		},
		Tasks:  cedarTaskOpts,
		Filter: cedarFilterOpts,
	}
}
