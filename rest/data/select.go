package data

import (
	"context"
	stderrors "errors"
	"maps"
	"net"
	"net/http"
	"slices"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/model"
	testselection "github.com/evergreen-ci/test-selection-client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
	"golang.org/x/sync/errgroup"
)

// Test selection service endpoint identifiers.
const (
	SelectTestsEndpoint       = "select_tests"
	SelectKnownTestsEndpoint  = "select_known_tests"
	TransitionTestsEndpoint   = "transition_tests"
	TransitionTaskEndpoint    = "transition_task"
	TransitionVariantEndpoint = "transition_variant"
	GetTestsStateEndpoint     = "get_tests_state"
	GetVariantStateEndpoint   = "get_variant_state"
)

const (
	testSelectionWriteTimeout      = 10 * time.Second
	testSelectionSelectTimeout     = 10 * time.Second
	testSelectionStatusTimeout     = 4 * time.Second
	testSelectionDecorationTimeout = 5 * time.Second
)

// Using a shared HTTP client so we can reuse idle connections between TSS calls and reduce the number of reconnects.
var testSelectionHTTPClient = newTestSelectionHTTPClient()

func newTestSelectionHTTPClient() *http.Client {
	// Each request's overall duration is bounded by the caller's context deadline but increase the idle connection pool to reduce the number of reconnects.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 100

	return &http.Client{
		Transport: otelhttp.NewTransport(transport),
	}
}

func logTSSError(ctx context.Context, err error, resp *http.Response, duration time.Duration, info message.Fields) {
	// A 404 means the requested resource isn't tracked in the test selection
	// service yet, which is expected and too noisy for Splunk. The error is
	// still returned to the caller.
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return
	}
	if ctx.Err() == context.Canceled || stderrors.Is(err, context.Canceled) {
		return
	}
	if endpoint, ok := info["endpoint"].(string); ok && endpoint == GetTestsStateEndpoint && isTimeoutError(ctx, err) {
		return
	}
	if resp != nil {
		info["status"] = resp.StatusCode
	}
	info["duration_ms"] = duration.Milliseconds()
	info["timeout"] = isTimeoutError(ctx, err)
	info["error"] = err.Error()
	var openAPIErr *testselection.GenericOpenAPIError
	if errors.As(err, &openAPIErr) {
		info["response_body"] = string(openAPIErr.Body())
	}
	grip.Error(ctx, info)
}

func isTimeoutError(ctx context.Context, err error) bool {
	if ctx.Err() == context.DeadlineExceeded || stderrors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	return errors.As(err, &netErr) && netErr.Timeout()
}

// wrapTSSError wraps test selection service errors with the response body in the message. Callers handle logging and adding context.
func wrapTSSError(err error) error {
	if err == nil {
		return nil
	}
	var openAPIErr *testselection.GenericOpenAPIError
	if errors.As(err, &openAPIErr) {
		return errors.Wrapf(err, "forwarding request to test selection service: %s", string(openAPIErr.Body()))
	}
	return errors.Wrap(err, "forwarding request to test selection service")
}

// newTestSelectionClient constructs a new test selection client using the
// provided HTTP client.
func newTestSelectionClient(c *http.Client) *testselection.APIClient {
	tssBaseURL := evergreen.GetEnvironment().Settings().TestSelection.URL
	conf := testselection.NewConfiguration()
	conf.HTTPClient = c
	conf.Servers = testselection.ServerConfigurations{
		testselection.ServerConfiguration{
			URL:         tssBaseURL,
			Description: "Test selection service",
		},
	}
	return testselection.NewAPIClient(conf)
}

