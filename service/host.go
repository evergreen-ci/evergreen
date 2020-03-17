package service

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/mongodb/grip"
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

	events, err := event.Find(event.AllLogCollection, event.MostRecentHostEvents(h.Id, h.Tag, 50))
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
		if ok && resourcePermissions[evergreen.PermissionHosts] > 0 {
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

	switch opts.Action {
	case "updateStatus":
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
	case "restartJasper":
		if err = h.SetNeedsJasperRestart(u.Username()); err != nil {
			gimlet.WriteResponse(w, gimlet.MakeTextInternalErrorResponder(err))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(HostRestartJasperConfirm))
		gimlet.WriteJSON(w, HostRestartJasperConfirm)
	default:
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Unrecognized action: %v", opts.Action))
	}
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
		var hostsUpdated int
		var permissions map[string]gimlet.Permissions
		env := evergreen.GetEnvironment()
		rm := env.RoleManager()
		rq := env.RemoteQueue()
		permissions, err = rolemanager.HighestPermissionsForRolesAndResourceType(user.Roles(), evergreen.DistroResourceType, rm)
		if err != nil {
			http.Error(w, "Unable to get permissions", http.StatusInternalServerError)
			return
		}
		hostsUpdated, err := modifyHostsWithPermissions(hosts, permissions, func(h *host.Host) error {
			_, updateErr := modifyHostStatus(rq, h, opts, user)
			return updateErr
		})
		if err != nil {
			gimlet.WriteResponse(w, gimlet.MakeTextInternalErrorResponder(errors.Wrap(err, "error updating status on selected hosts")))
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("%d hosts will be updated to '%s'", hostsUpdated, opts.Status)))
	case "restartJasper":
		var hostsUpdated int
		var permissions map[string]gimlet.Permissions
		rm := evergreen.GetEnvironment().RoleManager()
		permissions, err = rolemanager.HighestPermissionsForRolesAndResourceType(user.Roles(), evergreen.DistroResourceType, rm)
		if err != nil {
			gimlet.WriteResponse(w, gimlet.MakeTextInternalErrorResponder(errors.Wrap(err, "unable to get user permissions")))
			return
		}
		hostsUpdated, err = modifyHostsWithPermissions(hosts, permissions, func(h *host.Host) error {
			if modifyErr := h.SetNeedsJasperRestart(user.Username()); modifyErr != nil {
				return modifyErr
			}
			return nil
		})
		if err != nil {
			gimlet.WriteResponse(w, gimlet.MakeTextInternalErrorResponder(errors.Wrap(err, "error marking selected hosts as needing Jasper service restarted")))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("%d hosts marked as needing Jasper service restarted", hostsUpdated)))
	default:
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Unrecognized action: %v", opts.Action))
		return
	}
}

// modifyHostsWithPermissions performs an update on each of the given hosts
// for which the permissions allow updates on that host.
func modifyHostsWithPermissions(hosts []host.Host, perm map[string]gimlet.Permissions, modifyHost func(h *host.Host) error) (updated int, err error) {
	catcher := grip.NewBasicCatcher()
	for _, h := range hosts {
		if perm[h.Distro.Id][evergreen.PermissionHosts] < evergreen.HostsEdit.Value {
			continue
		}
		if err := modifyHost(&h); err != nil {
			catcher.Wrapf(err, "could not modify host '%s'", h.Id)
			continue
		}
		updated++
	}
	return updated, catcher.Resolve()
}

func (uis *UIServer) getHostDNS(r *http.Request) ([]string, error) {
	hostID := gimlet.GetVars(r)["host_id"]
	h, err := uis.getHostFromCache(hostID)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get host '%s'", hostID)
	}
	if h == nil {
		return nil, errors.Wrapf(err, "host '%s' does not exist", hostID)
	}

	return []string{fmt.Sprintf("%s:%d", h.dnsName, evergreen.VSCodePort)}, nil
}

func (uis *UIServer) getHostFromCache(hostID string) (*hostCacheItem, error) {
	h, ok := uis.hostCache[hostID]
	if !ok || time.Since(h.inserted) > hostCacheTTL {
		hDb, err := host.FindOneId(hostID)
		if err != nil {
			return nil, errors.Wrapf(err, "can't get host id '%s'", hostID)
		}
		if hDb == nil {
			return nil, nil
		}

		h = hostCacheItem{dnsName: hDb.Host, owner: hDb.StartedBy, isVirtualWorkstation: hDb.IsVirtualWorkstation, isRunning: hDb.Status == evergreen.HostRunning, inserted: time.Now()}
		uis.hostCache[hostID] = h
	}

	return &h, nil
}
