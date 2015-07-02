package apiserver

import (
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/bookkeeping"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/plugin"
	_ "github.com/evergreen-ci/evergreen/plugin/config"
	"github.com/evergreen-ci/evergreen/taskrunner"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"
)

type key int

// ErrLockTimeout is returned when the database lock takes too long to be acquired.
var ErrLockTimeout = errors.New("Timed out acquiring global lock")

type (
	//  special types used as key types in the request context map to prevent key collisions.
	userKey           int
	taskKey           int
	projectContextKey int
)

const (
	// Key values used to map user and project data to request context.
	// These are private custom types to avoid key collisions.
	apiUserKey    userKey           = 0
	apiTaskKey    taskKey           = 0
	apiProjCtxKey projectContextKey = 0
)

// APIServer handles communication with Evergreen agents and other back-end requests.
type APIServer struct {
	*render.Render
	UserManager auth.UserManager
	Settings    evergreen.Settings
	plugins     []plugin.Plugin
}

const (
	APIServerLockTitle = "apiserver"
	PatchLockTitle     = "patches"
)

// New returns an APIServer initialized with the given settings and plugins.
func New(settings *evergreen.Settings, plugins []plugin.Plugin) (*APIServer, error) {
	authManager, err := auth.LoadUserManager(settings.AuthConfig)
	if err != nil {
		return nil, err
	}

	return &APIServer{render.New(render.Options{}), authManager, *settings, plugins}, nil
}

// UserMiddleware checks for session tokens on the request, then looks up and attaches a user
// for that token if one is found.
func UserMiddleware(um auth.UserManager) func(rw http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
	return func(w http.ResponseWriter, r *http.Request, next http.HandlerFunc) {
		err := r.ParseForm()
		if err != nil {
			http.Error(w, "can't parse form?", http.StatusBadRequest)
			return
		}
		// Note: at this point the "token" is actually a json object in string form,
		// containing both the username and token.
		token := r.FormValue("id_token")
		if len(token) == 0 {
			next(w, r)
			return
		}
		authData := struct {
			Name   string `json:"auth_user"`
			Token  string `json:"auth_token"`
			APIKey string `json:"api_key"`
		}{}
		if err := util.ReadJSONInto(ioutil.NopCloser(strings.NewReader(token)), &authData); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}

		if len(authData.Token) == 0 && len(authData.APIKey) == 0 {
			next(w, r)
			return
		}
		if len(authData.Token) > 0 { // legacy auth - token lookup
			authedUser, err := um.GetUserByToken(authData.Token)
			if err != nil {
				evergreen.Logger.Logf(slogger.ERROR, "Error getting user: %v", err)
			} else {
				// Get the user's full details from the DB or create them if they don't exists
				dbUser, err := model.GetOrCreateUser(authedUser.Username(),
					authedUser.DisplayName(), authedUser.Email())
				if err != nil {
					evergreen.Logger.Logf(slogger.ERROR, "Error looking up user %v: %v", authedUser.Username(), err)
				} else {
					context.Set(r, apiUserKey, dbUser)
				}
			}
		} else if len(authData.APIKey) > 0 {
			dbUser, err := user.FindOne(user.ById(authData.Name))
			if dbUser != nil && err == nil {
				if dbUser.APIKey != authData.APIKey {
					http.Error(w, "Unauthorized - invalid API key", http.StatusUnauthorized)
					return
				}
				context.Set(r, apiUserKey, dbUser)
			} else {
				evergreen.Logger.Logf(slogger.ERROR, "Error getting user: %v", err)
			}
		}
		next(w, r)
	}
}

// MustHaveUser gets the DBUser from an HTTP Request.
// Panics if the user is not found.
func MustHaveUser(r *http.Request) *user.DBUser {
	u := GetUser(r)
	if u == nil {
		panic("no user attached to request")
	}
	return u
}

// MustHaveTask get the task from an HTTP Request.
// Panics if the task is not in request context.
func MustHaveTask(r *http.Request) *model.Task {
	t := GetTask(r)
	if t == nil {
		panic("no task attached to request")
	}
	return t
}

// GetListener creates a network listener on the given address.
func GetListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

