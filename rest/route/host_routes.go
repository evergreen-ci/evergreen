package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/spawn"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

type hostGetHandler struct {
	*PaginationExecutor
}

type hostPostHandler struct {
	Distro  string `json:"distro"`
	KeyName string `json:"keyname"`
}

func getHostRouteManager(route string, version int) *RouteManager {
	hgh := &hostGetHandler{}
	hostGet := MethodHandler{
		Authenticator:  &NoAuthAuthenticator{},
		RequestHandler: hgh.Handler(),
		MethodType:     http.MethodGet,
	}

	hostPost := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    &hostPostHandler{},
		MethodType:        http.MethodPost,
	}

	hostRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{hostGet, hostPost},
		Version: version,
	}
	return &hostRoute
}

func getHostIDRouteManager(route string, version int) *RouteManager {
	high := &hostIDGetHandler{}
	hostGet := MethodHandler{
		Authenticator:  &NoAuthAuthenticator{},
		RequestHandler: high.Handler(),
		MethodType:     http.MethodGet,
	}

	hostRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{hostGet},
		Version: version,
	}

	return &hostRoute
}

func getHostsByUserManager(route string, version int) *RouteManager {
	h := &hostsByUserHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				MethodType:        http.MethodGet,
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    h.Handler(),
			},
		},
	}
}

type hostIDGetHandler struct {
	hostId string
}

type hostsByUserHandler struct {
	PaginationExecutor
}

func (high *hostIDGetHandler) Handler() RequestHandler {
	return &hostIDGetHandler{}
}

// ParseAndValidate fetches the hostId from the http request.
func (high *hostIDGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := mux.Vars(r)
	high.hostId = vars["host_id"]
	return nil
}

// Execute calls the data FindHostById function and returns the host
// from the provider.
func (high *hostIDGetHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	foundHost, err := sc.FindHostById(high.hostId)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	hostModel := &model.APIHost{}
	if err = hostModel.BuildFromService(*foundHost); err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	if foundHost.RunningTask != "" {
		runningTask, err := sc.FindTaskById(foundHost.RunningTask)
		if err != nil {
			if apiErr, ok := err.(*rest.APIError); !ok || (ok && apiErr.StatusCode != http.StatusNotFound) {
				return ResponseData{}, errors.Wrap(err, "Database error")
			}
		}

		if err = hostModel.BuildFromService(runningTask); err != nil {
			return ResponseData{}, errors.Wrap(err, "problem adding task data to host response")
		}
	}

	return ResponseData{
		Result: []model.Model{hostModel},
	}, nil
}

func (hgh *hostGetHandler) Handler() RequestHandler {
	hostPaginationExecutor := &PaginationExecutor{
		KeyQueryParam:   "host_id",
		LimitQueryParam: "limit",
		Paginator:       hostPaginator,
		Args:            hostGetArgs{},
	}
	return &hostGetHandler{hostPaginationExecutor}
}

type hostGetArgs struct {
	status string
	user   string
}

func (hgh *hostGetHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	hgh.Args = hostGetArgs{
		status: r.URL.Query().Get("status"),
	}
	return hgh.PaginationExecutor.ParseAndValidate(ctx, r)
}

