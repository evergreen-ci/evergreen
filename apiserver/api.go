package apiserver

import (
	"10gen.com/mci"
	"10gen.com/mci/apimodels"
	"10gen.com/mci/auth"
	"10gen.com/mci/bookkeeping"
	"10gen.com/mci/cloud/providers"
	"10gen.com/mci/db"
	"10gen.com/mci/model"
	"10gen.com/mci/model/artifact"
	"10gen.com/mci/model/event"
	"10gen.com/mci/model/host"
	"10gen.com/mci/model/user"
	"10gen.com/mci/model/version"
	"10gen.com/mci/notify"
	"10gen.com/mci/plugin"
	_ "10gen.com/mci/plugin/config"
	"10gen.com/mci/taskrunner"
	"10gen.com/mci/util"
	"10gen.com/mci/validator"
	"crypto/tls"
	"errors"
	"fmt"
	"github.com/10gen-labs/slogger/v1"
	"github.com/codegangsta/negroni"
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

type APIServer struct {
	*render.Render
	UserManager auth.UserManager
	MCISettings mci.MCISettings
	plugins     []plugin.Plugin
}

const (
	APIServerLockTitle = "apiserver"
	PatchLockTitle     = "patches"
)

func New(mciSettings *mci.MCISettings, plugins []plugin.Plugin) (*APIServer, error) {
	crowdManager, err := auth.NewCrowdUserManager(
		mciSettings.Crowd.Username,
		mciSettings.Crowd.Password,
		mciSettings.Crowd.Urlroot,
	)
	if err != nil {
		return nil, err
	}

	return &APIServer{render.New(render.Options{}), crowdManager, *mciSettings, plugins}, nil
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
				mci.Logger.Logf(slogger.ERROR, "Error getting user: %v", err)
			} else {
				// Get the user's full details from the DB or create them if they don't exists
				dbUser, err := model.GetOrCreateUser(authedUser.Username(),
					authedUser.DisplayName(), authedUser.Email())
				if err != nil {
					mci.Logger.Logf(slogger.ERROR, "Error looking up user %v: %v", authedUser.Username(), err)
				} else {
					context.Set(r, apiUserKey, dbUser)
				}
			}
		} else if len(authData.APIKey) > 0 {
			dbUser, err := user.FindOne(user.ById(authData.Name))
			fmt.Println("checking for key", dbUser.APIKey, authData.APIKey)
			if dbUser != nil && err == nil {
				if dbUser.APIKey != authData.APIKey {
					http.Error(w, "Unauthorized - invalid API key", http.StatusUnauthorized)
					return
				}
				context.Set(r, apiUserKey, dbUser)
			} else {
				mci.Logger.Logf(slogger.ERROR, "Error getting user: %v", err)
			}
		}
		next(w, r)
	}
}

func MustHaveUser(r *http.Request) *user.DBUser {
	u := GetUser(r)
	if u == nil {
		panic("no user attached to request")
	}
	return u
}

func MustHaveTask(r *http.Request) *model.Task {
	t := GetTask(r)
	if t == nil {
		panic("no task attached to request")
	}
	return t
}

func GetListener(addr string) (net.Listener, error) {
	return net.Listen("tcp", addr)
}

