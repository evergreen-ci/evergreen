package apiserver

import (
	"fmt"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"net/http"
	"strconv"
	"time"
)

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
		timeoutResponse.Status = "ERROR"
	} else {
		timeoutResponse.Status = "SUCCESS"
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
	status := "SUCCESS"

	if distro != "" {
		taskQueue, err := model.FindTaskQueueForDistro(distro)
		if err != nil {
			as.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if taskQueue.Length() >= size {
			distroNames[distro] = taskQueue.Length()
			status = "ERROR"
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
				status = "ERROR"
			}
		}
	}
	growthResponse := struct {
		Status  string
		Distros map[string]int
	}{status, distroNames}

	as.WriteJSON(w, http.StatusOK, growthResponse)
}
