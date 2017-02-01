package service

import (
	"crypto/tls"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/codegangsta/negroni"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/alerts"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/auth"
	"github.com/evergreen-ci/evergreen/bookkeeping"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/cloud/providers"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/artifact"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/notify"
	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/taskrunner"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/evergreen/validator"
	"github.com/evergreen-ci/render"
	"github.com/gorilla/context"
	"github.com/gorilla/mux"
	"github.com/tychoish/grip/slogger"
)

type key int

type taskKey int
type hostKey int

const apiTaskKey taskKey = 0
const apiHostKey hostKey = 0

const maxTestLogSize = 16 * 1024 * 1024 // 16 MB

// ErrLockTimeout is returned when the database lock takes too long to be acquired.
var ErrLockTimeout = errors.New("Timed out acquiring global lock")

// APIServer handles communication with Evergreen agents and other back-end requests.
type APIServer struct {
	*render.Render
	UserManager  auth.UserManager
	Settings     evergreen.Settings
	plugins      []plugin.APIPlugin
	clientConfig *evergreen.ClientConfig
}

const (
	APIServerLockTitle = evergreen.APIServerTaskActivator
	PatchLockTitle     = "patches"
)

// NewAPIServer returns an APIServer initialized with the given settings and plugins.
func NewAPIServer(settings *evergreen.Settings, plugins []plugin.APIPlugin) (*APIServer, error) {
	authManager, err := auth.LoadUserManager(settings.AuthConfig)
	if err != nil {
		return nil, err
	}

	clientConfig, err := getClientConfig(settings)
	if err != nil {
		return nil, err
	}

	as := &APIServer{
		Render:       render.New(render.Options{}),
		UserManager:  authManager,
		Settings:     *settings,
		plugins:      plugins,
		clientConfig: clientConfig,
	}

	return as, nil
}

// MustHaveTask get the task from an HTTP Request.
// Panics if the task is not in request context.
func MustHaveTask(r *http.Request) *task.Task {
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
		t, err := task.FindOne(task.ById(taskId))
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		if t == nil {
			as.LoggedError(w, r, http.StatusNotFound, fmt.Errorf("task not found"))
			return
		}

		if checkSecret {
			secret := r.Header.Get(evergreen.TaskSecretHeader)

			// Check the secret - if it doesn't match, write error back to the client
			if secret != t.Secret {
				evergreen.Logger.Logf(slogger.ERROR, "Wrong secret sent for task %v: Expected %v but got %v", taskId, t.Secret, secret)
				http.Error(w, "wrong secret!", http.StatusConflict)
				return
			}
		}

		context.Set(r, apiTaskKey, t)
		// also set the task in the context visible to plugins
		plugin.SetTask(r, t)
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
				evergreen.Logger.Logf(slogger.WARN, "Request %v is missing host information", r.URL)
				// skip all host logic and just go on to the route
				next(w, r)
				return
				// TODO (EVG-1283) treat this as an error and fail the request
			}
		}
		secret := r.Header.Get(evergreen.HostSecretHeader)

		h, err := host.FindOne(host.ById(hostId))
		if h == nil {
			as.LoggedError(w, r, http.StatusBadRequest, fmt.Errorf("Host %v not found", hostId))
			return
		}
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError,
				fmt.Errorf("Error loading context for host %v: %v", hostId, err))
			return
		}
		// if there is a secret, ensure we are using the correct one -- fail if we arent
		if secret != "" && secret != h.Secret {
			// TODO (EVG-1283) error if secret is not attached as well
			as.LoggedError(w, r, http.StatusConflict, fmt.Errorf("Invalid host secret for host %v", h.Id))
			return
		}

		// if the task is attached to the context, check host-task relationship
		if ctxTask := context.Get(r, apiTaskKey); ctxTask != nil {
			if t, ok := ctxTask.(*task.Task); ok {
				if h.RunningTask != t.Id {
					as.LoggedError(w, r, http.StatusConflict,
						fmt.Errorf("Host %v should be running %v, not %v", h.Id, h.RunningTask, t.Id))
					return
				}
			}
		}
		// update host access time
		if err := h.UpdateLastCommunicated(); err != nil {
			evergreen.Logger.Logf(slogger.WARN,
				"Could not update host last communication time for %v: %v", h.Id)
		}

		context.Set(r, apiHostKey, h) // TODO is this worth doing?
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

