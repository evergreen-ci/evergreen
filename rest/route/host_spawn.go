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
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}

func makeSpawnHostCreateRoute(sc data.Connector, settings *evergreen.Settings) gimlet.RouteHandler {
	return &hostPostHandler{
		sc:       sc,
		settings: settings,
	}
}

type hostPostHandler struct {
	sc       data.Connector
	settings *evergreen.Settings

	options *model.HostRequestOptions
}

func (hph *hostPostHandler) Factory() gimlet.RouteHandler {
	return &hostPostHandler{
		sc:       hph.sc,
		settings: hph.settings,
	}
}

func (hph *hostPostHandler) Parse(ctx context.Context, r *http.Request) error {
	hph.options = &model.HostRequestOptions{}
	return errors.WithStack(util.ReadJSONInto(r.Body, hph.options))
}

func (hph *hostPostHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	// Validate instance type
	if hph.options.InstanceType != "" {
		d, err := distro.FindOne(distro.ById(hph.options.DistroID))
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error finding distro '%s'", hph.options.DistroID))
		}
		if err = checkInstanceTypeValid(d.Provider, hph.options.InstanceType, hph.settings); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Invalid host create request"))
		}
	}

	if hph.options.NoExpiration {
		if err := checkExpirableHostLimitExceeded(user.Id); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	intentHost, err := hph.sc.NewIntentHost(hph.options, user)
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
// PATCH /rest/v2/hosts/{host_id}

type hostModifyHandler struct {
	hostID string
	sc     data.Connector
	env    evergreen.Environment

	options *host.HostModifyOptions
}

func makeHostModifyRouteManager(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &hostModifyHandler{
		sc:  sc,
		env: env,
	}
}

func (h *hostModifyHandler) Factory() gimlet.RouteHandler {
	return &hostModifyHandler{
		sc:  h.sc,
		env: h.env,
	}
}

func (h *hostModifyHandler) Parse(ctx context.Context, r *http.Request) error {
	h.hostID = gimlet.GetVars(r)["host_id"]
	body := util.NewRequestReader(r)
	defer body.Close()

	h.options = &host.HostModifyOptions{}
	if err := util.ReadJSONInto(body, h.options); err != nil {
		return errors.Wrap(err, "Argument read error")
	}

	return nil
}

func (h *hostModifyHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	// Find host to be modified
	foundHost, err := h.sc.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", h.hostID))
	}

	// Validate host modify request
	catcher := grip.NewBasicCatcher()
	if len(h.options.AddInstanceTags) > 0 || len(h.options.DeleteInstanceTags) > 0 {
		catcher.Add(checkInstanceTagsCanBeModified(foundHost, h.options.AddInstanceTags, h.options.DeleteInstanceTags))
	}
	if h.options.InstanceType != "" {
		catcher.Add(checkInstanceTypeHostStopped(foundHost))
		catcher.Add(checkInstanceTypeValid(foundHost.Provider, h.options.InstanceType, h.env.Settings()))
	}
	if h.options.NoExpiration != nil && *h.options.NoExpiration {
		catcher.Add(checkExpirableHostLimitExceeded(user.Id))
	}
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(catcher.Resolve(), "Invalid host modify request"))
	}

	ts := util.RoundPartOfMinute(1).Format(tsFormat)
	modifyJob := units.NewSpawnhostModifyJob(foundHost, *h.options, ts)
	if err = h.env.RemoteQueue().Put(ctx, modifyJob); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error creating spawnhost modify job"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