// SelectTests uses the test selection service to return a filtered set of tests
// to run based on the provided SelectTestsRequest. It returns the list of
// selected tests.
func SelectTests(ctx context.Context, req model.SelectTestsRequest) ([]string, error) {
	ctx, cancel := context.WithTimeout(ctx, testSelectionSelectTimeout)
	defer cancel()
	c := newTestSelectionClient(testSelectionHTTPClient)
	var strategies []testselection.StrategyEnum
	for _, s := range req.Strategies {
		strategies = append(strategies, testselection.StrategyEnum(s))
	}

	var (
		selectedTestPtrs []*string
		resp             *http.Response
		err              error
	)
	startAt := time.Now()
	endpoint := SelectTestsEndpoint
	if len(req.Tests) == 0 {
		endpoint = SelectKnownTestsEndpoint
		reqBody := testselection.NewBodySelectAllKnownTestsOfATaskWithDataInBodyApiTestSelectionSelectKnownTestsPost(
			req.Project,
			req.Requester,
			req.BuildVariant,
			req.TaskID,
			req.TaskName,
		)
		reqBody.SetStrategies(strategies)
		if req.DisplayTaskName != "" {
			reqBody.SetDisplayTaskName(req.DisplayTaskName)
		}
		selectedTestPtrs, resp, err = c.TestSelectionAPI.SelectAllKnownTestsOfATaskWithDataInBodyApiTestSelectionSelectKnownTestsPost(ctx).
			BodySelectAllKnownTestsOfATaskWithDataInBodyApiTestSelectionSelectKnownTestsPost(*reqBody).
			Execute()
	} else {
		reqBody := testselection.NewBodySelectTestsWithDataInBodyApiTestSelectionSelectTestsPost(
			req.Project,
			req.Requester,
			req.BuildVariant,
			req.TaskID,
			req.TaskName,
			req.Tests,
		)
		reqBody.SetStrategies(strategies)
		if req.DisplayTaskName != "" {
			reqBody.SetDisplayTaskName(req.DisplayTaskName)
		}
		selectedTestPtrs, resp, err = c.TestSelectionAPI.SelectTestsWithDataInBodyApiTestSelectionSelectTestsPost(ctx).
			BodySelectTestsWithDataInBodyApiTestSelectionSelectTestsPost(*reqBody).
			Execute()
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, time.Since(startAt), message.Fields{
			"message":           "error selecting tests",
			"endpoint":          endpoint,
			"project_id":        req.Project,
			"requester":         req.Requester,
			"build_variant":     req.BuildVariant,
			"task_id":           req.TaskID,
			"task_name":         req.TaskName,
			"display_task_name": req.DisplayTaskName,
			"test_count":        len(req.Tests),
		})
		return nil, wrapTSSError(err)
	}
	selectedTests := make([]string, 0, len(selectedTestPtrs))
	for _, t := range selectedTestPtrs {
		if t != nil {
			selectedTests = append(selectedTests, *t)
		}
	}
	return selectedTests, nil
}

// RecordTestSelection records the outcome of a test selection request on the
// task run's test_results record: the tests that test selection checked, which
// are the only tests that can be quarantined from the task run's results, and
// the tests it skipped because they are quarantined in TSS. The checked tests
// are recorded even if finding the quarantined tests fails.
func RecordTestSelection(ctx context.Context, env evergreen.Environment, req model.SelectTestsRequest, selectedTests []string) error {
	checkedTests, quarantinedTests, summarizeErr := summarizeTestSelection(ctx, req, selectedTests)
	if len(checkedTests) == 0 {
		return summarizeErr
	}
	catcher := grip.NewBasicCatcher()
	catcher.Add(summarizeErr)
	catcher.Add(recordTestSelection(ctx, env, req.TaskID, checkedTests, quarantinedTests))
	return catcher.Resolve()
}

