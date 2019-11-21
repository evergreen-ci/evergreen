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
	"github.com/evergreen-ci/gimlet/rolemanager"
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
	u := MustHaveUser(r)

	id := gimlet.GetVars(r)["host_id"]

	h, err := host.FindOneByIdOrTag(id)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if h == nil {
		http.Error(w, "Host not found", http.StatusNotFound)
		return
	}

	opts := gimlet.PermissionOpts{Resource: h.Distro.Id, ResourceType: evergreen.DistroResourceType}
	permissions, err := rolemanager.HighestPermissionsForRoles(u.Roles(), evergreen.GetEnvironment().RoleManager(), opts)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
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
		if runningTask == nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("task %s running on host not found", h.RunningTask))
			return
		}
	}

	var containers []host.Host
	if h.HasContainers {
		containers, err = h.GetContainers()
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Events      []event.EventLogEntry
		Host        *host.Host
		Permissions gimlet.Permissions
		RunningTask *task.Task
		Containers  []host.Host
		ViewData
	}{events, h, permissions, runningTask, containers, uis.GetCommonViewData(w, r, false, true)},
		"base", "host.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) hostsPage(w http.ResponseWriter, r *http.Request) {
	u := MustHaveUser(r)
	permissions, err := rolemanager.HighestPermissionsForRolesAndResourceType(
		u.Roles(),
		evergreen.DistroResourceType,
		evergreen.GetEnvironment().RoleManager(),
	)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	includeSpawnedHosts, _ := strconv.ParseBool(r.FormValue(IncludeSpawnedHosts))
	hosts, err := getHostsData(includeSpawnedHosts)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	permittedHosts := &hostsData{}
	for i := range hosts.Hosts {
		resourcePermissions, ok := permissions[hosts.Hosts[i].Host.Distro.Id]
		if !evergreen.AclCheckingIsEnabled || (ok && resourcePermissions[evergreen.PermissionHosts] > 0) {
			permittedHosts.Hosts = append(permittedHosts.Hosts, hosts.Hosts[i])
		}
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Hosts               *hostsData
		IncludeSpawnedHosts bool
		ViewData
	}{permittedHosts, includeSpawnedHosts, uis.GetCommonViewData(w, r, false, true)},
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
	modifyResult, err := modifyHostStatus(queue, h, opts, u)

	if err != nil {
		gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(err))
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
		for _, h := range hosts {
			_, err := modifyHostStatus(evergreen.GetEnvironment().RemoteQueue(), &h, opts, user)
			if err != nil {
				gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(err))
				return
			}
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("%d hosts will be updated to '%s'", len(hosts), opts.Status)))
		return
	default:
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Unrecognized action: %v", opts.Action))
		return
	}
}
