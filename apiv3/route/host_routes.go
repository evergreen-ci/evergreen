package route

import (
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/pkg/errors"
)

type hostGetHandler struct {
	*PaginationExecutor
}

func getHostRouteManager(route string, version int) *RouteManager {
	hgh := &hostGetHandler{}
	hostGet := MethodHandler{
		Authenticator:  &NoAuthAuthenticator{},
		RequestHandler: hgh.Handler(),
		MethodType:     evergreen.MethodGet,
	}

	hostRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{hostGet},
		Version: version,
	}
	return &hostRoute
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

func (hgh *hostGetHandler) ParseAndValidate(r *http.Request) error {
	hgh.Args = hostGetArgs{
		status: r.URL.Query().Get("status"),
	}
	return hgh.PaginationExecutor.ParseAndValidate(r)
}

// hostPaginator is an instance of a PaginatorFunc that defines how to paginate on
// the host collection.
func hostPaginator(key string, limit int, sc servicecontext.ServiceContext) ([]model.Model,
	*PageResult, error) {
	// Fetch this page of hosts, plus the next one
	// Perhaps these could be cached in case user is making multiple calls idk?
	hpArgs, ok := args.(hostGetArgs)
	if !ok {
		panic("Wrong args type passed in for host paginator")
	}
	hosts, err := sc.FindHostsById(key, hpArgs.status, limit*2, 1)
	if err != nil {
		if apiErr, ok := err.(apiv3.APIError); !ok || apiErr.StatusCode != http.StatusNotFound {
			err = errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}
	nextPage := makeNextHostsPage(hosts, limit)

	// Make the previous page
	prevHosts, err := sc.FindHostsById(key, hpArgs.status, limit, -1)
	if err != nil {
		if apiErr, ok := err.(apiv3.APIError); !ok || apiErr.StatusCode != http.StatusNotFound {
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
		if apiErr, ok := err.(*apiv3.APIError); !ok ||
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
