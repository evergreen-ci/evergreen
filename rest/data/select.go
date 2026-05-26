package data

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/model"
	testselection "github.com/evergreen-ci/test-selection-client"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
)

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
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)

	c := newTestSelectionClient(httpClient)
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
		return nil, errors.Wrap(err, "forwarding request to test selection service")
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
func SetTestQuarantined(ctx context.Context, projectID, bvName, taskName, testName string, isQuarantined bool) error {
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)
	c := newTestSelectionClient(httpClient)

	_, resp, err := c.StateTransitionAPI.MarkTestsAsManuallyQuarantinedApiTestSelectionTransitionTestsProjectIdBuildVariantNameTaskNamePost(ctx, projectID, bvName, taskName).
		IsManuallyQuarantined(isQuarantined).
		RequestBody([]*string{&testName}).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	return errors.Wrap(err, "forwarding request to test selection service")
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
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)
	c := newTestSelectionClient(httpClient)

	result, resp, err := c.StateTransitionAPI.GetTestsStateApiTestSelectionGetTestsStateProjectIdBuildVariantNameTaskNamePost(ctx, projectID, bvName, taskName).
		RequestBody(testNames).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, errors.Wrap(err, "forwarding request to test selection service")
	}
	if result == nil {
		return nil, errors.New("nil response from test selection service")
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

// DecorateQuarantineStatus populates IsManuallyQuarantined on each test result
// in place. No-ops when test selection is disabled. Display tasks fan out to
// their execution tasks because TSS keys quarantine state by execution task name.
func DecorateQuarantineStatus(ctx context.Context, t *task.Task, results []testresult.TestResult) error {
	if !t.TestSelectionEnabled || len(results) == 0 {
		return nil
	}
	if t.DisplayOnly {
		return decorateDisplayTaskQuarantineStatus(ctx, results)
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
func decorateDisplayTaskQuarantineStatus(ctx context.Context, results []testresult.TestResult) error {
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
	for _, execTask := range execTasks {
		if !execTask.TestSelectionEnabled {
			continue
		}
		indices := resultIndicesByExecTaskID[execTask.Id]
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
	}
	return nil
}