func recordTestSelection(ctx context.Context, env evergreen.Environment, taskID string, checkedTests []string, quarantinedTests []testresult.QuarantinedTest) error {
	t, err := task.FindOneId(ctx, taskID)
	if err != nil {
		return errors.Wrapf(err, "finding task '%s'", taskID)
	}
	if t == nil {
		return errors.Errorf("task '%s' not found", taskID)
	}
	numAppended, err := task.AppendTestSelectionResults(ctx, t, env, checkedTests, quarantinedTests)
	if err != nil {
		return errors.Wrap(err, "recording test selection results on the test results record")
	}
	if numAppended == 0 {
		return nil
	}
	if err = t.IncNumQuarantinedTestsSkipped(ctx, numAppended); err != nil {
		return errors.Wrap(err, "incrementing the task's quarantined tests skipped count")
	}

	dt, err := t.GetDisplayTask(ctx)
	if err != nil {
		return errors.Wrap(err, "finding display task")
	}
	if dt == nil {
		return nil
	}
	return errors.Wrap(dt.IncNumQuarantinedTestsSkipped(ctx, numAppended), "incrementing the display task's quarantined tests skipped count")
}

// summarizeTestSelection returns the tests that test selection checked and the
// tests it skipped specifically because they are quarantined in TSS. When the
// request names its tests, those are the checked tests, and only the skipped
// subset is checked for quarantine status. When it does not (the
// select-known-tests path), the full test set is unknown, so the checked tests
// are the ones it returned plus the known tests that the variant's quarantine
// state shows were withheld. If the quarantine state can't be fetched, the
// checked tests that are known are still returned with the error.
func summarizeTestSelection(ctx context.Context, req model.SelectTestsRequest, selectedTests []string) ([]string, []testresult.QuarantinedTest, error) {
	selected := make(map[string]bool, len(selectedTests))
	for _, name := range selectedTests {
		selected[name] = true
	}

	var checkedTests, quarantinedNames []string
	if len(req.Tests) > 0 {
		checkedTests = req.Tests
		var skipped []string
		seen := make(map[string]bool, len(req.Tests))
		for _, name := range req.Tests {
			if !selected[name] && !seen[name] {
				skipped = append(skipped, name)
				seen[name] = true
			}
		}
		if len(skipped) == 0 {
			return checkedTests, nil, nil
		}
		statuses, err := GetTestsQuarantineStatus(ctx, req.Project, req.BuildVariant, req.TaskName, req.DisplayTaskName, skipped)
		if err != nil {
			return checkedTests, nil, errors.Wrap(err, "getting quarantine status for skipped tests")
		}
		for _, name := range skipped {
			if statuses[name] {
				quarantinedNames = append(quarantinedNames, name)
			}
		}
	} else {
		checkedTests = slices.Clone(selectedTests)
		variantState, err := GetVariantQuarantineStatus(ctx, req.Project, req.BuildVariant)
		if err != nil {
			return checkedTests, nil, errors.Wrap(err, "getting variant quarantine status")
		}
		for name, isQuarantined := range variantState[req.TaskName] {
			if isQuarantined && !selected[name] {
				quarantinedNames = append(quarantinedNames, name)
			}
		}
		// The variant state is a map, so sort for a deterministic snapshot.
		sort.Strings(quarantinedNames)
		checkedTests = append(checkedTests, quarantinedNames...)
	}

	// DisplayTestName is left unset because it isn't known until a test
	// actually executes.
	quarantinedTests := make([]testresult.QuarantinedTest, 0, len(quarantinedNames))
	for _, name := range quarantinedNames {
		quarantinedTests = append(quarantinedTests, testresult.QuarantinedTest{TestName: name})
	}
	return checkedTests, quarantinedTests, nil
}

