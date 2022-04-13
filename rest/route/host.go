package route

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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
}

func makeChangeHostsStatuses() gimlet.RouteHandler {
	return &hostsChangeStatusesHandler{}
}

func (h *hostsChangeStatusesHandler) Factory() gimlet.RouteHandler {
	return &hostsChangeStatusesHandler{}
}

func (h *hostsChangeStatusesHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(body, &h.HostToStatus); err != nil {
		return errors.Wrap(err, "Argument read error")
	}
	if len(h.HostToStatus) == 0 {
		return fmt.Errorf("Missing host id and status")
	}

	for hostID, status := range h.HostToStatus {
		if !utility.StringSliceContains(evergreen.ValidUserSetStatus, status.Status) {
			return fmt.Errorf("Invalid host status '%s' for host '%s'", status.Status, hostID)
		}
	}

	return nil
}

func (h *hostsChangeStatusesHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	for id := range h.HostToStatus {
		foundHost, err := data.FindHostByIdWithOwner(id, user)
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
		foundHost, err := data.FindHostByIdWithOwner(id, user)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", id))
		}
		if status.Status == evergreen.HostTerminated {
			if err = errors.WithStack(cloud.TerminateSpawnHost(ctx, evergreen.GetEnvironment(), foundHost, user.Id, "terminated via REST API")); err != nil {
				return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				})
			}
		} else {
			if err = foundHost.SetStatus(status.Status, user.Id, fmt.Sprintf("changed by %s from API", user.Id)); err != nil {
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

func makeGetHostByID() gimlet.RouteHandler {
	return &hostIDGetHandler{}
}

type hostIDGetHandler struct {
	hostID string
}

func (high *hostIDGetHandler) Factory() gimlet.RouteHandler {
	return &hostIDGetHandler{}
}

// ParseAndValidate fetches the hostId from the http request.
func (high *hostIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	high.hostID = gimlet.GetVars(r)["host_id"]

	return nil
}

// Execute calls the data FindHostById function and returns the host
// from the provider.
func (high *hostIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	foundHost, err := host.FindOneId(high.hostID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", high.hostID))
	}
	if foundHost == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with id '%s' not found", high.hostID),
		})
	}

	hostModel := &model.APIHost{}
	if err = hostModel.BuildFromService(*foundHost); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API Error converting from host.Host to model.APIHost"))
	}

	if foundHost.RunningTask != "" {
		runningTask, err := task.FindOneId(foundHost.RunningTask)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Database error"))
		}
		if runningTask == nil {
			return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("task with id %s not found", foundHost.RunningTask),
			})
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

func makeFetchHosts(url string) gimlet.RouteHandler {
	return &hostGetHandler{url: url}
}

type hostGetHandler struct {
	limit  int
	key    string
	status string
	user   string
	url    string
}

func (hgh *hostGetHandler) Factory() gimlet.RouteHandler {
	return &hostGetHandler{url: hgh.url}
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
	hosts, err := host.GetHostsByFromIDWithStatus(hgh.key, hgh.status, hgh.user, hgh.limit+1)
	if err != nil {
		gimlet.NewJSONErrorResponse(errors.Wrap(err, "Database error"))
	}
	if len(hosts) == 0 {
		gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "no hosts found",
		})
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
				BaseURL:         hgh.url,
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

	var tasks []task.Task
	if len(taskIds) > 0 {
		tasks, err = task.Find(task.ByIds(taskIds))
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "database error"))
		}
		if len(tasks) == 0 {
			return gimlet.MakeJSONInternalErrorResponder(errors.New("no tasks found"))
		}
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

	if limit <= 0 {
		return defaultLimit, nil
	}

	return limit, nil
}

////////////////////////////////////////////////////////////////////////
//
// POST /users/offboard_user

func makeOffboardUser(env evergreen.Environment) gimlet.RouteHandler {
	return &offboardUserHandler{
		env: env,
	}
}

type offboardUserHandler struct {
	user   string
	dryRun bool

	env evergreen.Environment
}

func (ch offboardUserHandler) Factory() gimlet.RouteHandler {
	return &offboardUserHandler{
		env: ch.env,
	}
}