// checkInstanceTagsCanBeModified checks whether the tags to be modified allow modifications.
func checkInstanceTagsCanBeModified(h *host.Host, toAdd []host.Tag, toDelete []string) error {
	catcher := grip.NewBasicCatcher()
	current := make(map[string]host.Tag)
	for _, tag := range h.InstanceTags {
		current[tag.Key] = tag
	}
	for _, key := range toDelete {
		old, ok := current[key]
		if ok && !old.CanBeModified {
			catcher.Add(errors.Errorf("tag '%s' cannot be modified", key))
		}
	}
	for _, tag := range toAdd {
		old, ok := current[tag.Key]
		if ok && !old.CanBeModified {
			catcher.Add(errors.Errorf("tag '%s' cannot be modified", tag.Key))
		}

		// Ensure that new tags can be modified (theoretically should always be the case).
		if !tag.CanBeModified {
			catcher.Add(errors.Errorf("programmer error: new tag '%s=%s' should be able to be modified", tag.Key, tag.Value))
		}
	}
	return catcher.Resolve()
}

// checkInstanceTypeValid checks whether the instance type is allowed by provider config
func checkInstanceTypeValid(providerName, instanceType string, s *evergreen.Settings) error {
	if cloud.IsEc2Provider(providerName) {
		for _, allowedType := range s.Providers.AWS.AllowedInstanceTypes {
			if instanceType == allowedType {
				return nil
			}
		}
	}
	return errors.Errorf("'%s' is not a valid instance type for provider '%s'", instanceType, providerName)
}

// checkInstanceTypeHostStopped checks whether a host is stopped before modifying an instance type
func checkInstanceTypeHostStopped(h *host.Host) error {
	if h.Status != evergreen.HostStopped {
		return errors.New("cannot modify instance type for non-stopped host")
	}
	return nil
}

func checkExpirableHostLimitExceeded(userId string) error {
	count, err := host.CountSpawnhostsWithNoExpirationByUser(userId)
	if err != nil {
		return errors.Wrapf(err, "error counting number of existing non-expiring hosts for '%s'", userId)
	}
	if count >= host.MaxSpawnhostsWithNoExpirationPerUser {
		return errors.Wrapf(err, "cannot create any more non-expiring spawn hosts for '%s'", userId)
	}
	return nil
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/stop

type hostStopHandler struct {
	hostID string
	sc     data.Connector
	env    evergreen.Environment
}

func makeHostStopManager(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &hostStopHandler{
		sc:  sc,
		env: env,
	}
}

func (h *hostStopHandler) Factory() gimlet.RouteHandler {
	return &hostStopHandler{
		sc:  h.sc,
		env: h.env,
	}
}

func (h *hostStopHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	return err
}

func (h *hostStopHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	// Find host to be stopped
	host, err := h.sc.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by host id '%s'", h.hostID))
	}

	// Error if host is not able to be stopped
	if host.Status == evergreen.HostStopped || host.Status == evergreen.HostStopping {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Host '%s' is already stopping or stopped", host.Id),
		})
	} else if host.Status != evergreen.HostRunning {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Host %s is not running", host.Id),
		})
	}

	// Stop the host
	ts := util.RoundPartOfMinute(1).Format(tsFormat)
	stopJob := units.NewSpawnhostStopJob(host, user.Id, ts)
	if err = h.env.RemoteQueue().Put(ctx, stopJob); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error creating spawnhost stop job"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/start

type hostStartHandler struct {
	hostID string
	sc     data.Connector
	env    evergreen.Environment
}

func makeHostStartManager(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &hostStartHandler{
		sc:  sc,
		env: env,
	}
}

func (h *hostStartHandler) Factory() gimlet.RouteHandler {
	return &hostStartHandler{
		sc:  h.sc,
		env: h.env,
	}
}

func (h *hostStartHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	return err
}

func (h *hostStartHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	// Find host to be started
	host, err := h.sc.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Database error for find() by distro id '%s'", h.hostID))
	}

	// Error if host is not able to be started
	if host.Status != evergreen.HostStopped {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Host %s is not stopped", host.Id),
		})
	}

	// Start the host
	ts := util.RoundPartOfMinute(1).Format(tsFormat)
	startJob := units.NewSpawnhostStartJob(host, user.Id, ts)
	if err = h.env.RemoteQueue().Put(ctx, startJob); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error creating spawnhost start job"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/attach

