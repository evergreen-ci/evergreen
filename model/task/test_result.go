package task

import (
	"bytes"
	"context"
	"github.com/evergreen-ci/evergreen/apimodels"
	"io"
	"regexp"
	"sort"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/pail"
	"github.com/evergreen-ci/utility"
	goparquet "github.com/fraugster/parquet-go"
	"github.com/fraugster/parquet-go/floor"
	"github.com/fraugster/parquet-go/parquetschema"
	"github.com/fraugster/parquet-go/parquetschema/autoschema"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

var ParquetTestResultsSchemaDef *parquetschema.SchemaDefinition

func init() {
	var err error
	ParquetTestResultsSchemaDef, err = autoschema.GenerateSchema(new(testresult.ParquetTestResults))
	if err != nil {
		panic(errors.Wrap(err, "generating Parquet test results schema definition"))
	}
}

// TestResultOutput is the versioned entry point for coordinating persistent
// storage of a task run's test result data.
type TestResultOutput struct {
	Version      int                    `bson:"version" json:"version"`
	BucketConfig evergreen.BucketConfig `bson:"bucket_config" json:"bucket_config"`

	AWSCredentials aws.CredentialsProvider `bson:"-" json:"-"`
}

// AppendTestResultMetadata appends test result metadata for the given task run.
func AppendTestResultMetadata(ctx context.Context, t *Task, env evergreen.Environment, failedTestSample []string, failedCount int, totalResults int, record testresult.DbTaskTestResults) error {
	output, ok := t.GetTaskOutputSafe()
	if !ok {
		return nil
	}
	svc, err := getTestResultService(env, output.TestResults.Version)
	if err != nil {
		return errors.Wrap(err, "getting test result service")
	}

	return svc.AppendTestResultMetadata(ctx, failedTestSample, failedCount, totalResults, record)
}

// getMergedTaskTestResults returns test results belonging to the specified task run.
func getMergedTaskTestResults(ctx context.Context, env evergreen.Environment, tasks []Task, getOpts *FilterOptions) (testresult.TaskTestResults, error) {
	if len(tasks) == 0 {
		return testresult.TaskTestResults{}, nil
	}
	output, ok := tasks[0].GetTaskOutputSafe()
	if !ok {
		return testresult.TaskTestResults{}, nil
	}
	svc, err := getTestResultService(env, output.TestResults.Version)
	if err != nil {
		return testresult.TaskTestResults{}, errors.Wrap(err, "getting test results service")
	}

	var baseTasks []Task
	if output.TestResults.Version == 0 && getOpts != nil {
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

	filteredResults, filteredCount, err := filterAndSortTestResults(ctx, env, mergedTaskResults.Results, getOpts)
	if err != nil {
		return testresult.TaskTestResults{}, err
	}
	mergedTaskResults.Results = filteredResults
	mergedTaskResults.Stats.FilteredCount = &filteredCount

	return mergedTaskResults, nil
}

// getTaskTestResultsStats returns test results belonging to the specified task run.
func getTaskTestResultsStats(ctx context.Context, env evergreen.Environment, tasks []Task) (testresult.TaskTestResultsStats, error) {
	if len(tasks) == 0 {
		return testresult.TaskTestResultsStats{}, nil
	}
	output, ok := tasks[0].GetTaskOutputSafe()
	if !ok {
		return testresult.TaskTestResultsStats{}, nil
	}
	svc, err := getTestResultService(env, output.TestResults.Version)
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

func getTestResultService(env evergreen.Environment, version int) (TestResultsService, error) {
	if version == 0 {
		return NewCedarService(env), nil
	} else if version == 1 {
		return NewEvergreenService(env), nil
	} else {
		return NewLocalService(env), nil
	}
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
func filterAndSortTestResults(ctx context.Context, env evergreen.Environment, results []testresult.TestResult, opts *FilterOptions) ([]testresult.TestResult, int, error) {
	if opts == nil {
		return results, len(results), nil
	}
	if err := validateFilterOptions(opts); err != nil {
		return nil, 0, errors.Wrap(err, "invalid filter options")
	}

	baseStatusMap := map[string]string{}
	if len(opts.BaseTasks) > 0 {
		baseResults, err := getMergedTaskTestResults(ctx, env, opts.BaseTasks, nil)
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

// downloadParquet downloads test results in parquet format from s3.
func (o TestResultOutput) downloadParquet(ctx context.Context, credentials evergreen.S3Credentials, t *testresult.DbTaskTestResults) ([]testresult.TestResult, error) {
	bucket, err := o.getBucket(ctx, credentials)
	if err != nil {
		return nil, err
	}

	r, err := bucket.Get(ctx, testresult.PartitionKey(t.CreatedAt, t.Info.Project, t.ID))
	if err != nil {
		return nil, errors.Wrap(err, "getting Parquet test results")
	}
	defer func() {
		err = r.Close()
		grip.Warning(message.WrapError(err, message.Fields{
			"message":        "closing test results bucket reader",
			"test_result_id": t.ID,
			"task_id":        t.Info.TaskID,
			"execution":      t.Info.Execution,
			"project":        t.Info.Project,
		}))
	}()
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, errors.Wrap(err, "reading Parquet test results")
	}

	fr, err := goparquet.NewFileReader(bytes.NewReader(data))
	if err != nil {
		return nil, errors.Wrap(err, "creating Parquet reader")
	}
	pr := floor.NewReader(fr)
	defer func() {
		err = pr.Close()
		grip.Warning(message.WrapError(err, message.Fields{
			"message":        "closing Parquet test results reader",
			"test_result_id": t.ID,
			"task_id":        t.Info.TaskID,
			"execution":      t.Info.Execution,
			"project":        t.Info.Project,
		}))
	}()

	var parquetResults []testresult.ParquetTestResults
	for pr.Next() {
		row := testresult.ParquetTestResults{}
		if err := pr.Scan(&row); err != nil {
			return nil, errors.Wrap(err, "reading Parquet test results row")
		}

		parquetResults = append(parquetResults, row)
	}

	if err := pr.Err(); err != nil {
		return nil, errors.Wrap(err, "reading Parquet test results rows")
	}

	var results []testresult.TestResult
	for _, result := range parquetResults {
		results = append(results, result.ConvertToTestResultSlice()...)
	}
	return results, nil
}

// getBucket constructs an s3 bucket to retrieve test results from.
func (o TestResultOutput) getBucket(ctx context.Context, credentials evergreen.S3Credentials) (pail.Bucket, error) {
	bucket, err := o.createBucket(ctx, credentials, o.BucketConfig.TestResultsPrefix, false)
	if err != nil {
		return nil, errors.Wrap(err, "creating bucket")
	}

	return bucket, nil
}

// createBucket returns a Pail Bucket backed by PailType
func (o TestResultOutput) createBucket(ctx context.Context, credentials evergreen.S3Credentials, prefix string, compress bool) (pail.Bucket, error) {
	var b pail.Bucket
	var stsConfig aws.Config
	var err error

	switch o.BucketConfig.Type {
	case evergreen.BucketTypeS3:
		stsConfig, err = config.LoadDefaultConfig(ctx,
			config.WithRegion(evergreen.DefaultS3Region),
		)
		if err != nil {
			return nil, errors.Wrap(err, "loading config")
		}
		stsConfig.Credentials = pail.CreateAWSStaticCredentials(credentials.Key, credentials.Secret, "")
		stsClient := sts.NewFromConfig(stsConfig)
		opts := pail.S3Options{
			Name:        o.BucketConfig.Name,
			Prefix:      prefix,
			Region:      evergreen.DefaultS3Region,
			Permissions: pail.S3PermissionsPrivate,
			Credentials: stscreds.NewAssumeRoleProvider(stsClient, o.BucketConfig.RoleARN),
			MaxRetries:  utility.ToIntPtr(evergreen.DefaultS3MaxRetries),
			Compress:    compress,
		}
		b, err = pail.NewS3Bucket(ctx, opts)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	case evergreen.BucketTypeLocal:
		opts := pail.LocalOptions{
			Path:   o.BucketConfig.Name,
			Prefix: prefix,
		}
		b, err = pail.NewLocalBucket(opts)
		if err != nil {
			return nil, errors.WithStack(err)
		}
	default:
		return nil, errors.Errorf("unsupported bucket type: %s", o.BucketConfig.Type)
	}

	if err = b.Check(ctx); err != nil {
		return nil, errors.WithStack(err)
	}
	return b, nil
}

// UploadTestResults downloads existing test results from s3, appends new results to the response, and re-writes the appended results
// to the same s3 bucket in parquet format.
func UploadTestResults(ctx context.Context, credentials evergreen.S3Credentials, results []testresult.TestResult, output *TaskOutput,
	t Task, displayTaskInfo *apimodels.DisplayTaskInfo) (*testresult.DbTaskTestResults, error) {
	createdAt := time.Now()
	info := makeTestResultsInfo(t, displayTaskInfo)
	newResults := makeTestResults(&t, results)
	key := testresult.PartitionKey(createdAt, info.Project, info.ID())

	tr := &testresult.DbTaskTestResults{
		CreatedAt: createdAt,
		Info:      info,
	}
	allResults, err := output.TestResults.downloadParquet(ctx, credentials, tr)
	if err != nil && !pail.IsKeyNotFoundError(err) {
		return nil, errors.Wrap(err, "getting uploaded test results")
	}
	allResults = append(allResults, newResults...)

	if err = uploadParquet(ctx, credentials, *output, convertToParquet(allResults, info, createdAt), key); err != nil {
		return nil, errors.Wrap(err, "uploading parquet test results")
	}

	var failedCount int
	var failedTests []string
	for _, result := range results {
		if result.Status == evergreen.TestFailedStatus {
			failedTests = append(failedTests, result.GetDisplayTestName())
			failedCount++
		}
	}
	tr.Stats = testresult.TaskTestResultsStats{
		FailedCount: failedCount,
		TotalCount:  len(allResults),
	}
	tr.FailedTestsSample = failedTests
	return tr, nil
}

func uploadParquet(ctx context.Context, credentials evergreen.S3Credentials, output TaskOutput, results *testresult.ParquetTestResults, key string) error {
	bucket, err := output.TestResults.getBucket(ctx, credentials)
	if err != nil {
		return err
	}
	w, err := bucket.Writer(ctx, key)
	if err != nil {
		return errors.Wrap(err, "creating Presto bucket writer")
	}
	defer w.Close()

	pw := floor.NewWriter(goparquet.NewFileWriter(w, goparquet.WithSchemaDefinition(ParquetTestResultsSchemaDef)))
	defer pw.Close()

	return errors.Wrap(pw.Write(results), "writing Parquet test results")
}

func makeTestResultsInfo(t Task, displayTaskInfo *apimodels.DisplayTaskInfo) testresult.TestResultsInfo {
	return testresult.TestResultsInfo{
		Project:         t.Project,
		Version:         t.Version,
		Variant:         t.BuildVariant,
		TaskID:          t.Id,
		TaskName:        t.DisplayName,
		DisplayTaskID:   displayTaskInfo.ID,
		DisplayTaskName: displayTaskInfo.Name,
		Execution:       t.Execution,
		Requester:       t.Requester,
		Mainline:        !t.IsPatchRequest(),
	}
}

func convertToParquet(results []testresult.TestResult, info testresult.TestResultsInfo, createdAt time.Time) *testresult.ParquetTestResults {
	convertedResults := make([]testresult.ParquetTestResult, len(results))
	for i, result := range results {
		convertedResults[i] = createParquetTestResult(result)
	}

	parquetResults := &testresult.ParquetTestResults{
		Version:   info.Version,
		Variant:   info.Variant,
		TaskName:  info.TaskName,
		TaskID:    info.TaskID,
		Execution: int32(info.Execution),
		Requester: info.Requester,
		CreatedAt: createdAt.UTC(),
		Results:   convertedResults,
	}
	if info.DisplayTaskName != "" {
		parquetResults.DisplayTaskName = utility.ToStringPtr(info.DisplayTaskName)
	}
	if info.DisplayTaskID != "" {
		parquetResults.DisplayTaskID = utility.ToStringPtr(info.DisplayTaskID)
	}
	return parquetResults
}

func createParquetTestResult(t testresult.TestResult) testresult.ParquetTestResult {
	result := testresult.ParquetTestResult{
		TestName:       t.TestName,
		Status:         t.Status,
		LogInfo:        t.LogInfo,
		TaskCreateTime: t.TaskCreateTime.UTC(),
		TestStartTime:  t.TestStartTime.UTC(),
		TestEndTime:    t.TestEndTime.UTC(),
	}
	if t.DisplayTestName != "" {
		result.DisplayTestName = utility.ToStringPtr(t.DisplayTestName)
	}
	if t.GroupID != "" {
		result.GroupID = utility.ToStringPtr(t.GroupID)
	}
	if t.LogTestName != "" {
		result.LogTestName = utility.ToStringPtr(t.LogTestName)
	}
	if t.LogURL != "" {
		result.LogURL = utility.ToStringPtr(t.LogURL)
	}
	if t.RawLogURL != "" {
		result.RawLogURL = utility.ToStringPtr(t.RawLogURL)
	}
	if t.LogTestName != "" || t.LogURL != "" || t.RawLogURL != "" {
		result.LineNum = utility.ToInt32Ptr(int32(t.LineNum))
	}
	return result
}

func makeTestResults(t *Task, results []testresult.TestResult) []testresult.TestResult {
	var newResults []testresult.TestResult
	for _, r := range results {
		if r.DisplayTestName == "" {
			r.DisplayTestName = r.TestName
		}
		var logInfo *testresult.TestLogInfo
		if r.LogInfo != nil {
			logInfo = &testresult.TestLogInfo{
				LogName:       r.LogInfo.LogName,
				LineNum:       r.LogInfo.LineNum,
				RenderingType: r.LogInfo.RenderingType,
				Version:       r.LogInfo.Version,
			}
			logInfo.LogsToMerge = append(logInfo.LogsToMerge, r.LogInfo.LogsToMerge...)
		}

		newResults = append(newResults, testresult.TestResult{
			TestName:        utility.RandomString(),
			DisplayTestName: r.DisplayTestName,
			Status:          r.Status,
			LogInfo:         logInfo,
			GroupID:         r.GroupID,
			LogURL:          r.LogURL,
			RawLogURL:       r.RawLogURL,
			LineNum:         r.LineNum,
			TaskCreateTime:  t.CreateTime,
			TestStartTime:   r.TestStartTime,
			TestEndTime:     r.TestEndTime,
		})
	}

	return newResults
}
