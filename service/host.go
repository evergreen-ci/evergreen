package service

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/api"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/gimlet/rolemanager"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
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

func (uis *UIServer) modifyHost(w http.ResponseWriter, r *http.Request) {
	env := uis.env
	u := MustHaveUser(r)
	id := gimlet.GetVars(r)["host_id"]

	h, err := host.FindOne(r.Context(), host.ById(id))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, err)
		return
	}

	if h == nil {
		http.Error(w, "Host not found", http.StatusNotFound)
		return
	}

	opts := &uiParams{}

	if err = utility.ReadJSON(utility.NewRequestReader(r), opts); err != nil {
		uis.LoggedError(w, r, http.StatusBadRequest, err)
		return
	}

	switch opts.Action {
	case "updateStatus":
		var (
			msg        string
			statusCode int
		)
		msg, statusCode, err = api.ModifyHostStatus(r.Context(), env, h, opts.Status, opts.Notes, u)
		if err != nil {
			gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(gimlet.ErrorResponse{
				StatusCode: statusCode,
				Message:    msg,
			}))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(msg))
		gimlet.WriteJSON(w, api.HostStatusWriteConfirm)
	case "restartJasper":
		var statusCode int
		statusCode, err = api.GetRestartJasperCallback(r.Context(), env, u.Username())(h)
		if err != nil {
			gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(gimlet.ErrorResponse{
				StatusCode: statusCode,
				Message:    err.Error(),
			}))
			return
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(api.HostRestartJasperConfirm))
		gimlet.WriteJSON(w, api.HostRestartJasperConfirm)
	case "reprovisionToNew":
		var statusCode int
		statusCode, err = api.GetReprovisionToNewCallback(r.Context(), env, u.Username())(h)
		if err != nil {
			gimlet.WriteResponse(w, gimlet.MakeTextErrorResponder(gimlet.ErrorResponse{
				StatusCode: statusCode,
				Message:    err.Error(),
			}))
			return
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(api.HostReprovisionConfirm))
		gimlet.WriteJSON(w, api.HostReprovisionConfirm)
	default:
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Unrecognized action: %v", opts.Action))
	}
}

func (uis *UIServer) modifyHosts(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)

	opts := &uiParams{}

	if err := utility.ReadJSON(utility.NewRequestReader(r), opts); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(r.Context(), user, opts.HostIds)
	if err != nil {
		http.Error(w, err.Error(), httpStatus)
		return
	}

	env := uis.env

	// determine what action needs to be taken
	switch opts.Action {
	case "updateStatus":
		hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetUpdateHostStatusCallback(r.Context(), env, opts.Status, opts.Notes, user))
		if err != nil {
			http.Error(w, fmt.Sprintf("error updating status on selected hosts: %s", err.Error()), httpStatus)
			return
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("%d host(s) will be updated to '%s'", hostsUpdated, opts.Status)))
	case "restartJasper":
		hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetRestartJasperCallback(r.Context(), env, user.Username()))
		if err != nil {
			http.Error(w, fmt.Sprintf("error marking selected hosts as needing Jasper service restarted: %s", err.Error()), httpStatus)
			return
		}

		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("%d host(s) marked as needing Jasper service restarted", hostsUpdated)))
	case "reprovisionToNew":
		hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetReprovisionToNewCallback(r.Context(), env, user.Username()))
		if err != nil {
			http.Error(w, fmt.Sprintf("error marking selected hosts as needing to reprovision: %s", err.Error()), httpStatus)
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("%d host(s) marked as needing to reprovision", hostsUpdated)))
	default:
		uis.LoggedError(w, r, http.StatusBadRequest, errors.Errorf("Unrecognized action: %v", opts.Action))
		return
	}
}

func (uis *UIServer) getHostDNS(r *http.Request) ([]string, error) {
	hostID := gimlet.GetVars(r)["host_id"]
	h, err := uis.getHostFromCache(r.Context(), hostID)
	if err != nil {
		return nil, errors.Wrapf(err, "can't get host '%s'", hostID)
	}
	if h == nil {
		return nil, errors.Wrapf(err, "host '%s' does not exist", hostID)
	}

	return []string{fmt.Sprintf("%s:%d", h.dnsName, evergreen.VSCodePort)}, nil
}

func (uis *UIServer) getHostFromCache(ctx context.Context, hostID string) (*hostCacheItem, error) {
	h, ok := uis.hostCache[hostID]
	if !ok || time.Since(h.inserted) > hostCacheTTL {
		hDb, err := host.FindOneId(ctx, hostID)
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

func (uis *UIServer) handleBackendError(message string, statusCode int) func(http.ResponseWriter, *http.Request, error) {
	return func(w http.ResponseWriter, r *http.Request, err error) {
		gimlet.WriteTextResponse(w, statusCode, message)
	}
}

// returns dockerfle as text
func getDockerfile(w http.ResponseWriter, r *http.Request) {
	parts := []string{
		"ARG BASE_IMAGE",
		"FROM $BASE_IMAGE",
		"ARG URL",
		"ARG EXECUTABLE_SUB_PATH",
		"ARG BINARY_NAME",
		"ADD ${URL}/${EXECUTABLE_SUB_PATH} /",
		"RUN chmod 0777 /${BINARY_NAME}",
	}

	gimlet.WriteText(w, strings.Join(parts, "\n"))
}