// GetTLSListener creates an encrypted listener with the given TLS config and address.
func GetTLSListener(addr string, conf *tls.Config) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return tls.NewListener(l, conf), nil
}

// Serve serves the handler on the given listener.
func Serve(l net.Listener, handler http.Handler) error {
	return (&http.Server{Handler: handler}).Serve(l)
}

func (as *APIServer) checkTask(checkSecret bool, next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		taskId := mux.Vars(r)["taskId"]
		if taskId == "" {
			as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("missing task id"))
			return
		}
		task, err := model.FindTask(taskId)
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if task == nil {
			as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("task not found"))
			return
		}

		if checkSecret {
			secret := r.Header.Get(evergreen.TaskSecretHeader)

			// Check the secret - if it doesn't match, write error back to the client
			if secret != task.Secret {
				evergreen.Logger.Logf(slogger.ERROR, "Wrong secret sent for task %v: Expected %v but got %v", taskId, task.Secret, secret)
				http.Error(w, "wrong secret!", http.StatusConflict)
				return
			}
		}

		context.Set(r, apiTaskKey, task)
		// also set the task in the context visible to plugins
		plugin.SetTask(r, task)
		next(w, r)
	}
}

func (as *APIServer) GetVersion(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)

	// Get the version for this task, so we can get its config data
	v, err := version.FindOne(version.ById(task.Version))
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
	task := MustHaveTask(r)

	p, err := model.FindOneProjectRef(task.Project)

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

