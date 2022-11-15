package route

// This file defines the handlers for the endpoints to query the test and task execution statistics.

import (
	"context"
	"database/sql"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	dbModel "github.com/evergreen-ci/evergreen/model"
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
	statsAPIRequesterGitTag   = "git_tag"
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
	StatsAPITaskGroupByDistro  = "task_variant_distro"
	StatsAPITaskGroupByVariant = "task_variant"
	StatsAPITaskGroupByTask    = "task"

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

	sh.filter.Requesters, err = sh.readRequesters(sh.readStringList(vals["requesters"]))
	if err != nil {
		return errors.Wrap(err, "invalid requesters")
	}

	sh.filter.BuildVariants = sh.readStringList(vals["variants"])

	sh.filter.Distros = sh.readStringList(vals["distros"])

	sh.filter.GroupNumDays, err = sh.readInt(vals.Get("group_num_days"), 1, statsAPIMaxGroupNumDays, 1)
	if err != nil {
		return errors.Wrap(err, "invalid grouping by number of days")
	}

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

	sh.filter.GroupBy, err = sh.readGroupBy(vals.Get("group_by"))
	if err != nil {
		return err
	}

	sh.filter.Limit, err = sh.readInt(vals.Get("limit"), 1, statsAPIMaxLimit, statsAPIMaxLimit)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrap(err, "invalid limit").Error(),
			StatusCode: http.StatusBadRequest,
		}
	}
	// Add 1 for pagination
	sh.filter.Limit++

	sh.filter.Tasks = sh.readStringList(vals["tasks"])
	if len(sh.filter.Tasks) > statsAPIMaxNumTasks {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("number of tasks given must not exceed %d", statsAPIMaxNumTasks),
			StatusCode: http.StatusBadRequest,
		}
	}

	beforeDate := vals.Get("before_date")
	if beforeDate == "" {
		return gimlet.ErrorResponse{
			Message:    "missing 'before' date",
			StatusCode: http.StatusBadRequest,
		}
	}
	sh.filter.BeforeDate, err = time.ParseInLocation(statsAPIDateFormat, beforeDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrapf(err, "parsing 'before' date in expected format (%s)", statsAPIDateFormat).Error(),
			StatusCode: http.StatusBadRequest,
		}
	}

	afterDate := vals.Get("after_date")
	if afterDate == "" {
		return gimlet.ErrorResponse{
			Message:    "missing 'after' date",
			StatusCode: http.StatusBadRequest,
		}
	}
	sh.filter.AfterDate, err = time.ParseInLocation(statsAPIDateFormat, afterDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrapf(err, "parsing 'after' date in expected format (%s)", statsAPIDateFormat).Error(),
			StatusCode: http.StatusBadRequest,
		}
	}

	sh.filter.Tests = sh.readStringList(vals["tests"])
	if len(sh.filter.Tests) > statsAPIMaxNumTests {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("number of tests given must not exceed %d", statsAPIMaxNumTests),
			StatusCode: http.StatusBadRequest,
		}
	}

	sh.filter.Sort, err = sh.readSort(vals.Get("sort"))
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrap(err, "invalid sort").Error(),
			StatusCode: http.StatusBadRequest,
		}
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
		case statsAPIRequesterGitTag:
			requesterValues = append(requesterValues, evergreen.GitTagRequester)
		case statsAPIRequesterAdhoc:
			requesterValues = append(requesterValues, evergreen.AdHocRequester)
		default:
			return nil, errors.Errorf("invalid requester '%s'", requester)
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
		return 0, gimlet.ErrorResponse{
			Message:    fmt.Sprintf("integer value %d must be between %d and %d", value, min, max),
			StatusCode: http.StatusBadRequest,
		}
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
			Message:    fmt.Sprintf("invalid sort '%s'", sortValue),
			StatusCode: http.StatusBadRequest,
		}
	}
}

