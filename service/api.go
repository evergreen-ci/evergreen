package service

import (
	"fmt"
	"io/ioutil"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/amboy"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	APIServerLockTitle = evergreen.APIServerTaskActivator
	TaskStartCaller    = "start task"
	EndTaskCaller      = "end task"
)

// APIServer handles communication with Evergreen agents and other back-end requests.
type APIServer struct {
	UserManager         gimlet.UserManager
	Settings            evergreen.Settings
	queue               amboy.Queue
	queueGroup          amboy.QueueGroup
	taskDispatcher      model.TaskQueueItemDispatcher
	taskAliasDispatcher model.TaskQueueItemDispatcher
}

// NewAPIServer returns an APIServer initialized with the given settings and plugins.
func NewAPIServer(settings *evergreen.Settings, queue amboy.Queue, queueGroup amboy.QueueGroup) (*APIServer, error) {
	authManager, _, err := auth.LoadUserManager(settings.AuthConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := settings.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	as := &APIServer{
		UserManager:         authManager,
		Settings:            *settings,
		queue:               queue,
		queueGroup:          queueGroup,
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

// MustHaveHost gets the host from the HTTP Request
// Panics if the host is not in the request context
func MustHaveHost(r *http.Request) *host.Host {
	h := GetHost(r)
	if h == nil {
		panic("no host attached to request")
	}
	return h
}

// MustHaveProject gets the project from the HTTP request and panics
// if there is no project specified
func MustHaveProject(r *http.Request) (*model.ProjectRef, *model.Project) {
	pref, p := GetProject(r)
	if pref == nil || p == nil {
		panic("no project attached to request")
	}
	return pref, p
}

// checkTask get the task from the request header and ensures that there is a task. It checks the secret
// in the header with the secret in the db to ensure that they are the same.
func (as *APIServer) checkTask(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, code, err := model.ValidateTask(gimlet.GetVars(r)["taskId"], false, r)
		if err != nil {
			as.LoggedError(w, r, code, errors.Wrap(err, "invalid task"))
			return
		}
		r = setAPITaskContext(r, t)
		next(w, r)
	}
}

func (as *APIServer) checkTaskStrict(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, code, err := model.ValidateTask(gimlet.GetVars(r)["taskId"], true, r)
		if err != nil {
			as.LoggedError(w, r, code, errors.Wrap(err, "invalid task"))
			return
		}
		r = setAPITaskContext(r, t)
		next(w, r)
	}
}

// checkProject finds the projectId in the request and adds the
// project and project ref to the request context.
func (as *APIServer) checkProject(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectId := gimlet.GetVars(r)["projectId"]
		if projectId == "" {
			as.LoggedError(w, r, http.StatusBadRequest, errors.New("missing project Id"))
			return
		}

		projectRef, err := model.FindOneProjectRef(projectId)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
		}
		if projectRef == nil {
			as.LoggedError(w, r, http.StatusNotFound, errors.New("project not found"))
			return
		}

		p, err := model.FindLastKnownGoodProject(projectRef.Identifier)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrap(err, "Error getting patch"))
			return
		}
		if p == nil {
			as.LoggedError(w, r, http.StatusNotFound,
				errors.Errorf("can't find project: %s", p.Identifier))
			return
		}

		r = setProjectReftContext(r, projectRef)
		r = setProjectContext(r, p)

		next(w, r)
	}
}

func (as *APIServer) checkHost(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		h, code, err := model.ValidateHost(gimlet.GetVars(r)["hostId"], r)
		if err != nil {
			as.LoggedError(w, r, code, errors.Wrap(err, "host not assigned to run task"))
			return
		}
		// update host access time
		if err := h.UpdateLastCommunicated(); err != nil {
			grip.Warningf("Could not update host last communication time for %s: %+v", h.Id, err)
		}
		r = setAPIHostContext(r, h)
		next(w, r)
	}
}

func (as *APIServer) GetVersion(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	// Get the version for this task, so we can get its config data
	v, err := model.VersionFindOne(model.VersionById(t.Version))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if v == nil {
		http.Error(w, "version not found", http.StatusNotFound)
		return
	}

	gimlet.WriteJSON(w, v)
}

func (as *APIServer) GetProjectRef(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	p, err := model.FindOneProjectRef(t.Project)

	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if p == nil {
		http.Error(w, "project ref not found", http.StatusNotFound)
		return
	}

	gimlet.WriteJSON(w, p)
}