// hostPaginator is an instance of a PaginatorFunc that defines how to paginate on
// the host collection.
func hostPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model,
	*PageResult, error) {
	// Fetch this page of hosts, plus the next one
	// Perhaps these could be cached in case user is making multiple calls idk?
	hpArgs, ok := args.(hostGetArgs)
	if !ok {
		panic("Wrong args type passed in for host paginator")
	}
	hosts, err := sc.FindHostsById(key, hpArgs.status, hpArgs.user, limit*2, 1)
	if err != nil {
		if apiErr, ok := err.(*rest.APIError); !ok || apiErr.StatusCode != http.StatusNotFound {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}
	nextPage := makeNextHostsPage(hosts, limit)

	// Make the previous page
	prevHosts, err := sc.FindHostsById(key, hpArgs.status, hpArgs.user, limit, -1)
	if err != nil && hosts == nil {
		if apiErr, ok := err.(*rest.APIError); !ok || apiErr.StatusCode != http.StatusNotFound {
			return []model.Model{}, nil, errors.Wrap(err, "Database error")
		}
	}

	prevPage := makePrevHostsPage(prevHosts)

	pageResults := &PageResult{
		Next: nextPage,
		Prev: prevPage,
	}

	lastIndex := len(hosts)
	if nextPage != nil {
		lastIndex = limit
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

	tasks, err := sc.FindTasksByIds(taskIds)
	if err != nil {
		if apiErr, ok := err.(*rest.APIError); !ok ||
			(ok && apiErr.StatusCode != http.StatusNotFound) {
			return []model.Model{}, nil, errors.Wrap(err, "Database error")
		}
	}
	models, err := makeHostModelsWithTasks(hosts, tasks)
	if err != nil {
		return []model.Model{}, &PageResult{}, err
	}
	return models, pageResults, nil
}

func makeHostModelsWithTasks(hosts []host.Host, tasks []task.Task) ([]model.Model, error) {
	// Build a map of tasks indexed by their Id to make them easily referenceable.
	tasksById := make(map[string]task.Task, len(tasks))
	for _, t := range tasks {
		tasksById[t.Id] = t
	}
	// Create a list of host models.
	models := make([]model.Model, len(hosts))
	for ix, h := range hosts {
		apiHost := model.APIHost{}
		err := apiHost.BuildFromService(h)
		if err != nil {
			return []model.Model{}, err
		}
		if h.RunningTask != "" {
			runningTask, ok := tasksById[h.RunningTask]
			if !ok {
				continue
			}
			// Add the task information to the host document.
			err := apiHost.BuildFromService(runningTask)
			if err != nil {
				return []model.Model{}, err
			}
		}
		// Put the model into the array
		models[ix] = &apiHost
	}
	return models, nil

}

func makeNextHostsPage(hosts []host.Host, limit int) *Page {
	var nextPage *Page
	if len(hosts) > limit {
		nextLimit := len(hosts) - limit
		nextPage = &Page{
			Relation: "next",
			Key:      hosts[limit].Id,
			Limit:    nextLimit,
		}
	}
	return nextPage
}

func makePrevHostsPage(hosts []host.Host) *Page {
	var prevPage *Page
	if len(hosts) > 1 {
		prevPage = &Page{
			Relation: "prev",
			Key:      hosts[0].Id,
			Limit:    len(hosts),
		}
	}
	return prevPage
}

// Handler for the GET /users/{user_id}/hosts route to return a user's hosts
func (h *hostsByUserHandler) Handler() RequestHandler {
	return &hostsByUserHandler{PaginationExecutor{
		KeyQueryParam:   "host_id",
		LimitQueryParam: "limit",
		Paginator:       hostPaginator,
		Args:            hostGetArgs{},
	}}
}

func (h *hostsByUserHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	h.Args = hostGetArgs{
		status: r.URL.Query().Get("status"),
		user:   mux.Vars(r)["user_id"],
	}
	return h.PaginationExecutor.ParseAndValidate(ctx, r)
}

func (hph *hostPostHandler) Handler() RequestHandler {
	return &hostPostHandler{}
}

func (hph *hostPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return errors.WithStack(util.ReadJSONInto(r.Body, hph))
}

func (hph *hostPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	user := MustHaveUser(ctx)

	intentHost, err := sc.NewIntentHost(hph.Distro, hph.KeyName, "", "", user)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "error spawning host")
		}
		return ResponseData{}, err
	}

	hostModel := &model.APIHost{}
	err = hostModel.BuildFromService(intentHost)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{hostModel},
	}, nil
}

func getHostTerminateRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				MethodType:        http.MethodPost,
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &hostTerminateHandler{},
			},
		},
	}
}

type hostTerminateHandler struct {
	hostID string
}

func (h *hostTerminateHandler) Handler() RequestHandler {
	return &hostTerminateHandler{}
}

func (h *hostTerminateHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	var err error
	h.hostID, err = validateHostID(mux.Vars(r)["host_id"])

	return err
}

