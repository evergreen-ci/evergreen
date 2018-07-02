package service

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/gimlet"
	"github.com/gorilla/mux"
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
	UserManager gimlet.UserManager
	Settings    evergreen.Settings
	queue       amboy.Queue
}

// NewAPIServer returns an APIServer initialized with the given settings and plugins.
func NewAPIServer(settings *evergreen.Settings, queue amboy.Queue) (*APIServer, error) {
	authManager, err := auth.LoadUserManager(settings.AuthConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := settings.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	as := &APIServer{
		UserManager: authManager,
		Settings:    *settings,
		queue:       queue,
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

// GetListener creates a network listener on the given address.
func GetListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

// GetTLSListener creates an encrypted listener with the given TLS config and address.
func GetTLSListener(addr string, conf *tls.Config) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return tls.NewListener(l, conf), nil
}

// Serve serves the handler on the given listener.
func Serve(l net.Listener, handler http.Handler) error {
	return (&http.Server{Handler: handler}).Serve(l)
}

// checkTask get the task from the request header and ensures that there is a task. It checks the secret
// in the header with the secret in the db to ensure that they are the same.
func (as *APIServer) checkTask(checkSecret bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		t, code, err := model.ValidateTask(gimlet.GetVars(r)["taskId"], checkSecret, r)
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

		p, err := model.FindProject("", projectRef)
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
	v, err := version.FindOne(version.ById(t.Version))
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
	length, err := util.ReadJSONIntoWithLength(util.NewRequestReader(r), taskLog)
	if err != nil {
		http.Error(w, "unable to read logs from request", http.StatusBadRequest)
		return
	}

	grip.Info(message.Fields{
		"message": "appending task log",
		"size":    length,
		"task_id": t.Id,
		"project": t.Project,
	})

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

// TaskSystemInfo is the handler for the system info collector, which
// reads grip/message.SystemInfo objects from the request body.
func (as *APIServer) TaskSystemInfo(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	info := &message.SystemInfo{}

	if err := util.ReadJSONInto(util.NewRequestReader(r), info); err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	event.LogTaskSystemData(t.Id, info)

	gimlet.WriteJSON(w, struct{}{})
}

// TaskProcessInfo is the handler for the process info collector, which
// reads slices of grip/message.ProcessInfo objects from the request body.
func (as *APIServer) TaskProcessInfo(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	procs := []*message.ProcessInfo{}

	if err := util.ReadJSONInto(util.NewRequestReader(r), &procs); err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	event.LogTaskProcessData(t.Id, procs)
	gimlet.WriteJSON(w, struct{}{})
}

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Welcome to the API server's home :)\n")
}

func (as *APIServer) getUserSession(w http.ResponseWriter, r *http.Request) {
	userCredentials := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}

	if err := util.ReadJSONInto(util.NewRequestReader(r), &userCredentials); err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "Error reading user credentials"))
		return
	}
	userToken, err := as.UserManager.CreateUserToken(userCredentials.Username, userCredentials.Password)
	if err != nil {
		gimlet.WriteJSONResponse(w, http.StatusUnauthorized, err.Error())
		return
	}

	dataOut := struct {
		User struct {
			Name string `json:"name"`
		} `json:"user"`
		Token string `json:"token"`
	}{}
	dataOut.User.Name = userCredentials.Username
	dataOut.Token = userToken
	gimlet.WriteJSON(w, dataOut)

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
		gimlet.WriteJSONError(w, []validator.ValidationError{validationErr})
		return
	}
	syntaxErrs, err := validator.CheckProjectSyntax(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	semanticErrs := validator.CheckProjectSemantics(project)
	if len(syntaxErrs)+len(semanticErrs) != 0 {
		gimlet.WriteJSONError(w, append(syntaxErrs, semanticErrs...))
		return
	}
	gimlet.WriteJSON(w, []validator.ValidationError{})
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

	// if JSON is the preferred content type for the request, reply with a json message
	if strings.HasPrefix(r.Header.Get("accept"), "application/json") {
		gimlet.WriteJSONResponse(w, code, struct {
			Error string `json:"error"`
		}{err.Error()})
	} else {
		// Not a JSON request, so write plaintext.
		http.Error(w, err.Error(), code)
	}
}

// Returns information about available updates for client binaries.
// Replies 404 if this data is not configured.
func (as *APIServer) getUpdate(w http.ResponseWriter, r *http.Request) {
	gimlet.WriteJSON(w, as.clientConfig)
}

// GetSettings returns the global evergreen settings.
func (as *APIServer) GetSettings() evergreen.Settings {
	return as.Settings
}

