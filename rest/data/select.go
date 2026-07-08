package data

import (
	"context"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/model"
	testselection "github.com/evergreen-ci/test-selection-client"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
	"golang.org/x/sync/errgroup"
)

// Test selection service endpoint identifiers.
const (
	SelectTestsEndpoint       = "select_tests"
	TransitionTestsEndpoint   = "transition_tests"
	TransitionTaskEndpoint    = "transition_task"
	TransitionVariantEndpoint = "transition_variant"
	GetTestsStateEndpoint     = "get_tests_state"
	GetVariantStateEndpoint   = "get_variant_state"
)

const (
	testSelectionWriteTimeout                     = 10 * time.Second
	testSelectionStatusTimeout                    = 4 * time.Second
	testSelectionDecorationTimeout                = 5 * time.Second
	testSelectionDisplayTaskStatusRequestParallel = 8
)

var testSelectionHTTPClient = newTestSelectionHTTPClient()

func newTestSelectionHTTPClient() *http.Client {
	// Each request's overall duration is bounded by the caller's context deadline but increase the idle connection pool to reduce the number of reconnects.
	transport := http.DefaultTransport.(*http.Transport).Clone()
	transport.MaxIdleConnsPerHost = 100

	return &http.Client{
		Timeout:   testSelectionWriteTimeout,
		Transport: transport,
	}
}

func logTSSError(ctx context.Context, err error, resp *http.Response, info message.Fields) {
	// A 404 means the requested resource isn't tracked in the test selection
	// service yet, which is expected and too noisy for Splunk. The error is
	// still returned to the caller.
	if resp != nil && resp.StatusCode == http.StatusNotFound {
		return
	}
	if resp != nil {
		info["status"] = resp.StatusCode
	}
	info["error"] = err.Error()
	var openAPIErr *testselection.GenericOpenAPIError
	if errors.As(err, &openAPIErr) {
		info["response_body"] = string(openAPIErr.Body())
	}
	grip.Error(ctx, info)
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
	ctx, cancel := context.WithTimeout(ctx, testSelectionWriteTimeout)
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
	if len(req.Tests) == 0 {
		selectedTestPtrs, resp, err = c.TestSelectionAPI.SelectAllKnownTestsOfATaskApiTestSelectionSelectKnownTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost(ctx, req.Project, req.Requester, req.BuildVariant, req.TaskID, req.TaskName).StrategyEnum(strategies).Execute()
	} else {
		reqBody := testselection.BodySelectTestsApiTestSelectionSelectTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost{
			TestNames:  req.Tests,
			Strategies: strategies,
		}

		selectedTestPtrs, resp, err = c.TestSelectionAPI.SelectTestsApiTestSelectionSelectTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost(ctx, req.Project, req.Requester, req.BuildVariant, req.TaskID, req.TaskName).
			BodySelectTestsApiTestSelectionSelectTestsProjectIdRequesterBuildVariantNameTaskIdTaskNamePost(reqBody).
			Execute()
	}
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, message.Fields{
			"message":       "error selecting tests",
			"endpoint":      SelectTestsEndpoint,
			"project_id":    req.Project,
			"requester":     req.Requester,
			"build_variant": req.BuildVariant,
			"task_id":       req.TaskID,
			"task_name":     req.TaskName,
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

// SetTestQuarantined marks the test as quarantined or unquarantined in the test
// selection service.
func SetTestQuarantined(ctx context.Context, projectID, bvName, taskName, testName string, isManuallyQuarantined bool) error {
	ctx, cancel := context.WithTimeout(ctx, testSelectionWriteTimeout)
	defer cancel()
	c := newTestSelectionClient(testSelectionHTTPClient)

	_, resp, err := c.StateTransitionAPI.MarkTestsAsManuallyQuarantinedApiTestSelectionTransitionTestsProjectIdBuildVariantNameTaskNamePost(ctx, projectID, bvName, taskName).
		IsManuallyQuarantined(isManuallyQuarantined).
		RequestBody([]*string{&testName}).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, message.Fields{
			"message":       "error setting test quarantine status",
			"endpoint":      TransitionTestsEndpoint,
			"project_id":    projectID,
			"build_variant": bvName,
			"task_name":     taskName,
			"test_name":     testName,
		})
	}
	return wrapTSSError(err)
}

