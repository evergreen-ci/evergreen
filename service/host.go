package service

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"github.com/pkg/errors"
)

var (
	validUpdateToStatuses = []string{
		evergreen.HostRunning,
		evergreen.HostQuarantined,
		evergreen.HostDecommissioned,
		evergreen.HostTerminated,
	}
)

const (
	IncludeSpawnedHosts            = "includeSpawnedHosts"
	InvalidStatusError             = "'%v' is not a valid status"
	DecommissionStaticHostError    = "Cannot decommission static hosts"
	HostTerminationQueueingError   = "Error starting background job for host termination"
	HostUpdateError                = "Error updating host"
	HostTerminationQueueingSuccess = "Host %v successfully queued for termination"
	HostStatusUpdateSuccess        = "Host status successfully updated from '%v' to '%v'"
	HostStatusWriteConfirm         = "Successfully updated host status"
	UnrecognizedAction             = "Unrecognized action: %v"
)

type uiParams struct {
	Action string `json:"action"`

	// the host ids for which an action applies
	HostIds []string `json:"host_ids"`

	// for the update status option
	Status string `json:"status"`
}

func (uis *UIServer) hostPage(w http.ResponseWriter, r *http.Request) {
	vars := mux.Vars(r)
	id := vars["host_id"]

	h, err := host.FindOne(host.ById(id))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if h == nil {
		http.Error(w, "Host not found", http.StatusNotFound)
		return
	}

	events, err := event.Find(event.AllLogCollection, event.MostRecentHostEvents(id, 50))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}
	runningTask := &task.Task{}
	if h.RunningTask != "" {
		runningTask, err = task.FindOne(task.ById(h.RunningTask))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		Events      []event.EventLogEntry
		Host        *host.Host
		RunningTask *task.Task
		ViewData
	}{events, h, runningTask, uis.GetCommonViewData(w, r, false, true)},
		"base", "host.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) hostsPage(w http.ResponseWriter, r *http.Request) {
	includeSpawnedHosts, _ := strconv.ParseBool(r.FormValue(IncludeSpawnedHosts))
	hosts, err := getHostsData(includeSpawnedHosts)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	uis.WriteHTML(w, http.StatusOK, struct {
		Hosts               *hostsData
		IncludeSpawnedHosts bool
		ViewData
	}{hosts, includeSpawnedHosts, uis.GetCommonViewData(w, r, false, true)},
		"base", "hosts.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyHost(w http.ResponseWriter, r *http.Request) {
	u := MustHaveUser(r)

	vars := mux.Vars(r)
	id := vars["host_id"]

	h, err := host.FindOne(host.ById(id))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if h == nil {
		http.Error(w, "Host not found", http.StatusNotFound)
		return
	}

	opts := &uiParams{}

	if err = util.ReadJSONInto(util.NewRequestReader(r), opts); err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	currentStatus := h.Status
	modifyResult, err := modifyHostStatus(h, opts, u)

	if err != nil {
		switch err.Error() {
		case fmt.Sprintf(UnrecognizedAction, opts.Action):
			uis.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf(UnrecognizedAction, opts.Action))
			return
		case fmt.Sprintf(InvalidStatusError, opts.Status):
			http.Error(w, fmt.Sprintf(InvalidStatusError, opts.Status), http.StatusBadRequest)
			return
		case DecommissionStaticHostError:
			http.Error(w, DecommissionStaticHostError, http.StatusBadRequest)
			return
		case HostTerminationQueueingError:
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.New(HostTerminationQueueingError))
			return
		default:
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	var msg flashMessage
	switch modifyResult {
	case fmt.Sprintf(HostTerminationQueueingSuccess, h.Id):
		msg = NewSuccessFlash(fmt.Sprintf(HostTerminationQueueingSuccess, h.Id))
	case fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status):
		msg = NewSuccessFlash(fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status))
	}
	PushFlash(uis.CookieStore, r, w, msg)
	uis.WriteJSON(w, http.StatusOK, HostStatusWriteConfirm)
}

func (uis *UIServer) modifyHosts(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)

	opts := &uiParams{}

	if err := util.ReadJSONInto(util.NewRequestReader(r), opts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hostIds := opts.HostIds
	if len(hostIds) == 1 && strings.TrimSpace(hostIds[0]) == "" {
		http.Error(w, "No host ID's found in request", http.StatusBadRequest)
		return
	}

	// fetch all relevant hosts
	hosts, err := host.Find(host.ByIds(hostIds))

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error finding hosts"))
		return
	}
	if len(hosts) == 0 {
		http.Error(w, "No matching hosts found.", http.StatusBadRequest)
		return
	}

	// determine what action needs to be taken
	switch opts.Action {
	case "updateStatus":
		newStatus := opts.Status
		if !util.StringSliceContains(validUpdateToStatuses, newStatus) {
			http.Error(w, fmt.Sprintf("Invalid status: %v", opts.Status), http.StatusBadRequest)
			return
		}
		numHostsUpdated := 0

		for _, host := range hosts {
			err := host.SetStatus(newStatus, user.Id, "")
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error updating host"))
				return
			}
			numHostsUpdated += 1
		}
		msg := NewSuccessFlash(fmt.Sprintf("%v host(s) status successfully updated to '%v'",
			numHostsUpdated, newStatus))
		PushFlash(uis.CookieStore, r, w, msg)
		return
	default:
		http.Error(w, fmt.Sprintf("Unrecognized action: %v", opts.Action), http.StatusBadRequest)
		return
	}
}