type attachVolumeHandler struct {
	sc     data.Connector
	env    evergreen.Environment
	hostID string

	attachment *host.VolumeAttachment
}

func makeAttachVolume(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &attachVolumeHandler{
		sc:  sc,
		env: env,
	}
}

func (h *attachVolumeHandler) Factory() gimlet.RouteHandler {
	return &attachVolumeHandler{
		sc:  h.sc,
		env: h.env,
	}
}

func (h *attachVolumeHandler) Parse(ctx context.Context, r *http.Request) error {
	h.attachment = &host.VolumeAttachment{}
	if err := errors.WithStack(util.ReadJSONInto(r.Body, h.attachment)); err != nil {
		return errors.Wrap(err, "error parsing input")
	}

	if h.attachment.VolumeID == "" {
		return errors.New("must provide a volume ID")
	}
	// TODO: can modify once Evergreen generates device names
	if h.attachment.DeviceName == "" {
		return errors.New("must provide a device name to attach (recommended form: /dev/sd[f-p][1-6])")
	}
	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return err
	}

	return nil
}

func (h *attachVolumeHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	targetHost, err := h.sc.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error getting host '%s'", h.hostID))
	}

	// Check whether attachment already attached to a host
	attachedHost, err := host.FindHostWithVolume(h.attachment.VolumeID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "error checking whether attachment '%s' is already attached to host", h.attachment.VolumeID).Error(),
		})
	}
	if attachedHost != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Errorf("attachment '%s' is already attached to a host", h.attachment.VolumeID).Error(),
		})
	}

	v, err := host.FindVolumeByID(h.attachment.VolumeID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "error checking whether attachment '%s' exists", h.attachment.VolumeID).Error(),
		})
	}
	if v == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Errorf("attachment '%s' does not exist", h.attachment.VolumeID).Error(),
		})
	}

	if v.AvailabilityZone != targetHost.Zone {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Errorf("Host and volume must have same availability zone").Error(),
		})
	}

	mgrOpts := cloud.ManagerOpts{
		Provider: attachedHost.Provider,
		Region:   cloud.GetRegion(attachedHost.Distro),
	}
	mgr, err := cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error getting cloud manager for spawnhost attach volume job"))
	}
	grip.Info(message.Fields{
		"message": "attaching volume to spawnhost",
		"host_id": h.hostID,
		"volume":  h.attachment,
	})
	if err = mgr.AttachVolume(ctx, attachedHost, h.attachment); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error attaching volume %s for spawnhost %s", h.attachment.VolumeID, h.hostID))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/detach

type detachVolumeHandler struct {
	sc     data.Connector
	env    evergreen.Environment
	hostID string

	attachment *host.VolumeAttachment
}

func makeDetachVolume(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &detachVolumeHandler{
		sc:  sc,
		env: env,
	}
}

func (h *detachVolumeHandler) Factory() gimlet.RouteHandler {
	return &detachVolumeHandler{
		sc:  h.sc,
		env: h.env,
	}
}

func (h *detachVolumeHandler) Parse(ctx context.Context, r *http.Request) error {
	h.attachment = &host.VolumeAttachment{}
	if err := errors.WithStack(util.ReadJSONInto(r.Body, h.attachment)); err != nil {
		return err
	}
	if h.attachment == nil {
		return errors.New("body is nil")
	}

	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return err
	}

	return nil
}

func (h *detachVolumeHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	targetHost, err := h.sc.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error getting targetHost '%s'", h.hostID))
	}

	found := false
	for _, attachment := range targetHost.Volumes {
		if attachment.VolumeID == h.attachment.VolumeID {
			found = true
			break
		}
	}
	if !found {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("attachment '%s' is not attached to targetHost '%s", h.attachment.VolumeID, h.hostID),
		})
	}

	grip.Info(message.Fields{
		"message": "detaching volume from spawnhost",
		"host_id": h.hostID,
		"volume":  h.attachment.VolumeID,
	})
	mgrOpts := cloud.ManagerOpts{
		Provider: targetHost.Provider,
		Region:   cloud.GetRegion(targetHost.Distro),
	}
	mgr, err := cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error getting cloud manager for spawnhost detach volume job"))
	}

	if err = mgr.DetachVolume(ctx, targetHost, h.attachment.VolumeID); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "error detaching volume %s from spawnhost %s", h.attachment.VolumeID, h.hostID))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/volumes