func (h *hostTerminateHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)

	host, err := fetchHostAndAuthorize(sc, h.hostID, u)
	if err != nil {
		return ResponseData{}, err
	}

	if host.Status == evergreen.HostTerminated {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Host %s is already terminated", host.Id),
		}

	} else if host.Status == evergreen.HostUninitialized {
		if err := sc.SetHostStatus(host, evergreen.HostTerminated); err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}

	} else {
		if err := sc.TerminateHost(host); err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}
	}

	return ResponseData{}, nil
}

func getHostChangeRDPPasswordRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				MethodType:        http.MethodPost,
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &hostChangeRDPPasswordHandler{},
			},
		},
	}
}

type hostChangeRDPPasswordHandler struct {
	hostID      string
	rdpPassword string
}

func (h *hostChangeRDPPasswordHandler) Handler() RequestHandler {
	return &hostChangeRDPPasswordHandler{}
}

func (h *hostChangeRDPPasswordHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	hostModify := model.APISpawnHostModify{}
	if err := util.ReadJSONInto(util.NewRequestReader(r), &hostModify); err != nil {
		return err
	}

	var err error
	h.hostID, err = validateHostID(mux.Vars(r)["host_id"])
	if err != nil {
		return err
	}

	h.rdpPassword = string(hostModify.RDPPwd)
	if !spawn.ValidateRDPPassword(h.rdpPassword) {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid password",
		}
	}

	return nil
}

func (h *hostChangeRDPPasswordHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)

	host, err := fetchHostAndAuthorize(sc, h.hostID, u)
	if err != nil {
		return ResponseData{}, err
	}

	if !host.Distro.IsWindows() {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "RDP passwords can only be set on Windows hosts",
		}
	}
	if host.Status != evergreen.HostRunning {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "RDP passwords can only be set on running hosts",
		}
	}

	if err := sc.SetHostPassword(ctx, host, h.rdpPassword); err != nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	return ResponseData{}, nil
}

func getHostExtendExpirationRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				MethodType:        http.MethodPost,
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &hostExtendExpirationHandler{},
			},
		},
	}
}

type hostExtendExpirationHandler struct {
	hostID   string
	addHours int
}

func (h *hostExtendExpirationHandler) Handler() RequestHandler {
	return &hostExtendExpirationHandler{}
}

func (h *hostExtendExpirationHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	hostModify := model.APISpawnHostModify{}
	if err := util.ReadJSONInto(util.NewRequestReader(r), &hostModify); err != nil {
		return err
	}

	var err error
	h.hostID, err = validateHostID(mux.Vars(r)["host_id"])
	if err != nil {
		return err
	}

	addHours, err := strconv.Atoi(string(hostModify.AddHours))
	if err != nil {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "expiration not a number",
		}
	}

	if addHours <= 0 {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "must add more than 0 hours to expiration",
		}
	}
	if addHours > spawn.MaxExpirationDurationHours {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot add more than %d hours", spawn.MaxExpirationDurationHours),
		}
	}
	h.addHours = addHours

	return nil
}

func (h *hostExtendExpirationHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)

	host, err := fetchHostAndAuthorize(sc, h.hostID, u)
	if err != nil {
		return ResponseData{}, err
	}

	var newExp time.Time
	newExp, err = spawn.MakeExtendedHostExpiration(host, h.addHours)
	if err != nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	if err := sc.SetHostExpirationTime(host, newExp); err != nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	return ResponseData{}, nil
}

func validateHostID(hostID string) (string, error) {
	if strings.TrimSpace(hostID) == "" {
		return "", &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "missing/empty host id",
		}
	}

	return hostID, nil
}

func fetchHostAndAuthorize(sc data.Connector, hostID string, user auth.User) (*host.Host, error) {
	host, err := sc.FindHostById(hostID)
	if err != nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "error fetching host information",
		}
	}
	if host == nil {
		return nil, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "host does not exist",
		}
	}

	if user.Username() != host.StartedBy {
		if !auth.IsSuperUser(sc.GetSuperUsers(), user) {
			return nil, &rest.APIError{
				StatusCode: http.StatusUnauthorized,
				Message:    "not authorized to modify host",
			}
		}
	}

	return host, nil
}
