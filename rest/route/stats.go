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
	requesterMainline = "mainline"
	requesterPatch    = "patch"
	requesterTrigger  = "trigger"
	requesterAdhoc    = "adhoc"

	// Sort API values
	sortEarliest = "earliest"
	sortLatest   = "latest"

	// GroupBy API values
	testGroupByDistro  = "test_task_variant_distro"
	testGroupByVariant = "test_task_variant"
	testGroupByTask    = "test_task"
	testGroupByTest    = "test"

	// API Limits
	maxGroupNumDays = 26 * 7 // 26 weeks which is the maximum amount of data available
	maxNumTests     = 50
	maxNumTasks     = 50

	// Format used to encode dates in the API
	dateFormat = "2006-01-02"
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
	if project == "" {
		return gimlet.ErrorResponse{
			Message:    "projectId cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}
	tsh.filter = stats.StatsFilter{Project: project}

	return parseTestStatsFilter(vals, &tsh.filter)
}

func (tsh *testStatsHandler) Run(ctx context.Context) gimlet.Responder {
	var err error
	var testStatsResult []stats.TestStats

	testStatsResult, err = tsh.sc.GetTestStats(&tsh.filter)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Failed to retrieve the test stats"))
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
				Key:             makeStartAtKey(testStatsResult[requestLimit]),
				Limit:           requestLimit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}
	testStatsResult = testStatsResult[:lastIndex]

	for _, apiTestStats := range testStatsResult {
		ats := &model.APITestStats{}
		err = ats.BuildFromService(&apiTestStats)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Model error"))
		}

		if err = resp.AddData(ats); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}

func makeGetProjectTestStats(sc data.Connector) gimlet.RouteHandler {
	return &testStatsHandler{sc: sc}
}

// parseTestStatsFilter parses the query parameter values and fills the corresponding StatsFilter fields.
func parseTestStatsFilter(vals url.Values, filter *stats.StatsFilter) error {
	var err error

	// requesters
	filter.Requesters, err = readRequesters(vals["requesters"])
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid requesters value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// before_date
	beforeDate := vals.Get("before_date")
	if beforeDate != "" {
		filter.BeforeDate, err = readDate(beforeDate)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    "Invalid before_date value",
				StatusCode: http.StatusBadRequest,
			}
		}
	} else {
		return gimlet.ErrorResponse{
			Message:    "Missing before_date parameter",
			StatusCode: http.StatusBadRequest,
		}
	}

	// after_date
	afterDate := vals.Get("after_date")
	if afterDate != "" {
		filter.AfterDate, err = readDate(afterDate)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    "Invalid after_date value",
				StatusCode: http.StatusBadRequest,
			}
		}
	} else {
		return gimlet.ErrorResponse{
			Message:    "Missing after_date parameter",
			StatusCode: http.StatusBadRequest,
		}
	}

	// tests
	filter.Tests = vals["tests"]
	if len(filter.Tests) == 0 {
		return gimlet.ErrorResponse{
			Message:    "Missing required tests parameter",
			StatusCode: http.StatusBadRequest,
		}
	} else if len(filter.Tests) > maxNumTests {
		return gimlet.ErrorResponse{
			Message:    "Too many tests values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// tasks
	filter.Tasks = vals["tasks"]
	if len(filter.Tasks) > maxNumTasks {
		return gimlet.ErrorResponse{
			Message:    "Too many tasks values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// variants
	filter.BuildVariants = vals["variants"]

	// distros
	filter.Distros = vals["distros"]

	// group_num_days
	filter.GroupNumDays, err = readInt(vals.Get("group_num_days"), 1, maxGroupNumDays, 1)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid group_num_days value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// limit
	filter.Limit, err = getLimit(vals)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid limit value",
			StatusCode: http.StatusBadRequest,
		}
	}
	// Add 1 for pagination
	filter.Limit += 1

	// sort
	filter.Sort, err = readSort(vals.Get("sort"))
	if err != nil {
		return err
	}

	// group_by
	filter.GroupBy, err = readTestGroupBy(vals.Get("group_by"))
	if err != nil {
		return err
	}

	// start_at
	filter.StartAt, err = readStartAt(vals.Get("start_at"))
	return err
}

// readRequesters parses requesters parameter values and translates them into a list of internal Evergreen requester names.
func readRequesters(requesters []string) ([]string, error) {
	if len(requesters) == 0 {
		requesters = []string{requesterMainline}
	}
	requesterValues := []string{}
	for _, requester := range requesters {
		switch requester {
		case requesterMainline:
			requesterValues = append(requesterValues, evergreen.RepotrackerVersionRequester)
		case requesterPatch:
			requesterValues = append(requesterValues, evergreen.PatchRequesters...)
		case requesterTrigger:
			requesterValues = append(requesterValues, evergreen.TriggerRequester)
		case requesterAdhoc:
			requesterValues = append(requesterValues, evergreen.AdHocRequester)
		default:
			return nil, errors.Errorf("Invalid requester value %v", requester)
		}
	}
	return requesterValues, nil
}

// readDate parses a date parameter in the expected YYYY-MM-DD format.
func readDate(dateString string) (time.Time, error) {
	return time.ParseInLocation(dateFormat, dateString, time.UTC)
}

// readInt parses an integer parameter value, given minimum, maximum, and default values.
func readInt(intString string, min, max, defaultValue int) (int, error) {
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
func readSort(sortValue string) (stats.Sort, error) {
	switch sortValue {
	case sortEarliest:
		return stats.SortEarliestFirst, nil
	case sortLatest:
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
func readTestGroupBy(groupByValue string) (stats.GroupBy, error) {
	switch groupByValue {
	case testGroupByDistro:
		return stats.GroupByDistro, nil
	case testGroupByVariant:
		return stats.GroupByVariant, nil
	case testGroupByTask:
		return stats.GroupByTask, nil
	case testGroupByTest:
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
func readStartAt(startAtValue string) (*stats.StartAt, error) {
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
	date, err := readDate(elements[0])
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
func makeStartAtKey(testStats stats.TestStats) string {
	startAt := stats.StartAtFromTestStats(&testStats)
	elements := []string{
		startAt.Date.Format(dateFormat),
		startAt.BuildVariant,
		startAt.Task,
		startAt.Test,
		startAt.Distro,
	}
	return strings.Join(elements, "|")
}