// NewRouter returns the root router for all APIServer endpoints.
func (as *APIServer) AttachRoutes(root *mux.Router) {
	root.HandleFunc("/api/2/", home)

	// Project lookup and validation routes
	root.HandleFunc("/api/ref/{identifier:[\\w_\\-\\@.]+}", as.fetchProjectRef)
	root.HandleFunc("/api/validate", as.validateProjectConfig).Methods("POST")
	root.HandleFunc("/api/projects", requireUser(as.listProjects, nil)).Methods("GET")
	root.HandleFunc("/api/tasks/{projectId}", requireUser(as.checkProject(as.listTasks), nil)).Methods("GET")
	root.HandleFunc("/api/variants/{projectId}", requireUser(as.checkProject(as.listVariants), nil)).Methods("GET")

	// User session routes
	root.HandleFunc("/api/token", as.getUserSession).Methods("POST")

	// Patches
	root.HandleFunc("/api/patches/", requireUser(as.submitPatch, nil)).Methods("PUT")
	root.HandleFunc("/api/patches/mine", requireUser(as.listPatches, nil)).Methods("GET")
	root.HandleFunc("/api/patches/{patchId:\\w+}", requireUser(as.summarizePatch, nil)).Methods("GET")
	root.HandleFunc("/api/patches/{patchId:\\w+}", requireUser(as.existingPatchRequest, nil)).Methods("POST")
	root.HandleFunc("/api/patches/{patchId:\\w+}/{projectId}/modules", requireUser(as.checkProject(as.listPatchModules), nil)).Methods("GET")
	root.HandleFunc("/api/patches/{patchId:\\w+}/modules", requireUser(as.deletePatchModule, nil)).Methods("DELETE")
	root.HandleFunc("/api/patches/{patchId:\\w+}/modules", requireUser(as.updatePatchModule, nil)).Methods("POST")

	// SpawnHosts
	root.HandleFunc("/api/spawn/{instance_id:[\\w_\\-\\@]+}/", requireUser(as.hostInfo, nil)).Methods("GET")
	root.HandleFunc("/api/spawn/{instance_id:[\\w_\\-\\@]+}/", requireUser(as.modifyHost, nil)).Methods("POST")
	root.HandleFunc("/api/spawns/", requireUser(as.requestHost, nil)).Methods("PUT")
	root.HandleFunc("/api/spawns/{user}/", requireUser(as.hostsInfoForUser, nil)).Methods("GET")
	root.HandleFunc("/api/spawns/distros/list/", requireUser(as.listDistros, nil)).Methods("GET")

	// Internal status reporting
	root.HandleFunc("/api/runtimes/", as.listRuntimes).Methods("GET")
	root.HandleFunc("/api/runtimes/timeout/{seconds:\\d*}", as.lateRuntimes).Methods("GET")
	root.HandleFunc("/api/status/consistent_task_assignment", as.consistentTaskAssignment).Methods("GET")
	root.HandleFunc("/api/status/info", requireUser(as.serviceStatusWithAuth, as.serviceStatusSimple)).Methods("GET")
	root.HandleFunc("/api/status/stuck_hosts", as.getStuckHosts).Methods("GET")
	root.HandleFunc("/api/task_queue", as.getTaskQueueSizes).Methods("GET")
	root.HandleFunc("/api/task_queue_limit", as.checkTaskQueueSize).Methods("GET")

	// Agent routes
	root.HandleFunc("/api/2/agent/next_task", as.checkHost(as.NextTask)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/end", as.checkTask(true, as.checkHost(as.EndTask))).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/start", as.checkTask(true, as.checkHost(as.StartTask))).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/log", as.checkTask(true, as.checkHost(as.AppendTaskLog))).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/heartbeat", as.checkTask(true, as.checkHost(as.Heartbeat))).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/results", as.checkTask(true, as.checkHost(as.AttachResults))).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/test_logs", as.checkTask(true, as.checkHost(as.AttachTestLog))).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/files", as.checkTask(false, as.checkHost(as.AttachFiles))).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/system_info", as.checkTask(true, as.checkHost(as.TaskSystemInfo))).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/process_info", as.checkTask(true, as.checkHost(as.TaskProcessInfo))).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/distro", as.checkTask(false, as.GetDistro)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/", as.checkTask(true, as.FetchTask)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/version", as.checkTask(false, as.GetVersion)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/project_ref", as.checkTask(false, as.GetProjectRef)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/fetch_vars", as.checkTask(true, as.FetchProjectVars)).Methods("GET")

	// plugins
	root.HandleFunc("/api/2/task/{taskId}/git/patchfile/{patchfile_id}", as.checkTask(false, as.gitServePatchFile)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/git/patch", as.checkTask(false, as.gitServePatch)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/keyval/inc", as.checkTask(false, as.keyValPluginInc)).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/manifest/load", as.checkTask(false, as.manifestLoadHandler)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/s3Copy/s3Copy", as.checkTask(false, as.s3copyPlugin)).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/json/tags/{task_name}/{name}", as.checkTask(false, as.getTaskJSONTagsForTask)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/json/history/{task_name}/{name}", as.checkTask(false, as.getTaskJSONTaskHistory)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/json/data/{name}", as.checkTask(false, as.insertTaskJSON)).Methods("POST")
	root.HandleFunc("/api/2/task/{taskId}/json/data/{task_name}/{name}", as.checkTask(false, as.getTaskJSONByName)).Methods("GET")
	root.HandleFunc("/api/2/task/{taskId}/json/data/{task_name}/{name}/{variant}", as.checkTask(false, as.getTaskJSONForVariant)).Methods("GET")
}
