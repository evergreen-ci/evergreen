package route

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// types and functions for Version Cost Route
type costByVersionHandler struct {
	versionId string
	sc        data.Connector
}

func makeCostByVersionHandler(sc data.Connector) gimlet.RouteHandler {
	return &costByVersionHandler{
		sc: sc,
	}
}

func (cbvh *costByVersionHandler) Factory() gimlet.RouteHandler {
	return &costByVersionHandler{
		sc: cbvh.sc,
	}
}

func (cbvh *costByVersionHandler) Parse(ctx context.Context, r *http.Request) error {
	cbvh.versionId = gimlet.GetVars(r)["version_id"]

	if cbvh.versionId == "" {
		return errors.New("request data incomplete")
	}

	return nil
}

func (cbvh *costByVersionHandler) Run(ctx context.Context) gimlet.Responder {
	foundVersionCost, err := cbvh.sc.FindCostByVersionId(cbvh.versionId)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	versionCostModel := &model.APIVersionCost{}

	if err = versionCostModel.BuildFromService(foundVersionCost); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(versionCostModel)
}

// types and functions for Distro Cost Route
type costByDistroHandler struct {
	distroId  string
	startTime time.Time
	duration  time.Duration
	sc        data.Connector
}

func makeCostByDistroHandler(sc data.Connector) gimlet.RouteHandler {
	return &costByDistroHandler{
		sc: sc,
	}
}

func (cbvh *costByDistroHandler) Factory() gimlet.RouteHandler {
	return &costByDistroHandler{sc: cbvh.sc}
}

func parseTime(r *http.Request) (time.Time, time.Duration, error) {
	// Parse start time and duration
	startTime := r.FormValue("starttime")
	duration := r.FormValue("duration")

	// Invalid if not both starttime and duration are given.
	if startTime == "" || duration == "" {
		return time.Time{}, 0, gimlet.ErrorResponse{
			Message:    "both starttime and duration must be given as form values",
			StatusCode: http.StatusBadRequest,
		}
	}

	// Parse time information.
	st, err := time.Parse(time.RFC3339, startTime)
	if err != nil {
		return time.Time{}, 0, gimlet.ErrorResponse{
			Message: fmt.Sprintf("problem parsing time from '%s' (%s). Time must be given in the following format: %s",
				startTime, err.Error(), time.RFC3339),
			StatusCode: http.StatusBadRequest,
		}
	}
	d, err := time.ParseDuration(duration)
	if err != nil {
		return time.Time{}, 0, gimlet.ErrorResponse{
			Message:    fmt.Sprintf("problem parsing duration from '%s' (%s). Duration must be given in the following format: 4h, 2h45m, etc.", duration, err.Error()),
			StatusCode: http.StatusBadRequest,
		}
	}
	return st, d, nil
}

func (cbvh *costByDistroHandler) Parse(ctx context.Context, r *http.Request) error {
	cbvh.distroId = gimlet.GetVars(r)["distro_id"]
	if cbvh.distroId == "" {
		return errors.New("request data incomplete")
	}

	st, d, err := parseTime(r)
	if err != nil {
		return err
	}
	cbvh.startTime = st
	cbvh.duration = d

	return nil
}

func (cbvh *costByDistroHandler) Run(ctx context.Context) gimlet.Responder {
	foundDistroCost, err := cbvh.sc.FindCostByDistroId(cbvh.distroId, cbvh.startTime, cbvh.duration)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Database error"))
	}

	distroCostModel := &model.APIDistroCost{}
	if err = distroCostModel.BuildFromService(foundDistroCost); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(distroCostModel)
}

type costTasksByProjectHandler struct {
	limit     int
	key       string
	duration  time.Duration
	starttime time.Time
	projectID string
	User      *user.DBUser
	sc        data.Connector
}

func makeTaskCostByProjectRoute(sc data.Connector) gimlet.RouteHandler {
	return &costTasksByProjectHandler{
		sc: sc,
	}
}

func (h *costTasksByProjectHandler) Factory() gimlet.RouteHandler {
	return &costTasksByProjectHandler{
		sc: h.sc,
	}
}

func (h *costTasksByProjectHandler) Parse(ctx context.Context, r *http.Request) error {
	h.User = MustHaveUser(ctx)
	h.projectID = gimlet.GetVars(r)["project_id"]
	if h.projectID == "" {
		return errors.New("request data incomplete")
	}

	st, d, err := parseTime(r)
	if err != nil {
		return err
	}
	h.starttime = st
	h.duration = d

	vals := r.URL.Query()

	h.key = vals.Get("start_at")

	h.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	return nil
}

func (h *costTasksByProjectHandler) Run(ctx context.Context) gimlet.Responder {
	endttime := h.starttime.Add(h.duration)
	tasks, err := h.sc.FindCostTaskByProject(h.projectID, h.key, h.starttime, endttime, h.limit+1, 1)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "Database error"))
	}

	if len(tasks) == 0 {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "not found",
		})
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	lastIndex := len(tasks)
	if len(tasks) > h.limit {
		lastIndex = h.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				Relation:        "next",
				LimitQueryParam: "limit",
				KeyQueryParam:   "start_at",
				BaseURL:         h.sc.GetURL(),
				Key:             tasks[h.limit].Id,
				Limit:           h.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	tasks = tasks[:lastIndex]
	for _, t := range tasks {
		taskCostModel := &model.APITaskCost{}
		if err = taskCostModel.BuildFromService(t); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
				Message:    "problem converting task to APITaskCost",
				StatusCode: http.StatusInternalServerError,
			})
		}

		if err = resp.AddData(taskCostModel); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}
