package route

// This file defines the handlers for the endpoints to query task reliability.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/reliability"
	"github.com/evergreen-ci/evergreen/model/taskstats"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
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
	url    string
}

// Factory creates an instance of the handler.
//
//	@Summary			Get task reliability scores
//	@Description		Returns a paginated list of reliability scores for the project's tasks, grouped by the group_by parameter. Scores cover the 26 weeks ending at before_date.
//	@Tags				projects
//	@Router				/projects/{project_id}/task_reliability [get]
//	@Security			Api-User || Api-Key
//	@Param				project_id		path	string	true	"the project ID or identifier"
//	@Param				start_at		query	string	false	"Compound cursor for pagination, as returned by the Link response header. Pages list results at or before this cursor. Omit for the first page."
//	@Param				limit			query	int		false	"The number of results to be returned per page of pagination. Defaults to 1000 and cannot exceed 1000."
//	@Param				group_by		query	string	false	"How to group results: 'task_variant_distro' (default), 'task_variant', or 'task'."
//	@Param				tasks			query	string	false	"Only return results for these task names. Can be a comma-separated list or repeated."
//	@Param				variants		query	string	false	"Only return results for these build variants. Can be a comma-separated list or repeated."
//	@Param				distros			query	string	false	"Only return results for these distros. Can be a comma-separated list or repeated."
//	@Param				before_date		query	string	false	"Only return results from before this date, in YYYY-MM-DD format. Defaults to today."
//	@Param				after_date		query	string	false	"Only return results from after this date, in YYYY-MM-DD format. Defaults to before_date."
//	@Param				sort			query	string	false	"Sort order of results: 'earliest' or 'latest'. Defaults to 'latest'."
//	@Param				significance	query	number	false	"Minimum test significance, from 0 to 1. Defaults to 0."
//	@Success			200				{array}	model.APITaskReliability
//	@Header				200				{string}	Link	"Pagination links (rel="next") returned when more results exist. Follow this header verbatim instead of constructing pagination URLs."
//	@x-pagination-type	"stats-cursor"
func makeGetProjectTaskReliability() gimlet.RouteHandler {
	return &taskReliabilityHandler{}
}

func (trh *taskReliabilityHandler) Factory() gimlet.RouteHandler {
	return &taskReliabilityHandler{}
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
func (trh *taskReliabilityHandler) readGroupBy(groupByValue string) (taskstats.GroupBy, error) {
	switch groupByValue {

	// Task query parameters.
	case StatsAPITaskGroupByDistro:
		return taskstats.GroupByDistro, nil
	case StatsAPITaskGroupByVariant:
		return taskstats.GroupByVariant, nil
	case StatsAPITaskGroupByTask:
		return taskstats.GroupByTask, nil

	// Default value.
	case "":
		return taskstats.GroupByTask, nil

	default:
		return taskstats.GroupBy(""), gimlet.ErrorResponse{
			Message:    "invalid 'group by'",
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
			Message:    "invalid limit",
			StatusCode: http.StatusBadRequest,
		}
	}

	// tasks
	trh.filter.Tasks = trh.readStringList(vals["tasks"])
	if len(trh.filter.Tasks) == 0 {
		return gimlet.ErrorResponse{
			Message:    "must specify at least one task",
			StatusCode: http.StatusBadRequest,
		}
	}
	if len(trh.filter.Tasks) > reliabilityAPIMaxNumTasksLimit {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("cannot request more than %d tasks", reliabilityAPIMaxNumTasksLimit),
			StatusCode: http.StatusBadRequest,
		}
	}

	// before_date, defaults to tomorrow
	beforeDate := trh.readString(vals.Get("before_date"), getDefaultBeforeDate())
	trh.filter.BeforeDate, err = time.ParseInLocation(statsAPIDateFormat, beforeDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "invalid 'before' date",
			StatusCode: http.StatusBadRequest,
		}
	}

	// after_date
	afterDate := trh.readString(vals.Get("after_date"), beforeDate)
	trh.filter.AfterDate, err = time.ParseInLocation(statsAPIDateFormat, afterDate, time.UTC)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "invalid 'after' date",
			StatusCode: http.StatusBadRequest,
		}
	}

	// sort
	trh.filter.Sort, err = trh.readSort(vals.Get("sort"))
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "invalid sort",
			StatusCode: http.StatusBadRequest,
		}
	}

	// significance
	trh.filter.Significance, err = trh.readFloat(vals.Get("significance"), 0.0, 1.0, reliability.DefaultSignificance)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "invalid significance value",
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
		return 0, errors.New("invalid float value")
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
func (trh *taskReliabilityHandler) readSort(sortValue string) (taskstats.Sort, error) {
	switch sortValue {
	case statsAPISortEarliest:
		return taskstats.SortEarliestFirst, nil
	case statsAPISortLatest:
		return taskstats.SortLatestFirst, nil
	case "":
		return taskstats.SortLatestFirst, nil
	default:
		return taskstats.Sort(""), gimlet.ErrorResponse{
			Message:    fmt.Sprintf("invalid sort '%s'", sortValue),
			StatusCode: http.StatusBadRequest,
		}
	}
}

func (trh *taskReliabilityHandler) Parse(ctx context.Context, r *http.Request) error {
	trh.url = util.HttpsUrl(r.Host)

	trh.filter = reliability.TaskReliabilityFilter{
		StatsFilter:  taskstats.StatsFilter{Project: gimlet.GetVars(r)["project_id"]},
		Significance: reliability.DefaultSignificance,
	}

	err := trh.parseTaskReliabilityFilter(r.URL.Query())
	if err != nil {
		return errors.Wrap(err, "parsing task reliability parameters")
	}
	err = trh.filter.ValidateForTaskReliability()
	if err != nil {
		return errors.Wrap(err, "invalid task reliability parameters")
	}
	return nil
}

func (trh *taskReliabilityHandler) Run(ctx context.Context) gimlet.Responder {
	flags, err := evergreen.GetServiceFlags(ctx)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "retrieving service flags"))
	}
	if flags.TaskReliabilityDisabled {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			Message:    "endpoint is disabled",
			StatusCode: http.StatusServiceUnavailable,
		})
	}

	var taskReliabilityResult []model.APITaskReliability

	taskReliabilityResult, err = data.GetTaskReliabilityScores(ctx, trh.filter)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting task reliability stats"))
	}

	resp := gimlet.NewResponseBuilder()
	requestLimit := trh.filter.Limit
	if len(taskReliabilityResult) == requestLimit {
		last := taskReliabilityResult[len(taskReliabilityResult)-1]
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         trh.url,
				Key:             last.StartAtKey(),
				Limit:           requestLimit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
		}
	}

	for _, apiTaskStats := range taskReliabilityResult {
		if err = resp.AddData(apiTaskStats); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding response data for task '%s' in build variant '%s'", utility.FromStringPtr(apiTaskStats.TaskName), utility.FromStringPtr(apiTaskStats.BuildVariant)))
		}
	}

	return resp
}
