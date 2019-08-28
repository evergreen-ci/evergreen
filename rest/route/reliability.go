package route

// This file defines the handlers for the endpoints to query task reliability.

import (
	"context"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/stats"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

const (
	// reliabilityAPIMaxNumTasksLimit is the max number of structs returned in a single
	// page of results. That is, the 'limit' URL param is math.Min(limit, reliabilityAPIMaxNumTasksLimit).
	// A high value like 1000 will generally ensures that there will only be a single page of results.
	reliabilityAPIMaxNumTasksLimit = 1000
	dayInHours                     = 24 * time.Hour
)

/////////////////////////////////////////////////////
// /projects/<project_id>/task_reliability handler //
/////////////////////////////////////////////////////

type taskReliabilityHandler struct {
	StatsHandler
	filter reliability.TaskReliabilityFilter
	sc     data.Connector
}

func makeGetProjectTaskReliability(sc data.Connector) gimlet.RouteHandler {
	return &taskReliabilityHandler{sc: sc}
}

func (trh *taskReliabilityHandler) Factory() gimlet.RouteHandler {
	return &taskReliabilityHandler{sc: trh.sc}
}

// Get the default before_date.
func getDefaultBeforeDate() string {
	before := time.Now().UTC()
	before = before.Truncate(dayInHours)
	return before.Format(statsAPIDateFormat)
}

// ParseCommonFilter wraps StatsHandler.ParseCommonFilter and copies the Statshandler
// struct contents into the TaskReliabilityHandler filter fields.
func (trh *taskReliabilityHandler) ParseCommonFilter(vals url.Values) error {
	err := trh.StatsHandler.ParseCommonFilter(vals)
	if err == nil {
		trh.filter.Requesters = trh.StatsHandler.filter.Requesters
		trh.filter.BuildVariants = trh.StatsHandler.filter.BuildVariants
		trh.filter.Distros = trh.StatsHandler.filter.Distros
		trh.filter.GroupNumDays = trh.StatsHandler.filter.GroupNumDays
		trh.filter.StartAt = trh.StatsHandler.filter.StartAt
		trh.filter.Sort = trh.StatsHandler.filter.Sort
	}
	return err
}

// readGroupBy parses a group_by parameter value and returns the corresponding GroupBy struct.
func (trh *taskReliabilityHandler) readGroupBy(groupByValue string) (stats.GroupBy, error) {
	switch groupByValue {

	// Task query parameters.
	case StatsAPITaskGroupByDistro:
		return stats.GroupByDistro, nil
	case StatsAPITaskGroupByVariant:
		return stats.GroupByVariant, nil
	case StatsAPITaskGroupByTask:
		return stats.GroupByTask, nil

	// Default value.
	case "":
		return stats.GroupByTask, nil

	default:
		return stats.GroupBy(""), gimlet.ErrorResponse{
			Message:    "Invalid group_by value",
			StatusCode: http.StatusBadRequest,
		}
	}
}

// parseStatsFilter parses the query parameter values and fills the struct filter field.
func (trh *taskReliabilityHandler) parseTaskReliabilityFilter(vals url.Values) error {
	var err error

	err = trh.ParseCommonFilter(vals)
	if err != nil {
		return err
	}

	// group_by
	trh.filter.GroupBy, err = trh.readGroupBy(vals.Get("group_by"))
	if err != nil {
		return err
	}

	// limit
	trh.filter.Limit, err = trh.readInt(vals.Get("limit"), 1, reliabilityAPIMaxNumTasksLimit, reliabilityAPIMaxNumTasksLimit)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid limit value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// tasks
	trh.filter.Tasks = trh.readStringList(vals["tasks"])
	if len(trh.filter.Tasks) == 0 {
		return gimlet.ErrorResponse{
			Message:    "Missing Tasks values",
			StatusCode: http.StatusBadRequest,
		}
	}
	if len(trh.filter.Tasks) > reliabilityAPIMaxNumTasksLimit {
		return gimlet.ErrorResponse{
			Message:    "Too many Tasks values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// before_date, defaults to tomorrow
	beforeDate := trh.readString(vals.Get("before_date"), getDefaultBeforeDate())
	trh.filter.BeforeDate, err = time.ParseInLocation(statsAPIDateFormat, beforeDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid before_date value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// after_date
	afterDate := trh.readString(vals.Get("after_date"), beforeDate)
	trh.filter.AfterDate, err = time.ParseInLocation(statsAPIDateFormat, afterDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid after_date value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// sort
	trh.filter.Sort, err = trh.readSort(vals.Get("sort"))
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid sort value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// significance
	trh.filter.Significance, err = trh.readFloat(vals.Get("significance"), 0.0, 1.0, reliability.DefaultSignificance)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid Significance value",
			StatusCode: http.StatusBadRequest,
		}
	}

	return nil
}

// readFloat parses an integer parameter value, given minimum, maximum, and default values.
func (trh *taskReliabilityHandler) readFloat(floatString string, min, max, defaultValue float64) (float64, error) {
	if floatString == "" {
		return defaultValue, nil
	}
	value, err := strconv.ParseFloat(floatString, 64)
	if err != nil {
		return 0, err
	}

	if value < min || value > max {
		return 0, errors.New("Invalid float parameter value")
	}
	return value, nil
}

// readString reads a string parameter value, and default values.
func (trh *taskReliabilityHandler) readString(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// readSort parses a sort parameter value and returns the corresponding Sort struct.
// defaults to latest first.
func (trh *taskReliabilityHandler) readSort(sortValue string) (stats.Sort, error) {
	switch sortValue {
	case statsAPISortEarliest:
		return stats.SortEarliestFirst, nil
	case statsAPISortLatest:
		return stats.SortLatestFirst, nil
	case "":
		return stats.SortLatestFirst, nil
	default:
		return stats.Sort(""), gimlet.ErrorResponse{
			Message:    "Invalid sort value",
			StatusCode: http.StatusBadRequest,
		}
	}
}

func (trh *taskReliabilityHandler) Parse(ctx context.Context, r *http.Request) error {
	trh.filter = reliability.TaskReliabilityFilter{
		StatsFilter:  stats.StatsFilter{Project: gimlet.GetVars(r)["project_id"]},
		Significance: reliability.DefaultSignificance,
	}

	err := trh.parseTaskReliabilityFilter(r.URL.Query())
	if err != nil {
		return errors.Wrap(err, "Invalid query parameters")
	}
	err = trh.filter.ValidateForTaskReliability()
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (trh *taskReliabilityHandler) Run(ctx context.Context) gimlet.Responder {
	flags, err := evergreen.GetServiceFlags()
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error retrieving service flags"))
	}
	if flags.TaskReliabilityDisabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "endpoint is disabled",
			StatusCode: http.StatusServiceUnavailable,
		})
	}

	var taskReliabilityResult []model.APITaskReliability

	taskReliabilityResult, err = trh.sc.GetTaskReliabilityScores(trh.filter)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Failed to retrieve the task stats"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	requestLimit := trh.filter.Limit
	if len(taskReliabilityResult) == requestLimit {
		last := taskReliabilityResult[len(taskReliabilityResult)-1]
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         trh.sc.GetURL(),
				Key:             last.StartAtKey(),
				Limit:           requestLimit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"Problem paginating response"))
		}
	}

	for _, apiTaskStats := range taskReliabilityResult {
		if err = resp.AddData(apiTaskStats); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	return resp
}
