package route

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apiv3"
	"github.com/evergreen-ci/evergreen/apiv3/model"
	"github.com/evergreen-ci/evergreen/apiv3/servicecontext"
	"github.com/evergreen-ci/evergreen/auth"
	serviceModel "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

func getTaskRestartRouteManager(route string, version int) *RouteManager {
	trh := &TaskRestartHandler{}
	taskRestart := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser, PrefetchProjectContext},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    trh.Handler(),
		MethodType:        evergreen.MethodPost,
	}

	taskRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{taskRestart},
		Version: version,
	}
	return &taskRoute
}

func getTaskRouteManager(route string, version int) *RouteManager {
	tep := &TaskExecutionPatchHandler{}
	taskExecutionPatch := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchProjectContext, PrefetchUser},
		Authenticator:     &NoAuthAuthenticator{},
		RequestHandler:    tep.Handler(),
		MethodType:        evergreen.MethodPatch,
	}

	tgh := &taskGetHandler{}
	taskGet := MethodHandler{
		PrefetchFunctions: []PrefetchFunc{PrefetchUser},
		Authenticator:     &RequireUserAuthenticator{},
		RequestHandler:    tgh.Handler(),
		MethodType:        evergreen.MethodGet,
	}

	taskRoute := RouteManager{
		Route:   route,
		Methods: []MethodHandler{taskExecutionPatch, taskGet},
		Version: version,
	}
	return &taskRoute
}

// taskGetHandler implements the route GET /task/{task_id}. It fetches the associated
// task and returns it to the user.
type taskGetHandler struct {
	taskId string
}

// ParseAndValidate fetches the taskId from the http request.
func (tgh *taskGetHandler) ParseAndValidate(r *http.Request) error {
	vars := mux.Vars(r)
	tgh.taskId = vars["task_id"]
	return nil
}

