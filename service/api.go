package service

import (
	"crypto/tls"
	"fmt"
	"io/ioutil"
	"net"
	"net/http"
	"runtime/debug"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type (
	taskKey       int
	hostKey       int
	projectKey    int
	projectRefKey int
)

const (
	apiTaskKey       taskKey       = 0
	apiHostKey       hostKey       = 0
	apiProjectKey    projectKey    = 0
	apiProjectRefKey projectRefKey = 0

	APIServerLockTitle = evergreen.APIServerTaskActivator
	TaskStartCaller    = "start task"
	EndTaskCaller      = "end task"
)

// APIServer handles communication with Evergreen agents and other back-end requests.
type APIServer struct {
	*render.Render
	UserManager  auth.UserManager
	Settings     evergreen.Settings
	clientConfig *evergreen.ClientConfig
}

// NewAPIServer returns an APIServer initialized with the given settings and plugins.
func NewAPIServer(settings *evergreen.Settings) (*APIServer, error) {
	authManager, err := auth.LoadUserManager(settings.AuthConfig)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	clientConfig, err := getClientConfig(settings)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if err := settings.Validate(); err != nil {
		return nil, errors.WithStack(err)
	}

	as := &APIServer{
		Render:       render.New(render.Options{}),
		UserManager:  authManager,
		Settings:     *settings,
		clientConfig: clientConfig,
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
		taskId := mux.Vars(r)["taskId"]
		if taskId == "" {
			as.LoggedError(w, r, http.StatusBadRequest, errors.New("missing task id"))
			return
		}
		t, err := task.FindOne(task.ById(taskId))
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if t == nil {
			as.LoggedError(w, r, http.StatusNotFound, errors.New("task not found"))
			return
		}

		if checkSecret {
			secret := r.Header.Get(evergreen.TaskSecretHeader)

			// Check the secret - if it doesn't match, write error back to the client
			if secret != t.Secret {
				grip.Errorf("Wrong secret sent for task %s: Expected %s but got %s",
					taskId, t.Secret, secret)
				http.Error(w, "wrong secret!", http.StatusConflict)
				return
			}
		}

		r = setAPITaskContext(r, t)

		next(w, r)
	}
}

// checkProject finds the projectId in the request and adds the
// project and project ref to the request context.
func (as *APIServer) checkProject(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		projectId := mux.Vars(r)["projectId"]
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
		hostId := mux.Vars(r)["hostId"]
		if hostId == "" {
			// fall back to the host header if host ids are not part of the path
			hostId = r.Header.Get(evergreen.HostHeader)
			if hostId == "" {
				message := fmt.Sprintf("Request %s is missing host information", r.URL)
				grip.Errorf(message)
				// skip all host logic and just go on to the route
				as.LoggedError(w, r, http.StatusBadRequest, errors.New(message))
				return
			}
		}
		secret := r.Header.Get(evergreen.HostSecretHeader)

		h, err := host.FindOne(host.ById(hostId))
		if h == nil {
			as.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Host %v not found", hostId))
			return
		}
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error loading context for host %v", hostId))
			return
		}
		// if there is a secret, ensure we are using the correct one -- fail if we arent
		if secret != "" && secret != h.Secret {
			// TODO (EVG-1283) error if secret is not attached as well
			as.LoggedError(w, r, http.StatusConflict, errors.Errorf("Invalid host secret for host %v", h.Id))
			return
		}

		// if the task is attached to the context, check host-task relationship
		t := GetTask(r)
		if t != nil && h.RunningTask != t.Id {
			as.LoggedError(w, r, http.StatusConflict,
				errors.Errorf("Host %v should be running %v, not %v", h.Id, h.RunningTask, t.Id))
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

	as.WriteJSON(w, http.StatusOK, v)
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

	as.WriteJSON(w, http.StatusOK, p)
}

// AttachTestLog is the API Server hook for getting
// the test logs and storing them in the test_logs collection.
func (as *APIServer) AttachTestLog(w http.ResponseWriter, r *http.Request) {
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
	as.WriteJSON(w, http.StatusOK, logReply)
}

// AttachResults attaches the received results to the task in the database.
func (as *APIServer) AttachResults(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	results := &task.TestResults{}
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
	as.WriteJSON(w, http.StatusOK, "test results successfully attached")
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
		as.WriteJSON(w, http.StatusOK, apimodels.ExpansionVars{})
		return
	}

	as.WriteJSON(w, http.StatusOK, projectVars.Vars)
}

