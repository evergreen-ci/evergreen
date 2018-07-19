package route

import (
	"context"
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/pkg/errors"
)

type hostPostHandler struct {
	Distro  string `json:"distro"`
	KeyName string `json:"keyname"`
}

func (hph *hostPostHandler) Handler() RequestHandler {
	return &hostPostHandler{}
}

func (hph *hostPostHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	return errors.WithStack(util.ReadJSONInto(r.Body, hph))
}

func (hph *hostPostHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	user := MustHaveUser(ctx)

	intentHost, err := sc.NewIntentHost(hph.Distro, hph.KeyName, "", user)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "error spawning host")
	}

	hostModel := &model.APIHost{}
	err = hostModel.BuildFromService(intentHost)
	if err != nil {
		return ResponseData{}, errors.Wrap(err, "API model error")
	}

	return ResponseData{
		Result: []model.Model{hostModel},
	}, nil
}

func getHostTerminateRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodPost,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: &hostTerminateHandler{},
			},
		},
	}
}

type hostTerminateHandler struct {
	hostID string
}

func (h *hostTerminateHandler) Handler() RequestHandler {
	return &hostTerminateHandler{}
}

func (h *hostTerminateHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	var err error
	h.hostID, err = validateHostID(gimlet.GetVars(r)["host_id"])

	return err
}

func (h *hostTerminateHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)

	host, err := sc.FindHostByIdWithOwner(h.hostID, u)
	if err != nil {
		return ResponseData{}, err
	}

	if host.Status == evergreen.HostTerminated {
		return ResponseData{}, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Host %s is already terminated", host.Id),
		}

	} else if host.Status == evergreen.HostUninitialized {
		if err := sc.SetHostStatus(host, evergreen.HostTerminated, u.Id); err != nil {
			return ResponseData{}, gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}

	} else {
		if err := sc.TerminateHost(ctx, host, u.Id); err != nil {
			return ResponseData{}, gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			}
		}
	}

	return ResponseData{}, nil
}

func getHostChangeRDPPasswordRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodPost,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: &hostChangeRDPPasswordHandler{},
			},
		},
	}
}

type hostChangeRDPPasswordHandler struct {
	hostID      string
	rdpPassword string
}

func (h *hostChangeRDPPasswordHandler) Handler() RequestHandler {
	return &hostChangeRDPPasswordHandler{}
}

func (h *hostChangeRDPPasswordHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	hostModify := model.APISpawnHostModify{}
	if err := util.ReadJSONInto(util.NewRequestReader(r), &hostModify); err != nil {
		return err
	}

	var err error
	h.hostID, err = validateHostID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return err
	}

	h.rdpPassword = model.FromAPIString(hostModify.RDPPwd)
	if !cloud.ValidateRDPPassword(h.rdpPassword) {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "invalid password",
		}
	}

	return nil
}

func (h *hostChangeRDPPasswordHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)

	host, err := sc.FindHostByIdWithOwner(h.hostID, u)
	if err != nil {
		return ResponseData{}, err
	}

	if !host.Distro.IsWindows() {
		return ResponseData{}, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "RDP passwords can only be set on Windows hosts",
		}
	}
	if host.Status != evergreen.HostRunning {
		return ResponseData{}, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "RDP passwords can only be set on running hosts",
		}
	}
	if err := cloud.SetHostRDPPassword(ctx, host, h.rdpPassword); err != nil {
		return ResponseData{}, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	return ResponseData{}, nil
}

func getHostExtendExpirationRouteManager(route string, version int) *RouteManager {
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodPost,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: &hostExtendExpirationHandler{},
			},
		},
	}
}

type hostExtendExpirationHandler struct {
	hostID   string
	addHours time.Duration
}

func (h *hostExtendExpirationHandler) Handler() RequestHandler {
	return &hostExtendExpirationHandler{}
}

func (h *hostExtendExpirationHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	hostModify := model.APISpawnHostModify{}
	if err := util.ReadJSONInto(util.NewRequestReader(r), &hostModify); err != nil {
		return err
	}

	var err error
	h.hostID, err = validateHostID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return err
	}

	addHours, err := strconv.Atoi(model.FromAPIString(hostModify.AddHours))
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "expiration not a number",
		}
	}
	h.addHours = time.Duration(addHours) * time.Hour

	if h.addHours <= 0 {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "must add more than 0 hours to expiration",
		}
	}
	if h.addHours > cloud.MaxSpawnHostExpirationDurationHours {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot add more than %s", cloud.MaxSpawnHostExpirationDurationHours.String()),
		}
	}

	return nil
}

func (h *hostExtendExpirationHandler) Execute(ctx context.Context, sc data.Connector) (ResponseData, error) {
	u := MustHaveUser(ctx)

	host, err := sc.FindHostByIdWithOwner(h.hostID, u)
	if err != nil {
		return ResponseData{}, err
	}
	if host.Status == evergreen.HostTerminated {
		return ResponseData{}, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "cannot extend expiration of a terminated host",
		}
	}

	var newExp time.Time
	newExp, err = cloud.MakeExtendedSpawnHostExpiration(host, h.addHours)
	if err != nil {
		return ResponseData{}, gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		}
	}

	if err := sc.SetHostExpirationTime(host, newExp); err != nil {
		return ResponseData{}, gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		}
	}

	return ResponseData{}, nil
}

func getHostsByUserManager(route string, version int) *RouteManager {
	h := &hostsByUserHandler{}
	return &RouteManager{
		Route:   route,
		Version: version,
		Methods: []MethodHandler{
			{
				MethodType:     http.MethodGet,
				Authenticator:  &RequireUserAuthenticator{},
				RequestHandler: h.Handler(),
			},
		},
	}
}

type hostsByUserHandler struct {
	PaginationExecutor
}

// Handler for the GET /users/{user_id}/hosts route to return a user's hosts
func (h *hostsByUserHandler) Handler() RequestHandler {
	return &hostsByUserHandler{PaginationExecutor{
		KeyQueryParam:   "host_id",
		LimitQueryParam: "limit",
		Paginator:       hostPaginator,
		Args:            hostGetArgs{},
	}}
}

func (h *hostsByUserHandler) ParseAndValidate(ctx context.Context, r *http.Request) error {
	h.Args = hostGetArgs{
		status: r.URL.Query().Get("status"),
		user:   gimlet.GetVars(r)["user_id"],
	}
	return h.PaginationExecutor.ParseAndValidate(ctx, r)
}

////////////////////////////////////////////////////////////////////////
//
// utility functions

func validateHostID(hostID string) (string, error) {
	if strings.TrimSpace(hostID) == "" {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "missing/empty host id",
		}
	}

	return hostID, nil
}