// Execute calls the servicecontext FindTaskById function and returns the task
// from the provider.
func (tgh *taskGetHandler) Execute(sc servicecontext.ServiceContext) (ResponseData, error) {
	foundTask, err := sc.FindTaskById(tgh.taskId)
	if err != nil {
		if _, ok := err.(*apiv3.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(foundTask)
	if err != nil {
		if _, ok := err.(*apiv3.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	err = taskModel.BuildFromService(sc.GetURL())
	if err != nil {
		if _, ok := err.(*apiv3.APIError); !ok {
			err = errors.Wrap(err, "API model error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{taskModel},
	}, nil
}

func (trh *taskGetHandler) Handler() RequestHandler {
	return &taskGetHandler{}
}

// TaskRestartHandler implements the route POST /task/{task_id}/restart. It
// fetches the needed task and project and calls the service function to
// set the proper fields when reseting the task.
type TaskRestartHandler struct {
	taskId  string
	project *serviceModel.Project

	username string
}

// ParseAndValidate fetches the taskId and Project from the request context and
// sets them on the TaskRestartHandler to be used by Execute.
func (trh *TaskRestartHandler) ParseAndValidate(r *http.Request) error {
	projCtx := MustHaveProjectContext(r)
	if projCtx.Task == nil {
		return apiv3.APIError{
			Message:    "Task not found",
			StatusCode: http.StatusNotFound,
		}
	}
	trh.taskId = projCtx.Task.Id
	if projCtx.Project == nil {
		return fmt.Errorf("Unable to fetch associated project")
	}
	trh.project = projCtx.Project
	u := MustHaveUser(r)
	trh.username = u.DisplayName()
	return nil
}

// Execute calls the servicecontext ResetTask function and returns the refreshed
// task from the service.
func (trh *TaskRestartHandler) Execute(sc servicecontext.ServiceContext) (ResponseData, error) {
	err := sc.ResetTask(trh.taskId, trh.username, trh.project)
	if err != nil {
		return ResponseData{},
			apiv3.APIError{
				Message:    err.Error(),
				StatusCode: http.StatusBadRequest,
			}
	}

	refreshedTask, err := sc.FindTaskById(trh.taskId)
	if err != nil {
		return ResponseData{}, err
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(refreshedTask)
	if err != nil {
		if _, ok := err.(*apiv3.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{taskModel},
	}, nil
}

func (trh *TaskRestartHandler) Handler() RequestHandler {
	return &TaskRestartHandler{}
}

// TaskExecutionPatchHandler implements the route PATCH /task/{task_id}. It
// fetches the changes from request, changes in activation and priority, and
// calls out to functions in the servicecontext to change these values.
type TaskExecutionPatchHandler struct {
	Activated *bool  `json:"activated"`
	Priority  *int64 `json:"priority"`

	user auth.User
	task *task.Task
}

// ParseAndValidate fetches the needed data from the request and errors otherwise.
// It fetches the task and user from the request context and fetches the changes
// in activation and priority from the request body.
func (tep *TaskExecutionPatchHandler) ParseAndValidate(r *http.Request) error {
	body := util.NewRequestReader(r)
	defer body.Close()

	decoder := json.NewDecoder(body)
	if err := decoder.Decode(tep); err != nil {
		if err == io.EOF {
			return apiv3.APIError{
				Message:    "No request body sent",
				StatusCode: http.StatusBadRequest,
			}
		}
		if e, ok := err.(*json.UnmarshalTypeError); ok {
			return apiv3.APIError{
				Message: fmt.Sprintf("Incorrect type given, expecting '%s' "+
					"but receieved '%s'",
					e.Type, e.Value),
				StatusCode: http.StatusBadRequest,
			}
		}
		return errors.Wrap(err, "JSON unmarshal error")
	}

	if tep.Activated == nil && tep.Priority == nil {
		return apiv3.APIError{
			Message:    "Must set 'activated' or 'priority'",
			StatusCode: http.StatusBadRequest,
		}
	}
	projCtx := MustHaveProjectContext(r)
	if projCtx.Task == nil {
		return apiv3.APIError{
			Message:    "Task not found",
			StatusCode: http.StatusNotFound,
		}
	}

	tep.task = projCtx.Task
	u := MustHaveUser(r)
	tep.user = u
	return nil
}

// Execute sets the Activated and Priority field of the given task and returns
// an updated version of the task.
func (tep *TaskExecutionPatchHandler) Execute(sc servicecontext.ServiceContext) (ResponseData, error) {
	if tep.Priority != nil {
		priority := *tep.Priority
		if priority > evergreen.MaxTaskPriority &&
			!auth.IsSuperUser(sc.GetSuperUsers(), tep.user) {
			return ResponseData{}, apiv3.APIError{
				Message: fmt.Sprintf("Insufficient privilege to set priority to %d, "+
					"non-superusers can only set priority at or below %d", priority, evergreen.MaxTaskPriority),
				StatusCode: http.StatusForbidden,
			}
		}
		if err := sc.SetTaskPriority(tep.task, priority); err != nil {
			return ResponseData{}, errors.Wrap(err, "Database error")
		}
	}
	if tep.Activated != nil {
		activated := *tep.Activated
		if err := sc.SetTaskActivated(tep.task.Id, tep.user.Username(), activated); err != nil {
			return ResponseData{}, errors.Wrap(err, "Database error")
		}
	}
	refreshedTask, err := sc.FindTaskById(tep.task.Id)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "Database error")
	}

	taskModel := &model.APITask{}
	err = taskModel.BuildFromService(refreshedTask)
	if err != nil {
		if _, ok := err.(*apiv3.APIError); !ok {
			err = errors.Wrap(err, "Database error")
		}
		return ResponseData{}, err
	}

	return ResponseData{
		Result: []model.Model{taskModel},
	}, nil
}

func (tep *TaskExecutionPatchHandler) Handler() RequestHandler {
	return &TaskExecutionPatchHandler{}
}
