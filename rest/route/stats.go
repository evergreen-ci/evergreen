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

	// GroupBy API values
	statsAPITestGroupByDistro  = "test_task_variant_distro"
	statsAPITestGroupByVariant = "test_task_variant"
	statsAPITestGroupByTask    = "test_task"
	statsAPITestGroupByTest    = "test"

	// API Limits
	statsAPIMaxGroupNumDays = 26 * 7 // 26 weeks which is the maximum amount of data available
	statsAPIMaxNumTests     = 50
	statsAPIMaxNumTasks     = 50

	// Format used to encode dates in the API
	statsAPIDateFormat = "2006-01-02"
)

type testStatsHandler struct {
	sc     data.Connector
	filter stats.StatsFilter
}

func (tsh *testStatsHandler) Factory() gimlet.RouteHandler {
	return &testStatsHandler{sc: tsh.sc}
}

func (tsh *testStatsHandler) Parse(ctx context.Context, r *http.Request) error {
	vals := r.URL.Query()
	project := gimlet.GetVars(r)["project_id"]
	tsh.filter = stats.StatsFilter{Project: project}

	err := tsh.parseTestStatsFilter(vals)
	if err != nil {
		return err
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
	var err error
	var testStatsResult []model.APITestStats

	testStatsResult, err = tsh.sc.GetTestStats(&tsh.filter)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Failed to retrieve the test stats"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
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
				Key:             tsh.makeStartAtKey(testStatsResult[requestLimit]),
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

// parseTestStatsFilter parses the query parameter values and fills the struct's filter field.
func (tsh *testStatsHandler) parseTestStatsFilter(vals url.Values) error {
	var err error

	// requesters
	tsh.filter.Requesters, err = tsh.readRequesters(vals["requesters"])
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid requesters value",
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
	tsh.filter.BeforeDate, err = time.ParseInLocation(statsAPIDateFormat, beforeDate, time.UTC)
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
	tsh.filter.AfterDate, err = time.ParseInLocation(statsAPIDateFormat, afterDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid after_date value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// tests
	tsh.filter.Tests = vals["tests"]
	if len(tsh.filter.Tests) > statsAPIMaxNumTests {
		return gimlet.ErrorResponse{
			Message:    "Too many tests values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// tasks
	tsh.filter.Tasks = vals["tasks"]
	if len(tsh.filter.Tasks) > statsAPIMaxNumTasks {
		return gimlet.ErrorResponse{
			Message:    "Too many tasks values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// variants
	tsh.filter.BuildVariants = vals["variants"]

	// distros
	tsh.filter.Distros = vals["distros"]

	// group_num_days
	tsh.filter.GroupNumDays, err = tsh.readInt(vals.Get("group_num_days"), 1, statsAPIMaxGroupNumDays, 1)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid group_num_days value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// limit
	tsh.filter.Limit, err = getLimit(vals)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid limit value",
			StatusCode: http.StatusBadRequest,
		}
	}
	// Add 1 for pagination
	tsh.filter.Limit += 1

	// sort
	tsh.filter.Sort, err = tsh.readSort(vals.Get("sort"))
	if err != nil {
		return err
	}

	// group_by
	tsh.filter.GroupBy, err = tsh.readTestGroupBy(vals.Get("group_by"))
	if err != nil {
		return err
	}

	// start_at
	tsh.filter.StartAt, err = tsh.readStartAt(vals.Get("start_at"))
	return err
}

// readRequesters parses requesters parameter values and translates them into a list of internal Evergreen requester names.
func (tsh *testStatsHandler) readRequesters(requesters []string) ([]string, error) {
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

// readInt parses an integer parameter value, given minimum, maximum, and default values.
func (tsh *testStatsHandler) readInt(intString string, min, max, defaultValue int) (int, error) {
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
func (tsh *testStatsHandler) readSort(sortValue string) (stats.Sort, error) {
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

// readTestGroupBy parses a group_by parameter value and returns the corresponding GroupBy struct.
func (tsh *testStatsHandler) readTestGroupBy(groupByValue string) (stats.GroupBy, error) {
	switch groupByValue {
	case statsAPITestGroupByDistro:
		return stats.GroupByDistro, nil
	case statsAPITestGroupByVariant:
		return stats.GroupByVariant, nil
	case statsAPITestGroupByTask:
		return stats.GroupByTask, nil
	case statsAPITestGroupByTest:
		return stats.GroupByTest, nil
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
func (tsh *testStatsHandler) readStartAt(startAtValue string) (*stats.StartAt, error) {
	if startAtValue == "" {
		return nil, nil
	}
	elements := strings.Split(startAtValue, "|")
	if len(elements) != 5 {
		return nil, gimlet.ErrorResponse{
			Message:    "Invalid start_by value",
			StatusCode: http.StatusBadRequest,
		}
	}
	date, err := time.ParseInLocation(statsAPIDateFormat, elements[0], time.UTC)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			Message:    "Invalid start_by value",
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

// makeStartAtKey creates a key string that can be used as a start_at value to fetch
// the next results with pagination.
func (tsh *testStatsHandler) makeStartAtKey(testStats model.APITestStats) string {
	elements := []string{
		testStats.Date,
		testStats.BuildVariant,
		testStats.TaskName,
		testStats.TestFile,
		testStats.Distro,
	}
	return strings.Join(elements, "|")
}

func makeGetProjectTestStats(sc data.Connector) gimlet.RouteHandler {
	return &testStatsHandler{sc: sc}
}