// GetTestsQuarantineStatus returns a map from each input test name to its
// quarantine status. Tests absent from the test selection service response
// are populated as false.
func GetTestsQuarantineStatus(ctx context.Context, projectID, bvName, taskName string, testNames []string) (testToQuarantineStatus map[string]bool, err error) {
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

	result, resp, err := c.StateTransitionAPI.GetTestsStateApiTestSelectionGetTestsStateProjectIdBuildVariantNameTaskNamePost(ctx, projectID, bvName, taskName).
		RequestBody(testNames).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		if resp != nil && resp.StatusCode == http.StatusNotFound {
			return testToQuarantineStatus, nil
		}
		logTSSError(ctx, err, resp, message.Fields{
			"message":       "error getting tests quarantine status; continuing with default statuses",
			"endpoint":      GetTestsStateEndpoint,
			"project_id":    projectID,
			"build_variant": bvName,
			"task_name":     taskName,
		})
		return testToQuarantineStatus, nil
	}
	if result == nil {
		grip.Error(ctx, message.Fields{
			"message":       "nil response from test selection service; continuing with default statuses",
			"endpoint":      GetTestsStateEndpoint,
			"project_id":    projectID,
			"build_variant": bvName,
			"task_name":     taskName,
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
// (or unquarantines them) in the test selection service.
func SetTaskQuarantined(ctx context.Context, projectID, bvName, taskName string, isManuallyQuarantined bool) error {
	ctx, cancel := context.WithTimeout(ctx, testSelectionWriteTimeout)
	defer cancel()
	c := newTestSelectionClient(testSelectionHTTPClient)

	_, resp, err := c.StateTransitionAPI.MarkTaskAsManuallyQuarantinedApiTestSelectionTransitionTaskProjectIdBuildVariantNameTaskNamePost(ctx, projectID, bvName, taskName).
		IsManuallyQuarantined(isManuallyQuarantined).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, message.Fields{
			"message":       "error setting task quarantine status",
			"endpoint":      TransitionTaskEndpoint,
			"project_id":    projectID,
			"build_variant": bvName,
			"task_name":     taskName,
		})
	}
	return wrapTSSError(err)
}

// SetVariantQuarantined marks all known tests of a build variant as manually
// quarantined (or unquarantines them) in the test selection service.
func SetVariantQuarantined(ctx context.Context, projectID, bvName string, isManuallyQuarantined bool) error {
	ctx, cancel := context.WithTimeout(ctx, testSelectionWriteTimeout)
	defer cancel()
	c := newTestSelectionClient(testSelectionHTTPClient)

	_, resp, err := c.StateTransitionAPI.MarkVariantAsManuallyQuarantinedApiTestSelectionTransitionVariantProjectIdBuildVariantNamePost(ctx, projectID, bvName).
		IsManuallyQuarantined(isManuallyQuarantined).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, message.Fields{
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

	result, resp, err := c.StateTransitionAPI.GetVariantStateApiTestSelectionGetVariantStateProjectIdBuildVariantNamePost(ctx, projectID, bvName).Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		logTSSError(ctx, err, resp, message.Fields{
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

// DecorateQuarantineStatus populates IsManuallyQuarantined on each test result
// in place. No-ops when test selection is disabled. Display tasks always fan
// out to their execution tasks; the display task's TestSelectionEnabled flag
// is treated as a hint because it can be stale relative to its execution
// tasks, so the fan-out re-checks each execution task's state directly.
func DecorateQuarantineStatus(ctx context.Context, t *task.Task, results []testresult.TestResult) error {
	if len(results) == 0 {
		return nil
	}
	if !t.DisplayOnly && !t.TestSelectionEnabled {
		return nil
	}
	ctx, cancel := context.WithTimeout(ctx, testSelectionDecorationTimeout)
	defer cancel()
	if t.DisplayOnly {
		return decorateDisplayTaskQuarantineStatus(ctx, t.Id, results)
	}
	testNames := make([]string, 0, len(results))
	for _, r := range results {
		testNames = append(testNames, r.GetDisplayTestName())
	}
	statuses, err := GetTestsQuarantineStatus(ctx, t.Project, t.BuildVariant, t.DisplayName, testNames)
	if err != nil {
		return errors.Wrap(err, "fetching test quarantine statuses")
	}
	for i := range results {
		results[i].IsManuallyQuarantined = statuses[results[i].GetDisplayTestName()]
	}
	return nil
}

// decorateDisplayTaskQuarantineStatus groups results by their execution task ID
// and fetches quarantine status once per execution task. Execution tasks
// without test selection enabled, and tests whose execution task can't be
// loaded, are left with the default IsManuallyQuarantined value.
func decorateDisplayTaskQuarantineStatus(ctx context.Context, displayTaskID string, results []testresult.TestResult) error {
	resultIndicesByExecTaskID := map[string][]int{}
	for i, r := range results {
		if r.TaskID == "" {
			continue
		}
		resultIndicesByExecTaskID[r.TaskID] = append(resultIndicesByExecTaskID[r.TaskID], i)
	}
	if len(resultIndicesByExecTaskID) == 0 {
		return nil
	}
	execTaskIDs := make([]string, 0, len(resultIndicesByExecTaskID))
	for id := range resultIndicesByExecTaskID {
		execTaskIDs = append(execTaskIDs, id)
	}
	execTasks, err := task.FindAll(ctx, db.Query(task.ByIds(execTaskIDs)))
	if err != nil {
		return errors.Wrap(err, "fetching execution tasks")
	}
	foundExecTaskIDs := make(map[string]bool, len(execTasks))
	for _, execTask := range execTasks {
		foundExecTaskIDs[execTask.Id] = true
	}
	var missingExecTaskIDs []string
	for _, id := range execTaskIDs {
		if !foundExecTaskIDs[id] {
			missingExecTaskIDs = append(missingExecTaskIDs, id)
		}
	}
	grip.WarningWhen(ctx, len(missingExecTaskIDs) > 0, message.Fields{
		"message":               "execution tasks not found when decorating display task quarantine status; their test results will be left undecorated",
		"display_task_id":       displayTaskID,
		"missing_exec_task_ids": missingExecTaskIDs,
	})
	eg, ctx := errgroup.WithContext(ctx)
	eg.SetLimit(testSelectionDisplayTaskStatusRequestParallel)
	for _, execTask := range execTasks {
		if !execTask.TestSelectionEnabled {
			continue
		}
		execTask := execTask
		indices := append([]int(nil), resultIndicesByExecTaskID[execTask.Id]...)
		eg.Go(func() error {
			testNames := make([]string, 0, len(indices))
			for _, i := range indices {
				testNames = append(testNames, results[i].GetDisplayTestName())
			}
			statuses, err := GetTestsQuarantineStatus(ctx, execTask.Project, execTask.BuildVariant, execTask.DisplayName, testNames)
			if err != nil {
				return errors.Wrapf(err, "fetching test quarantine statuses for execution task '%s'", execTask.Id)
			}
			for _, i := range indices {
				results[i].IsManuallyQuarantined = statuses[results[i].GetDisplayTestName()]
			}
			return nil
		})
	}
	return eg.Wait()
}
