package route

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type hostStatus struct {
	Status string `json:"status"`
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/hosts

type hostsChangeStatusesHandler struct {
	HostToStatus map[string]hostStatus
	sc           data.Connector
}

func makeChangeHostsStatuses(sc data.Connector) gimlet.RouteHandler {
	return &hostsChangeStatusesHandler{
		sc: sc,
	}
}

func (h *hostsChangeStatusesHandler) Factory() gimlet.RouteHandler {
	return &hostsChangeStatusesHandler{
		sc: h.sc,
	}
}

func (h *hostsChangeStatusesHandler) Parse(ctx context.Context, r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()

	if err := util.ReadJSONInto(body, &h.HostToStatus); err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	if len(h.HostToStatus) == 0 {
		return fmt.Errorf("Missing host id and status")
	}

	for hostID, status := range h.HostToStatus {
		if !util.StringSliceContains(evergreen.ValidUserSetStatus, status.Status) {
			return fmt.Errorf("Invalid host status '%s' for host '%s'", status.Status, hostID)
		}
	}

	return nil
}

func (h *hostsChangeStatusesHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	for id := range h.HostToStatus {
		foundHost, err := h.sc.FindHostByIdWithOwner(id, user)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", id))
		}
		if foundHost.Status == evergreen.HostTerminated {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Host '%s' is terminated; its status cannot be changed", foundHost.Id),
			})
		}
	}

	resp := gimlet.NewResponseBuilder()
	if err := resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	for id, status := range h.HostToStatus {
		foundHost, err := h.sc.FindHostByIdWithOwner(id, user)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", id))
		}
		if status.Status == evergreen.HostTerminated {
			if err = h.sc.TerminateHost(ctx, foundHost, user.Id); err != nil {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				})
			}
		} else {
			if err = h.sc.SetHostStatus(foundHost, status.Status, user.Id); err != nil {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				})
			}
		}

		host := &model.APIHost{}
		if err = host.BuildFromService(foundHost); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API Error converting from host.Host to model.APIHost"))
		}
		if err = resp.AddData(host); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/hosts/{host_id}

func makeGetHostByID(sc data.Connector) gimlet.RouteHandler {
	return &hostIDGetHandler{
		sc: sc,
	}
}

type hostIDGetHandler struct {
	hostID string
	sc     data.Connector
}

func (high *hostIDGetHandler) Factory() gimlet.RouteHandler {
	return &hostIDGetHandler{
		sc: high.sc,
	}
}

// ParseAndValidate fetches the hostId from the http request.
func (high *hostIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	high.hostID = gimlet.GetVars(r)["host_id"]

	return nil
}

// Execute calls the data FindHostById function and returns the host
// from the provider.
func (high *hostIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	foundHost, err := high.sc.FindHostById(high.hostID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", high.hostID))
	}

	hostModel := &model.APIHost{}
	if err = hostModel.BuildFromService(*foundHost); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API Error converting from host.Host to model.APIHost"))
	}

	if foundHost.RunningTask != "" {
		runningTask, err := high.sc.FindTaskById(foundHost.RunningTask)
		if err != nil {
			if apiErr, ok := err.(gimlet.ErrorResponse); !ok || (ok && apiErr.StatusCode != http.StatusNotFound) {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Database error"))
			}
		}

		if err = hostModel.BuildFromService(runningTask); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "problem adding task data to host response"))
		}
	}

	return gimlet.NewJSONResponse(hostModel)
}

////////////////////////////////////////////////////////////////////////
//
// GET /hosts
// GET /users/{user_id}/hosts

func makeFetchHosts(sc data.Connector) gimlet.RouteHandler {
	return &hostGetHandler{
		sc: sc,
	}
}

type hostGetHandler struct {
	limit  int
	key    string
	status string
	user   string

	sc data.Connector
}

func (hgh *hostGetHandler) Factory() gimlet.RouteHandler {
	return &hostGetHandler{
		sc: hgh.sc,
	}
}

