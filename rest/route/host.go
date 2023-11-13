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

// Factory creates an instance of the handler.
//
//	@Summary		Fetch all hosts
//	@Description	Returns a paginated list of all hosts in Evergreen
//	@Tags			hosts
//	@Router			/hosts [get]
//	@Security		Api-User || Api-Key
//	@Param			start_at	query		string	false	"The identifier of the host to start at in the pagination"
//	@Param			limit		query		int		false	"The number of hosts to be returned per page of pagination. Defaults to 100"
//	@Param			status		query		string	false	"A status of host to limit the results to"
//	@Success		200			{object}	model.APIHost
func (h *hostsChangeStatusesHandler) Factory() gimlet.RouteHandler {
	return &hostsChangeStatusesHandler{}
}

func (h *hostsChangeStatusesHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	defer body.Close()

	if err := utility.ReadJSON(body, &h.HostToStatus); err != nil {
		return errors.Wrap(err, "reading host-status mapping from JSON request body")
	}
	if len(h.HostToStatus) == 0 {
		return errors.New("must give at least one host ID to status mapping")
	}

	for hostID, status := range h.HostToStatus {
		if !utility.StringSliceContains(evergreen.ValidUserSetHostStatus, status.Status) {
			return errors.Errorf("invalid host status '%s' for host '%s'", status.Status, hostID)
		}
	}

	return nil
}

func (h *hostsChangeStatusesHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	resp := gimlet.NewResponseBuilder()
	for id, status := range h.HostToStatus {
		foundHost, err := data.FindHostByIdWithOwner(ctx, id, user)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' with owner '%s'", id, user.Id))
		}
		switch status.Status {
		case evergreen.HostTerminated:
			err = errors.WithStack(cloud.TerminateSpawnHost(ctx, evergreen.GetEnvironment(), foundHost, user.Id, "terminated via REST API"))
		default:
			err = foundHost.SetStatus(ctx, status.Status, user.Id, fmt.Sprintf("changed by %s from API", user.Id))
		}
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}

		h := &model.APIHost{}
		h.BuildFromService(foundHost, nil)
		if err = resp.AddData(h); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding response data for host '%s'", utility.FromStringPtr(h.Id)))
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

// Factory creates an instance of the handler.
//
//	@Summary		Fetch hosts by ID
//	@Description	Fetches a single host using its ID
//	@Tags			hosts
//	@Router			/hosts/{host_id} [get]
//	@Security		Api-User || Api-Key
//	@Param			host_id	path		string	true	"the host ID"
//	@Success		200		{object}	model.APIHost
func (h *hostIDGetHandler) Factory() gimlet.RouteHandler {
	return &hostIDGetHandler{}
}

// ParseAndValidate fetches the hostId from the http request.
func (h *hostIDGetHandler) Parse(ctx context.Context, r *http.Request) error {
	h.hostID = gimlet.GetVars(r)["host_id"]

	return nil
}

// Execute calls the data FindHostById function and returns the host
// from the provider.
func (h *hostIDGetHandler) Run(ctx context.Context) gimlet.Responder {
	foundHost, err := host.FindOneId(ctx, h.hostID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s'", h.hostID))
	}
	if foundHost == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host '%s' not found", h.hostID),
		})
	}

	var runningTask *task.Task
	if foundHost.RunningTask != "" {
		runningTask, err = task.FindOneIdAndExecution(foundHost.RunningTask, foundHost.RunningTaskExecution)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host's running task '%s' execution '%d'", foundHost.RunningTask, foundHost.RunningTaskExecution))
		}
		if runningTask == nil {
			return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusNotFound,
				Message:    fmt.Sprintf("host's running task '%s' execution '%d' not found", foundHost.RunningTask, foundHost.RunningTaskExecution),
			})
		}
	}
	hostModel := &model.APIHost{}
	hostModel.BuildFromService(foundHost, runningTask)
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

// Factory creates an instance of the handler.
//
//	@Summary		Fetch hosts spawned by user
//	@Description	Returns a list of hosts spawned by the given user.
//	@Tags			hosts
//	@Router			/users/{user_id}/hosts [get]
//	@Security		Api-User || Api-Key
//	@Param			user_id		path		string	true	"the user ID"
//	@Param			start_at	query		string	false	"The identifier of the host to start at in the pagination"
//	@Param			limit		query		int		false	"The number of hosts to be returned per page of pagination. Defaults to 100"
//	@Param			status		query		string	false	"A status of host to limit the results to"
//	@Success		200			{object}	model.APIHost
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
	hosts, err := host.GetHostsByFromIDWithStatus(ctx, hgh.key, hgh.status, hgh.user, hgh.limit+1)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "finding hosts with filters"))
	}
	if len(hosts) == 0 {
		gimlet.NewJSONErrorResponse(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "no hosts found",
		})
	}

	resp := gimlet.NewResponseBuilder()
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
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "paginating response"))
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
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding tasks %s", taskIds))
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
		var runningTask *task.Task
		if h.RunningTask != "" {
			t, ok := tasksById[h.RunningTask]
			if ok {
				runningTask = &t
			}
		}
		apiHost.BuildFromService(&h, runningTask)
		if err = resp.AddData(apiHost); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "adding host data to response"))
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
				Message:    errors.Wrap(err, "invalid limit").Error(),
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