// AttachFiles updates file mappings for a task or build
func (as *APIServer) AttachFiles(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	grip.Infoln("Attaching files to task:", t.Id)

	entry := &artifact.Entry{
		TaskId:          t.Id,
		TaskDisplayName: t.DisplayName,
		BuildId:         t.BuildId,
	}

	err := util.ReadJSONInto(util.NewRequestReader(r), &entry.Files)
	if err != nil {
		message := fmt.Sprintf("Error reading file definitions for task  %v: %v", t.Id, err)
		grip.Error(message)
		as.WriteJSON(w, http.StatusBadRequest, message)
		return
	}

	if err := entry.Upsert(); err != nil {
		message := fmt.Sprintf("Error updating artifact file info for task %v: %v", t.Id, err)
		grip.Error(message)
		as.WriteJSON(w, http.StatusInternalServerError, message)
		return
	}
	as.WriteJSON(w, http.StatusOK, fmt.Sprintf("Artifact files for task %v successfully attached", t.Id))
}

// AppendTaskLog appends the received logs to the task's internal logs.
func (as *APIServer) AppendTaskLog(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	taskLog := &model.TaskLog{}
	if err := util.ReadJSONInto(util.NewRequestReader(r), taskLog); err != nil {
		http.Error(w, "unable to read logs from request", http.StatusBadRequest)
		return
	}

	taskLog.TaskId = t.Id
	taskLog.Execution = t.Execution

	if err := taskLog.Insert(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, "Logs added")
}

// FetchTask loads the task from the database and sends it to the requester.
func (as *APIServer) FetchTask(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	as.WriteJSON(w, http.StatusOK, t)
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
	as.WriteJSON(w, http.StatusOK, heartbeatResponse)
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

	as.WriteJSON(w, http.StatusOK, struct{}{})
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
	as.WriteJSON(w, http.StatusOK, struct{}{})
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
		as.WriteJSON(w, http.StatusUnauthorized, err.Error())
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
	as.WriteJSON(w, http.StatusOK, dataOut)

}

// Get the host with the id specified in the request
func getHostFromRequest(r *http.Request) (*host.Host, error) {
	// get id and secret from the request.
	vars := mux.Vars(r)
	tag := vars["tag"]
	if len(tag) == 0 {
		return nil, errors.New("no host tag supplied")
	}
	// find the host
	h, err := host.FindOne(host.ById(tag))
	if h == nil {
		return nil, errors.Errorf("no host with tag: %v", tag)
	}
	if err != nil {
		return nil, err
	}
	return h, nil
}

func (as *APIServer) hostReady(w http.ResponseWriter, r *http.Request) {
	hostObj, err := getHostFromRequest(r)
	if err != nil {
		grip.Error(err)
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// if the host failed
	setupSuccess := mux.Vars(r)["status"]
	if setupSuccess == evergreen.HostStatusFailed {
		grip.Infof("Initializing host %s failed", hostObj.Id)
		// send notification to the Evergreen team about this provisioning failure
		subject := fmt.Sprintf("%v Evergreen provisioning failure on %v", notify.ProvisionFailurePreface, hostObj.Distro.Id)

		hostLink := fmt.Sprintf("%v/host/%v", as.Settings.Ui.Url, hostObj.Id)
		message := fmt.Sprintf("Provisioning failed on %v host -- %v (%v). %v",
			hostObj.Distro.Id, hostObj.Id, hostObj.Host, hostLink)
		if err = notify.NotifyAdmins(subject, message, &as.Settings); err != nil {
			grip.Errorln("Error sending email:", err)
		}

		// get/store setup logs
		var setupLog []byte
		body := util.NewRequestReader(r)
		defer body.Close()

		setupLog, err = ioutil.ReadAll(body)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		event.LogProvisionFailed(hostObj.Id, string(setupLog))

		err = hostObj.SetUnprovisioned()
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}

		as.WriteJSON(w, http.StatusOK, fmt.Sprintf("Initializing host %v failed", hostObj.Id))
		return
	}

	cloudManager, err := providers.GetCloudManager(hostObj.Provider, &as.Settings)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		subject := fmt.Sprintf("%v Evergreen provisioning completion failure on %v",
			notify.ProvisionFailurePreface, hostObj.Distro.Id)
		message := fmt.Sprintf("Failed to get cloud manager for host %v with provider %v: %v",
			hostObj.Id, hostObj.Provider, err)
		if err = notify.NotifyAdmins(subject, message, &as.Settings); err != nil {
			grip.Errorln("Error sending email:", err)
		}
		return
	}

	dns, err := cloudManager.GetDNSName(hostObj)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// mark host as provisioned
	if err := hostObj.MarkAsProvisioned(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	grip.Infof("Successfully marked host '%s' with dns '%s' as provisioned", hostObj.Id, dns)
}

