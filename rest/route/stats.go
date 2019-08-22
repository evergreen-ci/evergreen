package route

// This file defines the handlers for the endpoints to query the test and task execution statistics.

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

const (
	// Requester API values
	statsAPIRequesterMainline = "mainline"
	statsAPIRequesterPatch    = "patch"
	statsAPIRequesterTrigger  = "trigger"
	statsAPIRequesterAdhoc    = "adhoc"

	// Sort API values
	statsAPISortEarliest = "earliest"
	statsAPISortLatest   = "latest"

	// GroupBy API values for tests
	statsAPITestGroupByDistro  = "test_task_variant_distro"
	statsAPITestGroupByVariant = "test_task_variant"
	statsAPITestGroupByTask    = "test_task"
	statsAPITestGroupByTest    = "test"

	// GroupBy API values for tasks
	statsAPITaskGroupByDistro  = "task_variant_distro"
	statsAPITaskGroupByVariant = "task_variant"
	statsAPITaskGroupByTask    = "task"

	// API Limits
	statsAPIMaxGroupNumDays = 26 * 7 // 26 weeks which is the maximum amount of data available
	statsAPIMaxNumTests     = 50
	statsAPIMaxNumTasks     = 50
	statsAPIMaxLimit        = 1000

	// Format used to encode dates in the API
	statsAPIDateFormat = "2006-01-02"
)

///////////////////////////////////////////////////////////////////////
// Base handler with functionality common to test and stats handlers //
///////////////////////////////////////////////////////////////////////

type StatsHandler struct {
	filter stats.StatsFilter
}

// ParseCommonFilter parses the query parameter values and fills the struct filter field.
func (sh *StatsHandler) ParseCommonFilter(vals url.Values) error {
	var err error

	// requesters
	sh.filter.Requesters, err = sh.readRequesters(sh.readStringList(vals["requesters"]))
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid requesters value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// variants
	sh.filter.BuildVariants = sh.readStringList(vals["variants"])

	// distros
	sh.filter.Distros = sh.readStringList(vals["distros"])

	// group_num_days
	sh.filter.GroupNumDays, err = sh.readInt(vals.Get("group_num_days"), 1, statsAPIMaxGroupNumDays, 1)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid group_num_days value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// group_by
	sh.filter.GroupBy, err = sh.readGroupBy(vals.Get("group_by"))
	if err != nil {
		return err
	}

	// start_at
	sh.filter.StartAt, err = sh.readStartAt(vals.Get("start_at"))
	return err
}