// Factory creates an instance of the handler.
//
//	@Summary		Offboard user
//	@Description	Marks unexpirable volumes and hosts as expirable for the user, and removes the user as a project admin for any projects, if applicable.
//	@Tags			users
//	@Router			/users/offboard_user [post]
//	@Security		Api-User || Api-Key
//	@Param			dry_run		query		boolean				false	"If set to true, route returns the IDs of the hosts/volumes that *would* be modified."
//	@Param			{object}	body		offboardUserEmail	true	"parameters"
//	@Success		200			{object}	model.APIOffboardUserResults
func (ch offboardUserHandler) Factory() gimlet.RouteHandler {
	return &offboardUserHandler{
		env: ch.env,
	}
}

type offboardUserEmail struct {
	// the email of the user
	Email string `json:"email" bson:"email" validate:"required"`
}

func (ch *offboardUserHandler) Parse(ctx context.Context, r *http.Request) error {
	input := offboardUserEmail{}
	err := utility.ReadJSON(r.Body, &input)
	if err != nil {
		return errors.Wrap(err, "reading user offboarding information from JSON request body")
	}
	if len(input.Email) == 0 {
		return errors.New("missing email")
	}
	splitString := strings.Split(input.Email, "@")
	if len(splitString) == 1 {
		return errors.New("email address is missing '@'")
	}
	ch.user = splitString[0]
	if ch.user == "" {
		return errors.New("no user could be parsed from the email address")
	}
	u, err := user.FindOneById(ch.user)
	if err != nil {
		return gimlet.ErrorResponse{
			Message:    errors.Wrapf(err, "finding user '%s'", ch.user).Error(),
			StatusCode: http.StatusInternalServerError,
		}
	}
	if u == nil {
		return gimlet.ErrorResponse{
			Message:    fmt.Sprintf("user '%s' not found", ch.user),
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
	hosts, err := data.FindHostsInRange(ctx, opts, ch.user)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "getting user hosts from options"))
	}

	volumes, err := host.FindVolumesByUser(ch.user)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "finding user volumes"))
	}

	toTerminate := model.APIOffboardUserResults{
		TerminatedHosts:   []string{},
		TerminatedVolumes: []string{},
	}

	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if h.NoExpiration {
			if !ch.dryRun {
				catcher.Wrapf(h.MarkShouldExpire(ctx, ""), "marking host '%s' expirable", h.Id)
			}
			toTerminate.TerminatedHosts = append(toTerminate.TerminatedHosts, h.Id)
		}
	}

	for _, v := range volumes {
		if v.NoExpiration {
			if !ch.dryRun {
				catcher.Wrapf(v.SetNoExpiration(false), "marking volume '%s' expirable", v.ID)
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
		err = user.ClearUser(ch.user)
		catcher.Wrapf(err, "clearing user '%s'", ch.user)
	}

	if catcher.HasErrors() {
		err := catcher.Resolve()
		grip.CriticalWhen(!ch.dryRun, message.WrapError(err, message.Fields{
			"message": "the user did not offboard fully",
			"context": "user offboarding",
			"user":    ch.user,
		}))
		return gimlet.NewJSONInternalErrorResponse(errors.Wrapf(err, "offboarding user '%s'", ch.user))
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
		return errors.Wrap(err, "reading host filter parameters from request body")
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

	hosts, err := data.FindHostsInRange(ctx, h.params, username)
	if err != nil {
		return gimlet.NewJSONInternalErrorResponse(errors.Wrap(err, "finding hosts matching parameters"))
	}

	resp := gimlet.NewResponseBuilder()
	for _, h := range hosts {
		apiHost := &model.APIHost{}
		apiHost.BuildFromService(&h, nil)
		if err = resp.AddData(apiHost); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "adding response data for host '%s'", utility.FromStringPtr(apiHost.Id)))
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
	return &hostProvisioningOptionsGetHandler{
		env: rh.env,
	}
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
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "generating host provisioning script"))
	}
	apiOpts := model.APIHostProvisioningOptions{
		Content: script,
	}
	return gimlet.NewJSONResponse(apiOpts)
}

// //////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/disable
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
	host, err := host.FindOneId(ctx, h.hostID)
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
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "disabling host"))
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
	host, err := host.FindOne(ctx, host.ByIPAndRunning(h.IP))
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host with IP '%s'", h.IP))
	}
	if host == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("host with IP address '%s' not found", h.IP),
		})
	}

	hostModel := &model.APIHost{}
	hostModel.BuildFromService(host, nil)
	return gimlet.NewJSONResponse(hostModel)
}