func (ch *offboardUserHandler) Parse(ctx context.Context, r *http.Request) error {
	input := struct {
		Email string `json:"email" bson:"email"`
	}{}
	err := utility.ReadJSON(r.Body, &input)
	if err != nil {
		return errors.Wrap(err, "error reading input")
	}
	// get the username from the email
	splitString := strings.Split(input.Email, "@")
	if len(splitString) == 0 {
		return gimlet.ErrorResponse{
			Message:    "email is malformed",
			StatusCode: http.StatusBadRequest,
		}
	}
	ch.user = splitString[0]
	if ch.user == "" {
		return gimlet.ErrorResponse{
			Message:    "user cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}
	u, err := user.FindOneById(ch.user)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    "problem finding user",
			StatusCode: http.StatusInternalServerError,
		}
	}
	if u == nil {
		return gimlet.ErrorResponse{
			Message:    "user not found",
			StatusCode: http.StatusNotFound,
		}
	}

	vals := r.URL.Query()
	ch.dryRun = vals.Get("dry_run") == "true"

	return nil
}

func (ch *offboardUserHandler) Run(ctx context.Context) gimlet.Responder {
	opts := model.APIHostParams{
		UserSpawned: true,
	}
	// returns all up-hosts for user
	hosts, err := data.FindHostsInRange(opts, ch.user)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "database error getting hosts"))
	}

	volumes, err := host.FindVolumesByUser(ch.user)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "database error getting volumes"))
	}

	toTerminate := model.APIOffboardUserResults{
		TerminatedHosts:   []string{},
		TerminatedVolumes: []string{},
	}

	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if h.NoExpiration {
			if !ch.dryRun {
				catcher.Wrapf(h.MarkShouldExpire(""), "error marking host '%s' expirable", h.Id)
			}
			toTerminate.TerminatedHosts = append(toTerminate.TerminatedHosts, h.Id)
		}
	}

	for _, v := range volumes {
		if v.NoExpiration {
			if !ch.dryRun {
				catcher.Wrapf(v.SetNoExpiration(false), "error marking volume '%s' expirable", v.ID)
			}
			toTerminate.TerminatedVolumes = append(toTerminate.TerminatedVolumes, v.ID)
		}
	}

	if !ch.dryRun {
		grip.Info(message.Fields{
			"message":            "executing user offboarding",
			"user":               ch.user,
			"terminated_hosts":   toTerminate.TerminatedHosts,
			"terminated_volumes": toTerminate.TerminatedVolumes,
		})

		grip.Error(message.WrapError(serviceModel.RemoveAdminFromProjects(ch.user), message.Fields{
			"message": "could not remove user as an admin",
			"context": "user offboarding",
			"user":    ch.user,
		}))

		grip.Error(message.WrapError(ch.clearLogin(), message.Fields{
			"message": "could not clear login token",
			"context": "user offboarding",
			"user":    ch.user,
		}))
	}

	if catcher.HasErrors() {
		err := catcher.Resolve()
		grip.CriticalWhen(!ch.dryRun, message.WrapError(err, message.Fields{
			"message": "not all unexpirable hosts/volumes terminated",
			"context": "user offboarding",
			"user":    ch.user,
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "not all unexpirable hosts/volumes terminated"))
	}

	return gimlet.NewJSONResponse(toTerminate)
}

// clearLogin invalidates the user's login session.
func (ch *offboardUserHandler) clearLogin() error {
	usrMngr := ch.env.UserManager()
	if usrMngr == nil {
		return errors.New("no user manager found in environment")
	}
	usr, err := usrMngr.GetUserByID(ch.user)
	if err != nil {
		return errors.Wrap(err, "finding user")
	}
	return errors.Wrap(usrMngr.ClearUser(usr, false), "clearing login cache")
}

////////////////////////////////////////////////////////////////////////
//
// GET /host/filter

func makeFetchHostFilter() gimlet.RouteHandler {
	return &hostFilterGetHandler{}
}

type hostFilterGetHandler struct {
	params model.APIHostParams
}

