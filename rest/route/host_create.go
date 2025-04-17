package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

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

var createdOrCreatingHostStatuses = append([]string{evergreen.HostUninitialized}, evergreen.IsRunningOrWillRunStatuses...)

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
	t, err := task.FindOneId(ctx, h.taskID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding task '%s'", h.taskID))
	}
	if t == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("task '%s' not found", h.taskID),
		})
	}
	initialHosts, err := host.FindHostsSpawnedByTask(ctx, h.taskID, t.Execution, createdOrCreatingHostStatuses)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding hosts spawned by task '%s'", h.taskID))
	}
	numInitialHosts := len(initialHosts)

	for _, host := range initialHosts {
		ids = append(ids, host.Id)
	}

	// No-op if the number of hosts created/about to create is the same as the number of hosts already requested.
	if numInitialHosts == numHosts {
		return gimlet.NewJSONResponse(ids)
	} else if numInitialHosts > numHosts {
		// Should never create more hosts than the amount requested.
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Evergreen created %d hosts when %d were requested", numInitialHosts, numHosts),
		})
	}

	for i := 0; i < (numHosts - numInitialHosts); i++ {
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
	t, err := task.FindOneId(ctx, h.taskID)
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
