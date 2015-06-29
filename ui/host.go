package ui

import (
	"fmt"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/gorilla/mux"
	"net/http"
	"strconv"
	"strings"
)

var (
	validUpdateToStatuses = []string{
		evergreen.HostRunning,
		evergreen.HostQuarantined,
		evergreen.HostDecommissioned,
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
}

func (uis *UIServer) hostPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

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

	events, err := event.Find(event.MostRecentHostEvents(id, 50))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	flashes := PopFlashes(uis.CookieStore, r, w)
	uis.WriteHTML(w, http.StatusOK, struct {
		Flashes     []interface{}
		Events      []event.Event
		Host        *host.Host
		User        *user.DBUser
		ProjectData projectContext
	}{flashes, events, h, GetUser(r), projCtx},
		"base", "host.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) hostsPage(w http.ResponseWriter, r *http.Request) {
	projCtx := MustHaveProjectContext(r)

	includeSpawnedHosts, _ := strconv.ParseBool(r.FormValue(IncludeSpawnedHosts))
	hosts, err := getHostsData(includeSpawnedHosts)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	flashes := PopFlashes(uis.CookieStore, r, w)
	uis.WriteHTML(w, http.StatusOK, struct {
		Flashes             []interface{}
		Hosts               *hostsData
		IncludeSpawnedHosts bool
		User                *user.DBUser
		ProjectData         projectContext
	}{flashes, hosts, includeSpawnedHosts, GetUser(r), projCtx},
		"base", "hosts.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) modifyHost(w http.ResponseWriter, r *http.Request) {
	_ = MustHaveUser(r)

	vars := mux.Vars(r)
	id := vars["host_id"]

	host, err := host.FindOne(host.ById(id))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if host == nil {
		http.Error(w, "Host not found", http.StatusNotFound)
		return
	}

	opts := &uiParams{}
	err = util.ReadJSONInto(r.Body, opts)
	if err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	// determine what action needs to be taken
	switch opts.Action {
	case "updateStatus":
		currentStatus := host.Status
		newStatus := opts.Status
		if !util.SliceContains(validUpdateToStatuses, newStatus) {
			http.Error(w, fmt.Sprintf("'%v' is not a valid status", newStatus), http.StatusBadRequest)
			return
		}
		err := host.SetStatus(newStatus)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error updating host: %v", err))
			return
		}
		msg := NewSuccessFlash(fmt.Sprintf("Host status successfully updated from '%v' to '%v'", currentStatus, host.Status))
		PushFlash(uis.CookieStore, r, w, msg)
		uis.WriteJSON(w, http.StatusOK, "Successfully updated host status")
	default:
		uis.WriteJSON(w, http.StatusBadRequest, fmt.Sprintf("Unrecognized action: %v", opts.Action))
	}
}

func (uis *UIServer) modifyHosts(w http.ResponseWriter, r *http.Request) {
	_ = MustHaveUser(r)

	opts := &uiParams{}
	err := util.ReadJSONInto(r.Body, opts)
	if err != nil {
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
		uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error finding hosts: %v", err))
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
		if !util.SliceContains(validUpdateToStatuses, newStatus) {
			http.Error(w, fmt.Sprintf("Invalid status: %v", opts.Status), http.StatusBadRequest)
			return
		}
		numHostsUpdated := 0

		for _, host := range hosts {
			err := host.SetStatus(newStatus)
			if err != nil {
				uis.LoggedError(w, r, http.StatusInternalServerError, fmt.Errorf("Error updating host %v", err))
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