func GetTLSListener(addr string, conf *tls.Config) (net.Listener, error) {
	l, err := net.Listen("tcp", addr)
	if err != nil {
		return nil, err
	}
	return tls.NewListener(l, conf), nil
}

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
			secret := r.Header.Get(mci.TaskSecretHeader)

			// Check the secret - if it doesn't match, write error back to the client
			if secret != task.Secret {
				mci.Logger.Logf(slogger.ERROR, "Wrong secret sent for task %v: Expected %v but got %v", taskId, task.Secret, secret)
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

func (as *APIServer) StartTask(w http.ResponseWriter, r *http.Request) {
	if !getGlobalLock(APIServerLockTitle) {
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(APIServerLockTitle)

	task := MustHaveTask(r)

	mci.Logger.Logf(slogger.INFO, "Marking task started: %v", task.Id)

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
	fmt.Println("ENDING TASK")
	finishTime := time.Now()
	taskEndResponse := &apimodels.TaskEndResponse{}
	if !getGlobalLock(APIServerLockTitle) {
		fmt.Println("coudln't get global lock!!!!!!!")
		as.LoggedError(w, r, http.StatusInternalServerError, ErrLockTimeout)
		return
	}
	defer releaseGlobalLock(APIServerLockTitle)

	task := MustHaveTask(r)

	taskEndRequest := &apimodels.TaskEndRequest{}
	if err := util.ReadJSONInto(r.Body, taskEndRequest); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// Check that finishing status is a valid constant
	if taskEndRequest.Status != mci.TaskSucceeded &&
		taskEndRequest.Status != mci.TaskFailed &&
		taskEndRequest.Status != mci.TaskUndispatched {

		msg := fmt.Errorf("Invalid end status '%v' for task %v", taskEndRequest.Status, task.Id)
		as.LoggedError(w, r, http.StatusBadRequest, msg)
		return
	}

	project, err := model.FindProject("", task.Project, as.MCISettings.ConfigDir)
	if err != nil {
		fmt.Println("couldn't find projecT!!!!!!!!!!!!!")
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	// mark task as finished
	err = task.MarkEnd(APIServerLockTitle, finishTime, taskEndRequest, project)
	if err != nil {
		message := fmt.Errorf("Error calling mark finish on task %v : %v", task.Id, err)
		fmt.Println("ERROR CALLING MARKEND")
		as.LoggedError(w, r, http.StatusInternalServerError, message)
		return
	}

	// if task was aborted, reset to inactive
	if taskEndRequest.Status == mci.TaskUndispatched {
		if err = model.SetTaskActivated(task.Id, "", false); err != nil {
			fmt.Println("ERRORDEATASAFA")
			message := fmt.Sprintf("Error deactivating task after abort: %v", err)
			mci.Logger.Logf(slogger.ERROR, message)
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
		mci.Logger.Logf(slogger.ERROR, "Error updating expected duration: %v",
			err)
	}

	// log the task as finished
	mci.Logger.Logf(slogger.INFO, "Successfully marked task %v as finished",
		task.Id)

	// construct and return the appropriate response for the agent
	as.taskFinished(w, task, finishTime)
}

func markHostRunningTaskFinished(h *host.Host, task *model.Task, newTaskId string) {
	// update the given host's running_task field accordingly
	if err := h.UpdateRunningTask(task.Id, newTaskId, time.Now()); err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error updating running task "+
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
		mci.Logger.Logf(slogger.ERROR, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host == nil {
		message := fmt.Sprintf("Error finding host running for task %v - set to %v: %v", task.Id,
			task.HostId, err)
		mci.Logger.Logf(slogger.ERROR, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host.Status == mci.HostDecommissioned || host.Status == mci.HostQuarantined {
		markHostRunningTaskFinished(host, task, "")
		message := fmt.Sprintf("Host %v - running %v - is in state '%v'. Agent will terminate",
			task.HostId, task.Id, host.Status)
		mci.Logger.Logf(slogger.INFO, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}

	// b. check if the agent needs to be rebuilt
	taskRunnerInstance := taskrunner.NewTaskRunner(&as.MCISettings)
	agentRevision, err := taskRunnerInstance.HostGateway.GetAgentRevision()
	if err != nil {
		markHostRunningTaskFinished(host, task, "")
		mci.Logger.Logf(slogger.ERROR, "failed to get agent revision: %v", err)
		taskEndResponse.Message = err.Error()
		as.WriteJSON(w, http.StatusInternalServerError, taskEndResponse)
		return
	}
	if host.AgentRevision != agentRevision {
		markHostRunningTaskFinished(host, task, "")
		message := fmt.Sprintf("Remote agent needs to be rebuilt")
		mci.Logger.Logf(slogger.INFO, message)
		taskEndResponse.Message = message
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}

	// c. fetch the task's distro queue to dispatch the next pending task
	nextTask, err := getNextDistroTask(task, host)
	if err != nil {
		markHostRunningTaskFinished(host, task, "")
		mci.Logger.Logf(slogger.ERROR, err.Error())
		taskEndResponse.Message = err.Error()
		as.WriteJSON(w, http.StatusOK, taskEndResponse)
		return
	}
	if nextTask == nil {
		markHostRunningTaskFinished(host, task, "")
		taskEndResponse.Message = "No next task on queue"
	} else {
		taskEndResponse.Message = "Proceed with next task"
		markHostRunningTaskFinished(host, task, nextTask.Id)
		loadTaskEndResponseInto(taskEndResponse, nextTask, &as.MCISettings)
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

// loadTaskEndResponseInto fetches the necessary field members needed by the
// agent to continue working on a new task
func loadTaskEndResponseInto(taskEndResponse *apimodels.TaskEndResponse,
	task *model.Task, mciSettings *mci.MCISettings) {
	taskEndResponse.RunNext = true
	taskEndResponse.TaskId = task.Id
	taskEndResponse.TaskSecret = task.Secret
	taskEndResponse.ConfigDir = mciSettings.ConfigDir
	taskEndResponse.WorkDir = mci.RemoteShell
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
	mci.Logger.Logf(slogger.INFO, "Attaching files to task %v", task.Id)

	entry := &artifact.Entry{
		TaskId:          task.Id,
		TaskDisplayName: task.DisplayName,
		BuildId:         task.BuildId,
	}

	err := util.ReadJSONInto(r.Body, &entry.Files)
	if err != nil {
		message := fmt.Sprintf("Error reading file definitions for task  %v: %v", task.Id, err)
		mci.Logger.Errorf(slogger.ERROR, message)
		as.WriteJSON(w, http.StatusBadRequest, message)
		return
	}
	fmt.Printf("file entry is %#v\n", entry)

	if err := entry.Upsert(); err != nil {
		message := fmt.Sprintf("Error updating artifact file info for task %v: %v", task.Id, err)
		mci.Logger.Errorf(slogger.ERROR, message)
		as.WriteJSON(w, http.StatusInternalServerError, message)
		return
	}
	as.WriteJSON(w, http.StatusOK, fmt.Sprintf("Artifact files for task %v successfully attached", task.Id))
}

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

func (as *APIServer) FetchTask(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)
	as.WriteJSON(w, http.StatusOK, task)
}

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

func (as *APIServer) Heartbeat(w http.ResponseWriter, r *http.Request) {
	task := MustHaveTask(r)

	heartbeatResponse := apimodels.HeartbeatResponse{}
	if task.Aborted {
		//mci.Logger.Logf(slogger.INFO, "Sending abort signal for task %v", task.Id)
		heartbeatResponse.Abort = true
	}

	if err := task.UpdateHeartbeat(); err != nil {
		//mci.Logger.Errorf(slogger.ERROR, "Error updating heartbeat for task %v : %v", task.Id, err)
	}
	as.WriteJSON(w, http.StatusOK, heartbeatResponse)
}

func home(w http.ResponseWriter, r *http.Request) {
	fmt.Fprintf(w, "Welcome to the API server's home :)\n")
}

/*
func attachUser(userM auth.UserManager, r *http.Request) web.HTTPResponse {
	// Note: at this point the "token" is actually a json object in string form,
	// containing both the username and token.
	token := r.FormValue("id_token")
	authData := struct {
		Name  string `json:"auth_user"`
		Token string `json:"auth_token"`
	}{}
	err := json.Unmarshal([]byte(token), &authData)
	if err != nil {
		return web.JSONResponse{Data: map[string]string{"message": err.Error()},
			StatusCode: http.StatusInternalServerError}
	}

	user, err := userM.GetUserByToken(authData.Token)
	if err != nil {
		return web.JSONResponse{Data: map[string]string{"message": err.Error()},
			StatusCode: http.StatusUnauthorized}
	}

	// Get the user's full details from the DB or create them if they don't exists
	dbUser, err := model.GetOrCreateUser(user.Username(), user.DisplayName(), user.Email())
	if err != nil {
		mci.Logger.Logf(slogger.INFO, "Error looking up user: %v", err)
	} else {
		context.Set(r, userKey, dbUser)
	}

	if user != nil {
		context.Set(r, "isAdmin", true)
		context.Set(r, userKey, dbUser)
	}

	return web.ChainHttpResponse{}
}
*/

func GetUser(r *http.Request) *user.DBUser {
	if rv := context.Get(r, apiUserKey); rv != nil {
		return rv.(*user.DBUser)
	}
	return nil
}

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
		mci.Logger.Errorf(slogger.ERROR, err.Error())
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	// if the host failed
	setupSuccess := mux.Vars(r)["status"]
	if setupSuccess == mci.HostStatusFailed {
		mci.Logger.Logf(slogger.INFO, "Initializing host %v failed", hostObj.Id)
		// send notification to the MCI team about this provisioning failure
		subject := fmt.Sprintf("%v MCI provisioning failure on %v", notify.ProvisionFailurePreface, hostObj.Distro)

		hostLink := fmt.Sprintf("%v/host/%v", as.MCISettings.Ui.Url, hostObj.Id)
		message := fmt.Sprintf("Provisioning failed on %v host -- %v (%v). %v",
			hostObj.Distro, hostObj.Id, hostObj.Host, hostLink)
		if err = notify.NotifyAdmins(subject, message, &as.MCISettings); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error sending email: %v", err)
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

	cloudManager, err := providers.GetCloudManager(hostObj.Provider, &as.MCISettings)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		subject := fmt.Sprintf("%v MCI provisioning completion failure on %v",
			notify.ProvisionFailurePreface, hostObj.Distro)
		message := fmt.Sprintf("Failed to get cloud manager for host %v with provider %v: %v",
			hostObj.Id, hostObj.Provider, err)
		if err = notify.NotifyAdmins(subject, message, &as.MCISettings); err != nil {
			mci.Logger.Errorf(slogger.ERROR, "Error sending email: %v", err)
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

	mci.Logger.Logf(slogger.INFO, "Successfully marked host “%v” with dns “%v” as provisioned", hostObj.Id, dns)
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
	mci.Logger.Logf(slogger.DEBUG, "Attempting to acquire global lock for %v", requester)
	lockAcquired, err := db.WaitTillAcquireGlobalLock(requester, db.LockTimeout)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error acquiring global lock: %v", err)
		return false
	}
	if !lockAcquired {
		mci.Logger.Errorf(slogger.ERROR, "Cannot proceed with %v api method because the global lock could not be taken", requester)
		return false
	}

	mci.Logger.Logf(slogger.DEBUG, "Acquired global lock for %v", requester)
	return true
}

// helper function for releasing the global lock
func releaseGlobalLock(requester string) {
	mci.Logger.Logf(slogger.DEBUG, "Attempting to release global lock from %v", requester)

	err := db.ReleaseGlobalLock(requester)
	if err != nil {
		mci.Logger.Errorf(slogger.ERROR, "Error releasing global lock from %v - this is really bad: %v", requester, err)
	}
	mci.Logger.Logf(slogger.DEBUG, "Released global lock from %v", requester)
}

// LoggedError logs the given error and writes an HTTP response with its details formatted
// as JSON if the request headers indicate that it's acceptable (or plaintext otherwise).
func (as *APIServer) LoggedError(w http.ResponseWriter, r *http.Request, code int, err error) {
	mci.Logger.Logf(slogger.ERROR, fmt.Sprintf("%v %v %v", r.Method, r.URL, err.Error()))
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

func (as *APIServer) Handler() (http.Handler, error) {
	root := mux.NewRouter()
	r := root.PathPrefix("/api/2/").Subrouter()
	r.HandleFunc("/", home)

	apiRootOld := root.PathPrefix("/api/").Subrouter()

	// Project lookup and validation routes
	apiRootOld.HandleFunc("/ref/{identifier:[\\w_\\-\\@.]+}", as.fetchProjectRef)
	apiRootOld.HandleFunc("/validate", as.validateProjectConfig).Methods("POST")

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
	taskRouter.HandleFunc("/end", as.checkTask(true, as.EndTask)).Methods("POST")
	taskRouter.HandleFunc("/log", as.checkTask(true, as.AppendTaskLog)).Methods("POST")
	taskRouter.HandleFunc("/heartbeat", as.checkTask(true, as.Heartbeat)).Methods("POST")
	taskRouter.HandleFunc("/results", as.checkTask(true, as.AttachResults)).Methods("POST")
	taskRouter.HandleFunc("/test_logs", as.checkTask(true, as.AttachTestLog)).Methods("POST")
	taskRouter.HandleFunc("/patch", as.checkTask(true, as.GetPatch)).Methods("GET")
	taskRouter.HandleFunc("/distro", as.checkTask(false, as.GetDistro)).Methods("GET") // nosecret check
	taskRouter.HandleFunc("/", as.checkTask(true, as.FetchTask)).Methods("GET")
	taskRouter.HandleFunc("/version", as.checkTask(false, as.GetVersion)).Methods("GET")
	taskRouter.HandleFunc("/fetch_vars", as.checkTask(true, as.FetchProjectVars)).Methods("GET")
	taskRouter.HandleFunc("/files", as.checkTask(false, as.AttachFiles)).Methods("POST")

	// Install plugin routes
	for _, pl := range as.plugins {
		if pl == nil {
			continue
		}
		pluginSettings := as.MCISettings.Plugins[pl.Name()]
		err := pl.Configure(pluginSettings)
		if err != nil {
			return nil, fmt.Errorf("Failed to configure plugin %v: %v", pl.Name(), err)
		}
		handler := pl.GetAPIHandler()
		if handler == nil {
			mci.Logger.Logf(slogger.WARN, "no API handlers to install for %v", pl.Name())
			continue
		}
		mci.Logger.Logf(slogger.WARN, "Installing API plugin handlers for %v", pl.Name())
		util.MountHandler(taskRouter, fmt.Sprintf("/%v/", pl.Name()), as.checkTask(false, handler.ServeHTTP))
	}

	n := negroni.New()
	n.Use(negroni.NewLogger())
	n.Use(negroni.HandlerFunc(UserMiddleware(as.UserManager)))
	n.UseHandler(root)
	return n, nil
}
