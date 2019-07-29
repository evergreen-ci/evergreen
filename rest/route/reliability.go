package route

// This file defines the handlers for the endpoints to query task reliability.

import (
        "context"
        "net/http"
        "net/url"

        "github.com/evergreen-ci/evergreen"
        "github.com/evergreen-ci/evergreen/model/reliability"
        "github.com/evergreen-ci/evergreen/rest/data"
        "github.com/evergreen-ci/gimlet"
        "github.com/pkg/errors"
)

/////////////////////////////////////////////////////
// /projects/<project_id>/task_reliability handler //
/////////////////////////////////////////////////////

type taskReliabilityHandler struct {
        filter reliability.TaskReliabilityFilter
        sc     data.Connector
}

// parseStatsFilter parses the query parameter values and fills the struct's filter field.
func (tsh *taskReliabilityHandler) parseTaskReliabilityFilter(vals url.Values) error {
        return nil
}

func (tsh *taskReliabilityHandler) Factory() gimlet.RouteHandler {
        return &taskReliabilityHandler{sc: tsh.sc}
}

func (tsh *taskReliabilityHandler) Parse(ctx context.Context, r *http.Request) error {
        tsh.filter = reliability.TaskReliabilityFilter{Project: gimlet.GetVars(r)["project_id"]}

        err := tsh.parseTaskReliabilityFilter(r.URL.Query())
        if err != nil {
                return errors.Wrap(err, "Invalid query parameters")
        }
        err = tsh.filter.ValidateForTaskReliability()
        if err != nil {
                return gimlet.ErrorResponse{
                        Message:    err.Error(),
                        StatusCode: http.StatusBadRequest,
                }
        }
        return nil
}

func (tsh *taskReliabilityHandler) Run(ctx context.Context) gimlet.Responder {
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
        resp := gimlet.NewResponseBuilder()
        if err = resp.SetFormat(gimlet.JSON); err != nil {
                return gimlet.MakeJSONInternalErrorResponder(err)
        }
        return resp
}

func makeGetProjectTaskReliability(sc data.Connector) gimlet.RouteHandler {
        return &taskReliabilityHandler{sc: sc}
}
