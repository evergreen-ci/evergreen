package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
	"golang.org/x/net/context"
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
		MethodType:     evergreen.MethodGet,
	}

	hostPost := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    &hostPostHandler{},
		MethodType:        evergreen.MethodPost,
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
		MethodType:     evergreen.MethodGet,
	}

	hostRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{hostGet},
		Version: version,
	}

	return &hostRoute
}

type hostIDGetHandler struct {
	hostId string
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
	err = hostModel.BuildFromService(*foundHost)
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
	hosts, err := sc.FindHostsById(key, hpArgs.status, limit*2, 1)
	if err != nil {
		if apiErr, ok := err.(*rest.APIError); !ok || apiErr.StatusCode != http.StatusNotFound {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}
	nextPage := makeNextHostsPage(hosts, limit)

	// Make the previous page
	prevHosts, err := sc.FindHostsById(key, hpArgs.status, limit, -1)
	if err != nil {
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

func (hph *hostPostHandler) Handler() RequestHandler {
	return &hostPostHandler{}
}

func (hph *hostPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	if err := util.ReadJSONInto(r.Body, hph); err != nil {
		return err
	}

	return nil
}

func (hph *hostPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	hostModel := &model.SpawnHost{}
	user := MustHaveUser(ctx)

	intentHost, err := sc.NewIntentHost(hph.Distro, hph.KeyName, user)
	if err != nil {
		if _, ok := err.(*rest.APIError); !ok {
			err = errors.Wrap(err, "error spawning host")
		}
		return ResponseData{}, err
	}

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