func (h *hostFilterGetHandler) Factory() gimlet.RouteHandler {
	return &hostFilterGetHandler{}
}

func (h *hostFilterGetHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()
	if err := utility.ReadJSON(body, &h.params); err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	return nil
}

func (h *hostFilterGetHandler) Run(ctx context.Context) gimlet.Responder {
	dbUser := MustHaveUser(ctx)
	username := ""
	// only admins see hosts that aren't theirs
	if h.params.Mine || !dbUser.HasPermission(gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionDistroCreate,
		RequiredLevel: evergreen.DistroCreate.Value,
	}) {
		username = dbUser.Username()
	}

	hosts, err := data.FindHostsInRange(h.params, username)
	if err != nil {
		return gimlet.NewJSONErrorResponse(errors.Wrap(err, "Database error"))
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

// GET /hosts/{host_id}/provisioning_options

type hostProvisioningOptionsGetHandler struct {
	hostID string
	env    evergreen.Environment
}

func makeHostProvisioningOptionsGetHandler(env evergreen.Environment) gimlet.RouteHandler {
	return &hostProvisioningOptionsGetHandler{
		env: env,
	}
}

func (rh *hostProvisioningOptionsGetHandler) Factory() gimlet.RouteHandler {
	return &hostProvisioningOptionsGetHandler{}
}

func (rh *hostProvisioningOptionsGetHandler) Parse(ctx context.Context, r *http.Request) error {
	hostID := gimlet.GetVars(r)["host_id"]
	if hostID == "" {
		return errors.New("missing host ID")
	}
	rh.hostID = hostID
	return nil
}

func (rh *hostProvisioningOptionsGetHandler) Run(ctx context.Context) gimlet.Responder {
	script, err := data.GenerateHostProvisioningScript(ctx, rh.env, rh.hostID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	apiOpts := model.APIHostProvisioningOptions{
		Content: script,
	}
	return gimlet.NewJSONResponse(apiOpts)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/host/{host_id}/disable
type disableHost struct {
	env evergreen.Environment

	hostID string
	reason string
}

func makeDisableHostHandler(env evergreen.Environment) gimlet.RouteHandler {
	return &disableHost{
		env: env,
	}
}

func (h *disableHost) Factory() gimlet.RouteHandler {
	return &disableHost{
		env: h.env,
	}
}

func (h *disableHost) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()
	h.hostID = gimlet.GetVars(r)["host_id"]
	if h.hostID == "" {
		return errors.New("host ID must be specified")
	}

	info := apimodels.DisableInfo{}
	if err := utility.ReadJSON(body, &info); err != nil {
		return errors.Wrap(err, "unable to parse request body")
	}
	h.reason = info.Reason

	return nil
}

func (h *disableHost) Run(ctx context.Context) gimlet.Responder {
	host, err := host.FindOneId(h.hostID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting host"))
	}
	if host == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host '%s' not found", h.hostID)},
		)
	}

	if err = units.HandlePoisonedHost(ctx, h.env, host, h.reason); err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/hosts/ip_address/{ip_address}

type hostIpAddressGetHandler struct {
	IP   string
	Host *host.Host
}

func makeGetHostByIpAddress() gimlet.RouteHandler {
	return &hostIpAddressGetHandler{}
}

func (h *hostIpAddressGetHandler) Factory() gimlet.RouteHandler {
	return &hostIpAddressGetHandler{}
}
func (h *hostIpAddressGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.IP = gimlet.GetVars(r)["ip_address"]

	if h.IP == "" {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "ip_address cannot be empty",
		}
	}

	return nil
}

func (h *hostIpAddressGetHandler) Run(ctx context.Context) gimlet.Responder {
	host, err := host.FindOne(host.ByIPAndRunning(h.IP))
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error fetching host information for '%s'", h.IP))
	}
	if host == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with ip address '%s' may not exist or may not have IP address stored", h.IP),
		})
	}

	hostModel := &model.APIHost{}
	if err = hostModel.BuildFromService(*host); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API Error converting from host.Host to model.APIHost"))
	}

	return gimlet.NewJSONResponse(hostModel)
}