// fetchProjectRef returns a project ref given the project identifier
func (as *APIServer) fetchProjectRef(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["identifier"]
	projectRef, err := model.FindOneProjectRef(id)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if projectRef == nil {
		http.Error(w, fmt.Sprintf("no project found named '%v'", id), http.StatusNotFound)
		return
	}
	as.WriteJSON(w, http.StatusOK, projectRef)
}

func (as *APIServer) listProjects(w http.ResponseWriter, r *http.Request) {
	allProjs, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	as.WriteJSON(w, http.StatusOK, allProjs)
}

func (as *APIServer) listTasks(w http.ResponseWriter, r *http.Request) {
	_, project := MustHaveProject(r)

	// zero out the depends on and commands fields because they are
	// unnecessary and may not get marshaled properly
	for i := range project.Tasks {
		project.Tasks[i].DependsOn = []model.TaskDependency{}
		project.Tasks[i].Commands = []model.PluginCommandConf{}

	}
	as.WriteJSON(w, http.StatusOK, project.Tasks)
}
func (as *APIServer) listVariants(w http.ResponseWriter, r *http.Request) {
	_, project := MustHaveProject(r)

	as.WriteJSON(w, http.StatusOK, project.BuildVariants)
}

// validateProjectConfig returns a slice containing a list of any errors
// found in validating the given project configuration
func (as *APIServer) validateProjectConfig(w http.ResponseWriter, r *http.Request) {
	body := util.NewRequestReader(r)
	defer body.Close()
	yamlBytes, err := ioutil.ReadAll(body)
	if err != nil {
		as.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf("Error reading request body: %v", err))
		return
	}

	project := &model.Project{}
	validationErr := validator.ValidationError{}
	if err = model.LoadProjectInto(yamlBytes, "", project); err != nil {
		validationErr.Message = err.Error()
		as.WriteJSON(w, http.StatusBadRequest, []validator.ValidationError{validationErr})
		return
	}
	syntaxErrs, err := validator.CheckProjectSyntax(project)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	semanticErrs := validator.CheckProjectSemantics(project)
	if len(syntaxErrs)+len(semanticErrs) != 0 {
		as.WriteJSON(w, http.StatusBadRequest, append(syntaxErrs, semanticErrs...))
		return
	}
	as.WriteJSON(w, http.StatusOK, []validator.ValidationError{})
}

