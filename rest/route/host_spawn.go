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

func makeSpawnHostCreateRoute(sc data.Connector) gimlet.RouteHandler {
	return &hostPostHandler{
		sc: sc,
	}
}

type hostPostHandler struct {
	Task         string            `json:"task_id"`
	Distro       string            `json:"distro"`
	KeyName      string            `json:"keyname"`
	UserData     string            `json:"userdata"`
	InstanceTags map[string]string `json:"instance_tags"`

	sc data.Connector
}

func (hph *hostPostHandler) Factory() gimlet.RouteHandler {
	return &hostPostHandler{
		sc: hph.sc,
	}
}

func (hph *hostPostHandler) Parse(ctx context.Context, r *http.Request) error {
	return errors.WithStack(util.ReadJSONInto(r.Body, hph))
}

func (hph *hostPostHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	options := &model.HostPostRequest{
		DistroID:     hph.Distro,
		TaskID:       hph.Task,
		KeyName:      hph.KeyName,
		UserData:     hph.UserData,
		InstanceTags: hph.InstanceTags,
	}

	intentHost, err := hph.sc.NewIntentHost(options, user)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error spawning host"))
	}

	hostModel := &model.APIHost{}
	err = hostModel.BuildFromService(intentHost)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(hostModel)
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/terminate

// TODO this should be a DELETE method on the hosts route rather than
// a post on terminate.

type hostTerminateHandler struct {
	hostID string
	sc     data.Connector
}

func makeTerminateHostRoute(sc data.Connector) gimlet.RouteHandler {
	return &hostTerminateHandler{
		sc: sc,
	}
}

func (h *hostTerminateHandler) Factory() gimlet.RouteHandler {
	return &hostTerminateHandler{
		sc: h.sc,
	}
}

func (h *hostTerminateHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error

	h.hostID, err = validateHostID(gimlet.GetVars(r)["host_id"])

	return err
}

func (h *hostTerminateHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	host, err := h.sc.FindHostByIdWithOwner(h.hostID, u)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	if host.Status == evergreen.HostTerminated {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Host %s is already terminated", host.Id),
		})

	} else if host.Status == evergreen.HostUninitialized {
		if err := h.sc.SetHostStatus(host, evergreen.HostTerminated, u.Id); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			})
		}

	} else {
		if err := h.sc.TerminateHost(ctx, host, u.Id); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			})
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/change_password
//

// TODO (?) should this be a patch route?

type hostChangeRDPPasswordHandler struct {
	hostID      string
	rdpPassword string
	sc          data.Connector
	env         evergreen.Environment
}

func makeHostChangePassword(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &hostChangeRDPPasswordHandler{
		sc:  sc,
		env: env,
	}

}

func (h *hostChangeRDPPasswordHandler) Factory() gimlet.RouteHandler {
	return &hostChangeRDPPasswordHandler{
		sc:  h.sc,
		env: h.env,
	}
}

func (h *hostChangeRDPPasswordHandler) Parse(ctx context.Context, r *http.Request) error {
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

func (h *hostChangeRDPPasswordHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	host, err := h.sc.FindHostByIdWithOwner(h.hostID, u)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	if !host.Distro.IsWindows() {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "RDP passwords can only be set on Windows hosts",
		})
	}
	if host.Status != evergreen.HostRunning {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "RDP passwords can only be set on running hosts",
		})
	}
	if err := cloud.SetHostRDPPassword(ctx, h.env, host, h.rdpPassword); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/extend_expiration
//

type hostExtendExpirationHandler struct {
	hostID   string
	addHours time.Duration
	sc       data.Connector
}

func makeExtendHostExpiration(sc data.Connector) gimlet.RouteHandler {
	return &hostExtendExpirationHandler{
		sc: sc,
	}
}

func (h *hostExtendExpirationHandler) Factory() gimlet.RouteHandler {
	return &hostExtendExpirationHandler{
		sc: h.sc,
	}
}

func (h *hostExtendExpirationHandler) Parse(ctx context.Context, r *http.Request) error {
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

func (h *hostExtendExpirationHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	host, err := h.sc.FindHostByIdWithOwner(h.hostID, u)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}
	if host.Status == evergreen.HostTerminated {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "cannot extend expiration of a terminated host",
		})
	}

	var newExp time.Time
	newExp, err = cloud.MakeExtendedSpawnHostExpiration(host, h.addHours)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
	}

	if err := h.sc.SetHostExpirationTime(host, newExp); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
	}

	return gimlet.NewJSONResponse(struct{}{})
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
