package data

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/rest/model"
	testselection "github.com/evergreen-ci/test-selection-client"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// wrapTSSError augments errors from the test selection service with the
// response body in the error message and structured fields in the grip log.
func wrapTSSError(ctx context.Context, err error, fields message.Fields) error {
	if err == nil {
		return nil
	}
	var openAPIErr *testselection.GenericOpenAPIError
	if errors.As(err, &openAPIErr) {
		body := string(openAPIErr.Body())
		logFields := message.Fields{
			"message": "test selection service request failed",
			"body":    body,
		}
		for k, v := range fields {
			logFields[k] = v
		}
		grip.Error(ctx, message.WrapError(err, logFields))
		return errors.Wrapf(err, "forwarding request to test selection service: %s", body)
	}
	grip.Error(ctx, message.WrapError(err, fields))
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
		return nil, wrapTSSError(ctx, err, message.Fields{
			"endpoint":      "select_tests",
			"project":       req.Project,
			"requester":     req.Requester,
			"build_variant": req.BuildVariant,
			"task_id":       req.TaskID,
			"task_name":     req.TaskName,
		})
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
	return wrapTSSError(ctx, err, message.Fields{
		"endpoint":                "transition_tests",
		"project":                 projectID,
		"build_variant":           bvName,
		"task_name":               taskName,
		"test_name":               testName,
		"is_manually_quarantined": isQuarantined,
	})
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
		return nil, wrapTSSError(ctx, err, message.Fields{
			"endpoint":      "get_tests_state",
			"project":       projectID,
			"build_variant": bvName,
			"task_name":     taskName,
			"test_count":    len(testNames),
		})
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

// SetTaskQuarantined marks all known tests of a task as manually quarantined
// (or unquarantines them) in the test selection service.
func SetTaskQuarantined(ctx context.Context, projectID, bvName, taskName string, isQuarantined bool) error {
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)
	c := newTestSelectionClient(httpClient)

	_, resp, err := c.StateTransitionAPI.MarkTaskAsManuallyQuarantinedApiTestSelectionTransitionTaskProjectIdBuildVariantNameTaskNamePost(ctx, projectID, bvName, taskName).
		IsManuallyQuarantined(isQuarantined).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	return wrapTSSError(ctx, err, message.Fields{
		"endpoint":                "transition_task",
		"project":                 projectID,
		"build_variant":           bvName,
		"task_name":               taskName,
		"is_manually_quarantined": isQuarantined,
	})
}

// SetVariantQuarantined marks all known tests of a build variant as manually
// quarantined (or unquarantines them) in the test selection service.
func SetVariantQuarantined(ctx context.Context, projectID, bvName string, isQuarantined bool) error {
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)
	c := newTestSelectionClient(httpClient)

	_, resp, err := c.StateTransitionAPI.MarkVariantAsManuallyQuarantinedApiTestSelectionTransitionVariantProjectIdBuildVariantNamePost(ctx, projectID, bvName).
		IsManuallyQuarantined(isQuarantined).
		Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	return wrapTSSError(ctx, err, message.Fields{
		"endpoint":                "transition_variant",
		"project":                 projectID,
		"build_variant":           bvName,
		"is_manually_quarantined": isQuarantined,
	})
}

// GetVariantQuarantineStatus returns the manual-quarantine status for every
// test in every known task of a build variant. Tests use the override-state
// precedence rule: if override_state is present and non-null, it determines
// the result; otherwise state is used.
func GetVariantQuarantineStatus(ctx context.Context, projectID, bvName string) (map[string]map[string]bool, error) {
	httpClient := utility.GetHTTPClient()
	defer utility.PutHTTPClient(httpClient)
	c := newTestSelectionClient(httpClient)

	result, resp, err := c.StateTransitionAPI.GetVariantStateApiTestSelectionGetVariantStateProjectIdBuildVariantNamePost(ctx, projectID, bvName).Execute()
	if resp != nil {
		defer resp.Body.Close()
	}
	if err != nil {
		return nil, wrapTSSError(ctx, err, message.Fields{
			"endpoint":      "get_variant_state",
			"project":       projectID,
			"build_variant": bvName,
		})
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
// in place. No-ops on display tasks and on tasks without test selection
// enabled.
func DecorateQuarantineStatus(ctx context.Context, t *task.Task, results []testresult.TestResult) error {
	if !t.TestSelectionEnabled || t.DisplayOnly || len(results) == 0 {
		return nil
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