// LoggedError logs the given error and writes an HTTP response with its details formatted
// as JSON if the request headers indicate that it's acceptable (or plaintext otherwise).
func (as *APIServer) LoggedError(w http.ResponseWriter, r *http.Request, code int, err error) {
	errorFields := message.Fields{
		"method": r.Method,
		"url":    r.URL,
		"err":    err.Error(),
		"code":   code,
	}
	if code >= 500 {
		errorFields["stack"] = string(debug.Stack())
	}
	grip.Error(errorFields)

	// if JSON is the preferred content type for the request, reply with a json message
	if strings.HasPrefix(r.Header.Get("accept"), "application/json") {
		as.WriteJSON(w, code, struct {
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
	as.WriteJSON(w, http.StatusOK, as.clientConfig)
}

// GetSettings returns the global evergreen settings.
func (as *APIServer) GetSettings() evergreen.Settings {
	return as.Settings
}

// NewRouter returns the root router for all APIServer endpoints.
func (as *APIServer) NewRouter() *mux.Router {
	root := mux.NewRouter()

	// attaches the /rest/v1 routes
	AttachRESTHandler(root, as)
	// attaches /rest/v2 routes
	APIV2Prefix := evergreen.APIRoutePrefix + "/" + evergreen.RestRoutePrefix
	route.AttachHandler(root, as.Settings.SuperUsers, as.Settings.ApiUrl, APIV2Prefix)

	r := root.PathPrefix("/api/2/").Subrouter()
	r.HandleFunc("/", home)

	apiRootOld := root.PathPrefix("/api/").Subrouter()

	// Project lookup and validation routes
	apiRootOld.HandleFunc("/ref/{identifier:[\\w_\\-\\@.]+}", as.fetchProjectRef)
	apiRootOld.HandleFunc("/validate", as.validateProjectConfig).Methods("POST")
	apiRootOld.HandleFunc("/projects", requireUser(as.listProjects, nil)).Methods("GET")
	apiRootOld.HandleFunc("/tasks/{projectId}", requireUser(as.checkProject(as.listTasks), nil)).Methods("GET")
	apiRootOld.HandleFunc("/variants/{projectId}", requireUser(as.checkProject(as.listVariants), nil)).Methods("GET")

	// Task Queue routes
	apiRootOld.HandleFunc("/task_queue", as.getTaskQueueSizes).Methods("GET")
	apiRootOld.HandleFunc("/task_queue_limit", as.checkTaskQueueSize).Methods("GET")

	// Client auto-update routes
	apiRootOld.HandleFunc("/update", as.getUpdate).Methods("GET")

	// User session routes
	apiRootOld.HandleFunc("/token", as.getUserSession).Methods("POST")

	// Patches
	patchPath := apiRootOld.PathPrefix("/patches").Subrouter()
	patchPath.HandleFunc("/", requireUser(as.submitPatch, nil)).Methods("PUT")
	patchPath.HandleFunc("/mine", requireUser(as.listPatches, nil)).Methods("GET")
	patchPath.HandleFunc("/{patchId:\\w+}", requireUser(as.summarizePatch, nil)).Methods("GET")
	patchPath.HandleFunc("/{patchId:\\w+}", requireUser(as.existingPatchRequest, nil)).Methods("POST")
	patchPath.HandleFunc("/{patchId:\\w+}/{projectId}/modules", requireUser(as.checkProject(as.listPatchModules), nil)).Methods("GET")
	patchPath.HandleFunc("/{patchId:\\w+}/modules", requireUser(as.deletePatchModule, nil)).Methods("DELETE")
	patchPath.HandleFunc("/{patchId:\\w+}/modules", requireUser(as.updatePatchModule, nil)).Methods("POST")

	// Routes for operating on existing spawn hosts - get info, terminate, etc.
	spawn := apiRootOld.PathPrefix("/spawn/").Subrouter()
	spawn.HandleFunc("/{instance_id:[\\w_\\-\\@]+}/", requireUser(as.hostInfo, nil)).Methods("GET")
	spawn.HandleFunc("/{instance_id:[\\w_\\-\\@]+}/", requireUser(as.modifyHost, nil)).Methods("POST")
	spawn.HandleFunc("/ready/{instance_id:[\\w_\\-\\@]+}/{status}", requireUser(as.spawnHostReady, nil)).Methods("POST")

	runtimes := apiRootOld.PathPrefix("/runtimes/").Subrouter()
	runtimes.HandleFunc("/", as.listRuntimes).Methods("GET")
	runtimes.HandleFunc("/timeout/{seconds:\\d*}", as.lateRuntimes).Methods("GET")

	// Internal status
	status := apiRootOld.PathPrefix("/status/").Subrouter()
	status.HandleFunc("/consistent_task_assignment", as.consistentTaskAssignment).Methods("GET")
	status.HandleFunc("/info", requireUser(as.serviceStatusWithAuth, as.serviceStatusSimple)).Methods("GET")
	status.HandleFunc("/recent_tasks", as.recentTaskStatuses).Methods("GET")
	status.HandleFunc("/stuck_hosts", as.getStuckHosts).Methods("GET")

	// Hosts callback
	host := r.PathPrefix("/host/{tag:[\\w_\\-\\@]+}/").Subrouter()
	host.HandleFunc("/ready/{status}", as.hostReady).Methods("POST")

	// Spawnhost routes - creating new hosts, listing existing hosts, listing distros
	spawns := apiRootOld.PathPrefix("/spawns/").Subrouter()
	spawns.HandleFunc("/", requireUser(as.requestHost, nil)).Methods("PUT")
	spawns.HandleFunc("/{user}/", requireUser(as.hostsInfoForUser, nil)).Methods("GET")
	spawns.HandleFunc("/distros/list/", requireUser(as.listDistros, nil)).Methods("GET")

	// Agent routes
	agentRouter := r.PathPrefix("/agent").Subrouter()
	agentRouter.HandleFunc("/next_task", as.checkHost(as.NextTask)).Methods("GET")

	taskRouter := r.PathPrefix("/task/{taskId}").Subrouter()

	taskRouter.HandleFunc("/end", as.checkTask(true, as.checkHost(as.EndTask))).Methods("POST")
	taskRouter.HandleFunc("/start", as.checkTask(true, as.checkHost(as.StartTask))).Methods("POST")
	taskRouter.HandleFunc("/new_end", as.checkTask(true, as.checkHost(as.EndTask))).Methods("POST")
	taskRouter.HandleFunc("/new_start", as.checkTask(true, as.checkHost(as.StartTask))).Methods("POST")

	taskRouter.HandleFunc("/log", as.checkTask(true, as.checkHost(as.AppendTaskLog))).Methods("POST")
	taskRouter.HandleFunc("/heartbeat", as.checkTask(true, as.checkHost(as.Heartbeat))).Methods("POST")
	taskRouter.HandleFunc("/results", as.checkTask(true, as.checkHost(as.AttachResults))).Methods("POST")
	taskRouter.HandleFunc("/test_logs", as.checkTask(true, as.checkHost(as.AttachTestLog))).Methods("POST")
	taskRouter.HandleFunc("/files", as.checkTask(false, as.checkHost(as.AttachFiles))).Methods("POST")
	taskRouter.HandleFunc("/system_info", as.checkTask(true, as.checkHost(as.TaskSystemInfo))).Methods("POST")
	taskRouter.HandleFunc("/process_info", as.checkTask(true, as.checkHost(as.TaskProcessInfo))).Methods("POST")
	taskRouter.HandleFunc("/distro", as.checkTask(false, as.GetDistro)).Methods("GET")
	taskRouter.HandleFunc("/", as.checkTask(true, as.FetchTask)).Methods("GET")
	taskRouter.HandleFunc("/version", as.checkTask(false, as.GetVersion)).Methods("GET")
	taskRouter.HandleFunc("/project_ref", as.checkTask(false, as.GetProjectRef)).Methods("GET")
	taskRouter.HandleFunc("/fetch_vars", as.checkTask(true, as.FetchProjectVars)).Methods("GET")

	// plugins
	taskRouter.HandleFunc("/git/patchfile/{patchfile_id}", as.checkTask(false, as.gitServePatchFile)).Methods("GET")
	taskRouter.HandleFunc("/git/patch", as.checkTask(false, as.gitServePatch)).Methods("GET")
	taskRouter.HandleFunc("/keyval/inc", as.checkTask(false, as.keyValPluginInc)).Methods("POST")
	taskRouter.HandleFunc("/manifest/load", as.checkTask(false, as.manifestLoadHandler)).Methods("GET")
	taskRouter.HandleFunc("/s3Copy/s3Copy", as.checkTask(false, as.s3copyPlugin)).Methods("POST")

	taskRouter.HandleFunc("/json/tags/{task_name}/{name}", as.checkTask(false, as.getTaskJSONTagsForTask)).Methods("GET")
	taskRouter.HandleFunc("/json/history/{task_name}/{name}", as.checkTask(false, as.getTaskJSONTaskHistory)).Methods("GET")
	taskRouter.HandleFunc("/json/data/{name}", as.checkTask(false, as.insertTaskJSON)).Methods("POST")
	taskRouter.HandleFunc("/json/data/{task_name}/{name}", as.checkTask(false, as.getTaskJSONByName)).Methods("GET")
	taskRouter.HandleFunc("/json/data/{task_name}/{name}/{variant}", as.checkTask(false, as.getTaskJSONForVariant)).Methods("GET")

	return root
}