func (as *APIServer) GetExpansions(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	h := MustHaveHost(r)
	settings := as.GetSettings()
	oauthToken, err := settings.GetGithubOauthToken()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}

	e, err := model.PopulateExpansions(t, h, oauthToken)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, e)
}

// AttachTestLog is the API Server hook for getting
// the test logs and storing them in the test_logs collection.
func (as *APIServer) AttachTestLog(w http.ResponseWriter, r *http.Request) {
	if as.GetSettings().ServiceFlags.TaskLoggingDisabled {
		http.Error(w, "task logging is disabled", http.StatusConflict)
		return
	}
	t := MustHaveTask(r)
	log := &model.TestLog{}
	err := util.ReadJSONInto(util.NewRequestReader(r), log)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	// enforce proper taskID and Execution
	log.Task = t.Id
	log.TaskExecution = t.Execution

	if err := log.Insert(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	logReply := struct {
		Id string `json:"_id"`
	}{log.Id}
	gimlet.WriteJSON(w, logReply)
}

// AttachResults attaches the received results to the task in the database.
func (as *APIServer) AttachResults(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	results := &task.LocalTestResults{}
	err := util.ReadJSONInto(util.NewRequestReader(r), results)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}
	// set test result of task
	if err := t.SetResults(results.Results); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	gimlet.WriteJSON(w, "test results successfully attached")
}

// FetchProjectVars is an API hook for returning the project variables
// associated with a task's project.
func (as *APIServer) FetchProjectVars(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	projectVars, err := model.FindOneProjectVars(t.Project)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if projectVars == nil {
		gimlet.WriteJSON(w, apimodels.ExpansionVars{})
		return
	}

	gimlet.WriteJSON(w, projectVars)
}

// AttachFiles updates file mappings for a task or build
func (as *APIServer) AttachFiles(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	grip.Infoln("Attaching files to task:", t.Id)

	entry := &artifact.Entry{
		TaskId:          t.Id,
		TaskDisplayName: t.DisplayName,
		BuildId:         t.BuildId,
		Execution:       t.Execution,
	}

	err := util.ReadJSONInto(util.NewRequestReader(r), &entry.Files)
	if err != nil {
		message := fmt.Sprintf("Error reading file definitions for task  %v: %v", t.Id, err)
		grip.Error(message)
		gimlet.WriteJSONError(w, message)
		return
	}

	if err := entry.Upsert(); err != nil {
		message := fmt.Sprintf("Error updating artifact file info for task %v: %v", t.Id, err)
		grip.Error(message)
		gimlet.WriteJSONInternalError(w, message)
		return
	}
	gimlet.WriteJSON(w, fmt.Sprintf("Artifact files for task %v successfully attached", t.Id))
}

// AppendTaskLog appends the received logs to the task's internal logs.
func (as *APIServer) AppendTaskLog(w http.ResponseWriter, r *http.Request) {
	if as.GetSettings().ServiceFlags.TaskLoggingDisabled {
		http.Error(w, "task logging is disabled", http.StatusConflict)
		return
	}
	t := MustHaveTask(r)
	taskLog := &model.TaskLog{}
	_, err := util.ReadJSONIntoWithLength(util.NewRequestReader(r), taskLog)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "unable to read logs from request"))
		return
	}

	taskLog.TaskId = t.Id
	taskLog.Execution = t.Execution

	if err := taskLog.Insert(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	gimlet.WriteJSON(w, "Logs added")
}

// FetchTask loads the task from the database and sends it to the requester.
func (as *APIServer) FetchTask(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	gimlet.WriteJSON(w, t)
}

// Heartbeat handles heartbeat pings from Evergreen agents. If the heartbeating
// task is marked to be aborted, the abort response is sent.
func (as *APIServer) Heartbeat(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	heartbeatResponse := apimodels.HeartbeatResponse{}
	if t.Aborted {
		grip.Noticef("Sending abort signal for task %s", t.Id)
		heartbeatResponse.Abort = true
	}

	if err := t.UpdateHeartbeat(); err != nil {
		grip.Warningf("Error updating heartbeat for task %s: %+v", t.Id, err)
	}
	gimlet.WriteJSON(w, heartbeatResponse)
}

// fetchProjectRef returns a project ref given the project identifier
func (as *APIServer) fetchProjectRef(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["identifier"]
	projectRef, err := model.FindOneProjectRef(id)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if projectRef == nil {
		http.Error(w, fmt.Sprintf("no project found named '%v'", id), http.StatusNotFound)
		return
	}
	gimlet.WriteJSON(w, projectRef)
}