func (as *APIServer) StartTask(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	if !getGlobalLock(r.RemoteAddr, t.Id) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(r.RemoteAddr, t.Id)

	evergreen.Logger.Logf(slogger.INFO, "Marking task started: %v", t.Id)

	taskStartInfo := &apimodels.TaskStartRequest{}
	if err := util.ReadJSONInto(r.Body, taskStartInfo); err != nil {
		http.Error(w, fmt.Sprintf("Error reading task start request for %v: %v", t.Id, err), http.StatusBadRequest)
		return
	}

	if err := model.MarkStart(t.Id); err != nil {
		message := fmt.Errorf("Error marking task '%v' started: %v", t.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	h, err := host.FindOne(host.ByRunningTaskId(t.Id))
	if err != nil {
		message := fmt.Errorf("Error finding host running task %v: %v", t.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	// Fall back to checking host field on task doc
	if h == nil && len(t.HostId) > 0 {
		evergreen.Logger.Logf(slogger.DEBUG, "Falling back to host field of task: %v", t.Id)
		h, err = host.FindOne(host.ById(t.HostId))
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		h.SetRunningTask(t.Id, h.AgentRevision, h.TaskDispatchTime)
	}

	if h == nil {
		message := fmt.Errorf("No host found running task %v", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if err := h.SetTaskPid(taskStartInfo.Pid); err != nil {
		message := fmt.Errorf("Error calling set pid on task %v : %v", t.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}
	as.WriteJSON(w, http.StatusOK, fmt.Sprintf("Task %v started on host %v", t.Id, h.Id))
}

func (as *APIServer) EndTask(w http.ResponseWriter, r *http.Request) {
	finishTime := time.Now()
	taskEndResponse := &apimodels.TaskEndResponse{}

	t := MustHaveTask(r)

	details := &apimodels.TaskEndDetail{}
	if err := util.ReadJSONInto(r.Body, details); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check that finishing status is a valid constant
	if details.Status != evergreen.TaskSucceeded &&
		details.Status != evergreen.TaskFailed &&
		details.Status != evergreen.TaskUndispatched {
		msg := fmt.Errorf("Invalid end status '%v' for task %v", details.Status, t.Id)
		as.LoggedError(w, r, http.StatusBadRequest, msg)
		return
	}

	projectRef, err := model.FindOneProjectRef(t.Project)

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

	if !getGlobalLock(r.RemoteAddr, t.Id) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(r.RemoteAddr, t.Id)

	// mark task as finished
	err = model.MarkEnd(t.Id, APIServerLockTitle, finishTime, details, project, projectRef.DeactivatePrevious)
	if err != nil {
		message := fmt.Errorf("Error calling mark finish on task %v : %v", t.Id, err)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	if t.Requester != evergreen.PatchVersionRequester {
		evergreen.Logger.Logf(slogger.INFO, "Processing alert triggers for task %v", t.Id)
		err := alerts.RunTaskFailureTriggers(t.Id)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR, "Error processing alert triggers for task %v: %v", t.Id, err)
		}
	} else {
		//TODO(EVG-223) process patch-specific triggers
	}

	// if task was aborted, reset to inactive
	if details.Status == evergreen.TaskUndispatched {
		if err = model.SetActiveState(t.Id, "", false); err != nil {
			message := fmt.Sprintf("Error deactivating task after abort: %v", err)
			evergreen.Logger.Logf(slogger.ERROR, message)
			taskEndResponse.Message = message
			as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
			return
		}

		as.taskFinished(w, t, finishTime)
		return
	}

	// update the bookkeeping entry for the task
	err = bookkeeping.UpdateExpectedDuration(t, t.TimeTaken)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR, "Error updating expected duration: %v",
			err)
	}

	// log the task as finished
	evergreen.Logger.Logf(slogger.INFO, "Successfully marked task %v as finished", t.Id)

	// construct and return the appropriate response for the agent
	as.taskFinished(w, t, finishTime)
}

// updateTaskCost determines a task's cost based on the host it ran on. Hosts that
// are unable to calculate their own costs will not set a task's Cost field. Errors
// are logged but not returned, since any number of API failures could happen and
// we shouldn't sacrifice a task's status for them.
func (as *APIServer) updateTaskCost(t *task.Task, h *host.Host, finishTime time.Time) {
	manager, err := providers.GetCloudManager(h.Provider, &as.Settings)
	if err != nil {
		evergreen.Logger.Logf(slogger.ERROR,
			"Error loading provider for host %v cost calculation: %v ", t.HostId, err)
		return
	}
	if calc, ok := manager.(cloud.CloudCostCalculator); ok {
		evergreen.Logger.Logf(slogger.INFO, "Calculating cost for task %v", t.Id)
		cost, err := calc.CostForDuration(h, t.StartTime, finishTime)
		if err != nil {
			evergreen.Logger.Logf(slogger.ERROR,
				"Error calculating cost for task %v: %v ", t.Id, err)
			return
		}
		if err := t.SetCost(cost); err != nil {
			evergreen.Logger.Logf(slogger.ERROR,
				"Error updating cost for task %v: %v ", t.Id, err)
			return
		}
	}
}

func markHostRunningTaskFinished(h *host.Host, t *task.Task, newTaskId string) {
	// update the given host's running_task field accordingly
	if err := h.UpdateRunningTask(t.Id, newTaskId, time.Now()); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error updating running task "+
			"%v on host %v to '': %v", t.Id, h.Id, err)
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
func (as *APIServer) taskFinished(w http.ResponseWriter, t *task.Task, finishTime time.Time) {
	taskEndResponse := &apimodels.TaskEndResponse{}

	// a. fetch the host this task just completed on to see if it's
	// now decommissioned
	host, err := host.FindOne(host.ByRunningTaskId(t.Id))
	if err != nil {
		message := fmt.Sprintf("Error locating host for task %v - set to %v: %v", t.Id,
			t.HostId, err)
		evergreen.Logger.Logf(slogger.ERROR, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host == nil {
		message := fmt.Sprintf("Error finding host running for task %v - set to %v", t.Id,
			t.HostId)
		evergreen.Logger.Logf(slogger.ERROR, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host.Status == evergreen.HostDecommissioned || host.Status == evergreen.HostQuarantined {
		markHostRunningTaskFinished(host, t, "")
		message := fmt.Sprintf("Host %v - running %v - is in state '%v'. Agent will terminate",
			t.HostId, t.Id, host.Status)
		evergreen.Logger.Logf(slogger.INFO, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}

	// task cost calculations have no impact on task results, so do them in their own goroutine
	go as.updateTaskCost(t, host, finishTime)

	// b. check if the agent needs to be rebuilt
	taskRunnerInstance := taskrunner.NewTaskRunner(&as.Settings)
	agentRevision, err := taskRunnerInstance.HostGateway.GetAgentRevision()
	if err != nil {
		markHostRunningTaskFinished(host, t, "")
		evergreen.Logger.Logf(slogger.ERROR, "failed to get agent revision: %v", err)
		taskEndResponse.Message = err.Error()
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host.AgentRevision != agentRevision {
		markHostRunningTaskFinished(host, t, "")
		message := fmt.Sprintf("Remote agent needs to be rebuilt")
		evergreen.Logger.Logf(slogger.INFO, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}

	// c. fetch the task's distro queue to dispatch the next pending task
	nextTask, err := getNextDistroTask(t, host)
	if err != nil {
		markHostRunningTaskFinished(host, t, "")
		evergreen.Logger.Logf(slogger.ERROR, err.Error())
		taskEndResponse.Message = err.Error()
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}
	if nextTask == nil {
		markHostRunningTaskFinished(host, t, "")
		taskEndResponse.Message = "No next task on queue"
	} else {
		taskEndResponse.Message = "Proceed with next task"
		taskEndResponse.RunNext = true
		taskEndResponse.TaskId = nextTask.Id
		taskEndResponse.TaskSecret = nextTask.Secret
		markHostRunningTaskFinished(host, t, nextTask.Id)
	}

	// give the agent the green light to keep churning
	as.WriteJSON(w, http.StatusOK, taskEndResponse)
}

// getNextDistroTask fetches the next task to run for the given distro and marks
// the task as dispatched in the given host's document
func getNextDistroTask(currentTask *task.Task, host *host.Host) (
	nextTask *task.Task, err error) {
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
	t := MustHaveTask(r)
	// define a LimitedReader to prevent overly large logs from getting into memory
	lr := &io.LimitedReader{R: r.Body, N: maxTestLogSize}
	// manually close Body since LimitedReader is not a ReadCloser
	defer r.Body.Close()
	log := &model.TestLog{}
	err := util.ReadJSONInto(ioutil.NopCloser(lr), log)
	if lr.N == 0 {
		// error if we used every available byte in the limit reader
		as.LoggedError(w, r, http.StatusBadRequest,
			fmt.Errorf("test log size exceeds %v bytes", maxTestLogSize))
		return
	}
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
	err := util.ReadJSONInto(r.Body, results)
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
	evergreen.Logger.Logf(slogger.INFO, "Attaching files to task %v", t.Id)

	entry := &artifact.Entry{
		TaskId:          t.Id,
		TaskDisplayName: t.DisplayName,
		BuildId:         t.BuildId,
	}

	err := util.ReadJSONInto(r.Body, &entry.Files)
	if err != nil {
		message := fmt.Sprintf("Error reading file definitions for task  %v: %v", t.Id, err)
		evergreen.Logger.Errorf(slogger.ERROR, message)
		as.WriteJSON(w, http.StatusBadRequest, message)
		return
	}

	if err := entry.Upsert(); err != nil {
		message := fmt.Sprintf("Error updating artifact file info for task %v: %v", t.Id, err)
		evergreen.Logger.Errorf(slogger.ERROR, message)
		as.WriteJSON(w, http.StatusInternalServerError, message)
		return
	}
	as.WriteJSON(w, http.StatusOK, fmt.Sprintf("Artifact files for task %v successfully attached", t.Id))
}

// AppendTaskLog appends the received logs to the task's internal logs.
func (as *APIServer) AppendTaskLog(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)
	taskLog := &model.TaskLog{}
	if err := util.ReadJSONInto(r.Body, taskLog); err != nil {
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

// GetDistro loads the task's distro and sends it to the requester.
func (as *APIServer) GetDistro(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	// Get the distro for this task
	h, err := host.FindOne(host.ByRunningTaskId(t.Id))
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// Fall back to checking host field on task doc
	if h == nil && len(t.HostId) > 0 {
		h, err = host.FindOne(host.ById(t.HostId))
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		h.SetRunningTask(t.Id, h.AgentRevision, h.TaskDispatchTime)
	}

	if h == nil {
		message := fmt.Errorf("No host found running task %v", t.Id)
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	// agent can't properly unmarshal provider settings map
	h.Distro.ProviderSettings = nil
	as.WriteJSON(w, http.StatusOK, h.Distro)
}

// Heartbeat handles heartbeat pings from Evergreen agents. If the heartbeating
// task is marked to be aborted, the abort response is sent.
func (as *APIServer) Heartbeat(w http.ResponseWriter, r *http.Request) {
	t := MustHaveTask(r)

	heartbeatResponse := apimodels.HeartbeatResponse{}
	if t.Aborted {
		//evergreen.Logger.Logf(slogger.INFO, "Sending abort signal for task %v", task.Id)
		heartbeatResponse.Abort = true
	}

	if err := t.UpdateHeartbeat(); err != nil {
		//evergreen.Logger.Errorf(slogger.ERROR, "Error updating heartbeat for task %v : %v", task.Id, err)
	}
	as.WriteJSON(w, http.StatusOK, heartbeatResponse)
}

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Welcome to the API server's home :)\n")
}

// GetTask loads the task attached to a request.
func GetTask(r *http.Request) *task.Task {
	if rv := context.Get(r, apiTaskKey); rv != nil {
		return rv.(*task.Task)
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

func (as *APIServer) listTasks(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["projectId"]
	projectRef, err := model.FindOneProjectRef(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	project, err := model.FindProject("", projectRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	// zero out the depends on and commands fields because they are
	// unnecessary and may not get marshaled properly
	for i := range project.Tasks {
		project.Tasks[i].DependsOn = []model.TaskDependency{}
		project.Tasks[i].Commands = []model.PluginCommandConf{}

	}
	as.WriteJSON(w, http.StatusOK, project.Tasks)
}
func (as *APIServer) listVariants(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["projectId"]
	projectRef, err := model.FindOneProjectRef(id)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	project, err := model.FindProject("", projectRef)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	as.WriteJSON(w, http.StatusOK, project.BuildVariants)
}

// validateProjectConfig returns a slice containing a list of any errors
// found in validating the given project configuration
func (as *APIServer) validateProjectConfig(w http.ResponseWriter, r *http.Request) {
	defer r.Body.Close()
	yamlBytes, err := ioutil.ReadAll(r.Body)
	if err != nil {
		as.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf("Error reading request body: %v", err))
		return
	}

	project := &model.Project{}
	validationErr := validator.ValidationError{}
	if err := model.LoadProjectInto(yamlBytes, "", project); err != nil {
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

// helper function for grabbing the global lock
func getGlobalLock(client, taskId string) bool {
	evergreen.Logger.Logf(slogger.DEBUG, "Attempting to acquire global lock for %v (remote addr: %v)", taskId, client)

	lockAcquired, err := db.WaitTillAcquireGlobalLock(client, db.LockTimeout)
	if err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error acquiring global lock for %v (remote addr: %v): %v", taskId, client, err)
		return false
	}
	if !lockAcquired {
		evergreen.Logger.Errorf(slogger.ERROR, "Timed out attempting to acquire global lock for %v (remote addr: %v)", taskId, client)
		return false
	}

	evergreen.Logger.Logf(slogger.DEBUG, "Acquired global lock for %v (remote addr: %v)", taskId, client)
	return true
}

// helper function for releasing the global lock
func releaseGlobalLock(client, taskId string) {
	evergreen.Logger.Logf(slogger.DEBUG, "Attempting to release global lock for %v (remote addr: %v)", taskId, client)
	if err := db.ReleaseGlobalLock(client); err != nil {
		evergreen.Logger.Errorf(slogger.ERROR, "Error releasing global lock for %v (remote addr: %v) - this is really bad: %v", taskId, client, err)
	}
	evergreen.Logger.Logf(slogger.DEBUG, "Released global lock for %v (remote addr: %v)", taskId, client)
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

// Returns information about available updates for client binaries.
// Replies 404 if this data is not configured.
func (as *APIServer) getUpdate(w http.ResponseWriter, r *http.Request) {
	as.WriteJSON(w, http.StatusOK, as.clientConfig)
}

// GetSettings returns the global evergreen settings.
func (as *APIServer) GetSettings() evergreen.Settings {
	return as.Settings
}

// Handler returns the root handler for all APIServer endpoints.
func (as *APIServer) Handler() (http.Handler, error) {
	root := mux.NewRouter()
	AttachRESTHandler(root, as)

	r := root.PathPrefix("/api/2/").Subrouter()
	r.HandleFunc("/", home)

	apiRootOld := root.PathPrefix("/api/").Subrouter()

	// Project lookup and validation routes
	apiRootOld.HandleFunc("/ref/{identifier:[\\w_\\-\\@.]+}", as.fetchProjectRef)
	apiRootOld.HandleFunc("/validate", as.validateProjectConfig).Methods("POST")
	apiRootOld.HandleFunc("/projects", requireUser(as.listProjects, nil)).Methods("GET")
	apiRootOld.HandleFunc("/tasks/{projectId}", requireUser(as.listTasks, nil)).Methods("GET")
	apiRootOld.HandleFunc("/variants/{projectId}", requireUser(as.listVariants, nil)).Methods("GET")

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

	// Hosts callback
	host := r.PathPrefix("/host/{tag:[\\w_\\-\\@]+}/").Subrouter()
	host.HandleFunc("/ready/{status}", as.hostReady).Methods("POST")

	// Spawnhost routes - creating new hosts, listing existing hosts, listing distros
	spawns := apiRootOld.PathPrefix("/spawns/").Subrouter()
	spawns.HandleFunc("/", requireUser(as.requestHost, nil)).Methods("PUT")
	spawns.HandleFunc("/{user}/", requireUser(as.hostsInfoForUser, nil)).Methods("GET")
	spawns.HandleFunc("/distros/list/", requireUser(as.listDistros, nil)).Methods("GET")

	taskRouter := r.PathPrefix("/task/{taskId}").Subrouter()
	taskRouter.HandleFunc("/start", as.checkTask(true, as.checkHost(as.StartTask))).Methods("POST")
	taskRouter.HandleFunc("/end", as.checkTask(true, as.checkHost(as.EndTask))).Methods("POST")
	taskRouter.HandleFunc("/log", as.checkTask(true, as.checkHost(as.AppendTaskLog))).Methods("POST")
	taskRouter.HandleFunc("/heartbeat", as.checkTask(true, as.checkHost(as.Heartbeat))).Methods("POST")
	taskRouter.HandleFunc("/results", as.checkTask(true, as.checkHost(as.AttachResults))).Methods("POST")
	taskRouter.HandleFunc("/test_logs", as.checkTask(true, as.checkHost(as.AttachTestLog))).Methods("POST")
	taskRouter.HandleFunc("/files", as.checkTask(false, as.checkHost(as.AttachFiles))).Methods("POST")
	taskRouter.HandleFunc("/distro", as.checkTask(false, as.GetDistro)).Methods("GET")
	taskRouter.HandleFunc("/", as.checkTask(true, as.FetchTask)).Methods("GET")
	taskRouter.HandleFunc("/version", as.checkTask(false, as.GetVersion)).Methods("GET")
	taskRouter.HandleFunc("/project_ref", as.checkTask(false, as.GetProjectRef)).Methods("GET")
	taskRouter.HandleFunc("/fetch_vars", as.checkTask(true, as.FetchProjectVars)).Methods("GET")

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
		evergreen.Logger.Logf(slogger.DEBUG, "Installing API handlers for %v plugin", pl.Name())
		util.MountHandler(taskRouter, fmt.Sprintf("/%v/", pl.Name()), as.checkTask(false, handler.ServeHTTP))
	}

	n := negroni.New()
	n.Use(NewLogger())
	n.Use(negroni.HandlerFunc(UserMiddleware(as.UserManager)))
	n.UseHandler(root)
	return n, nil
}
