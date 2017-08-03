package service

import (
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/mongodb/grip/message"
)

const (
	apiStatusSuccess = "SUCCESS"
	apiStatusError   = "ERROR"
)

// taskAssignmentResp holds the status, errors and four separate lists of task and host ids
// this is so that when addressing inconsistencies we can differentiate between the states of
// the tasks and hosts.
// Status is either SUCCESS or ERROR.
// Errors is a list of all the errors that exist.
// TaskIds are a list of the tasks that have errors associated with them.
// TaskHostIds are a list of the hosts that exist that are said to be running on tasks with errors.
// HostIds are a list of hosts that have errors associated with them.
// HostRunningTasks are list of the tasks that are said to be running on inconsistent hosts.
type taskAssignmentResp struct {
	Status           string   `json:"status"`
	Errors           []string `json:"errors"`
	TaskIds          []string `json:"tasks"`
	TaskHostIds      []string `json:"task_host_ids"`
	HostIds          []string `json:"hosts"`
	HostRunningTasks []string `json:"host_running_tasks"`
}

type stuckHostResp struct {
	Status  string   `json:"status"`
	Errors  []string `json:"errors"`
	TaskIds []string `json:"tasks"`
	HostIds []string `json:"hosts"`
}

// consistentTaskAssignment returns any disparities between tasks' and hosts's views
// of their mapping between each other. JSON responses take the form of
//  {status: “ERROR/SUCCESS”, errors:[error strings], tasks:[ids], hosts:[ids]}
func (as *APIServer) consistentTaskAssignment(w http.ResponseWriter, r *http.Request) {
	disparities, err := model.AuditHostTaskConsistency()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	resp := taskAssignmentResp{Status: apiStatusSuccess}
	if len(disparities) > 0 {
		resp.Status = apiStatusError
		for _, d := range disparities {
			resp.Errors = append(resp.Errors, d.Error())
			if d.Task != "" {
				resp.TaskIds = append(resp.TaskIds, d.Task)
			}
			if d.HostTaskCache != "" {
				resp.HostRunningTasks = append(resp.HostRunningTasks, d.HostTaskCache)
			}
			if d.Host != "" {
				resp.HostIds = append(resp.HostIds, d.Host)
			}
			if d.TaskHostCache != "" {
				resp.TaskHostIds = append(resp.TaskHostIds, d.TaskHostCache)
			}
		}
		// dedupe id slices before returning, for simplicity
		resp.TaskIds = util.UniqueStrings(resp.TaskIds)
		resp.HostIds = util.UniqueStrings(resp.HostIds)
		resp.HostRunningTasks = util.UniqueStrings(resp.HostRunningTasks)
		resp.TaskHostIds = util.UniqueStrings(resp.TaskHostIds)
	}
	as.WriteJSON(w, http.StatusOK, resp)
}

// Returns a list of all processes with runtime entries, i.e. all processes being tracked.
func (as *APIServer) listRuntimes(w http.ResponseWriter, r *http.Request) {
	runtimes, err := model.FindEveryProcessRuntime()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	as.WriteJSON(w, http.StatusOK, runtimes)
}

// Given a timeout cutoff in seconds, returns a JSON response with a SUCCESS flag
// if all processes have run within the cutoff, or ERROR and a list of late processes
// if one or more processes last finished before the timeout cutoff. DevOps tools
// should be able to do a regex for "SUCCESS" or "ERROR" to check for timeouts.
func (as *APIServer) lateRuntimes(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	timeAsString := vars["seconds"]
	if len(timeAsString) == 0 {
		http.Error(w, "Must supply an amount in seconds with timeout query", http.StatusBadRequest)
		return
	}
	timeInSeconds, err := strconv.Atoi(timeAsString)
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid time param: %v", timeAsString), http.StatusBadRequest)
		return
	}
	cutoff := time.Now().Add(time.Duration(-1*timeInSeconds) * time.Second)
	runtimes, err := model.FindAllLateProcessRuntimes(cutoff)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	timeoutResponse := apimodels.ProcessTimeoutResponse{}
	timeoutResponse.LateProcesses = &runtimes
	if len(runtimes) > 0 {
		timeoutResponse.Status = apiStatusError
	} else {
		timeoutResponse.Status = apiStatusSuccess
	}
	as.WriteJSON(w, http.StatusOK, timeoutResponse)
}