// readGroupBy parses a group_by parameter value and returns the corresponding GroupBy struct.
func (sh *StatsHandler) readGroupBy(groupByValue string) (stats.GroupBy, error) {
	switch groupByValue {

	// Task query parameters.
	case StatsAPITaskGroupByDistro:
		return stats.GroupByDistro, nil
	case StatsAPITaskGroupByVariant:
		return stats.GroupByVariant, nil
	case StatsAPITaskGroupByTask:
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
			Message:    fmt.Sprintf("invalid grouping '%s'", groupByValue),
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
			Message:    "invalid 'start' time",
			StatusCode: http.StatusBadRequest,
		}
	}
	date, err := time.ParseInLocation(statsAPIDateFormat, elements[0], time.UTC)
	if err != nil {
		return nil, gimlet.ErrorResponse{
			Message:    errors.Wrapf(err, "parsing date in expected format (%s)", statsAPIDateFormat).Error(),
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
	StatsHandler
	url string

	prestoFilter *stats.PrestoTestStatsFilter
	db           *sql.DB
}

func (tsh *testStatsHandler) Factory() gimlet.RouteHandler {
	return &testStatsHandler{
		url: tsh.url,
		db:  tsh.db,
	}
}

func makeGetProjectTestStats(url string, db *sql.DB) gimlet.RouteHandler {
	return &testStatsHandler{
		url: url,
		db:  db,
	}
}

func (tsh *testStatsHandler) Parse(ctx context.Context, r *http.Request) error {
	project := gimlet.GetVars(r)["project_id"]
	vals := r.URL.Query()

	identifier, err := dbModel.GetIdentifierForProject(project)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("getting project identifier for '%s'", project),
			StatusCode: http.StatusInternalServerError,
		}
	}
	if identifier == project {
		// If the passed in project_id is actually an identifier, verify that we have the correct ID
		project, err = dbModel.GetIdForProject(project)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    fmt.Sprintf("getting project ID for '%s'", project),
				StatusCode: http.StatusInternalServerError,
			}
		}
	}

	// If this project is owned by Server and uses Resmoke, we need to use
	// Evergreen test stats.
	if dbModel.IsServerResmokeProject(identifier) {
		tsh.filter = stats.StatsFilter{Project: project}
		err := tsh.parseStatsFilter(vals)
		if err != nil {
			return errors.Wrap(err, "invalid query parameters")
		}

		return errors.Wrap(tsh.filter.ValidateForTests(), "invalid filter")
	}

	return tsh.parsePrestoStatsFilter(project, vals)
}

func (tsh *testStatsHandler) parsePrestoStatsFilter(project string, vals url.Values) error {
	var err error

	tsh.prestoFilter = &stats.PrestoTestStatsFilter{
		Project:  project,
		Variant:  vals.Get("variants"),
		TaskName: vals.Get("tasks"),
		TestName: vals.Get("tests"),
		SortDesc: vals.Get("sort") == statsAPISortLatest,
		DB:       tsh.db,
	}
	for _, requester := range vals["requesters"] {
		switch requester {
		case statsAPIRequesterMainline:
			tsh.prestoFilter.Requesters = append(tsh.prestoFilter.Requesters, evergreen.RepotrackerVersionRequester)
		case statsAPIRequesterPatch:
			tsh.prestoFilter.Requesters = append(tsh.prestoFilter.Requesters, evergreen.PatchRequesters...)
		case statsAPIRequesterTrigger:
			tsh.prestoFilter.Requesters = append(tsh.prestoFilter.Requesters, evergreen.TriggerRequester)
		case statsAPIRequesterGitTag:
			tsh.prestoFilter.Requesters = append(tsh.prestoFilter.Requesters, evergreen.GitTagRequester)
		case statsAPIRequesterAdhoc:
			tsh.prestoFilter.Requesters = append(tsh.prestoFilter.Requesters, evergreen.AdHocRequester)
		default:
			return errors.Errorf("invalid requester value '%s'", requester)
		}
	}
	if groupBy := vals.Get("group_by"); groupBy != "" {
		if groupBy != statsAPITestGroupByTest {
			return errors.New("invalid group by value")
		}
		tsh.prestoFilter.GroupByTest = true
	}
	if afterDate := vals.Get("after_date"); afterDate != "" {
		tsh.prestoFilter.AfterDate, err = time.ParseInLocation(statsAPIDateFormat, afterDate, time.UTC)
		if err != nil {
			return errors.Wrap(err, "invalid after date value")
		}
	}
	if beforeDate := vals.Get("before_date"); beforeDate != "" {
		tsh.prestoFilter.BeforeDate, err = time.ParseInLocation(statsAPIDateFormat, beforeDate, time.UTC)
		if err != nil {
			return errors.Wrap(err, "invalid before date value")
		}
	}
	if groupNumDays := vals.Get("group_num_days"); groupNumDays != "" {
		numDays, err := strconv.Atoi(groupNumDays)
		if err != nil {
			return errors.Wrap(err, "invalid group num days value")
		}

		totalDays := int(tsh.prestoFilter.BeforeDate.Sub(tsh.prestoFilter.AfterDate).Hours() / 24)
		if numDays == totalDays {
			tsh.prestoFilter.GroupDays = true
		} else if numDays != 1 {
			return errors.New("invalid group num days value: must be either 1 or number of days in the given date range")
		}
	}
	if offset := vals.Get("start_at"); offset != "" {
		tsh.prestoFilter.Offset, err = strconv.Atoi(offset)
		if err != nil {
			return errors.Wrap(err, "invalid start at value")
		}
	}
	if limit := vals.Get("limit"); limit != "" {
		tsh.prestoFilter.Limit, err = strconv.Atoi(limit)
		if err != nil {
			return errors.Wrap(err, "invalid limit")
		}
		// Increment limit by one for pagination.
		tsh.prestoFilter.Limit++
	}

	return errors.Wrap(tsh.prestoFilter.Validate(), "invalid query parameters")
}