func (hgh *hostGetHandler) Parse(ctx context.Context, r *http.Request) error {
	vals := r.URL.Query()
	hgh.status = vals.Get("status")
	hgh.key = vals.Get("host_id")
	var err error

	hgh.limit, err = getLimit(vals)
	if err != nil {
		return errors.WithStack(err)
	}

	// only populated in the case of the /users/{user}/hosts route
	hgh.user = gimlet.GetVars(r)["user_id"]

	return nil
}

func (hgh *hostGetHandler) Run(ctx context.Context) gimlet.Responder {
	hosts, err := hgh.sc.FindHostsById(hgh.key, hgh.status, hgh.user, hgh.limit+1)
	if err != nil {
		gimlet.NewJSONErrorResponse(errors.Wrap(err, "Database error"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	lastIndex := len(hosts)
	if len(hosts) > hgh.limit {
		lastIndex = hgh.limit
		err = resp.SetPages(&gimlet.ResponsePages{
			Next: &gimlet.Page{
				BaseURL:         hgh.sc.GetURL(),
				LimitQueryParam: "limit",
				KeyQueryParam:   "host_id",
				Relation:        "next",
				Key:             hosts[hgh.limit].Id,
				Limit:           hgh.limit,
			},
		})
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err,
				"problem paginating response"))
		}
	}

	// Truncate the hosts to just those that will be returned.
	hosts = hosts[:lastIndex]

	// Grab the taskIds associated as running on the hosts.
	taskIds := []string{}
	for _, h := range hosts {
		if h.RunningTask != "" {
			taskIds = append(taskIds, h.RunningTask)
		}
	}

	tasks, err := hgh.sc.FindTasksByIds(taskIds)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
	}

	tasksById := make(map[string]task.Task, len(tasks))
	for _, t := range tasks {
		tasksById[t.Id] = t
	}

	for _, h := range hosts {
		apiHost := &model.APIHost{}
		if err = apiHost.BuildFromService(h); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}

		if h.RunningTask != "" {
			runningTask, ok := tasksById[h.RunningTask]
			if !ok {
				continue
			}
			// Add the task information to the host document.

			if err = apiHost.BuildFromService(runningTask); err != nil {
				return gimlet.MakeJSONErrorResponder(err)
			}
		}
		if err = resp.AddData(apiHost); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}

func getLimit(vals url.Values) (int, error) {
	var (
		limit int
		err   error
	)

	if l, ok := vals["limit"]; ok && len(l) > 0 {
		limit, err = strconv.Atoi(l[0])
		if err != nil {
			return 0, gimlet.ErrorResponse{
				Message:    fmt.Sprintf("invalid Limit Specified: %s", err.Error()),
				StatusCode: http.StatusBadRequest,
			}
		}
	}

	if limit == 0 {
		return defaultLimit, nil
	}

	return limit, nil
}

////////////////////////////////////////////////////////////////////////
//
// GET /host/filter

func makeFetchHostFilter(sc data.Connector) gimlet.RouteHandler {
	return &hostFilterGetHandler{
		sc: sc,
	}
}

type hostFilterGetHandler struct {
	params model.APIHostParams
	sc     data.Connector
}

func (h *hostFilterGetHandler) Factory() gimlet.RouteHandler {
	return &hostFilterGetHandler{
		sc: h.sc,
	}
}

func (h *hostFilterGetHandler) Parse(ctx context.Context, r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()
	if err := util.ReadJSONInto(body, &h.params); err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	return nil
}

func (h *hostFilterGetHandler) Run(ctx context.Context) gimlet.Responder {
	dbUser := MustHaveUser(ctx)
	username := ""
	// only admins see hosts that aren't theirs
	if !util.StringSliceContains(h.sc.GetSuperUsers(), dbUser.Username()) || h.params.Mine {
		username = dbUser.Username()
	}

	hosts, err := h.sc.FindHostsInRange(h.params, username)
	if err != nil {
		gimlet.NewJSONErrorResponse(errors.Wrap(err, "Database error"))
	}

	resp := gimlet.NewResponseBuilder()
	if err = resp.SetFormat(gimlet.JSON); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	for _, host := range hosts {
		apiHost := &model.APIHost{}
		if err = apiHost.BuildFromService(host); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
		if err = resp.AddData(apiHost); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	return resp
}