func (as *APIServer) getTaskQueueSizes(w http.ResponseWriter, r *http.Request) {

	distroNames := make(map[string]int)
	taskQueues, err := model.FindAllTaskQueues()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	for _, queue := range taskQueues {
		distroNames[queue.Distro] = queue.Length()
	}
	taskQueueResponse := struct {
		Distros map[string]int
	}{distroNames}

	as.WriteJSON(w, http.StatusOK, taskQueueResponse)
}

// getTaskQueueSize returns a JSON response with a SUCCESS flag if all task queues have a size
// less than the size indicated. If a distro's task queue has size greater than or equal to the size given,
// there will be an ERROR flag along with a map of the distro name to the size of the task queue.
// If the size is 0 or the size is not sent, the JSON response will be SUCCESS with a list of all distros and their
// task queue sizes.
func (as *APIServer) checkTaskQueueSize(w http.ResponseWriter, r *http.Request) {
	size, err := util.GetIntValue(r, "size", 0)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	distro := r.FormValue("distro")

	distroNames := make(map[string]int)
	status := apiStatusSuccess

	if distro != "" {
		taskQueue, err := model.FindTaskQueueForDistro(distro)
		if err != nil {
			as.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if taskQueue.Length() >= size {
			distroNames[distro] = taskQueue.Length()
			status = apiStatusError
		}
	} else {
		taskQueues, err := model.FindAllTaskQueues()
		if err != nil {
			as.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		for _, queue := range taskQueues {
			if queue.Length() >= size {
				distroNames[queue.Distro] = queue.Length()
				status = apiStatusError
			}
		}
	}
	growthResponse := struct {
		Status  string
		Distros map[string]int
	}{status, distroNames}

	as.WriteJSON(w, http.StatusOK, growthResponse)
}

// getStuckHosts returns hosts that have tasks running that are completed
func (as *APIServer) getStuckHosts(w http.ResponseWriter, r *http.Request) {
	stuckHosts, err := model.CheckStuckHosts()
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	errors := []string{}
	hosts := []string{}
	tasks := []string{}
	for _, disparity := range stuckHosts {
		errors = append(errors, disparity.Error())
		hosts = append(hosts, disparity.Host)
		tasks = append(tasks, disparity.RunningTask)
	}
	status := apiStatusSuccess
	if len(stuckHosts) > 0 {
		status = apiStatusError
	}

	as.WriteJSON(w, http.StatusOK, stuckHostResp{
		Status:  status,
		Errors:  errors,
		HostIds: hosts,
		TaskIds: tasks,
	})
}

func (as *APIServer) serviceStatusWithAuth(w http.ResponseWriter, r *http.Request) {
	out := struct {
		BuildId    string              `json:"build_revision"`
		SystemInfo *message.SystemInfo `json:"sys_info"`
		Pid        int                 `json:"pid"`
	}{
		BuildId:    evergreen.BuildRevision,
		SystemInfo: message.CollectSystemInfo().(*message.SystemInfo),
		Pid:        os.Getpid(),
	}

	as.WriteJSON(w, http.StatusOK, &out)
}

func (as *APIServer) serviceStatusSimple(w http.ResponseWriter, r *http.Request) {
	out := struct {
		BuildId string `json:"build_revision"`
	}{
		BuildId: evergreen.BuildRevision,
	}

	as.WriteJSON(w, http.StatusOK, &out)
}

func (as *APIServer) recentTaskStatuses(w http.ResponseWriter, r *http.Request) {
	tasks, err := task.GetRecentTasks(30 * time.Minute)
	if err != nil {
		as.LoggedError(w, r, http.StatusInternalServerError, err)
	}

	as.WriteJSON(w, http.StatusOK, task.GetTaskResultCounts(tasks))
}
