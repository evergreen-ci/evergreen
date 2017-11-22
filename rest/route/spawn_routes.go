package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/spawn"
	"github.com/evergreen-ci/evergreen/util"
)

func getSpawnHostsRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route: route,
		Methods: []MethodHandler{
			{
				PrefetchFunctions: []PrefetchFunc{PrefetchUser},
				Authenticator:     &RequireUserAuthenticator{},
				RequestHandler:    &spawnHostModifyHandler{},
				MethodType:        http.MethodGet,
			},
		},
		Version: version,
	}
}

const (
	HostPasswordUpdate         = "updateRDPPassword"
	HostExpirationExtension    = "extendHostExpiration"
	HostTerminate              = "terminate"
	MaxExpirationDurationHours = 24 * 7 // 7 days
)

type spawnHostModifyHandler struct {
	action      string
	hostID      string
	rdpPassword string
	addHours    int
}

func (h *spawnHostModifyHandler) Handler() RequestHandler {
	return &spawnHostModifyHandler{}
}

func (h *spawnHostModifyHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	hostModify := model.APISpawnHostModify{}
	if err := util.ReadJSONInto(util.NewRequestReader(r), &hostModify); err != nil {
		return err
	}

	h.action = string(hostModify.Action)
	if h.action != HostTerminate && h.action != HostExpirationExtension && h.action != HostPasswordUpdate {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid action",
		}
	}

	h.hostID = string(hostModify.HostID)
	if h.hostID == "" {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid host id",
		}
	}

	if h.action == HostPasswordUpdate {
		h.rdpPassword = string(hostModify.RDPPwd)

		if !spawn.ValidateRDPPassword(h.rdpPassword) {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "invalid password",
			}
		}

	} else if h.action == HostExpirationExtension {
		addHours, err := strconv.Atoi(string(hostModify.AddHours))
		if err != nil {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "expiration not a number",
			}
		}

		if addHours <= 0 {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "expiration must be greater than 0",
			}
		}
		h.addHours = addHours
	}

	return nil
}

func (h *spawnHostModifyHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	host, err := sc.FindHostById(h.hostID)
	if err != nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusInternalServerError,
			Message:    "error fetching host information",
		}
	}
	if host == nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "unknown host id",
		}
	}

	if h.action == HostTerminate {
		if host.Status == evergreen.HostTerminated {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Host %s is already terminated", host.Id),
			}

		} else if host.Status == evergreen.HostUninitialized {
			if err := sc.SetHostStatus(host, evergreen.HostTerminated); err != nil {
				return ResponseData{}, &rest.APIError{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				}
			}

		} else {
			if err := sc.TerminateHost(host); err != nil {
				return ResponseData{}, &rest.APIError{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				}
			}
		}

	} else if h.action == HostPasswordUpdate {
		if err := sc.SetHostPassword(ctx, host, h.rdpPassword); err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}

	} else if h.action == HostExpirationExtension {
		newExp, err := makeNewHostExpiration(host, h.addHours)
		if err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			}

		}

		if err := sc.SetHostExpirationTime(host, newExp); err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}
	}

	return ResponseData{}, nil
}
