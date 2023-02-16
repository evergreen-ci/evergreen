package testresult

import (
	"context"
	"regexp"
	"sort"
	"sync"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

const maxSampleSize = 10

var globalInMemStore = newInMemStore()

// ClearLocal resets the global in-memory test results store for testing and
// and local developement.
func ClearLocal() {
	globalInMemStore = newInMemStore()
}

// InsertLocal inserts the given test results into the global in-memory test
// results store for testing and local development.
func InsertLocal(results ...TestResult) {
	globalInMemStore.appendResults(results...)
}

// inMemStore is an in-memory test results store for testing and local
// development.
type inMemStore struct {
	results map[inMemKey]*TaskTestResults
	mu      sync.RWMutex
}

type inMemKey struct {
	taskID    string
	execution int
}

func newInMemStore() *inMemStore {
	return &inMemStore{results: map[inMemKey]*TaskTestResults{}}
}

func (s *inMemStore) appendResults(results ...TestResult) {
	s.mu.Lock()
	defer s.mu.Unlock()

	for _, result := range results {
		key := inMemKey{taskID: result.TaskID, execution: result.Execution}
		taskResults, ok := s.results[key]
		if !ok {
			taskResults = &TaskTestResults{}
			s.results[key] = taskResults
		}

		taskResults.Stats.TotalCount++
		if result.Status == evergreen.TestFailedStatus {
			taskResults.Stats.FailedCount++
		}
		taskResults.Results = append(taskResults.Results, result)
	}
}

func (s *inMemStore) get(taskID string, execution int) (*TaskTestResults, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	taskResults, ok := s.results[inMemKey{taskID: taskID, execution: execution}]
	return taskResults, ok
}

// inMemService implements the test results service for in-memory test results.
type inMemService struct {
	store *inMemStore
}

func newInMemService(store *inMemStore) *inMemService {
	return &inMemService{store: store}
}

func (s *inMemService) GetMergedTaskTestResults(_ context.Context, taskOpts []TaskOptions, filterOpts *FilterOptions) (TaskTestResults, error) {
	// Sort the tasks in order by (task ID, execution) to ensure that
	// paginated responses return consistent results.
	sort.SliceStable(taskOpts, func(i, j int) bool {
		if taskOpts[i].TaskID == taskOpts[j].TaskID {
			return taskOpts[i].Execution < taskOpts[j].Execution
		}
		return taskOpts[i].TaskID < taskOpts[j].TaskID
	})

	var mergedTaskResults TaskTestResults
	for _, task := range taskOpts {
		taskResults, ok := s.store.get(task.TaskID, task.Execution)
		if !ok {
			continue
		}

		mergedTaskResults.Stats.TotalCount += taskResults.Stats.TotalCount
		mergedTaskResults.Stats.FailedCount += taskResults.Stats.FailedCount
		mergedTaskResults.Results = append(mergedTaskResults.Results, taskResults.Results...)
	}

	filteredResults, filteredCount, err := s.filterAndSortTestResults(mergedTaskResults.Results, filterOpts)
	if err != nil {
		return TaskTestResults{}, err
	}
	mergedTaskResults.Results = filteredResults
	mergedTaskResults.Stats.FilteredCount = &filteredCount

	return mergedTaskResults, nil
}

func (s *inMemService) GetMergedTaskTestResultsStats(_ context.Context, taskOpts []TaskOptions) (TaskTestResultsStats, error) {
	var mergedStats TaskTestResultsStats
	for _, task := range taskOpts {
		results, ok := s.store.get(task.TaskID, task.Execution)
		if !ok {
			continue
		}

		mergedStats.TotalCount += results.Stats.TotalCount
		mergedStats.FailedCount += results.Stats.FailedCount
	}

	return mergedStats, nil
}

func (s *inMemService) GetMergedFailedTestSample(_ context.Context, taskOpts []TaskOptions) ([]string, error) {
	var mergedSample []string
Tasks:
	for _, task := range taskOpts {
		results, ok := s.store.get(task.TaskID, task.Execution)
		if !ok {
			continue
		}

		for _, result := range results.Results {
			if result.Status == evergreen.TestFailedStatus {
				mergedSample = append(mergedSample, result.GetDisplayTestName())
			}
			if len(mergedSample) == maxSampleSize {
				break Tasks
			}
		}
	}

	return mergedSample, nil
}

func (s *inMemService) GetFailedTestSamples(_ context.Context, _ []TaskOptions, _ []string) ([]TaskTestResultsFailedSample, error) {
	return nil, errors.New("not implemented")
}

// filterAndSortTestResults takes a slice of test results and returns a
// filtered, sorted, and paginated version of that slice.
func (s *inMemService) filterAndSortTestResults(results []TestResult, opts *FilterOptions) ([]TestResult, int, error) {
	if opts == nil {
		return results, len(results), nil
	}
	if err := s.validateFilterOptions(opts); err != nil {
		return nil, 0, errors.Wrap(err, "invalid filter options")
	}

	baseStatusMap := map[string]string{}
	baseResults, err := s.GetMergedTaskTestResults(nil, opts.BaseTasks, nil)
	if err != nil {
		return nil, 0, errors.Wrap(err, "getting base test results")
	}
	for _, result := range baseResults.Results {
		baseStatusMap[result.GetDisplayTestName()] = result.Status
	}

	results, err = s.filterTestResults(results, opts)
	if err != nil {
		return nil, 0, errors.Wrap(err, "filtering test results")
	}
	s.sortTestResults(results, opts, baseStatusMap)

	totalCount := len(results)
	if opts.Limit > 0 {
		offset := opts.Limit * opts.Page
		end := offset + opts.Limit
		if offset > totalCount {
			offset = totalCount
		}
		if end > totalCount {
			end = totalCount
		}
		results = results[offset:end]
	}

	for i := range results {
		results[i].BaseStatus = baseStatusMap[results[i].GetDisplayTestName()]
	}

	return results, totalCount, nil
}

func (s *inMemService) validateFilterOptions(opts *FilterOptions) error {
	catcher := grip.NewBasicCatcher()

	switch opts.SortBy {
	case SortByStart, SortByDuration, SortByTestName, SortByStatus, SortByBaseStatus, "":
	default:
		catcher.Errorf("unrecognized sort by criteria '%s'", opts.SortBy)
	}
	catcher.NewWhen(opts.Limit < 0, "limit cannot be negative")
	catcher.NewWhen(opts.Page < 0, "page cannot be negative")
	catcher.NewWhen(opts.Limit == 0 && opts.Page > 0, "cannot specify a page without a limit")

	return catcher.Resolve()
}

func (s *inMemService) filterTestResults(results []TestResult, opts *FilterOptions) ([]TestResult, error) {
	if opts.TestName == "" && len(opts.Statuses) == 0 && opts.GroupID == "" {
		return results, nil
	}

	var testNameRegex *regexp.Regexp
	if opts.TestName != "" {
		var err error
		testNameRegex, err = regexp.Compile(opts.TestName)
		if err != nil {
			return nil, errors.Wrap(err, "compiling test name filter regex")
		}
	}

	var filteredResults []TestResult
	for _, result := range results {
		if testNameRegex != nil && !testNameRegex.MatchString(result.GetDisplayTestName()) {
			continue
		}
		if len(opts.Statuses) > 0 && !utility.StringSliceContains(opts.Statuses, result.Status) {
			continue
		}
		if opts.GroupID != "" && opts.GroupID != result.GroupID {
			continue
		}

		filteredResults = append(filteredResults, result)
	}

	return filteredResults, nil
}

func (s *inMemService) sortTestResults(results []TestResult, opts *FilterOptions, baseStatusMap map[string]string) {
	switch opts.SortBy {
	case SortByStart:
		sort.SliceStable(results, func(i, j int) bool {
			if opts.SortOrderDSC {
				return results[i].Start.After(results[j].Start)
			}
			return results[i].Start.Before(results[j].Start)
		})
	case SortByDuration:
		sort.SliceStable(results, func(i, j int) bool {
			if opts.SortOrderDSC {
				return results[i].getDuration() > results[j].getDuration()
			}
			return results[i].getDuration() < results[j].getDuration()
		})
	case SortByTestName:
		sort.SliceStable(results, func(i, j int) bool {
			if opts.SortOrderDSC {
				return results[i].GetDisplayTestName() > results[j].GetDisplayTestName()
			}
			return results[i].GetDisplayTestName() < results[j].GetDisplayTestName()
		})
	case SortByStatus:
		sort.SliceStable(results, func(i, j int) bool {
			if opts.SortOrderDSC {
				return results[i].Status > results[j].Status
			}
			return results[i].Status < results[j].Status
		})
	case SortByBaseStatus:
		if len(baseStatusMap) == 0 {
			break
		}
		sort.SliceStable(results, func(i, j int) bool {
			if opts.SortOrderDSC {
				return baseStatusMap[results[i].GetDisplayTestName()] > baseStatusMap[results[j].GetDisplayTestName()]
			}
			return baseStatusMap[results[i].GetDisplayTestName()] < baseStatusMap[results[j].GetDisplayTestName()]
		})
	}
}