func (as *APIServer) listProjects(w http.ResponseWriter, r *http.Request) {
	allProjs, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	gimlet.WriteJSON(w, allProjs)
}

func (as *APIServer) listTasks(w http.ResponseWriter, r *http.Request) {
	_, project := MustHaveProject(r)

	// zero out the depends on and commands fields because they are
	// unnecessary and may not get marshaled properly
	for i := range project.Tasks {
		project.Tasks[i].DependsOn = []model.TaskUnitDependency{}
		project.Tasks[i].Commands = []model.PluginCommandConf{}

	}
	gimlet.WriteJSON(w, project.Tasks)
}
func (as *APIServer) listVariants(w http.ResponseWriter, r *http.Request) {
	_, project := MustHaveProject(r)

	gimlet.WriteJSON(w, project.BuildVariants)
}

// validateProjectConfig returns a slice containing a list of any errors
// found in validating the given project configuration
func (as *APIServer) validateProjectConfig(w http.ResponseWriter, r *http.Request) {
	body := util.NewRequestReader(r)
	defer body.Close()
	yamlBytes, err := ioutil.ReadAll(body)
	if err != nil {
		gimlet.WriteJSONError(w, fmt.Sprintf("Error reading request body: %v", err))
		return
	}

	project := &model.Project{}
	validationErr := validator.ValidationError{}
	if err = model.LoadProjectInto(yamlBytes, "", project); err != nil {
		validationErr.Message = err.Error()
		gimlet.WriteJSONError(w, validator.ValidationErrors{validationErr})
		return
	}
	syntaxErrs := validator.CheckProjectSyntax(project)
	semanticErrs := validator.CheckProjectSemantics(project)
	if len(syntaxErrs)+len(semanticErrs) != 0 {
		gimlet.WriteJSONError(w, append(syntaxErrs, semanticErrs...))
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
		"method":  r.Method,
		"url":     r.URL.String(),
		"code":    code,
		"len":     r.ContentLength,
		"request": gimlet.GetRequestID(r.Context()),
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
func (as *APIServer) GetServiceApp() *gimlet.APIApp {
	checkProject := gimlet.WrapperMiddleware(as.checkProject)
	checkTaskSecret := gimlet.WrapperMiddleware(as.checkTaskStrict)
	checkUser := gimlet.NewRequireAuthHandler()
	checkTask := gimlet.WrapperMiddleware(as.checkTask)
	checkHost := gimlet.WrapperMiddleware(as.checkHost)

	app := gimlet.NewApp()
	app.SetPrefix("/api")
	app.NoVersions = true
	app.SimpleVersions = true

	// Project lookup and validation routes
	app.AddRoute("/ref/{identifier}").Handler(as.fetchProjectRef).Get()
	app.AddRoute("/validate").Handler(as.validateProjectConfig).Post()

	// Internal status reporting
	app.AddRoute("/runtimes/").Handler(as.listRuntimes).Get()
	app.AddRoute("/runtimes/timeout/{seconds:\\d*}").Handler(as.lateRuntimes).Get()
	app.AddRoute("/status/consistent_task_assignment").Handler(as.consistentTaskAssignment).Get()
	app.AddRoute("/status/stuck_hosts").Handler(as.getStuckHosts).Get()
	app.AddRoute("/status/info").Handler(as.serviceStatusSimple).Get()
	app.AddRoute("/task_queue").Handler(as.getTaskQueueSizes).Get()
	app.AddRoute("/task_queue/limit").Handler(as.checkTaskQueueSize).Get()

	// CLI Operation Backends
	app.AddRoute("/tasks/{projectId}").Wrap(checkUser, checkProject).Handler(as.listTasks).Get()
	app.AddRoute("/variants/{projectId}").Wrap(checkUser, checkProject).Handler(as.listVariants).Get()
	app.AddRoute("/projects").Wrap(checkUser).Handler(as.listProjects).Get()

	// Patches
	app.PrefixRoute("/patches").Route("/").Wrap(checkUser).Handler(as.submitPatch).Put()
	app.PrefixRoute("/patches").Route("/mine").Wrap(checkUser).Handler(as.listPatches).Get()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}").Wrap(checkUser).Handler(as.summarizePatch).Get()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}").Wrap(checkUser).Handler(as.existingPatchRequest).Post()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}/{projectId}/modules").Wrap(checkUser, checkProject).Handler(as.listPatchModules).Get()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}/modules").Wrap(checkUser).Handler(as.deletePatchModule).Delete()
	app.PrefixRoute("/patches").Route("/{patchId:\\w+}/modules").Wrap(checkUser).Handler(as.updatePatchModule).Post()

	// SpawnHosts
	app.Route().Prefix("/spawn").Wrap(checkUser).Route("/{instance_id:[\\w_\\-\\@]+}/").Handler(as.hostInfo).Get()
	app.Route().Prefix("/spawn").Wrap(checkUser).Route("/{instance_id:[\\w_\\-\\@]+}/").Handler(as.modifyHost).Post()
	app.Route().Prefix("/spawns").Wrap(checkUser).Route("/").Handler(as.requestHost).Put()
	app.Route().Prefix("/spawns").Wrap(checkUser).Route("/{user}/").Handler(as.hostsInfoForUser).Get()
	app.Route().Prefix("/spawns").Wrap(checkUser).Route("/distros/list/").Handler(as.listDistros).Get()
	app.AddRoute("/dockerfile").Handler(getDockerfile).Get()

	// Agent routes
	app.Route().Version(2).Route("/agent/next_task").Wrap(checkHost).Handler(as.NextTask).Get()
	app.Route().Version(2).Route("/task/{taskId}/end").Wrap(checkTaskSecret, checkHost).Handler(as.EndTask).Post()
	app.Route().Version(2).Route("/task/{taskId}/start").Wrap(checkTaskSecret, checkHost).Handler(as.StartTask).Post()
	app.Route().Version(2).Route("/task/{taskId}/log").Wrap(checkTaskSecret, checkHost).Handler(as.AppendTaskLog).Post()
	app.Route().Version(2).Route("/task/{taskId}/").Wrap(checkTaskSecret).Handler(as.FetchTask).Get()
	app.Route().Version(2).Route("/task/{taskId}/fetch_vars").Wrap(checkTaskSecret).Handler(as.FetchProjectVars).Get()
	app.Route().Version(2).Route("/task/{taskId}/heartbeat").Wrap(checkTaskSecret, checkHost).Handler(as.Heartbeat).Post()
	app.Route().Version(2).Route("/task/{taskId}/results").Wrap(checkTaskSecret, checkHost).Handler(as.AttachResults).Post()
	app.Route().Version(2).Route("/task/{taskId}/test_logs").Wrap(checkTaskSecret, checkHost).Handler(as.AttachTestLog).Post()
	app.Route().Version(2).Route("/task/{taskId}/files").Wrap(checkTask, checkHost).Handler(as.AttachFiles).Post()
	app.Route().Version(2).Route("/task/{taskId}/distro").Wrap(checkTask).Handler(as.GetDistro).Get()
	app.Route().Version(2).Route("/task/{taskId}/version").Wrap(checkTask).Handler(as.GetVersion).Get()
	app.Route().Version(2).Route("/task/{taskId}/project_ref").Wrap(checkTask).Handler(as.GetProjectRef).Get()
	app.Route().Version(2).Route("/task/{taskId}/expansions").Wrap(checkTask, checkHost).Handler(as.GetExpansions).Get()

	// plugins
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/git/patchfile/{patchfile_id}").Wrap(checkTask).Handler(as.gitServePatchFile).Get()
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/git/patch").Wrap(checkTask).Handler(as.gitServePatch).Get()
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/keyval/inc").Wrap(checkTask).Handler(as.keyValPluginInc).Post()
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/manifest/load").Wrap(checkTask).Handler(as.manifestLoadHandler).Get()
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/s3Copy/s3Copy").Wrap(checkTask).Handler(as.s3copyPlugin).Post()
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/json/tags/{task_name}/{name}").Wrap(checkTask).Handler(as.getTaskJSONTagsForTask).Get()
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/json/history/{task_name}/{name}").Wrap(checkTask).Handler(as.getTaskJSONTaskHistory).Get()
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/json/data/{name}").Wrap(checkTask).Handler(as.insertTaskJSON).Post()
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/json/data/{task_name}/{name}").Wrap(checkTask).Handler(as.getTaskJSONByName).Get()
	app.Route().Version(2).Prefix("/task/{taskId}").Route("/json/data/{task_name}/{name}/{variant}").Wrap(checkTask).Handler(as.getTaskJSONForVariant).Get()

	return app
}