// SetTestQuarantined marks the test as quarantined or unquarantined in the test
// selection service. displayTaskName is the name of the display task that
// contains the task, if any. metadata records who made the change and why.
func SetTestQuarantined(ctx context.Context, projectID, bvName, taskName, displayTaskName, testName string, isManuallyQuarantined bool, metadata QuarantineMetadata) error {
	ctx, cancel := context.WithTimeout(ctx, testSelectionWriteTimeout)
	defer cancel()
	c := newTestSelectionClient(testSelectionHTTPClient)

	startAt := time.Now()
	reqBody := testselection.NewBodyMarkTestsAsManuallyQuarantinedDataInBodyApiTestSelectionTransitionTestsPost(
		projectID,
		bvName,
		taskName,
		[]string{testName},
		isManuallyQuarantined,
	)
	if displayTaskName != "" {
		reqBody.SetDisplayTaskName(displayTaskName)
	}
	reqBody.SetMetadata(metadata.toClient())
	_, resp, err := c.StateTransitionAPI.MarkTestsAsManuallyQuarantinedDataInBodyApiTestSelectionTransitionTestsPost(ctx).
		BodyMarkTestsAsManuallyQuarantinedDataInBodyApiTestSelectionTransitionTestsPost(*reqBody).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, time.Since(startAt), message.Fields{
			"message":           "error setting test quarantine status",
			"endpoint":          TransitionTestsEndpoint,
			"project_id":        projectID,
			"build_variant":     bvName,
			"task_name":         taskName,
			"display_task_name": displayTaskName,
			"test_name":         testName,
		})
	}
	return wrapTSSError(err)
}

// GetTestsQuarantineStatus returns a map from each input test name to its
// quarantine status. Tests absent from the test selection service response
// are populated as false. displayTaskName is the name of the display task that
// contains the task, if any.
func GetTestsQuarantineStatus(ctx context.Context, projectID, bvName, taskName, displayTaskName string, testNames []string) (testToQuarantineStatus map[string]bool, err error) {
	testToQuarantineStatus = make(map[string]bool, len(testNames))
	for _, name := range testNames {
		testToQuarantineStatus[name] = false
	}
	if len(testNames) == 0 {
		return testToQuarantineStatus, nil
	}
	ctx, cancel := context.WithTimeout(ctx, testSelectionStatusTimeout)
	defer cancel()
	c := newTestSelectionClient(testSelectionHTTPClient)

	startAt := time.Now()
	reqBody := testselection.NewBodyGetTestsStateDataInBodyApiTestSelectionGetTestsStatePost(projectID, bvName, taskName, testNames)
	if displayTaskName != "" {
		reqBody.SetDisplayTaskName(displayTaskName)
	}
	result, resp, err := c.StateTransitionAPI.GetTestsStateDataInBodyApiTestSelectionGetTestsStatePost(ctx).
		BodyGetTestsStateDataInBodyApiTestSelectionGetTestsStatePost(*reqBody).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return testToQuarantineStatus, nil
		}
		logTSSError(ctx, err, resp, time.Since(startAt), message.Fields{
			"message":           "error getting tests quarantine status; continuing with default statuses",
			"endpoint":          GetTestsStateEndpoint,
			"project_id":        projectID,
			"build_variant":     bvName,
			"task_name":         taskName,
			"display_task_name": displayTaskName,
			"test_count":        len(testNames),
		})
		return testToQuarantineStatus, nil
	}
	if result == nil {
		grip.Error(ctx, message.Fields{
			"message":           "nil response from test selection service; continuing with default statuses",
			"endpoint":          GetTestsStateEndpoint,
			"project_id":        projectID,
			"build_variant":     bvName,
			"task_name":         taskName,
			"display_task_name": displayTaskName,
			"duration_ms":       time.Since(startAt).Milliseconds(),
			"test_count":        len(testNames),
		})
		return testToQuarantineStatus, nil
	}

	for testName, stateInfo := range *result {
		if overrideState, ok := stateInfo.GetOverrideStateOk(); ok && overrideState != nil {
			testToQuarantineStatus[testName] = *overrideState == testselection.STATEMACHINEENUM_MANUALLY_QUARANTINED
			continue
		}
		testToQuarantineStatus[testName] = stateInfo.State == testselection.STATEMACHINEENUM_MANUALLY_QUARANTINED
	}
	return testToQuarantineStatus, nil
}

