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
	"github.com/evergreen-ci/gimlet"
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
	IncludeSpawnedHosts = "includeSpawnedHosts"
)

type uiParams struct {
	Action string `json:"action"`

	// the host ids for which an action applies
	HostIds []string `json:"host_ids"`

	// for the update status option
	Status string `json:"status"`

	// additional notes that will be added to the event log_path
	Notes string `json:"notes"`
}

func (uis *UIServer) hostPage(w http.ResponseWriter, r *http.Request) {
	id := gimlet.GetVars(r)["host_id"]

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

	uis.render.WriteResponse(w, http.StatusOK, struct {
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

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Hosts               *hostsData
		IncludeSpawnedHosts bool
		ViewData
	}{hosts, includeSpawnedHosts, uis.GetCommonViewData(w, r, false, true)},
		"base", "hosts.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyHost(w http.ResponseWriter, r *http.Request) {
	env := evergreen.GetEnvironment()
	queue := env.RemoteQueue()
	u := MustHaveUser(r)
	id := gimlet.GetVars(r)["host_id"]

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
	modifyResult, restErr := modifyHostStatus(queue, h, opts, u)

	if restErr != nil {
		uis.LoggedError(w, r, restErr.StatusCode, restErr)
		return
	}

	var msg flashMessage
	switch modifyResult {
	case fmt.Sprintf(HostTerminationQueueingSuccess, h.Id):
		msg = NewSuccessFlash(fmt.Sprintf(HostTerminationQueueingSuccess, h.Id))
	case fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status):
		msg = NewSuccessFlash(fmt.Sprintf(HostStatusUpdateSuccess, currentStatus, h.Status))
	}
	PushFlash(uis.CookieStore, r, w, msg)
	gimlet.WriteJSON(w, HostStatusWriteConfirm)
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
			err := host.SetStatus(newStatus, user.Id, opts.Notes)
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
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Unrecognized action: %v", opts.Action))
		return
	}
}
