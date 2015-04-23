package apiserver

import (
	"10gen.com/mci/apimodels"
	"10gen.com/mci/model"
	"fmt"
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
