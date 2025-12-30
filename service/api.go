package service

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	taskDispatcherTTL = time.Minute
)

// APIServer handles communication with Evergreen agents and other back-end requests.
type APIServer struct {
	UserManager         gimlet.UserManager
	Settings            evergreen.Settings
	env                 evergreen.Environment
	queue               amboy.Queue
	taskDispatcher      model.TaskQueueItemDispatcher
	taskAliasDispatcher model.TaskQueueItemDispatcher
}

// NewAPIServer returns an APIServer initialized with the given settings and plugins.
func NewAPIServer(env evergreen.Environment, queue amboy.Queue) (*APIServer, error) {
	settings := env.Settings()

	if err := settings.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	as := &APIServer{
		UserManager:         env.UserManager(),
		Settings:            *settings,
		env:                 env,
		queue:               queue,
		taskDispatcher:      model.NewTaskDispatchService(taskDispatcherTTL),
		taskAliasDispatcher: model.NewTaskDispatchAliasService(taskDispatcherTTL),
	}

	return as, nil
}

// MustHaveTask gets the task from an HTTP Request.
// Panics if the task is not in request context.
func MustHaveTask(r *http.Request) *task.Task {
	t := GetTask(r)
	if t == nil {
		panic("no task attached to request")
	}
	return t
}

// MustHaveProject gets the project from the HTTP request and panics
// if there is no project specified
func MustHaveProject(r *http.Request) *model.Project {
	p := GetProject(r)
	if p == nil {
		panic("no project attached to request")
	}
	return p
}

// requireProject finds the projectId in the request and adds the
// project and project ref to the request context.
func (as *APIServer) requireProject(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectId := gimlet.GetVars(r)["projectId"]
		if projectId == "" {
			as.LoggedError(w, r, http.StatusBadRequest, errors.New("missing project Id"))
			return
		}

		projectRef, err := model.FindBranchProjectRef(r.Context(), projectId)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		if projectRef == nil {
			as.LoggedError(w, r, http.StatusNotFound, errors.New("project not found"))
			return
		}

		_, p, _, err := model.FindLatestVersionWithValidProject(r.Context(), projectRef.Id, false)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrap(err, "Error getting patch"))
			return
		}
		if p == nil {
			as.LoggedError(w, r, http.StatusNotFound,
				errors.Errorf("can't find config for : %s", projectRef.Id))
			return
		}

		r = setProjectContext(r, p)

		next(w, r)
	}
}

// FetchTask loads the task from the database and sends it to the requester.
func (as *APIServer) FetchTask(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	gimlet.WriteJSON(w, t)
}

// fetchLimitedProjectRef returns a limited project ref given the project identifier.
// No new information should be added to this route, instead a REST v2 route should be added.
func (as *APIServer) fetchLimitedProjectRef(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["projectId"]
	p, err := model.FindMergedProjectRef(r.Context(), id, "", true)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if p == nil {
		http.Error(w, fmt.Sprintf("no project found named '%v'", id), http.StatusNotFound)
		return
	}

	wc := restModel.APIWorkstationConfig{}
	wc.BuildFromService(p.WorkstationConfig)

	limitedRef := restModel.APIProjectRef{
		Id:                utility.ToStringPtr(p.Id),
		Identifier:        utility.ToStringPtr(p.Identifier),
		Owner:             utility.ToStringPtr(p.Owner),
		Repo:              utility.ToStringPtr(p.Repo),
		Branch:            utility.ToStringPtr(p.Branch),
		RemotePath:        utility.ToStringPtr(p.RemotePath),
		WorkstationConfig: wc,
		CommitQueue: restModel.APICommitQueueParams{
			Message: utility.ToStringPtr(p.CommitQueue.Message),
			Enabled: p.CommitQueue.Enabled,
		},
	}

	gimlet.WriteJSON(w, limitedRef)
}

// listProjects returns the projects merged with the repo settings
func (as *APIServer) listProjects(w http.ResponseWriter, r *http.Request) {
	allProjs, err := model.FindAllMergedTrackedProjectRefs(r.Context())
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gimlet.WriteJSON(w, allProjs)
}

func (as *APIServer) listTasks(w http.ResponseWriter, r *http.Request) {
	project := MustHaveProject(r)

	// zero out the depends on and commands fields because they are
	// unnecessary and may not get marshaled properly
	for i := range project.Tasks {
		project.Tasks[i].DependsOn = []model.TaskUnitDependency{}
		project.Tasks[i].Commands = []model.PluginCommandConf{}

	}
	gimlet.WriteJSON(w, project.Tasks)
}
func (as *APIServer) listVariants(w http.ResponseWriter, r *http.Request) {
	project := MustHaveProject(r)

	gimlet.WriteJSON(w, project.BuildVariants)
}