func (as *APIServer) StartTask(w http.ResponseWriter, r *http.Request) {
	if !getGlobalLock(APIServerLockTitle) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(APIServerLockTitle)

	task := MustHaveTask(r)

	evergreen.Logger.Logf(slogger.INFO, "Marking task started: %v", task.Id)

	taskStartInfo := &apimodels.TaskStartRequest{}
	if err := util.ReadJSONInto(r.Body, taskStartInfo); err != nil {
		http.Error(w, fmt.Sprintf("Error reading task start request for %v: %v", task.Id, err), http.StatusBadRequest)
		return
	}

	if err := task.MarkStart(); err != nil {
		message := fmt.Errorf("Error marking task '%v' started: %v", task.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	host, err := host.FindOne(host.ByRunningTaskId(task.Id))
	if err != nil {
		message := fmt.Errorf("Error finding host running task %v: %v", task.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if host == nil {
		message := fmt.Errorf("No host found running task %v: %v", task.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if err := host.SetTaskPid(taskStartInfo.Pid); err != nil {
		message := fmt.Errorf("Error calling set pid on task %v : %v", task.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}
	as.WriteJSON(w, http.StatusOK, fmt.Sprintf("Task %v started on host %v", task.Id, host.Id))
}

func (as *APIServer) EndTask(w http.ResponseWriter, r *http.Request) {
	finishTime := time.Now()
	taskEndResponse := &apimodels.TaskEndResponse{}

	task := MustHaveTask(r)

	details := &apimodels.TaskEndDetail{}
	if err := util.ReadJSONInto(r.Body, details); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check that finishing status is a valid constant
	if details.Status != evergreen.TaskSucceeded &&
		details.Status != evergreen.TaskFailed &&
		details.Status != evergreen.TaskUndispatched {
		msg := fmt.Errorf("Invalid end status '%v' for task %v", details.Status, task.Id)
		as.LoggedError(w, r, http.StatusBadRequest, msg)
		return
	}

	projectRef, err := model.FindOneProjectRef(task.Project)

	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}

	if projectRef == nil {
		as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("empty projectRef for task"))
		return
	}

	project, err := model.FindProject("", projectRef)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if !getGlobalLock(APIServerLockTitle) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(APIServerLockTitle)

	// mark task as finished
	err = task.MarkEnd(APIServerLockTitle, finishTime, details, project, projectRef.DeactivatePrevious)
	if err != nil {
		message := fmt.Errorf("Error calling mark finish on task %v : %v", task.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if task.Requester != evergreen.PatchVersionRequester {
		alerts.RunTaskFailureTriggers(task)
	} else {
		//TODO(EVG-223) process patch-specific triggers
	}

	// if task was aborted, reset to inactive
	if details.Status == evergreen.TaskUndispatched {
		if err = model.SetTaskActivated(task.Id, "", false); err != nil {
			message := fmt.Sprintf("Error deactivating task after abort: %v", err)
			evergreen.Logger.Logf(slogger.ERROR, message)
			taskEndResponse.Message = message
			as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
			return
		}

		as.taskFinished(w, task, finishTime)
		return
	}

	// update the bookkeeping entry for the task
	err = bookkeeping.UpdateExpectedDuration(task, task.TimeTaken)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Error updating expected duration: %v",
			err)
	}

	// log the task as finished
	evergreen.Logger.Logf(slogger.INFO, "Successfully marked task %v as finished", task.Id)

	// construct and return the appropriate response for the agent
	as.taskFinished(w, task, finishTime)
}

func markHostRunningTaskFinished(h *host.Host, task *model.Task, newTaskId string) {
	// update the given host's running_task field accordingly
	if err := h.UpdateRunningTask(task.Id, newTaskId, time.Now()); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error updating running task "+
			"%v on host %v to '': %v", task.Id, h.Id, err)
	}
}

// taskFinished constructs the appropriate response for each markEnd
// request the API server receives from an agent. The two possible responses are:
// 1. Inform the agent of another task to run
// 2. Inform the agent that it should terminate immediately
// The first case is the usual expected flow. The second case however, could
// occur for a number of reasons including:
// a. The version of the agent running on the remote machine is stale
// b. The host the agent is running on has been decommissioned
// c. There is no currently queued dispatchable and activated task
// In any of these aforementioned cases, the agent in question should terminate
// immediately and cease running any tasks on its host.
func (as *APIServer) taskFinished(w http.ResponseWriter, task *model.Task, finishTime time.Time) {
	taskEndResponse := &apimodels.TaskEndResponse{}

	// a. fetch the host this task just completed on to see if it's
	// now decommissioned
	host, err := host.FindOne(host.ByRunningTaskId(task.Id))
	if err != nil {
		message := fmt.Sprintf("Error locating host for task %v - set to %v: %v", task.Id,
			task.HostId, err)
		evergreen.Logger.Logf(slogger.ERROR, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host == nil {
		message := fmt.Sprintf("Error finding host running for task %v - set to %v: %v", task.Id,
			task.HostId, err)
		evergreen.Logger.Logf(slogger.ERROR, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host.Status == evergreen.HostDecommissioned || host.Status == evergreen.HostQuarantined {
		markHostRunningTaskFinished(host, task, "")
		message := fmt.Sprintf("Host %v - running %v - is in state '%v'. Agent will terminate",
			task.HostId, task.Id, host.Status)
		evergreen.Logger.Logf(slogger.INFO, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}

	// b. check if the agent needs to be rebuilt
	taskRunnerInstance := taskrunner.NewTaskRunner(&as.Settings)
	agentRevision, err := taskRunnerInstance.HostGateway.GetAgentRevision()
	if err != nil {
		markHostRunningTaskFinished(host, task, "")
		evergreen.Logger.Logf(slogger.ERROR, "failed to get agent revision: %v", err)
		taskEndResponse.Message = err.Error()
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host.AgentRevision != agentRevision {
		markHostRunningTaskFinished(host, task, "")
		message := fmt.Sprintf("Remote agent needs to be rebuilt")
		evergreen.Logger.Logf(slogger.INFO, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}

	// c. fetch the task's distro queue to dispatch the next pending task
	nextTask, err := getNextDistroTask(task, host)
	if err != nil {
		markHostRunningTaskFinished(host, task, "")
		evergreen.Logger.Logf(slogger.ERROR, err.Error())
		taskEndResponse.Message = err.Error()
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}
	if nextTask == nil {
		markHostRunningTaskFinished(host, task, "")
		taskEndResponse.Message = "No next task on queue"
	} else {
		taskEndResponse.Message = "Proceed with next task"
		taskEndResponse.RunNext = true
		taskEndResponse.TaskId = nextTask.Id
		taskEndResponse.TaskSecret = nextTask.Secret
		markHostRunningTaskFinished(host, task, nextTask.Id)
	}

	// give the agent the green light to keep churning
	as.WriteJSON(w, http.StatusOK, taskEndResponse)
}

// getNextDistroTask fetches the next task to run for the given distro and marks
// the task as dispatched in the given host's document
func getNextDistroTask(currentTask *model.Task, host *host.Host) (
	nextTask *model.Task, err error) {
	taskQueue, err := model.FindTaskQueueForDistro(currentTask.DistroId)
	if err != nil {
		return nil, fmt.Errorf("Error locating distro queue (%v) for task "+
			"'%v': %v", currentTask.DistroId, currentTask.Id, err)
	}

	if taskQueue == nil {
		return nil, fmt.Errorf("Nil task queue found for task '%v's distro "+
			"queue - '%v'", currentTask.Id, currentTask.DistroId)
	}

	// dispatch the next task for this host
	nextTask, err = taskrunner.DispatchTaskForHost(taskQueue, host)
	if err != nil {
		return nil, fmt.Errorf("Error dequeuing task for host %v: %v",
			host.Id, err)
	}
	if nextTask == nil {
		return nil, nil
	}
	return nextTask, nil
}

// AttachTestLog is the API Server hook for getting
// the test logs and storing them in the test_logs collection.
func (as *APIServer) AttachTestLog(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)
	log := &model.TestLog{}
	err := util.ReadJSONInto(r.Body, log)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	// enforce proper taskID and Execution
	log.Task = task.Id
	log.TaskExecution = task.Execution

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
	task := MustHaveTask(r)
	results := &model.TestResults{}
	err := util.ReadJSONInto(r.Body, results)
	if err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}
	// set test result of task
	if err := task.SetResults(results.Results); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	as.WriteJSON(w, http.StatusOK, "test results successfully attached")
}

// FetchProjectVars is an API hook for returning the project variables
// associated with a task's project.
func (as *APIServer) FetchProjectVars(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)
	projectVars, err := model.FindOneProjectVars(task.Project)
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
	task := MustHaveTask(r)
	evergreen.Logger.Logf(slogger.INFO, "Attaching files to task %v", task.Id)

	entry := &artifact.Entry{
		TaskId:          task.Id,
		TaskDisplayName: task.DisplayName,
		BuildId:         task.BuildId,
	}

	err := util.ReadJSONInto(r.Body, &entry.Files)
	if err != nil {
		message := fmt.Sprintf("Error reading file definitions for task  %v: %v", task.Id, err)
		evergreen.Logger.Errorf(slogger.ERROR, message)
		as.WriteJSON(w, http.StatusBadRequest, message)
		return
	}
	fmt.Printf("file entry is %#v\n", entry)

	if err := entry.Upsert(); err != nil {
		message := fmt.Sprintf("Error updating artifact file info for task %v: %v", task.Id, err)
		evergreen.Logger.Errorf(slogger.ERROR, message)
		as.WriteJSON(w, http.StatusInternalServerError, message)
		return
	}
	as.WriteJSON(w, http.StatusOK, fmt.Sprintf("Artifact files for task %v successfully attached", task.Id))
}

// AppendTaskLog appends the received logs to the task's internal logs.
func (as *APIServer) AppendTaskLog(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)
	taskLog := &model.TaskLog{}

	if err := util.ReadJSONInto(r.Body, taskLog); err != nil {
		http.Error(w, "unable to read logs from request", http.StatusBadRequest)
		return
	}

	taskLog.TaskId = task.Id
	taskLog.Execution = task.Execution

	if err := taskLog.Insert(); err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	as.WriteJSON(w, http.StatusOK, "Logs added")
}

// GetPatch loads the task's patch data from the database and sends
// it to the requester.
func (as *APIServer) GetPatch(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)

	patch, err := task.FetchPatch()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	if patch == nil {
		http.Error(w, "patch not found", http.StatusNotFound)
		return
	}
	as.WriteJSON(w, http.StatusOK, patch)
}

// FetchTask loads the task from the database and sends it to the requester.
func (as *APIServer) FetchTask(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)
	as.WriteJSON(w, http.StatusOK, task)
}

// GetDistro loads the task's distro and sends it to the requester.
func (as *APIServer) GetDistro(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)

	// Get the distro for this task
	h, err := host.FindOne(host.ByRunningTaskId(task.Id))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// agent can't properly unmarshal provider settings map
	h.Distro.ProviderSettings = nil
	as.WriteJSON(w, http.StatusOK, h.Distro)
}

// Heartbeat handles heartbeat pings from Evergreen agents. If the heartbeating
// task is marked to be aborted, the abort response is sent.
func (as *APIServer) Heartbeat(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)

	heartbeatResponse := apimodels.HeartbeatResponse{}
	if task.Aborted {
		//evergreen.Logger.Logf(slogger.INFO, "Sending abort signal for task %v", task.Id)
		heartbeatResponse.Abort = true
	}

	if err := task.UpdateHeartbeat(); err != nil {
		//evergreen.Logger.Errorf(slogger.ERROR, "Error updating heartbeat for task %v : %v", task.Id, err)
	}
	as.WriteJSON(w, http.StatusOK, heartbeatResponse)
}

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Welcome to the API server's home :)\n")
}

// GetUser loads the user attached to a request.
func GetUser(r *http.Request) *user.DBUser {
	if rv := context.Get(r, apiUserKey); rv != nil {
		return rv.(*user.DBUser)
	}
	return nil
}

// GetTask loads the task attached to a request.
func GetTask(r *http.Request) *model.Task {
	if rv := context.Get(r, apiTaskKey); rv != nil {
		return rv.(*model.Task)
	}
	return nil
}

func (as *APIServer) getUserSession(w http.ResponseWriter, r *http.Request) {
	userCredentials := struct {
		Username string `json:"username"`
		Password string `json:"password"`
	}{}

	if err := util.ReadJSONInto(r.Body, &userCredentials); err != nil {
		as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Error reading user credentials: %v", err))
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
		return nil, fmt.Errorf("no host tag supplied")
	}
	// find the host
	host, err := host.FindOne(host.ById(tag))
	if host == nil {
		return nil, fmt.Errorf("no host with tag: %v", tag)
	}
	if err != nil {
		return nil, err
	}
	return host, nil
}

func (as *APIServer) hostReady(w http.ResponseWriter, r *http.Request) {
	hostObj, err := getHostFromRequest(r)
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// if the host failed
	setupSuccess := mux.Vars(r)["status"]
	if setupSuccess == evergreen.HostStatusFailed {
		evergreen.Logger.Logf(slogger.INFO, "Initializing host %v failed", hostObj.Id)
		// send notification to the Evergreen team about this provisioning failure
		subject := fmt.Sprintf("%v Evergreen provisioning failure on %v", notify.ProvisionFailurePreface, hostObj.Distro.Id)

		hostLink := fmt.Sprintf("%v/host/%v", as.Settings.Ui.Url, hostObj.Id)
		message := fmt.Sprintf("Provisioning failed on %v host -- %v (%v). %v",
			hostObj.Distro.Id, hostObj.Id, hostObj.Host, hostLink)
		if err = notify.NotifyAdmins(subject, message, &as.Settings); err != nil {
			evergreen.Logger.Errorf(slogger.ERROR, "Error sending email: %v", err)
		}

		// get/store setup logs
		setupLog, err := ioutil.ReadAll(r.Body)
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
			evergreen.Logger.Errorf(slogger.ERROR, "Error sending email: %v", err)
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

	evergreen.Logger.Logf(slogger.INFO, "Successfully marked host “%v” with dns “%v” as provisioned", hostObj.Id, dns)
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

// validateProjectConfig returns a slice containing a list of any errors
// found in validating the given project configuration
func (as *APIServer) validateProjectConfig(w http.ResponseWriter, r *http.Request) {
	project := &model.Project{}
	validationErr := validator.ValidationError{}
	if err := util.ReadYAMLInto(r.Body, project); err != nil {
		validationErr.Message = err.Error()
		as.WriteJSON(w, http.StatusBadRequest, []validator.ValidationError{validationErr})
		return
	}
	syntaxErrs := validator.CheckProjectSyntax(project)
	semanticErrs := validator.CheckProjectSemantics(project)
	if len(syntaxErrs)+len(semanticErrs) != 0 {
		as.WriteJSON(w, http.StatusBadRequest, append(syntaxErrs, semanticErrs...))
		return
	}
	as.WriteJSON(w, http.StatusOK, []validator.ValidationError{})
}

// helper function for grabbing the global lock
func getGlobalLock(requester string) bool {
	evergreen.Logger.Logf(slogger.DEBUG, "Attempting to acquire global lock for %v", requester)
	lockAcquired, err := db.WaitTillAcquireGlobalLock(requester, db.LockTimeout)
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error acquiring global lock: %v", err)
		return false
	}
	if !lockAcquired {
		evergreen.Logger.Errorf(slogger.ERROR, "Cannot proceed with %v api method because the global lock could not be taken", requester)
		return false
	}

	evergreen.Logger.Logf(slogger.DEBUG, "Acquired global lock for %v", requester)
	return true
}

// helper function for releasing the global lock
func releaseGlobalLock(requester string) {
	evergreen.Logger.Logf(slogger.DEBUG, "Attempting to release global lock from %v", requester)

	err := db.ReleaseGlobalLock(requester)
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error releasing global lock from %v - this is really bad: %v", requester, err)
	}
	evergreen.Logger.Logf(slogger.DEBUG, "Released global lock from %v", requester)
}

// LoggedError logs the given error and writes an HTTP response with its details formatted
// as JSON if the request headers indicate that it's acceptable (or plaintext otherwise).
func (as *APIServer) LoggedError(w http.ResponseWriter, r *http.Request, code int, err error) {
	evergreen.Logger.Logf(slogger.ERROR, fmt.Sprintf("%v %v %v", r.Method, r.URL, err.Error()))
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

func requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if GetUser(r) == nil {
			http.Error(w, "not authorized", http.StatusUnauthorized)
			return
		}
		next(w, r)
	}
}