type createVolumeHandler struct {
	sc  data.Connector
	env evergreen.Environment

	volume *host.Volume
}

func makeCreateVolume(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &createVolumeHandler{
		sc:  sc,
		env: env,
	}
}

func (h *createVolumeHandler) Factory() gimlet.RouteHandler {
	return &createVolumeHandler{
		sc:  h.sc,
		env: h.env,
	}
}

func (h *createVolumeHandler) Parse(ctx context.Context, r *http.Request) error {
	h.volume = &host.Volume{}
	if err := util.ReadJSONInto(r.Body, h.volume); err != nil {
		grip.Debug(message.WrapError(err, "problem reading JSON into"))
		return err
	}
	if h.volume.Size == 0 {
		grip.Debug("Size is required")
		return errors.New("Size is required")
	}
	return nil
}

func (h *createVolumeHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	h.volume.CreatedBy = u.Id

	if h.volume.Type == "" {
		h.volume.Type = evergreen.DefaultEBSType
	}
	if h.volume.AvailabilityZone == "" {
		h.volume.AvailabilityZone = evergreen.DefaultEBSAvailabilityZone
	}

	mgrOpts := cloud.ManagerOpts{
		Provider: evergreen.ProviderNameEc2OnDemand,
		Region:   evergreen.DefaultEC2Region,
	}
	mgr, err := cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		grip.Debug(message.WrapError(err, "manager error"))
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
	}

	if h.volume, err = mgr.CreateVolume(ctx, h.volume); err != nil {
		grip.Debug(message.WrapError(err, "create volume error"))
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
	}

	volumeModel := &model.APIVolume{}
	err = volumeModel.BuildFromService(h.volume)
	if err != nil {
		message.WrapError(err, "build from service")
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "API model error"))
	}

	return gimlet.NewJSONResponse(volumeModel)
}

////////////////////////////////////////////////////////////////////////
//
// DELETE /rest/v2/volumes/{volume_id}

type deleteVolumeHandler struct {
	sc  data.Connector
	env evergreen.Environment

	VolumeID string
}

func makeDeleteVolume(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &deleteVolumeHandler{
		sc:  sc,
		env: env,
	}
}

func (h *deleteVolumeHandler) Factory() gimlet.RouteHandler {
	return &deleteVolumeHandler{
		sc:  h.sc,
		env: h.env,
	}
}

func (h *deleteVolumeHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	h.VolumeID, err = validateID(gimlet.GetVars(r)["volume_id"])

	return err
}

func (h *deleteVolumeHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	volume, err := host.FindVolumeByID(h.VolumeID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	// Volume does not exist
	if volume == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("attachment '%s' does not exist", h.VolumeID),
		})
	}

	// Only allow users to delete their own volumes
	if u.Id != volume.CreatedBy {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    fmt.Sprintf("not authorized to delete attachment '%s'", volume.ID),
		})
	}
	// TODO: Allow different providers/regions
	mgrOpts := cloud.ManagerOpts{
		Provider: evergreen.ProviderNameEc2OnDemand,
		Region:   evergreen.DefaultEC2Region,
	}
	mgr, err := cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
	}
	if err = mgr.DeleteVolume(ctx, volume); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
	}

	return gimlet.NewJSONResponse(struct{}{})
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

	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])

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
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return err
	}

	h.rdpPassword = model.FromAPIString(hostModify.RDPPwd)
	if !host.ValidateRDPPassword(h.rdpPassword) {
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
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
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

func validateID(id string) (string, error) {
	if strings.TrimSpace(id) == "" {
		return "", gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "missing/empty id",
		}
	}

	return id, nil
}
