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
	"github.com/evergreen-ci/evergreen/model/distro"
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

	// need to first generate the number of container hosts if it's relevant
	if h.createHost.CloudProvider == evergreen.ProviderNameDocker {
		distro, err := distro.FindOne(distro.ById(h.createHost.Distro))
		if err != nil {
			return gimlet.NewJSONErrorResponse(errors.Wrapf(err, "error getting distro '%s'", h.createHost.Distro))
		}
		settings, err := evergreen.GetConfig()
		if err != nil {
			return gimlet.NewJSONErrorResponse(errors.Wrap(err, "error getting settings config"))
		}

		containerPool := settings.ContainerPools.GetContainerPool(distro.ContainerPool)
		if containerPool == nil {
			return gimlet.NewJSONErrorResponse(errors.Errorf("container pool '%s' not found", distro.ContainerPool))
		}

		_, numHosts, err = host.InsertParentIntentsAndGetNumHostsToSpawn(containerPool, 1, true)
		if err != nil {
			return gimlet.NewJSONErrorResponse(errors.Wrap(err, "error creating parent intents and number of hosts to spawn"))
		}
	}

	ids := []string{}
	for i := 0; i < numHosts; i++ {
		intentHost, err := h.sc.MakeIntentHost(h.taskID, "", "", h.createHost)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		hosts = append(hosts, *intentHost)
		ids = append(ids, intentHost.Id)
	}

	if err := host.InsertMany(hosts); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(ids)
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
// GET /rest/v2/hosts/{host_id}/logs

type containerLogsHandler struct {
	startTime string
	endTime   string
	tail      string

	host *host.Host

	isError bool
	sc      data.Connector
}

func makeContainerLogsRouteManager(sc data.Connector, isError bool) *containerLogsHandler {
	return &containerLogsHandler{sc: sc, isError: isError}
}

func (h *containerLogsHandler) Factory() gimlet.RouteHandler {
	h = &containerLogsHandler{sc: h.sc, isError: h.isError}
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
	logs, err := h.sc.GetDockerLogs(ctx, h.host.ExternalIdentifier, parent, settings, options)
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

	sc data.Connector
}

func makeContainerStatusManager(sc data.Connector) *containerStatusHandler {
	return &containerStatusHandler{sc: sc}
}

func (h *containerStatusHandler) Factory() gimlet.RouteHandler {
	return &containerStatusHandler{sc: h.sc}
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
	status, err := h.sc.GetDockerStatus(ctx, h.host.ExternalIdentifier, parent, settings)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "error getting docker status"))
	}
	return gimlet.NewJSONResponse(status)
}