// Returns information about available updates for client binaries.
// Replies 404 if this data is not configured.
func (as *APIServer) getUpdate(w http.ResponseWriter, r *http.Request) {
	if len(as.Settings.Api.Clients.LatestRevision) == 0 {
		// auto-update is not configured
		as.WriteJSON(w, http.StatusNotFound, "{}")
		return
	}
	as.WriteJSON(w, http.StatusOK, as.Settings.Api.Clients)
}

// Handler returns the root handler for all APIServer endpoints.
func (as *APIServer) Handler() (http.Handler, error) {
	root := mux.NewRouter()
	r := root.PathPrefix("/api/2/").Subrouter()
	r.HandleFunc("/", home)

	apiRootOld := root.PathPrefix("/api/").Subrouter()

	// Project lookup and validation routes
	apiRootOld.HandleFunc("/ref/{identifier:[\\w_\\-\\@.]+}", as.fetchProjectRef)
	apiRootOld.HandleFunc("/validate", as.validateProjectConfig).Methods("POST")
	apiRootOld.HandleFunc("/projects", requireUser(as.listProjects)).Methods("GET")

	// Client auto-update routes
	apiRootOld.HandleFunc("/update", as.getUpdate).Methods("GET")

	// User session routes
	apiRootOld.HandleFunc("/token", as.getUserSession).Methods("POST")

	// Patches
	patchPath := apiRootOld.PathPrefix("/patches").Subrouter()
	patchPath.HandleFunc("/", requireUser(as.submitPatch)).Methods("PUT")
	patchPath.HandleFunc("/mine", requireUser(as.listPatches)).Methods("GET")
	patchPath.HandleFunc("/{patchId:\\w+}", requireUser(as.summarizePatch)).Methods("GET")
	patchPath.HandleFunc("/{patchId:\\w+}", requireUser(as.existingPatchRequest)).Methods("POST")
	patchPath.HandleFunc("/{patchId:\\w+}/modules", requireUser(as.deletePatchModule)).Methods("DELETE")
	patchPath.HandleFunc("/{patchId:\\w+}/modules", requireUser(as.updatePatchModule)).Methods("POST")

	// Routes for operating on existing spawn hosts - get info, terminate, etc.
	spawn := apiRootOld.PathPrefix("/spawn/").Subrouter()
	spawn.HandleFunc("/{instance_id:[\\w_\\-\\@]+}/", requireUser(as.hostInfo)).Methods("GET")
	spawn.HandleFunc("/{instance_id:[\\w_\\-\\@]+}/", requireUser(as.modifyHost)).Methods("POST")
	spawn.HandleFunc("/ready/{instance_id:[\\w_\\-\\@]+}/{status}", requireUser(as.spawnHostReady)).Methods("POST")

	runtimes := apiRootOld.PathPrefix("/runtimes/").Subrouter()
	runtimes.HandleFunc("/", as.listRuntimes).Methods("GET")
	runtimes.HandleFunc("/timeout/{seconds:\\d*}", as.lateRuntimes).Methods("GET")

	// Hosts callback
	host := r.PathPrefix("/host/{tag:[\\w_\\-\\@]+}/").Subrouter()
	host.HandleFunc("/ready/{status}", as.hostReady).Methods("POST")

	// Spawnhost routes - creating new hosts, listing existing hosts, listing distros
	spawns := apiRootOld.PathPrefix("/spawns/").Subrouter()
	spawns.HandleFunc("/", requireUser(as.requestHost)).Methods("PUT")
	spawns.HandleFunc("/{user}/", requireUser(as.hostsInfoForUser)).Methods("GET")
	spawns.HandleFunc("/distros/list/", requireUser(as.listDistros)).Methods("GET")

	taskRouter := r.PathPrefix("/task/{taskId:[\\w_\\.]+}").Subrouter()
	taskRouter.HandleFunc("/start", as.checkTask(true, as.StartTask)).Methods("POST")
	taskRouter.HandleFunc("/end2", as.checkTask(true, as.EndTask)).Methods("POST")
	taskRouter.HandleFunc("/end", as.checkTask(true, as.EndTask)).Methods("POST")
	taskRouter.HandleFunc("/log", as.checkTask(true, as.AppendTaskLog)).Methods("POST")
	taskRouter.HandleFunc("/heartbeat", as.checkTask(true, as.Heartbeat)).Methods("POST")
	taskRouter.HandleFunc("/results", as.checkTask(true, as.AttachResults)).Methods("POST")
	taskRouter.HandleFunc("/test_logs", as.checkTask(true, as.AttachTestLog)).Methods("POST")
	taskRouter.HandleFunc("/patch", as.checkTask(true, as.GetPatch)).Methods("GET")
	taskRouter.HandleFunc("/distro", as.checkTask(false, as.GetDistro)).Methods("GET") // nosecret check
	taskRouter.HandleFunc("/", as.checkTask(true, as.FetchTask)).Methods("GET")
	taskRouter.HandleFunc("/version", as.checkTask(false, as.GetVersion)).Methods("GET")
	taskRouter.HandleFunc("/project_ref", as.checkTask(false, as.GetProjectRef)).Methods("GET")
	taskRouter.HandleFunc("/fetch_vars", as.checkTask(true, as.FetchProjectVars)).Methods("GET")
	taskRouter.HandleFunc("/files", as.checkTask(false, as.AttachFiles)).Methods("POST")

	// Install plugin routes
	for _, pl := range as.plugins {
		if pl == nil {
			continue
		}
		pluginSettings := as.Settings.Plugins[pl.Name()]
		err := pl.Configure(pluginSettings)
		if err != nil {
			return nil, fmt.Errorf("Failed to configure plugin %v: %v", pl.Name(), err)
		}
		handler := pl.GetAPIHandler()
		if handler == nil {
			evergreen.Logger.Logf(slogger.WARN, "no API handlers to install for %v plugin", pl.Name())
			continue
		}
		evergreen.Logger.Logf(slogger.WARN, "Installing API handlers for %v plugin", pl.Name())
		util.MountHandler(taskRouter, fmt.Sprintf("/%v/", pl.Name()), as.checkTask(false, handler.ServeHTTP))
	}

	n := negroni.New()
	n.Use(negroni.NewLogger())
	n.Use(negroni.HandlerFunc(UserMiddleware(as.UserManager)))
	n.UseHandler(root)
	return n, nil
}