// parseStatsFilter parses the query parameter values and fills the struct filter field.
func (sh *StatsHandler) parseStatsFilter(vals url.Values) error {
	var err error

	err = sh.ParseCommonFilter(vals)
	if err != nil {
		return err
	}

	// limit
	sh.filter.Limit, err = sh.readInt(vals.Get("limit"), 1, statsAPIMaxLimit, statsAPIMaxLimit)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid limit value",
			StatusCode: http.StatusBadRequest,
		}
	}
	// Add 1 for pagination
	sh.filter.Limit++

	// tasks
	sh.filter.Tasks = sh.readStringList(vals["tasks"])
	if len(sh.filter.Tasks) > statsAPIMaxNumTasks {
		return gimlet.ErrorResponse{
			Message:    "Too many tasks values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// before_date
	beforeDate := vals.Get("before_date")
	if beforeDate == "" {
		return gimlet.ErrorResponse{
			Message:    "Missing before_date parameter",
			StatusCode: http.StatusBadRequest,
		}
	}
	sh.filter.BeforeDate, err = time.ParseInLocation(statsAPIDateFormat, beforeDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid before_date value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// after_date
	afterDate := vals.Get("after_date")
	if afterDate == "" {
		return gimlet.ErrorResponse{
			Message:    "Missing after_date parameter",
			StatusCode: http.StatusBadRequest,
		}
	}
	sh.filter.AfterDate, err = time.ParseInLocation(statsAPIDateFormat, afterDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid after_date value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// tests
	sh.filter.Tests = sh.readStringList(vals["tests"])
	if len(sh.filter.Tests) > statsAPIMaxNumTests {
		return gimlet.ErrorResponse{
			Message:    "Too many tests values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// sort
	sh.filter.Sort, err = sh.readSort(vals.Get("sort"))
	if err != nil {
		return err
	}

	return err
}

// readRequesters parses requesters parameter values and translates them into a list of internal Evergreen requester names.
func (sh *StatsHandler) readRequesters(requesters []string) ([]string, error) {
	if len(requesters) == 0 {
		requesters = []string{statsAPIRequesterMainline}
	}
	requesterValues := []string{}
	for _, requester := range requesters {
		switch requester {
		case statsAPIRequesterMainline:
			requesterValues = append(requesterValues, evergreen.RepotrackerVersionRequester)
		case statsAPIRequesterPatch:
			requesterValues = append(requesterValues, evergreen.PatchRequesters...)
		case statsAPIRequesterTrigger:
			requesterValues = append(requesterValues, evergreen.TriggerRequester)
		case statsAPIRequesterAdhoc:
			requesterValues = append(requesterValues, evergreen.AdHocRequester)
		default:
			return nil, errors.Errorf("Invalid requester value %v", requester)
		}
	}
	return requesterValues, nil
}

// readStringList parses a string list parameter value, the values can be comma separated or specified multiple times.
func (sh *StatsHandler) readStringList(values []string) []string {
	var parsedValues []string
	for _, val := range values {
		elements := strings.Split(val, ",")
		parsedValues = append(parsedValues, elements...)
	}
	return parsedValues
}

// readInt parses an integer parameter value, given minimum, maximum, and default values.
func (sh *StatsHandler) readInt(intString string, min, max, defaultValue int) (int, error) {
	if intString == "" {
		return defaultValue, nil
	}
	value, err := strconv.Atoi(intString)
	if err != nil {
		return 0, err
	}

	if value < min || value > max {
		return 0, errors.New("Invalid int parameter value")
	}
	return value, nil
}

// readTestGroupBy parses a sort parameter value and returns the corresponding Sort struct.
func (sh *StatsHandler) readSort(sortValue string) (stats.Sort, error) {
	switch sortValue {
	case statsAPISortEarliest:
		return stats.SortEarliestFirst, nil
	case statsAPISortLatest:
		return stats.SortLatestFirst, nil
	case "":
		return stats.SortEarliestFirst, nil
	default:
		return stats.Sort(""), gimlet.ErrorResponse{
			Message:    "Invalid sort value",
			StatusCode: http.StatusBadRequest,
		}
	}
}

// readGroupBy parses a group_by parameter value and returns the corresponding GroupBy struct.
func (sh *StatsHandler) readGroupBy(groupByValue string) (stats.GroupBy, error) {
	switch groupByValue {

	// Task query parameters.
	case statsAPITaskGroupByDistro:
		return stats.GroupByDistro, nil
	case statsAPITaskGroupByVariant:
		return stats.GroupByVariant, nil
	case statsAPITaskGroupByTask:
		return stats.GroupByTask, nil

	// Test query parameters.
	case statsAPITestGroupByDistro:
		return stats.GroupByDistro, nil
	case statsAPITestGroupByVariant:
		return stats.GroupByVariant, nil
	case statsAPITestGroupByTask:
		return stats.GroupByTask, nil
	case statsAPITestGroupByTest:
		return stats.GroupByTest, nil

	// Default value.
	case "":
		return stats.GroupByDistro, nil

	default:
		return stats.GroupBy(""), gimlet.ErrorResponse{
			Message:    "Invalid group_by value",
			StatusCode: http.StatusBadRequest,
		}
	}
}

// readStartAt parses a start_at key value and returns the corresponding StartAt struct.
func (sh *StatsHandler) readStartAt(startAtValue string) (*stats.StartAt, error) {
	if startAtValue == "" {
		return nil, nil
	}
	elements := strings.Split(startAtValue, "|")
	if len(elements) != 5 {
		return nil, gimlet.ErrorResponse{
			Message:    "Invalid start_at value",
			StatusCode: http.StatusBadRequest,
		}
	}
	date, err := time.ParseInLocation(statsAPIDateFormat, elements[0], time.UTC)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			Message:    "Invalid start_at value",
			StatusCode: http.StatusBadRequest,
		}
	}
	return &stats.StartAt{
		Date:         date,
		BuildVariant: elements[1],
		Task:         elements[2],
		Test:         elements[3],
		Distro:       elements[4],
	}, nil
}

///////////////////////////////////////////////
// /projects/<project_id>/test_stats handler //
///////////////////////////////////////////////

type testStatsHandler struct {
	sc data.Connector
	StatsHandler
}

func (tsh *testStatsHandler) Factory() gimlet.RouteHandler {
	return &testStatsHandler{sc: tsh.sc}
}

func (tsh *testStatsHandler) Parse(ctx context.Context, r *http.Request) error {
	tsh.filter = stats.StatsFilter{Project: gimlet.GetVars(r)["project_id"]}

	err := tsh.StatsHandler.parseStatsFilter(r.URL.Query())
	if err != nil {
		return errors.Wrap(err, "Invalid query parameters")
	}
	err = tsh.filter.ValidateForTests()
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (tsh *testStatsHandler) Run(ctx context.Context) gimlet.Responder {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error retrieving service flags"))
	}
	if flags.CacheStatsEndpointDisabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "endpoint is disabled",
			StatusCode: http.StatusServiceUnavailable,
		})
	}

	var testStatsResult []model.APITestStats

	testStatsResult, err = tsh.sc.GetTestStats(tsh.filter)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Failed to retrieve the test stats"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	requestLimit := tsh.filter.Limit - 1
	lastIndex := len(testStatsResult)
	if len(testStatsResult) > requestLimit {
		lastIndex = requestLimit

		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tsh.sc.GetURL(),
				Key:             testStatsResult[requestLimit].StartAtKey(),
				Limit:           requestLimit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"Problem paginating response"))
		}
	}
	testStatsResult = testStatsResult[:lastIndex]

	for _, apiTestStats := range testStatsResult {
		if err = resp.AddData(apiTestStats); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	return resp
}