// SetTaskQuarantined marks all known tests of a task as manually quarantined
// (or unquarantines them) in the test selection service. displayTaskName is the
// name of the display task that contains the task, if any. metadata records
// who made the change and why.
func SetTaskQuarantined(ctx context.Context, projectID, bvName, taskName, displayTaskName string, isManuallyQuarantined bool, metadata QuarantineMetadata) error {
	ctx, cancel := context.WithTimeout(ctx, testSelectionWriteTimeout)
	defer cancel()
	c := newTestSelectionClient(testSelectionHTTPClient)

	startAt := time.Now()
	reqBody := testselection.NewBodyMarkTaskAsManuallyQuarantinedDataInBodyApiTestSelectionTransitionTaskPost(
		projectID,
		bvName,
		taskName,
		isManuallyQuarantined,
	)
	if displayTaskName != "" {
		reqBody.SetDisplayTaskName(displayTaskName)
	}
	reqBody.SetMetadata(metadata.toClient())
	_, resp, err := c.StateTransitionAPI.MarkTaskAsManuallyQuarantinedDataInBodyApiTestSelectionTransitionTaskPost(ctx).
		BodyMarkTaskAsManuallyQuarantinedDataInBodyApiTestSelectionTransitionTaskPost(*reqBody).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, time.Since(startAt), message.Fields{
			"message":           "error setting task quarantine status",
			"endpoint":          TransitionTaskEndpoint,
			"project_id":        projectID,
			"build_variant":     bvName,
			"task_name":         taskName,
			"display_task_name": displayTaskName,
		})
	}
	return wrapTSSError(err)
}

// SetVariantQuarantined marks all known tests of a build variant as manually
// quarantined (or unquarantines them) in the test selection service. metadata
// records who made the change and why.
func SetVariantQuarantined(ctx context.Context, projectID, bvName string, isManuallyQuarantined bool, metadata QuarantineMetadata) error {
	ctx, cancel := context.WithTimeout(ctx, testSelectionWriteTimeout)
	defer cancel()
	c := newTestSelectionClient(testSelectionHTTPClient)

	startAt := time.Now()
	reqBody := testselection.NewBodyMarkVariantAsManuallyQuarantinedDataInBodyApiTestSelectionTransitionVariantPost(
		projectID,
		bvName,
		isManuallyQuarantined,
	)
	reqBody.SetMetadata(metadata.toClient())
	_, resp, err := c.StateTransitionAPI.MarkVariantAsManuallyQuarantinedDataInBodyApiTestSelectionTransitionVariantPost(ctx).
		BodyMarkVariantAsManuallyQuarantinedDataInBodyApiTestSelectionTransitionVariantPost(*reqBody).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, time.Since(startAt), message.Fields{
			"message":       "error setting variant quarantine status",
			"endpoint":      TransitionVariantEndpoint,
			"project_id":    projectID,
			"build_variant": bvName,
		})
	}
	return wrapTSSError(err)
}

