package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/k0kubun/pp"
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
	hostId      string
	rdpPassword string
	addHours    time.Duration

	host model.APIHost
}

func (h *spawnHostModifyHandler) Handler() RequestHandler {
	return &spawnHostModifyHandler{}
}

func (h *spawnHostModifyHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	hostModify := model.APISpawnHostModify{}
	err := util.ReadJSONInto(util.NewRequestReader(r), &hostModify)
	if err != nil {
		return err
	}

	h.action = string(h.action)
	if h.action != HostTerminate || h.action != HostExpirationExtension || h.action != HostPasswordUpdate {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid action",
		}
	}

	h.hostId = string(hostModify.HostId)
	if h.hostId == "" {
		return &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid host id",
		}
	}

	if h.action == HostPasswordUpdate {
		h.rdpPassword = string(hostModify.RDPPwd)
		// TODO validate password same way ui does it
		if h.rdpPassword == "" {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "password must not be empty",
			}
		}

	} else if h.action == HostExpirationExtension {
		var addHours int
		addHours, err = strconv.Atoi(string(hostModify.AddHours))
		if err != nil {
			return &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    "expiration not a number",
			}
		}
		h.addHours = time.Duration(addHours) * time.Hour
	}

	return nil
}

func (h *spawnHostModifyHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	host, err := sc.FindHostById(h.hostId)
	if err != nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}
	if host == nil {
		return ResponseData{}, &rest.APIError{
			StatusCode: http.StatusBadRequest,
			Message:    "unknown host id",
		}
	}

	msg := ""
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

			msg = "host terminated"

		} else {
			if err := sc.TerminateHost(host); err != nil {
				return ResponseData{}, &rest.APIError{
					StatusCode: http.StatusInternalServerError,
					Message:    err.Error(),
				}
			}

			// TODO
			msg = "host terminated"
		}

	} else if h.action == HostPasswordUpdate {
		if err := sc.SetHostPassword(ctx, host, h.rdpPassword); err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}

		// TODO flash
		//PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host RDP password successfully updated."))

		// TODO
		msg = "Host RDP password successfully updated."

	} else if h.action == HostExpirationExtension {
		newExp := host.ExpirationTime.Add(h.addHours)
		hoursUntilExpiration := newExp.Sub(time.Now()).Hours() // nolint
		if hoursUntilExpiration > MaxExpirationDurationHours {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("Can not extend host '%s' expiration by '%d' hours. Maximum extension is limited to %d hours", h.hostId, int(hoursUntilExpiration), MaxExpirationDurationHours),
			}
		}

		if err := sc.ExtendHostExpiration(host, h.addHours); err != nil {
			return ResponseData{}, &rest.APIError{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}

		}

		// TODO
		//PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Host expiration "+
		//	"extension successful; %v will expire on %v", hostId,
		//	futureExpiration.Format(time.RFC850))))
		//fmt.Sprintf("Host expiration extension successful; %s will expire on %s", hostId, futureExpiration.Format(time.RFC850))

		// TODO
		msg = "Successfully extended host expiration time"
	}
	pp.Println(msg)

	return ResponseData{}, nil
}
