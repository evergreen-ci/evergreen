package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

// taskByProjectHandler implements the GET /projects/{project_id}/revisions/{commit_hash}/tasks.
// It fetches the associated tasks and returns them to the user.
type tasksByProjectHandler struct {
	*PaginationExecutor
}

func getTasksByProjectAndCommitRouteManager(route string, version int) *RouteManager {
	tph := &tasksByProjectHandler{}
	tasksByProj := MethodHandler{
		Authenticator:  &RequireUserAuthenticator{},
		RequestHandler: tph.Handler(),
		MethodType:     http.MethodGet,
	}

	taskRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{tasksByProj},
		Version: version,
	}
	return &taskRoute
}

type tasksByProjectArgs struct {
	projectId  string
	commitHash string
	status     string
}

// ParseAndValidate fetches the project context and task status from the request
// and loads them into the arguments to be used by the execution.
func (tph *tasksByProjectHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	vars := gimlet.GetVars(r)
	args := tasksByProjectArgs{
		projectId:  vars["project_id"],
		commitHash: vars["commit_hash"],
		status:     r.URL.Query().Get("status"),
	}
	if args.projectId == "" {
		return gimlet.ErrorResponse{
			Message:    "ProjectId cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}

	if args.commitHash == "" {
		return gimlet.ErrorResponse{
			Message:    "Revision cannot be empty",
			StatusCode: http.StatusBadRequest,
		}
	}
	tph.Args = args
	return tph.PaginationExecutor.ParseAndValidate(ctx, r)
}

func tasksByProjectPaginator(key string, limit int, args interface{}, sc data.Connector) ([]model.Model,
	*PageResult, error) {
	ptArgs, ok := args.(tasksByProjectArgs)
	if !ok {
		panic("ARGS HAD WRONG TYPE!")
	}
	tasks, err := sc.FindTasksByProjectAndCommit(ptArgs.projectId, ptArgs.commitHash, key, ptArgs.status, limit*2, 1)
	if err != nil {
		return []model.Model{}, nil, errors.Wrap(err, "Database error")
	}

	// Make the previous page
	prevTasks, err := sc.FindTasksByProjectAndCommit(ptArgs.projectId, ptArgs.commitHash, key, ptArgs.status, limit, -1)
	if err != nil {
		if apiErr, ok := err.(gimlet.ErrorResponse); !ok || apiErr.StatusCode != http.StatusNotFound {
			return []model.Model{}, nil, errors.Wrap(err, "Database error")
		}
		return []model.Model{}, nil, err
	}

	nextPage := makeNextTasksPage(tasks, limit)

	pageResults := &PageResult{
		Next: nextPage,
		Prev: makePrevTasksPage(prevTasks),
	}

	lastIndex := len(tasks)
	if nextPage != nil {
		lastIndex = limit
	}

	// Truncate the tasks to just those that will be returned.
	tasks = tasks[:lastIndex]

	models := make([]model.Model, len(tasks))
	for ix, st := range tasks {
		taskModel := &model.APITask{}
		err = taskModel.BuildFromService(&st)
		if err != nil {
			return []model.Model{}, nil, err
		}
		err = taskModel.BuildFromService(sc.GetURL())
		if err != nil {
			return []model.Model{}, nil, err
		}
		models[ix] = taskModel
	}
	return models, pageResults, nil
}

func makeNextTasksPage(tasks []task.Task, limit int) *Page {
	var nextPage *Page
	if len(tasks) > limit {
		nextLimit := len(tasks) - limit
		nextPage = &Page{
			Relation: "next",
			Key:      tasks[limit].Id,
			Limit:    nextLimit,
		}
	}
	return nextPage
}

func makePrevTasksPage(tasks []task.Task) *Page {
	var prevPage *Page
	if len(tasks) > 1 {
		prevPage = &Page{
			Relation: "prev",
			Key:      tasks[0].Id,
			Limit:    len(tasks),
		}
	}
	return prevPage
}

func (tph *tasksByProjectHandler) Handler() RequestHandler {
	taskPaginationExecutor := &PaginationExecutor{
		KeyQueryParam:   "start_at",
		LimitQueryParam: "limit",
		Paginator:       tasksByProjectPaginator,
		Args:            tasksByProjectArgs{},
	}

	return &tasksByProjectHandler{taskPaginationExecutor}
}