func (tsh *testStatsHandler) Run(ctx context.Context) gimlet.Responder {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting service flags"))
	}
	if flags.CacheStatsEndpointDisabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "endpoint is disabled",
			StatusCode: http.StatusServiceUnavailable,
		})
	}

	var testStatsResult []model.APITestStats
	if tsh.prestoFilter != nil {
		testStatsResult, err = data.GetPrestoTestStats(ctx, *tsh.prestoFilter)
	} else {
		testStatsResult, err = data.GetTestStats(tsh.filter)
	}
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting filtered test stats"))
	}

	resp := gimlet.NewJSONResponse(nil)
	requestLimit := tsh.filter.Limit - 1
	if tsh.prestoFilter != nil {
		requestLimit = tsh.prestoFilter.Limit - 1
	}
	lastIndex := len(testStatsResult)
	if len(testStatsResult) > requestLimit {
		lastIndex = requestLimit

		var key string
		if tsh.prestoFilter != nil {
			key = strconv.Itoa(tsh.prestoFilter.Offset + requestLimit)
		} else {
			key = testStatsResult[requestLimit].StartAtKey()
		}

		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tsh.url,
				Key:             key,
				Limit:           requestLimit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
	}
	if err := resp.AddData(testStatsResult[:lastIndex]); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "adding response data"))
	}

	return resp
}

///////////////////////////////////////////////
// /projects/<project_id>/task_stats handler //
///////////////////////////////////////////////

type taskStatsHandler struct {
	StatsHandler
	url string
}

func (tsh *taskStatsHandler) Factory() gimlet.RouteHandler {
	return &taskStatsHandler{url: tsh.url}
}

func makeGetProjectTaskStats(url string) gimlet.RouteHandler {
	return &taskStatsHandler{url: url}
}

func (tsh *taskStatsHandler) Parse(ctx context.Context, r *http.Request) error {
	project := gimlet.GetVars(r)["project_id"]
	projectId, err := dbModel.GetIdForProject(project)
	if err != nil {
		return errors.Wrapf(err, "project ID not found for project '%s'", project)
	}
	tsh.filter = stats.StatsFilter{Project: projectId}

	if err = tsh.StatsHandler.parseStatsFilter(r.URL.Query()); err != nil {
		return errors.Wrap(err, "invalid query parameters")
	}
	if err = tsh.filter.ValidateForTasks(); err != nil {
		return errors.Wrap(err, "invalid filter")
	}
	return nil
}

func (tsh *taskStatsHandler) Run(ctx context.Context) gimlet.Responder {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting service flags"))
	}
	if flags.CacheStatsEndpointDisabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "endpoint is disabled",
			StatusCode: http.StatusServiceUnavailable,
		})
	}

	var taskStatsResult []model.APITaskStats

	taskStatsResult, err = data.GetTaskStats(tsh.filter)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting task stats"))
	}
	if len(taskStatsResult) == 0 {
		statsStatus, err := stats.GetStatsStatus(tsh.filter.Project)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting stats status for project '%s'", tsh.filter.Project))
		}
		if statsStatus.ProcessedTasksUntil.Before(tsh.filter.AfterDate) {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				Message:    "stats for this time range have not been generated yet",
				StatusCode: http.StatusServiceUnavailable,
			})
		}
	}

	resp := gimlet.NewResponseBuilder()
	requestLimit := tsh.filter.Limit - 1
	lastIndex := len(taskStatsResult)
	if len(taskStatsResult) > requestLimit {
		lastIndex = requestLimit

		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         tsh.url,
				Key:             taskStatsResult[requestLimit].StartAtKey(),
				Limit:           requestLimit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"paginating response"))
		}
	}
	taskStatsResult = taskStatsResult[:lastIndex]

	for i, apiTaskStats := range taskStatsResult {
		if err = resp.AddData(apiTaskStats); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding task stats at index %d", i))
		}
	}

	return resp
}