// validateProjectConfig returns a slice containing a list of any errors
// found in validating the given project configuration
// Please do not add any functionality to this, it is deprecated.
// Use the REST v2 /validate route instead.
func (as *APIServer) validateProjectConfig(w http.ResponseWriter, r *http.Request) {
	body := utility.NewRequestReader(r)
	defer body.Close()

	bytes, err := io.ReadAll(body)
	if err != nil {
		gimlet.WriteJSONError(w, fmt.Sprintf("Error reading request body: %v", err))
		return
	}

	input := validator.ValidationInput{}
	if err := json.Unmarshal(bytes, &input); err != nil {
		// try the legacy structure
		input.ProjectYaml = bytes
	}

	project := &model.Project{}
	var projectConfig *model.ProjectConfig
	ctx := context.Background()
	opts := &model.GetProjectOpts{
		ReadFileFrom: model.ReadFromLocal,
	}
	validationErr := validator.ValidationError{}
	if _, err = model.LoadProjectInto(ctx, input.ProjectYaml, opts, input.ProjectID, project); err != nil {
		validationErr.Message = err.Error()
		gimlet.WriteJSONError(w, validator.ValidationErrors{validationErr})
		return
	}
	if projectConfig, err = model.CreateProjectConfig(input.ProjectYaml, ""); err != nil {
		validationErr.Message = err.Error()
		gimlet.WriteJSONError(w, validator.ValidationErrors{validationErr})
		return
	}

	projectRef, err := model.FindMergedProjectRef(r.Context(), input.ProjectID, "", false)
	errs := validator.CheckProject(ctx, project, projectConfig, projectRef, input.ProjectID, err)

	if input.Quiet {
		errs = errs.AtLevel(validator.Error)
	}
	if len(errs) > 0 {
		gimlet.WriteJSONError(w, errs)
		return
	}
	gimlet.WriteJSON(w, validator.ValidationErrors{})
}

// LoggedError logs the given error and writes an HTTP response with its details formatted
// as JSON if the request headers indicate that it's acceptable (or plaintext otherwise).
func (as *APIServer) LoggedError(w http.ResponseWriter, r *http.Request, code int, err error) {
	if err == nil {
		return
	}

	grip.Error(message.WrapError(err, message.Fields{
		"method":     r.Method,
		"url":        r.URL.String(),
		"code":       code,
		"len":        r.ContentLength,
		"spawn_host": r.Host,
		"request":    gimlet.GetRequestID(r.Context()),
	}))

	var resp gimlet.Responder

	// if JSON is the preferred content type for the request, reply with a json message
	if strings.HasPrefix(r.Header.Get("accept"), "application/json") {
		resp = gimlet.MakeJSONErrorResponder(err)
	} else {
		resp = gimlet.MakeTextErrorResponder(err)
	}

	if err := resp.SetStatus(code); err != nil {
		grip.Warning(errors.WithStack(resp.SetStatus(http.StatusInternalServerError)))
	}

	gimlet.WriteResponse(w, resp)
}

// GetSettings returns the global evergreen settings.
func (as *APIServer) GetSettings() evergreen.Settings {
	return as.Settings
}

// NewRouter returns the root router for all APIServer endpoints.
// These routes are deprecated; any new functionality should be added to REST v2
func (as *APIServer) GetServiceApp() *gimlet.APIApp {
	requireProject := gimlet.WrapperMiddleware(as.requireProject)
	requireUser := gimlet.NewRequireAuthHandler()
	viewTasks := route.RequiresProjectPermission(evergreen.PermissionTasks, evergreen.TasksView)
	submitPatch := route.RequiresProjectPermission(evergreen.PermissionPatches, evergreen.PatchSubmit)

	app := gimlet.NewApp()
	app.SetPrefix("/api")
	app.NoVersions = true
	app.SimpleVersions = true

	// Project lookup and validation routes
	app.AddRoute("/ref/{projectId}").Wrap(requireUser).Handler(as.fetchLimitedProjectRef).Get()
	// Please do not use this route internally, it is deprecated. Use the REST v2 /validate route instead.
	app.AddRoute("/validate").Wrap(requireUser).Handler(as.validateProjectConfig).Post()

	// Internal status reporting
	// This route is called by the app server's setup scripts which
	// doesn't pass user info, so middleware is omitted.
	app.AddRoute("/status/info").Handler(as.serviceStatusSimple).Get()

	// CLI Operation Backends
	app.AddRoute("/tasks/{projectId}").Wrap(requireUser, requireProject, viewTasks).Handler(as.listTasks).Get()
	app.AddRoute("/variants/{projectId}").Wrap(requireUser, requireProject, viewTasks).Handler(as.listVariants).Get()
	app.AddRoute("/projects").Wrap(requireUser).Handler(as.listProjects).Get()

	// Patches
	app.PrefixRoute("/patches").Route("/").Wrap(requireUser).Handler(as.submitPatch).Put()
	app.PrefixRoute("/patches").Route("/mine").Wrap(requireUser).Handler(as.listPatches).Get()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}").Wrap(requireUser, viewTasks).Handler(as.summarizePatch).Get()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}").Wrap(requireUser, submitPatch).Handler(as.existingPatchRequest).Post()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}/{projectId}/modules").Wrap(requireUser, requireProject, viewTasks).Handler(as.listPatchModules).Get()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}/modules").Wrap(requireUser, submitPatch).Handler(as.deletePatchModule).Delete()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}/modules").Wrap(requireUser, submitPatch).Handler(as.updatePatchModule).Post()

	// Dockerfile
	app.AddRoute("/dockerfile").Wrap().Handler(getDockerfile).Get()

	return app
}
