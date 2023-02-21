package testresult

import (
	"context"
	"regexp"
	"sort"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
	"go.mongodb.org/mongo-driver/mongo/options"
)

const maxSampleSize = 10

// ClearLocal clears the local test results store.
func ClearLocal(ctx context.Context, env evergreen.Environment) error {
	return errors.Wrap(env.DB().Collection(Collection).Drop(ctx), "clearing the local test results store")
}

// InsertLocal inserts the given test results into the local test results store
// for testing and local development.
func InsertLocal(ctx context.Context, env evergreen.Environment, results ...TestResult) error {
	return errors.Wrap(appendDBResults(ctx, env, results), "inserting local test results")
}

// localService implements the local test results service.
type localService struct {
	env evergreen.Environment
}

// newLocalService returns a local test results service implementation.
func newLocalService(env evergreen.Environment) *localService {
	return &localService{env: env}
}

func (s *localService) GetMergedTaskTestResults(ctx context.Context, taskOpts []TaskOptions, filterOpts *FilterOptions) (TaskTestResults, error) {
	allTaskResults, err := s.get(ctx, taskOpts)
	if err != nil {
		return TaskTestResults{}, errors.Wrap(err, "getting local test results")
	}

	var mergedTaskResults TaskTestResults
	for _, taskResults := range allTaskResults {
		mergedTaskResults.Stats.TotalCount += taskResults.Stats.TotalCount
		mergedTaskResults.Stats.FailedCount += taskResults.Stats.FailedCount
		mergedTaskResults.Results = append(mergedTaskResults.Results, taskResults.Results...)
	}

	filteredResults, filteredCount, err := s.filterAndSortTestResults(ctx, mergedTaskResults.Results, filterOpts)
	if err != nil {
		return TaskTestResults{}, err
	}
	mergedTaskResults.Results = filteredResults
	mergedTaskResults.Stats.FilteredCount = &filteredCount

	return mergedTaskResults, nil
}

func (s *localService) GetMergedTaskTestResultsStats(ctx context.Context, taskOpts []TaskOptions) (TaskTestResultsStats, error) {
	allTaskResults, err := s.get(ctx, taskOpts, statsKey)
	if err != nil {
		return TaskTestResultsStats{}, errors.Wrap(err, "getting local test results")
	}

	var mergedStats TaskTestResultsStats
	for _, taskResults := range allTaskResults {
		mergedStats.TotalCount += taskResults.Stats.TotalCount
		mergedStats.FailedCount += taskResults.Stats.FailedCount
	}

	return mergedStats, nil
}

func (s *localService) GetMergedFailedTestSample(ctx context.Context, taskOpts []TaskOptions) ([]string, error) {
	mergedTaskResults, err := s.GetMergedTaskTestResults(ctx, taskOpts, &FilterOptions{Statuses: []string{evergreen.TestFailedStatus}})
	if err != nil {
		return nil, errors.Wrap(err, "getting failed test results")
	}

	sampleSize := maxSampleSize
	if len(mergedTaskResults.Results) < sampleSize {
		sampleSize = len(mergedTaskResults.Results)
	}
	var mergedSample []string
	for i := 0; i < sampleSize; i++ {
		mergedSample = append(mergedSample, mergedTaskResults.Results[i].GetDisplayTestName())
	}

	return mergedSample, nil
}

func (s *localService) GetFailedTestSamples(ctx context.Context, taskOpts []TaskOptions, regexFilters []string) ([]TaskTestResultsFailedSample, error) {
	allTaskResults, err := s.get(ctx, taskOpts, resultsKey)
	if err != nil {
		return nil, errors.Wrap(err, "getting local test results")
	}

	regexes := make([]*regexp.Regexp, len(regexFilters))
	for _, filter := range regexFilters {
		testNameRegex, err := regexp.Compile(filter)
		if err != nil {
			return nil, errors.Wrap(err, "compiling regex")
		}
		regexes = append(regexes, testNameRegex)
	}

	samples := make([]TaskTestResultsFailedSample, len(allTaskResults))
	for i, taskResults := range allTaskResults {
		samples[i].TaskID = taskResults.Results[0].TaskID
		samples[i].Execution = taskResults.Results[0].Execution

		if taskResults.Stats.FailedCount == 0 {
			continue
		}

		samples[i].TotalFailedNames = taskResults.Stats.FailedCount
		for _, result := range taskResults.Results {
			if result.Status == evergreen.TestFailedStatus {
				match := true
				for _, regex := range regexes {
					if match = regex.MatchString(result.GetDisplayTestName()); match {
						break
					}
				}
				if match {
					samples[i].MatchingFailedTestNames = append(samples[i].MatchingFailedTestNames, result.GetDisplayTestName())
				}
			}
		}
	}

	return samples, nil
}

func (s *localService) get(ctx context.Context, taskOpts []TaskOptions, fields ...string) ([]TaskTestResults, error) {
	ids := make([]dbTaskTestResultsID, len(taskOpts))
	for i, task := range taskOpts {
		ids[i].TaskID = task.TaskID
		ids[i].Execution = task.Execution
	}

	filter := bson.M{idKey: bson.M{"$in": ids}}
	opts := options.Find()
	opts.SetSort(bson.D{{taskIDKey, 1}, {executionKey, 1}})
	if len(fields) > 0 {
		projection := bson.M{}
		for _, field := range fields {
			projection[field] = 1
		}
		opts.SetProjection(projection)
	}

	var allDBTaskResults []dbTaskTestResults
	cur, err := s.env.DB().Collection(Collection).Find(ctx, filter, opts)
	if err != nil {
		return nil, errors.Wrap(err, "finding DB test results")
	}
	if err = cur.All(ctx, &allDBTaskResults); err != nil {
		return nil, errors.Wrap(err, "reading DB test results")
	}

	allTaskResults := make([]TaskTestResults, len(allDBTaskResults))
	for i, dbTaskResults := range allDBTaskResults {
		allTaskResults[i].Stats = dbTaskResults.Stats
		allTaskResults[i].Results = dbTaskResults.Results
	}

	return allTaskResults, nil
}

// filterAndSortTestResults takes a slice of test results and returns a
// filtered, sorted, and paginated version of that slice.
func (s *localService) filterAndSortTestResults(ctx context.Context, results []TestResult, opts *FilterOptions) ([]TestResult, int, error) {
	if opts == nil {
		return results, len(results), nil
	}
	if err := s.validateFilterOptions(opts); err != nil {
		return nil, 0, errors.Wrap(err, "invalid filter options")
	}

	baseStatusMap := map[string]string{}
	baseResults, err := s.GetMergedTaskTestResults(ctx, opts.BaseTasks, nil)
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

func (s *localService) validateFilterOptions(opts *FilterOptions) error {
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

func (s *localService) filterTestResults(results []TestResult, opts *FilterOptions) ([]TestResult, error) {
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

func (s *localService) sortTestResults(results []TestResult, opts *FilterOptions, baseStatusMap map[string]string) {
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