func makeGetProjectTestStats(sc data.Connector) gimlet.RouteHandler {
	return &testStatsHandler{sc: sc}
}

///////////////////////////////////////////////
// /projects/<project_id>/task_stats handler //
///////////////////////////////////////////////

type taskStatsHandler struct {
	sc data.Connector
	StatsHandler
}

func (tsh *taskStatsHandler) Factory() gimlet.RouteHandler {
	return &taskStatsHandler{sc: tsh.sc}
}

func (tsh *taskStatsHandler) Parse(ctx context.Context, r *http.Request) error {
	tsh.filter = stats.StatsFilter{Project: gimlet.GetVars(r)["project_id"]}

	err := tsh.StatsHandler.parseStatsFilter(r.URL.Query())
	if err != nil {
		return errors.Wrap(err, "Invalid query parameters")
	}
	err = tsh.filter.ValidateForTasks()
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (tsh *taskStatsHandler) Run(ctx context.Context) gimlet.Responder {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error retrieving service flags"))
	}
	if flags.CacheStatsEndpointDisabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "endpoint is disabled",
			StatusCode: http.StatusServiceUnavailable,
		})
	}

	var taskStatsResult []model.APITaskStats

	taskStatsResult, err = tsh.sc.GetTaskStats(tsh.filter)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Failed to retrieve the task stats"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	requestLimit := tsh.filter.Limit - 1
	lastIndex := len(taskStatsResult)
	if len(taskStatsResult) > requestLimit {
		lastIndex = requestLimit

		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tsh.sc.GetURL(),
				Key:             taskStatsResult[requestLimit].StartAtKey(),
				Limit:           requestLimit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"Problem paginating response"))
		}
	}
	taskStatsResult = taskStatsResult[:lastIndex]

	for _, apiTaskStats := range taskStatsResult {
		if err = resp.AddData(apiTaskStats); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	return resp
}

func makeGetProjectTaskStats(sc data.Connector) gimlet.RouteHandler {
	return &taskStatsHandler{sc: sc}
}
