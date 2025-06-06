package task

import (
	"context"
	"regexp"
	"sort"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// TestResultOutput is the versioned entry point for coordinating persistent
// storage of a task run's test result data.
type TestResultOutput struct {
	Version      int                    `bson:"version" json:"version"`
	BucketConfig evergreen.BucketConfig `bson:"bucket_config" json:"bucket_config"`

	AWSCredentials aws.CredentialsProvider `bson:"-" json:"-"`
}

// AppendTestResults appends test results for the given task run.
func (o TestResultOutput) AppendTestResults(ctx context.Context, env evergreen.Environment, testResults []testresult.TestResult) error {
	svc, err := o.getTestResultService(env)
	if err != nil {
		return errors.Wrap(err, "getting test result service")
	}

	return svc.AppendTestResults(ctx, testResults)
}

// GetMergedTaskTestResults returns test results belonging to the specified task run.
func (o TestResultOutput) GetMergedTaskTestResults(ctx context.Context, env evergreen.Environment, tasks []Task, getOpts *FilterOptions) (testresult.TaskTestResults, error) {
	svc, err := o.getTestResultService(env)
	if err != nil {
		return testresult.TaskTestResults{}, errors.Wrap(err, "getting test results service")
	}

	var baseTasks []Task
	if o.Version == 0 && getOpts != nil {
		baseTasks = getOpts.BaseTasks
	}
	allTestResults, err := svc.GetTaskTestResults(ctx, tasks, baseTasks)
	if err != nil {
		return testresult.TaskTestResults{}, errors.Wrap(err, "getting test results")
	}

	var mergedTaskResults testresult.TaskTestResults
	for _, taskResults := range allTestResults {
		mergedTaskResults.Stats.TotalCount += taskResults.Stats.TotalCount
		mergedTaskResults.Stats.FailedCount += taskResults.Stats.FailedCount
		mergedTaskResults.Results = append(mergedTaskResults.Results, taskResults.Results...)
	}

	filteredResults, filteredCount, err := o.filterAndSortTestResults(ctx, env, mergedTaskResults.Results, getOpts)
	if err != nil {
		return testresult.TaskTestResults{}, err
	}
	mergedTaskResults.Results = filteredResults
	mergedTaskResults.Stats.FilteredCount = &filteredCount

	// TODO: DEVPROD-16200 Download test results from s3 based on the bucket config of each individual task.
	return mergedTaskResults, nil
}

// GetTaskTestResultsStats returns test results belonging to the specified task run.
func (o TestResultOutput) GetTaskTestResultsStats(ctx context.Context, env evergreen.Environment, tasks []Task) (testresult.TaskTestResultsStats, error) {
	svc, err := o.getTestResultService(env)
	if err != nil {
		return testresult.TaskTestResultsStats{}, errors.Wrap(err, "getting test results service")
	}

	return svc.GetTaskTestResultsStats(ctx, tasks)
}

// GetFailedTestSamples returns failed test samples filtered as specified by
// the optional regex filters for each task specified.
func GetFailedTestSamples(ctx context.Context, env evergreen.Environment, tasks []Task, regexFilters []string) ([]testresult.TaskTestResultsFailedSample, error) {
	if len(tasks) == 0 {
		return nil, errors.New("must specify task options")
	}

	var allSamples []testresult.TaskTestResultsFailedSample
	for service, tasksByService := range groupTasksByService(tasks) {
		svc, err := GetServiceImpl(env, service)
		if err != nil {
			return nil, errors.Wrap(err, "getting test result service")
		}
		// TODO: DEVPROD-17978 Cedar does not have a way to return unmerged test results via API, so we need to keep
		// GetFailedTestSamples as an available interface function until we shutdown cedar.
		var samples []testresult.TaskTestResultsFailedSample
		var allTaskResults []testresult.TaskTestResults
		if service == TestResultsServiceCedar {
			samples, err = svc.GetFailedTestSamples(ctx, tasksByService, regexFilters)
		} else {
			allTaskResults, err = svc.GetTaskTestResults(ctx, tasksByService, nil)
			if err != nil {
				return nil, errors.Wrap(err, "getting test results")
			}
			samples, err = getFailedTestSamples(allTaskResults, regexFilters)
		}
		if err != nil {
			return nil, errors.Wrap(err, "getting failed test result samples")
		}
		allSamples = append(allSamples, samples...)
	}
	return allSamples, nil
}

func getFailedTestSamples(allTaskResults []testresult.TaskTestResults, regexFilters []string) ([]testresult.TaskTestResultsFailedSample, error) {
	regexes := make([]*regexp.Regexp, len(regexFilters))
	for i, filter := range regexFilters {
		testNameRegex, err := regexp.Compile(filter)
		if err != nil {
			return nil, errors.Wrap(err, "compiling regex")
		}
		regexes[i] = testNameRegex
	}

	samples := make([]testresult.TaskTestResultsFailedSample, len(allTaskResults))
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

func (o TestResultOutput) getTestResultService(env evergreen.Environment) (TestResultsService, error) {
	if o.Version == 0 {
		return NewCedarService(env), nil
	}
	return NewLocalService(env), nil
}

func groupTasksByService(tasks []Task) map[string][]Task {
	servicesToTasks := map[string][]Task{}
	for _, task := range tasks {
		servicesToTasks[task.ResultsService] = append(servicesToTasks[task.ResultsService], task)
	}
	return servicesToTasks
}

// filterAndSortTestResults takes a slice of test results and returns a
// filtered, sorted, and paginated version of that slice.
func (o TestResultOutput) filterAndSortTestResults(ctx context.Context, env evergreen.Environment, results []testresult.TestResult, opts *FilterOptions) ([]testresult.TestResult, int, error) {
	if opts == nil {
		return results, len(results), nil
	}
	if err := validateFilterOptions(opts); err != nil {
		return nil, 0, errors.Wrap(err, "invalid filter options")
	}

	baseStatusMap := map[string]string{}
	if len(opts.BaseTasks) > 0 {
		baseResults, err := o.GetMergedTaskTestResults(ctx, env, opts.BaseTasks, nil)
		if err != nil {
			return nil, 0, errors.Wrap(err, "getting base test results")
		}
		for _, result := range baseResults.Results {
			baseStatusMap[result.GetDisplayTestName()] = result.Status
		}
	}

	var err error
	results, err = filterTestResults(results, opts)
	if err != nil {
		return nil, 0, errors.Wrap(err, "filtering test results")
	}
	sortTestResults(results, opts, baseStatusMap)

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

func validateFilterOptions(opts *FilterOptions) error {
	catcher := grip.NewBasicCatcher()

	seenSortByKeys := map[string]bool{}
	for _, sortBy := range opts.Sort {
		switch sortBy.Key {
		case testresult.SortByStartKey, testresult.SortByDurationKey, testresult.SortByTestNameKey, testresult.SortByStatusKey, testresult.SortByBaseStatusKey, "":
		default:
			catcher.Errorf("unrecognized sort by key '%s'", sortBy.Key)
			continue
		}

		if seenSortByKeys[sortBy.Key] {
			catcher.Errorf("duplicate sort by key '%s'", sortBy.Key)
		} else {
			catcher.NewWhen(sortBy.Key == testresult.SortByBaseStatusKey && len(opts.BaseTasks) == 0, "must specify base task ID when sorting by base status")
		}

		seenSortByKeys[sortBy.Key] = true
	}

	catcher.NewWhen(opts.Limit < 0, "limit cannot be negative")
	catcher.NewWhen(opts.Page < 0, "page cannot be negative")
	catcher.NewWhen(opts.Limit == 0 && opts.Page > 0, "cannot specify a page without a limit")

	return catcher.Resolve()
}

func filterTestResults(results []testresult.TestResult, opts *FilterOptions) ([]testresult.TestResult, error) {
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

	var filteredResults []testresult.TestResult
	for _, result := range results {
		if testNameRegex != nil {
			if opts.ExcludeDisplayNames {
				if !testNameRegex.MatchString(result.TestName) {
					continue
				}
			} else if !testNameRegex.MatchString(result.GetDisplayTestName()) {
				continue
			}
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

func sortTestResults(results []testresult.TestResult, opts *FilterOptions, baseStatusMap map[string]string) {
	sort.SliceStable(results, func(i, j int) bool {
		for _, sortBy := range opts.Sort {
			switch sortBy.Key {
			case testresult.SortByStartKey:
				if results[i].TestStartTime == results[j].TestStartTime {
					continue
				}
				if sortBy.OrderDSC {
					return results[i].TestStartTime.After(results[j].TestStartTime)
				}
				return results[i].TestStartTime.Before(results[j].TestStartTime)
			case testresult.SortByDurationKey:
				if results[i].Duration() == results[j].Duration() {
					continue
				}
				if sortBy.OrderDSC {
					return results[i].Duration() > results[j].Duration()
				}
				return results[i].Duration() < results[j].Duration()
			case testresult.SortByTestNameKey:
				if results[i].GetDisplayTestName() == results[j].GetDisplayTestName() {
					continue
				}
				if sortBy.OrderDSC {
					return results[i].GetDisplayTestName() > results[j].GetDisplayTestName()
				}
				return results[i].GetDisplayTestName() < results[j].GetDisplayTestName()
			case testresult.SortByStatusKey:
				if results[i].Status == results[j].Status {
					continue
				}
				if sortBy.OrderDSC {
					return results[i].Status > results[j].Status
				}
				return results[i].Status < results[j].Status
			case testresult.SortByBaseStatusKey:
				if baseStatusMap[results[i].GetDisplayTestName()] == baseStatusMap[results[j].GetDisplayTestName()] {
					continue
				}
				if sortBy.OrderDSC {
					return baseStatusMap[results[i].GetDisplayTestName()] > baseStatusMap[results[j].GetDisplayTestName()]
				}
				return baseStatusMap[results[i].GetDisplayTestName()] < baseStatusMap[results[j].GetDisplayTestName()]
			}
		}

		return false
	})
}
