package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
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
	distro     distro.Distro
	env        evergreen.Environment
}

func makeHostCreateRouteManager(env evergreen.Environment) gimlet.RouteHandler {
	return &hostCreateHandler{env: env}
}

func (h *hostCreateHandler) Factory() gimlet.RouteHandler {
	return &hostCreateHandler{
		env: h.env,
	}
}

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
	d, err := distro.GetHostCreateDistro(ctx, h.createHost)
	if err != nil {
		return err
	}
	h.distro = *d
	return h.createHost.Validate(ctx)
}

func (h *hostCreateHandler) Run(ctx context.Context) gimlet.Responder {
	numHosts, err := strconv.Atoi(h.createHost.NumHosts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "converting number of hosts to create to integer value"))
	}

	ids := []string{}
	for i := 0; i < numHosts; i++ {
		intentHost, err := data.MakeHost(ctx, h.env, h.taskID, "", "", h.createHost, h.distro)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "creating intent host"))
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
		return errors.New("must provide task ID")
	}
	h.taskID = taskID

	return nil
}

func (h *hostListHandler) Run(ctx context.Context) gimlet.Responder {
	hosts, err := data.ListHostsForTask(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "listing hosts for task '%s'", h.taskID))
	}
	t, err := task.FindOneId(h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	catcher := grip.NewBasicCatcher()
	result := model.HostListResults{
		Hosts:   make([]model.APICreateHost, len(hosts)),
		Details: make([]model.APIHostCreateDetail, len(t.HostCreateDetails)),
	}
	for i := range hosts {
		createHost := model.APICreateHost{}
		createHost.BuildFromService(hosts[i])
		result.Hosts[i] = createHost
	}

	for i := range t.HostCreateDetails {
		details := model.APIHostCreateDetail{}
		if err := details.BuildFromService(t.HostCreateDetails[i]); err != nil {
			catcher.Wrapf(err, "converting host creation details to API model for host at index %d", i)
		}
		result.Details[i] = details
	}

	if catcher.HasErrors() {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(catcher.Resolve(), "building host and details for response"))
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
	host, err := host.FindOneId(ctx, id)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding container '%s'", id).Error(),
		}
	}
	if host == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("container '%s' not found", id),
		}
	}
	h.host = host

	if startTime := r.FormValue("start_time"); startTime != "" {
		if _, err := time.Parse(time.RFC3339, startTime); err != nil {
			return errors.Wrapf(err, "parsing start time '%s' in RPC3339 format", startTime)
		}
		h.startTime = startTime
	}
	if endTime := r.FormValue("end_time"); endTime != "" {
		if _, err := time.Parse(time.RFC3339, endTime); err != nil {
			return errors.Wrapf(err, "parsing start time '%s' in RPC3339 format", endTime)
		}
		h.endTime = endTime
	}
	if tailStr := r.FormValue("tail"); tailStr != "" {
		tail, err := strconv.Atoi(tailStr)
		if (err == nil && tail >= 0) || (err != nil && tailStr == "all") {
			h.tail = tailStr
		} else {
			return errors.Errorf("tail '%s' must be non-negative integer or 'all'", tailStr)
		}
	}

	return nil
}

func (h *containerLogsHandler) Run(ctx context.Context) gimlet.Responder {
	parent, err := h.host.GetParent(ctx)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding parent for container '%s'", h.host.Id))
	}
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting admin settings"))
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
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "getting Docker logs"))
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
	host, err := host.FindOneId(ctx, id)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "finding container '%s'", id).Error(),
		}
	}
	if host == nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("container '%s' not found", id),
		}
	}
	h.host = host
	return nil
}

func (h *containerStatusHandler) Run(ctx context.Context) gimlet.Responder {
	parent, err := h.host.GetParent(ctx)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "finding parent for container '%s'", h.host.Id))
	}
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting admin settings"))
	}
	status, err := data.GetDockerStatus(ctx, h.host.Id, parent, settings)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting Docker status"))
	}
	return gimlet.NewJSONResponse(status)
}