// GetVariantQuarantineStatus returns the manual-quarantine status for every
// test in every known task of a build variant. Tests use the override-state
// precedence rule: if override_state is present and non-null, it determines
// the result; otherwise state is used.
func GetVariantQuarantineStatus(ctx context.Context, projectID, bvName string) (map[string]map[string]bool, error) {
	ctx, cancel := context.WithTimeout(ctx, testSelectionStatusTimeout)
	defer cancel()
	c := newTestSelectionClient(testSelectionHTTPClient)

	startAt := time.Now()
	reqBody := testselection.NewBodyGetVariantStateDataInBodyApiTestSelectionGetVariantStatePost(projectID, bvName)
	result, resp, err := c.StateTransitionAPI.GetVariantStateDataInBodyApiTestSelectionGetVariantStatePost(ctx).
		BodyGetVariantStateDataInBodyApiTestSelectionGetVariantStatePost(*reqBody).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, time.Since(startAt), message.Fields{
			"message":       "error getting variant quarantine status",
			"endpoint":      GetVariantStateEndpoint,
			"project_id":    projectID,
			"build_variant": bvName,
		})
		return nil, wrapTSSError(err)
	}
	if result == nil {
		return nil, errors.New("nil response from test selection service")
	}

	tasks := make(map[string]map[string]bool, len(*result))
	for taskName, taskState := range *result {
		tests := make(map[string]bool, len(taskState.TestStats))
		for testName, stateInfo := range taskState.TestStats {
			if overrideState, ok := stateInfo.GetOverrideStateOk(); ok && overrideState != nil {
				tests[testName] = *overrideState == testselection.STATEMACHINEENUM_MANUALLY_QUARANTINED
				continue
			}
			tests[testName] = stateInfo.State == testselection.STATEMACHINEENUM_MANUALLY_QUARANTINED
		}
		tasks[taskName] = tests
	}
	return tasks, nil
}

// DecorateQuarantineStatus populates CanQuarantine and IsManuallyQuarantined
// on each test result in place. A test can only be quarantined if test
// selection checked it during its task run, because only those tests are
// affected by quarantine, so only those tests are looked up in TSS. Display
// tasks are checked per execution task run.
func DecorateQuarantineStatus(ctx context.Context, env evergreen.Environment, t *task.Task, results []testresult.TestResult) error {
	if len(results) == 0 {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, testSelectionDecorationTimeout)
	defer cancel()

	type taskRunKey struct {
		taskID    string
		execution int
	}
	resultIndicesByTaskRun := map[taskRunKey][]int{}
	testNameSet := map[string]bool{}
	for i, r := range results {
		key := taskRunKey{taskID: r.TaskID, execution: r.Execution}
		resultIndicesByTaskRun[key] = append(resultIndicesByTaskRun[key], i)
		testNameSet[r.GetDisplayTestName()] = true
	}
	allTasks, err := t.GetTestResultsTasks(ctx)
	if err != nil {
		return errors.Wrap(err, "getting test results tasks")
	}
	// Only look up the task runs and tests in the results, since the checked
	// tests for a task run can be large.
	var tasks []task.Task
	for _, tsk := range allTasks {
		if _, ok := resultIndicesByTaskRun[taskRunKey{taskID: tsk.Id, execution: tsk.Execution}]; ok {
			tasks = append(tasks, tsk)
		}
	}
	taskRuns, err := task.GetTestSelectionCheckedTests(ctx, env, tasks, slices.Collect(maps.Keys(testNameSet)))
	if err != nil {
		return errors.Wrap(err, "getting tests checked by test selection")
	}

	eg, ctx := errgroup.WithContext(ctx)
	for _, taskRun := range taskRuns {
		checked := make(map[string]bool, len(taskRun.TestSelectionCheckedTests))
		for _, name := range taskRun.TestSelectionCheckedTests {
			checked[name] = true
		}
		var indices []int
		var testNames []string
		for _, i := range resultIndicesByTaskRun[taskRunKey{taskID: taskRun.Info.TaskID, execution: taskRun.Info.Execution}] {
			name := results[i].GetDisplayTestName()
			if !checked[name] {
				continue
			}
			results[i].CanQuarantine = true
			indices = append(indices, i)
			testNames = append(testNames, name)
		}
		if len(indices) == 0 {
			continue
		}
		info := taskRun.Info
		eg.Go(func() error {
			statuses, err := GetTestsQuarantineStatus(ctx, info.Project, info.Variant, info.TaskName, info.DisplayTaskName, testNames)
			if err != nil {
				return errors.Wrapf(err, "fetching test quarantine statuses for task '%s'", info.TaskID)
			}
			for _, i := range indices {
				results[i].IsManuallyQuarantined = statuses[results[i].GetDisplayTestName()]
			}
			return nil
		})
	}
	return eg.Wait()
}
