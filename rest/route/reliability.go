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
	reliabilityAPIMaxNumTasksLimit = 50
	dayInHours                     = 24 * time.Hour
)

/////////////////////////////////////////////////////
// /projects/<project_id>/task_reliability handler //
/////////////////////////////////////////////////////

type taskReliabilityHandler struct {
	filter reliability.TaskReliabilityFilter
	sc     data.Connector
}

func makeGetProjectTaskReliability(sc data.Connector) gimlet.RouteHandler {
	return &taskReliabilityHandler{sc: sc}
}

func (sh *taskReliabilityHandler) Factory() gimlet.RouteHandler {
	return &taskReliabilityHandler{sc: sh.sc}
}

// Get the default before_date.
func getDefaultBeforeDate() string {
	before := time.Now().UTC()
	before = before.Add(dayInHours).Truncate(dayInHours)
	return before.Format(statsAPIDateFormat)
}

// Get the default after_date.
func getDefaultAfterDate() string {
	after := time.Now().UTC()
	after = after.Add(-20 * dayInHours).Truncate(dayInHours)
	return after.Format(statsAPIDateFormat)
}

// parseCommonFilter parses common query parameter values and fills the filter struct fields.
func (sh *taskReliabilityHandler) parseCommonFilter(vals url.Values) error {
	var err error
	statsHandler := StatsHandler{stats.StatsFilter{Project: sh.filter.Project}}
	err = statsHandler.ParseCommonFilter(vals)
	if err == nil {
		sh.filter.Requesters = statsHandler.filter.Requesters
		sh.filter.BuildVariants = statsHandler.filter.BuildVariants
		sh.filter.Distros = statsHandler.filter.Distros
		sh.filter.GroupNumDays = statsHandler.filter.GroupNumDays
		sh.filter.GroupBy = statsHandler.filter.GroupBy
		sh.filter.StartAt = statsHandler.filter.StartAt
		sh.filter.Sort = statsHandler.filter.Sort

		// limit
		sh.filter.Limit, err = statsHandler.readInt(vals.Get("limit"), 1, reliabilityAPIMaxNumTasksLimit, reliabilityAPIMaxNumTasksLimit)
		if err != nil {
			return gimlet.ErrorResponse{
				Message:    "Invalid limit value",
				StatusCode: http.StatusBadRequest,
			}
		}
		// Add 1 for pagination
		sh.filter.Limit++

		// tasks
		sh.filter.Tasks = statsHandler.readStringList(vals["tasks"])
		if len(sh.filter.Tasks) == 0 {
			return gimlet.ErrorResponse{
				Message:    "Missing Tasks values",
				StatusCode: http.StatusBadRequest,
			}
		}
		if len(sh.filter.Tasks) > reliabilityAPIMaxNumTasksLimit {
			return gimlet.ErrorResponse{
				Message:    "Too many Tasks values",
				StatusCode: http.StatusBadRequest,
			}
		}
	}
	return err
}

// parseStatsFilter parses the query parameter values and fills the struct filter field.
func (sh *taskReliabilityHandler) parseTaskReliabilityFilter(vals url.Values) error {
	var err error

	err = sh.parseCommonFilter(vals)
	if err != nil {
		return err
	}

	// before_date, defaults to tomorrow
	beforeDate := sh.readString(vals.Get("before_date"), getDefaultBeforeDate())
	sh.filter.BeforeDate, err = time.ParseInLocation(statsAPIDateFormat, beforeDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid before_date value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// after_date
	afterDate := sh.readString(vals.Get("after_date"), getDefaultAfterDate())
	sh.filter.AfterDate, err = time.ParseInLocation(statsAPIDateFormat, afterDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid after_date value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// sort
	sh.filter.Sort, err = sh.readSort(vals.Get("sort"))
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid sort value",
			StatusCode: http.StatusBadRequest,
		}
	}

	// significance
	sh.filter.Significance, err = sh.readFloat(vals.Get("significance"), 0.0, 1.0, reliability.DefaultSignificance)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "Invalid Significance value",
			StatusCode: http.StatusBadRequest,
		}
	}

	return nil
}

// readFloat parses an integer parameter value, given minimum, maximum, and default values.
func (sh *taskReliabilityHandler) readFloat(floatString string, min, max, defaultValue float64) (float64, error) {
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
func (sh *taskReliabilityHandler) readString(value string, defaultValue string) string {
	if value == "" {
		return defaultValue
	}
	return value
}

// readSort parses a sort parameter value and returns the corresponding Sort struct.
// defaults to latest first.
func (sh *taskReliabilityHandler) readSort(sortValue string) (stats.Sort, error) {
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

func (sh *taskReliabilityHandler) Parse(ctx context.Context, r *http.Request) error {
	sh.filter = reliability.TaskReliabilityFilter{
		Project:      gimlet.GetVars(r)["project_id"],
		Significance: reliability.DefaultSignificance,
	}

	err := sh.parseTaskReliabilityFilter(r.URL.Query())
	if err != nil {
		return errors.Wrap(err, "Invalid query parameters")
	}
	err = sh.filter.ValidateForTaskReliability()
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    err.Error(),
			StatusCode: http.StatusBadRequest,
		}
	}
	return nil
}

func (sh *taskReliabilityHandler) Run(ctx context.Context) gimlet.Responder {
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

	taskReliabilityResult, err = sh.sc.GetTaskReliabilityScores(sh.filter)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Failed to retrieve the task stats"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	requestLimit := sh.filter.Limit - 1
	lastIndex := len(taskReliabilityResult)
	if len(taskReliabilityResult) > requestLimit {
		lastIndex = requestLimit

		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         sh.sc.GetURL(),
				Key:             taskReliabilityResult[requestLimit].StartAtKey(),
				Limit:           requestLimit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"Problem paginating response"))
		}
	}
	taskReliabilityResult = taskReliabilityResult[:lastIndex]

	for _, apiTaskStats := range taskReliabilityResult {
		if err = resp.AddData(apiTaskStats); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	return resp
}
