package task

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/timber"
	"github.com/evergreen-ci/timber/testresults"
	"github.com/pkg/errors"
)

// cedarService implements the test results service interface for test results
// stored in Cedar.
type cedarService struct {
	baseURL string
}

// NewCedarService returns a Cedar backed test results service implementation.
func NewCedarService(env evergreen.Environment) *cedarService {
	cedarSettings := env.Settings().Cedar
	httpScheme := "https"
	if cedarSettings.Insecure {
		httpScheme = "http"
	}

	return &cedarService{baseURL: fmt.Sprintf("%s://%s", httpScheme, cedarSettings.BaseURL)}
}

func (s *cedarService) AppendTestResultMetadata(ctx context.Context, failedTestSample []string, failedCount int, totalResults int, tr testresult.DbTaskTestResults) error {
	return errors.New("not implemented")
}

func (s *cedarService) GetTaskTestResults(ctx context.Context, taskOpts []Task, baseTasks []Task) ([]testresult.TaskTestResults, error) {
	var filterOpts *FilterOptions
	if len(baseTasks) > 0 {
		filterOpts = &FilterOptions{
			BaseTasks: baseTasks,
		}
	}
	data, status, err := testresults.Get(ctx, s.convertOpts(taskOpts, filterOpts))
	if err != nil {
		return nil, errors.Wrap(err, "getting test results from Cedar")
	}
	if status != http.StatusOK {
		return nil, errors.Errorf("getting test results from Cedar returned HTTP status '%d'", status)
	}

	var testResults testresult.TaskTestResults
	if err := json.Unmarshal(data, &testResults); err != nil {
		return nil, errors.Wrap(err, "unmarshalling test results from Cedar")
	}

	return []testresult.TaskTestResults{testResults}, nil
}

func (s *cedarService) GetTaskTestResultsStats(ctx context.Context, taskOpts []Task) (testresult.TaskTestResultsStats, error) {
	opts := s.convertOpts(taskOpts, nil)
	opts.Stats = true
	data, status, err := testresults.Get(ctx, opts)
	if err != nil {
		return testresult.TaskTestResultsStats{}, errors.Wrap(err, "getting test results stats from Cedar")
	}
	if status != http.StatusOK {
		return testresult.TaskTestResultsStats{}, errors.Errorf("getting test results stats from Cedar returned HTTP status '%d'", status)
	}

	var stats testresult.TaskTestResultsStats
	if err := json.Unmarshal(data, &stats); err != nil {
		return testresult.TaskTestResultsStats{}, errors.Wrap(err, "unmarshalling test results stats from Cedar")
	}

	return stats, nil
}

func (s *cedarService) GetFailedTestSamples(ctx context.Context, taskOpts []Task, regexFilters []string) ([]testresult.TaskTestResultsFailedSample, error) {
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
		opts.SampleOptions.Tasks[i].TaskID = t.Id
		opts.SampleOptions.Tasks[i].Execution = t.Execution
	}

	data, err := testresults.GetFailedSamples(ctx, opts)
	if err != nil {
		return nil, errors.Wrap(err, "getting failed test samples from Cedar")
	}

	var samples []testresult.TaskTestResultsFailedSample
	if err := json.Unmarshal(data, &samples); err != nil {
		return nil, errors.Wrap(err, "unmarshalling failed test samples from Cedar")
	}

	return samples, nil
}

func (s *cedarService) convertOpts(taskOpts []Task, filterOpts *FilterOptions) testresults.GetOptions {
	cedarTaskOpts := make([]testresults.TaskOptions, len(taskOpts))
	for i, task := range taskOpts {
		cedarTaskOpts[i].TaskID = task.Id
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
				TaskID:    task.Id,
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
