package route

import (
	"context"
	"fmt"
	"github.com/evergreen-ci/evergreen/model/task"
	"net/http"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type hostCreateHandler struct {
	taskID     string
	createHost apimodels.CreateHost
}

func makeHostCreateRouteManager() gimlet.RouteHandler {
	return &hostCreateHandler{}
}

func (h *hostCreateHandler) Factory() gimlet.RouteHandler { return &hostCreateHandler{} }

func (h *hostCreateHandler) Parse(ctx context.Context, r *http.Request) error {
	taskID := gimlet.GetVars(r)["task_id"]
	if taskID == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide task ID",
		}
	}
	h.taskID = taskID

	if err := utility.ReadJSON(r.Body, &h.createHost); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	return h.createHost.Validate()
}

func (h *hostCreateHandler) Run(ctx context.Context) gimlet.Responder {
	numHosts, err := strconv.Atoi(h.createHost.NumHosts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	ids := []string{}
	for i := 0; i < numHosts; i++ {
		intentHost, err := data.MakeIntentHost(h.taskID, "", "", h.createHost)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		ids = append(ids, intentHost.Id)

		grip.Debug(message.Fields{
			"message": "host.create intent",
			"host_id": intentHost.Id,
			"task_id": h.taskID,
		})
	}
	return gimlet.NewJSONResponse(ids)
}

type hostListHandler struct {
	taskID string
}

func makeHostListRouteManager() gimlet.RouteHandler {
	return &hostListHandler{}
}

func (h *hostListHandler) Factory() gimlet.RouteHandler { return &hostListHandler{} }

func (h *hostListHandler) Parse(ctx context.Context, r *http.Request) error {
	taskID := gimlet.GetVars(r)["task_id"]
	if taskID == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide task ID",
		}
	}
	h.taskID = taskID

	return nil
}

func (h *hostListHandler) Run(ctx context.Context) gimlet.Responder {
	hosts, err := data.ListHostsForTask(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task with id %s not found", h.taskID),
		})
	}
	catcher := grip.NewBasicCatcher()
	result := model.HostListResults{
		Hosts:   make([]model.CreateHost, len(hosts)),
		Details: make([]model.APIHostCreateDetail, len(t.HostCreateDetails)),
	}
	for i := range hosts {
		createHost := model.CreateHost{}
		if err := createHost.BuildFromService(&hosts[i]); err != nil {
			catcher.Wrapf(err, "error building api host from service")
		}
		result.Hosts[i] = createHost
	}

	for i := range t.HostCreateDetails {
		details := model.APIHostCreateDetail{}
		if err := details.BuildFromService(t.HostCreateDetails[i]); err != nil {
			catcher.Wrapf(err, "error building api host create details from service")
		}
		result.Details[i] = details
	}

	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(catcher.Resolve())
	}
	return gimlet.NewJSONResponse(result)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/hosts/{host_id}/logs

type containerLogsHandler struct {
	startTime string
	endTime   string
	tail      string

	host *host.Host

	isError bool
}

func makeContainerLogsRouteManager(isError bool) *containerLogsHandler {
	return &containerLogsHandler{isError: isError}
}

func (h *containerLogsHandler) Factory() gimlet.RouteHandler {
	h = &containerLogsHandler{isError: h.isError}
	return h
}

func (h *containerLogsHandler) Parse(ctx context.Context, r *http.Request) error {
	id := gimlet.GetVars(r)["host_id"]
	host, err := host.FindOneId(id)
	if host == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("Container _id %s not found", id),
		}
	}
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    "Error loading host for container _id " + id,
		}
	}
	h.host = host

	if startTime := r.FormValue("start_time"); startTime != "" {
		if _, err := time.Parse(time.RFC3339, startTime); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message: fmt.Sprintf("problem parsing start time from '%s' (%s). Format must be RFC339",
					startTime, err.Error()),
			}
		}
		h.startTime = startTime
	}
	if endTime := r.FormValue("end_time"); endTime != "" {
		if _, err := time.Parse(time.RFC3339, endTime); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message: fmt.Sprintf("problem parsing end time from '%s' (%s). Format must be RFC339",
					endTime, err.Error()),
			}
		}
		h.endTime = endTime
	}
	if tailStr := r.FormValue("tail"); tailStr != "" {
		tail, err := strconv.Atoi(tailStr)
		if (err == nil && tail >= 0) || (err != nil && tailStr == "all") {
			h.tail = tailStr
		} else {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message: fmt.Sprintf("tail '%s' invalid, must be non-negative integer or 'all'",
					tailStr),
			}
		}
	}

	return nil
}

func (h *containerLogsHandler) Run(ctx context.Context) gimlet.Responder {
	parent, err := h.host.GetParent()
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrapf(err, "error finding parent for container _id %s", h.host.Id))
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "error getting settings config"))
	}
	options := types.ContainerLogsOptions{
		Timestamps: true,
		Tail:       h.tail,
		Since:      h.startTime,
		Until:      h.endTime,
	}
	if h.isError {
		options.ShowStderr = true
	} else {
		options.ShowStdout = true
	}
	logs, err := data.GetDockerLogs(ctx, h.host.Id, parent, settings, options)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "error getting docker logs"))
	}
	return gimlet.NewTextResponse(logs)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/hosts/{host_id}/status

type containerStatusHandler struct {
	host *host.Host
}

func makeContainerStatusManager() *containerStatusHandler {
	return &containerStatusHandler{}
}

func (h *containerStatusHandler) Factory() gimlet.RouteHandler {
	return &containerStatusHandler{}
}

func (h *containerStatusHandler) Parse(ctx context.Context, r *http.Request) error {
	id := gimlet.GetVars(r)["host_id"]
	host, err := host.FindOneId(id)
	if host == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("Container _id %s not found", id),
		}
	}
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message: fmt.Sprintf("Error loading host for container _id %s (%s)",
				id, err.Error()),
		}
	}
	h.host = host
	return nil
}

func (h *containerStatusHandler) Run(ctx context.Context) gimlet.Responder {
	parent, err := h.host.GetParent()
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrapf(err, "error finding parent for container _id %s", h.host.Id))
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "error getting settings config"))
	}
	status, err := data.GetDockerStatus(ctx, h.host.Id, parent, settings)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "error getting docker status"))
	}
	return gimlet.NewJSONResponse(status)
}
