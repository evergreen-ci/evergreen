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
	dbModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

type hostCreateHandler struct {
	taskID     string
	createHost apimodels.CreateHost

	sc data.Connector
}

func makeHostCreateRouteManager(sc data.Connector) gimlet.RouteHandler {
	return &hostCreateHandler{sc: sc}
}

func (h *hostCreateHandler) Factory() gimlet.RouteHandler { return &hostCreateHandler{sc: h.sc} }

func (h *hostCreateHandler) Parse(ctx context.Context, r *http.Request) error {
	taskID := gimlet.GetVars(r)["task_id"]
	if taskID == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide task ID",
		}
	}
	h.taskID = taskID
	if _, code, err := dbModel.ValidateTask(h.taskID, true, r); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: code,
			Message:    "task is invalid",
		}
	}
	if _, code, err := dbModel.ValidateHost("", r); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: code,
			Message:    "host is invalid",
		}
	}
	if err := util.ReadJSONInto(r.Body, &h.createHost); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	return h.createHost.Validate()
}

func (h *hostCreateHandler) Run(ctx context.Context) gimlet.Responder {
	hosts := []host.Host{}
	numHosts, err := strconv.Atoi(h.createHost.NumHosts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	for i := 0; i < numHosts; i++ {
		intentHost, err := h.sc.MakeIntentHost(h.taskID, "", "", h.createHost)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		hosts = append(hosts, *intentHost)
	}

	if err := host.InsertMany(hosts); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}

type hostListHandler struct {
	taskID string

	sc data.Connector
}

func makeHostListRouteManager(sc data.Connector) gimlet.RouteHandler {
	return &hostListHandler{sc: sc}
}

func (h *hostListHandler) Factory() gimlet.RouteHandler { return &hostListHandler{sc: h.sc} }

func (h *hostListHandler) Parse(ctx context.Context, r *http.Request) error {
	taskID := gimlet.GetVars(r)["task_id"]
	if taskID == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide task ID",
		}
	}
	h.taskID = taskID
	if _, code, err := dbModel.ValidateTask(h.taskID, true, r); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: code,
			Message:    "task is invalid",
		}
	}

	return nil
}

func (h *hostListHandler) Run(ctx context.Context) gimlet.Responder {
	hosts, err := h.sc.ListHostsForTask(h.taskID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	catcher := grip.NewBasicCatcher()
	results := make([]model.Model, len(hosts))
	for i := range hosts {
		createHost := model.CreateHost{}
		if err := createHost.BuildFromService(&hosts[i]); err != nil {
			catcher.Add(errors.Wrap(err, "error building api host from service"))
		}
		results[i] = &createHost
	}
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(catcher.Resolve())
	}
	return gimlet.NewJSONResponse(results)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/hosts/{container_id}/logs

type containerLogsHandler struct {
	containerID string
	startTime   string
	endTime     string

	sc data.Connector
}

func makeContainerLogsRouteManager(sc data.Connector) gimlet.RouteHandler {
	return &containerLogsHandler{sc: sc}
}

func (h *containerLogsHandler) Factory() gimlet.RouteHandler { return &containerLogsHandler{sc: h.sc} }

func (h *containerLogsHandler) Parse(ctx context.Context, r *http.Request) error {
	id := gimlet.GetVars(r)["container_id"]
	if id == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must provide 'container_id'",
		}
	}
	h.containerID = id
	if _, code, err := dbModel.ValidateContainer(h.containerID, r); err != nil {
		return gimlet.ErrorResponse{
			StatusCode: code,
			Message:    "container is invalid",
		}
	}

	if h.startTime = r.FormValue("start_time"); h.startTime != "" {
		if _, err := time.Parse(time.RFC3339, h.startTime); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message: fmt.Sprintf("problem parsing start time from '%s' (%s). Must be given in the following format: %s",
					h.startTime, err.Error(), time.RFC3339),
			}
		}
	}
	if h.endTime = r.FormValue("end_time"); h.endTime != "" {
		if _, err := time.Parse(time.RFC3339, h.startTime); err != nil {
			return gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message: fmt.Sprintf("problem parsing start time from '%s' (%s). Must be given in the following format: %s",
					h.startTime, err.Error(), time.RFC3339),
			}
		}
	}

	return nil
}

func (h *containerLogsHandler) Run(ctx context.Context) gimlet.Responder {
	host, err := host.FindOneId(h.containerID)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrapf(err, "error finding container %s", h.containerID))
	}
	if host == nil {
		return gimlet.NewJSONErrorResponse(errors.New(fmt.Sprintf("container %s not found", h.containerID)))
	}

	parent, err := host.GetParent()
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrapf(err, "error finding parent for container %s", h.containerID))
	}
	settings, err := evergreen.GetConfig()
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "error getting settings config"))
	}
	options := types.ContainerLogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Since:      h.startTime,
		Until:      h.endTime,
	}
	reader, err := h.sc.GetDockerLogs(ctx, h.containerID, parent, settings, options)
	if err != nil {
		return gimlet.NewJSONErrorResponse(err)
	}
	return gimlet.NewJSONResponse(reader)
}
