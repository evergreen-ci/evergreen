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
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/units"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/jasper"
	"github.com/mongodb/jasper/options"
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
	return errors.WithStack(utility.ReadJSON(r.Body, hph.options))
}

func (hph *hostPostHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)

	if hph.options.NoExpiration {
		if err := CheckUnexpirableHostLimitExceeded(user.Id, hph.settings.Spawnhost.UnexpirableHostsPerUser); err != nil {
			return gimlet.MakeJSONErrorResponder(err)
		}
	}

	intentHost, err := hph.sc.NewIntentHost(ctx, hph.options, user, hph.settings)
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
	if err := utility.ReadJSON(body, h.options); err != nil {
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

	if foundHost.Status == evergreen.HostTerminated {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    "cannot modify a terminated host",
		})
	}

	// Validate host modify request
	catcher := grip.NewBasicCatcher()
	if len(h.options.AddInstanceTags) > 0 || len(h.options.DeleteInstanceTags) > 0 {
		catcher.Add(checkInstanceTagsCanBeModified(foundHost, h.options.AddInstanceTags, h.options.DeleteInstanceTags))
	}
	if h.options.InstanceType != "" {
		catcher.Add(checkInstanceTypeHostStopped(foundHost))
		allowedTypes := h.env.Settings().Providers.AWS.AllowedInstanceTypes
		catcher.Add(cloud.CheckInstanceTypeValid(ctx, foundHost.Distro, h.options.InstanceType, allowedTypes))
	}
	if h.options.NoExpiration != nil && *h.options.NoExpiration {
		catcher.AddWhen(h.options.AddHours != 0, errors.New("can't specify no expiration and new expiration"))
		catcher.Add(CheckUnexpirableHostLimitExceeded(user.Id, h.env.Settings().Spawnhost.UnexpirableHostsPerUser))
	}
	if catcher.HasErrors() {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(catcher.Resolve(), "Invalid host modify request"))
	}

	modifyJob := units.NewSpawnhostModifyJob(foundHost, *h.options, utility.RoundPartOfMinute(1).Format(units.TSFormat))
	if err = h.env.RemoteQueue().Put(ctx, modifyJob); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "Error creating spawnhost modify job"))
	}

	if h.options.SubscriptionType != "" {
		subscription, err := makeSpawnHostSubscription(h.hostID, h.options.SubscriptionType, user)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't make subscription"))
		}
		if err = h.sc.SaveSubscriptions(user.Username(), []model.APISubscription{subscription}, false); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't save subscription"))
		}
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

// checkInstanceTypeHostStopped checks whether a host is stopped before modifying an instance type
func checkInstanceTypeHostStopped(h *host.Host) error {
	if h.Status != evergreen.HostStopped {
		return errors.New("cannot modify instance type for non-stopped host")
	}
	return nil
}

func CheckUnexpirableHostLimitExceeded(userId string, maxHosts int) error {
	count, err := host.CountSpawnhostsWithNoExpirationByUser(userId)
	if err != nil {
		return errors.Wrapf(err, "error counting number of existing non-expiring hosts for '%s'", userId)
	}
	if count >= maxHosts {
		return errors.Errorf("can have at most %d unexpirable hosts", maxHosts)
	}
	return nil
}

func checkVolumeLimitExceeded(user string, newSize int, maxSize int) error {
	totalSize, err := host.FindTotalVolumeSizeByUser(user)
	if err != nil {
		return errors.Wrapf(err, "error finding total volume size for user")
	}
	if totalSize+newSize > maxSize {
		return errors.Errorf("volume size limit %d exceeded", maxSize)
	}
	return nil
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/stop

type hostStopHandler struct {
	hostID           string
	subscriptionType string
	sc               data.Connector
	env              evergreen.Environment
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
	if err != nil {
		return errors.Wrap(err, "can't get host id")
	}

	body := util.NewRequestReader(r)
	defer body.Close()
	options := struct {
		SubscriptionType string `json:"subscription_type"`
	}{}
	if err := utility.ReadJSON(body, &options); err != nil {
		h.subscriptionType = ""
	}
	h.subscriptionType = options.SubscriptionType

	return nil
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
	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	stopJob := units.NewSpawnhostStopJob(host, user.Id, ts)
	if err = h.env.RemoteQueue().Put(ctx, stopJob); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error creating spawnhost stop job"))
	}

	if h.subscriptionType != "" {
		subscription, err := makeSpawnHostSubscription(h.hostID, h.subscriptionType, user)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't make subscription"))
		}
		if err = h.sc.SaveSubscriptions(user.Username(), []model.APISubscription{subscription}, false); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't save subscription"))
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/start

type hostStartHandler struct {
	hostID           string
	subscriptionType string
	sc               data.Connector
	env              evergreen.Environment
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
	if err != nil {
		return errors.Wrap(err, "can't get host id")
	}

	body := util.NewRequestReader(r)
	defer body.Close()
	options := struct {
		SubscriptionType string `json:"subscription_type"`
	}{}
	if err := utility.ReadJSON(body, &options); err != nil {
		h.subscriptionType = ""
	}
	h.subscriptionType = options.SubscriptionType

	return nil
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
	ts := utility.RoundPartOfMinute(1).Format(units.TSFormat)
	startJob := units.NewSpawnhostStartJob(host, user.Id, ts)
	if err = h.env.RemoteQueue().Put(ctx, startJob); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "Error creating spawnhost start job"))
	}

	if h.subscriptionType != "" {
		subscription, err := makeSpawnHostSubscription(h.hostID, h.subscriptionType, user)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't make subscription"))
		}
		if err = h.sc.SaveSubscriptions(user.Username(), []model.APISubscription{subscription}, false); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "can't save subscription"))
		}
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
	if err := errors.WithStack(utility.ReadJSON(r.Body, h.attachment)); err != nil {
		return errors.Wrap(err, "error parsing input")
	}

	if h.attachment.VolumeID == "" {
		return errors.New("must provide a volume ID")
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

	if utility.StringSliceContains(evergreen.DownHostStatus, targetHost.Status) {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    errors.Errorf("host '%s' status is %s", targetHost.Id, targetHost.Status).Error(),
		})
	}
	if h.attachment.DeviceName != "" {
		if utility.StringSliceContains(targetHost.HostVolumeDeviceNames(), h.attachment.DeviceName) {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    errors.Errorf("host '%s' already has a volume with device name '%s'", h.hostID, h.attachment.DeviceName).Error(),
			})
		}
	}

	// Check whether attachment already attached to a host
	attachedHost, err := h.sc.FindHostWithVolume(h.attachment.VolumeID)
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

	v, err := h.sc.FindVolumeById(h.attachment.VolumeID)
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

	mgrOpts, err := cloud.GetManagerOptions(targetHost.Distro)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error getting manager options for spawnhost attach volume job"))
	}
	mgr, err := cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "error getting cloud manager for spawnhost attach volume job").Error(),
		})
	}
	grip.Info(message.Fields{
		"message": "attaching volume to spawnhost",
		"host_id": h.hostID,
		"volume":  h.attachment,
	})
	if err = mgr.AttachVolume(ctx, targetHost, h.attachment); err != nil {
		if cloud.ModifyVolumeBadRequest(err) {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			})
		}
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "error attaching volume %s for spawnhost %s", h.attachment.VolumeID, h.hostID).Error(),
		})
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
	if err := errors.WithStack(utility.ReadJSON(r.Body, h.attachment)); err != nil {
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

	if targetHost.HomeVolumeID == h.attachment.VolumeID {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot detach home volume for host '%s'", h.hostID),
		})
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
	mgrOpts, err := cloud.GetManagerOptions(targetHost.Distro)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "error getting manager options for spawnhost detach volume job"))
	}
	mgr, err := cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrap(err, "error getting cloud manager for spawnhost detach volume job").Error(),
		})
	}

	if err = mgr.DetachVolume(ctx, targetHost, h.attachment.VolumeID); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "error detaching volume %s from spawnhost %s", h.attachment.VolumeID, h.hostID).Error(),
		})
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/volumes

type createVolumeHandler struct {
	sc  data.Connector
	env evergreen.Environment

	volume   *host.Volume
	provider string
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
	if err := utility.ReadJSON(r.Body, h.volume); err != nil {
		return err
	}
	if h.volume.Size == 0 {
		return errors.New("Size is required")
	}
	h.provider = evergreen.ProviderNameEc2OnDemand
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

	if err := cloud.ValidVolumeOptions(h.volume, h.env.Settings()); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
	}

	maxVolumeFromSettings := h.env.Settings().Providers.AWS.MaxVolumeSizePerUser
	if err := checkVolumeLimitExceeded(u.Username(), h.volume.Size, maxVolumeFromSettings); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    err.Error(),
		})
	}

	res, err := cloud.CreateVolume(ctx, h.env, h.volume, h.provider)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	volumeModel := &model.APIVolume{}
	err = volumeModel.BuildFromService(res)
	if err != nil {
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
	provider string
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
	h.provider = evergreen.ProviderNameEc2OnDemand
	return err
}

func (h *deleteVolumeHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	volume, err := h.sc.FindVolumeById(h.VolumeID)
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

	attachedHost, err := h.sc.FindHostWithVolume(h.VolumeID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("problem finding host with volume"),
		})
	}
	if attachedHost != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Must detach from host '%s'", attachedHost.Id),
		})
	}

	mgrOpts := cloud.ManagerOpts{
		Provider: h.provider,
		Region:   cloud.AztoRegion(volume.AvailabilityZone),
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
// PATCH /rest/v2/volumes/{volume_id}

type modifyVolumeHandler struct {
	sc  data.Connector
	env evergreen.Environment

	provider string
	volumeID string
	opts     *model.VolumeModifyOptions
}

func makeModifyVolume(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &modifyVolumeHandler{
		sc:  sc,
		env: env,
	}
}

func (h *modifyVolumeHandler) Factory() gimlet.RouteHandler {
	return &modifyVolumeHandler{
		sc:   h.sc,
		env:  h.env,
		opts: &model.VolumeModifyOptions{},
	}
}

func (h *modifyVolumeHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	if err = utility.ReadJSON(r.Body, h.opts); err != nil {
		return err
	}
	if h.volumeID, err = validateID(gimlet.GetVars(r)["volume_id"]); err != nil {
		return err
	}

	h.provider = evergreen.ProviderNameEc2OnDemand

	return nil
}

func (h *modifyVolumeHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	volume, err := h.sc.FindVolumeById(h.volumeID)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(err)
	}

	// Volume does not exist
	if volume == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("attachment '%s' does not exist", h.volumeID),
		})
	}

	// Only allow users to modify their own volumes
	if u.Id != volume.CreatedBy {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    fmt.Sprintf("not authorized to modify attachment '%s'", volume.ID),
		})
	}

	if h.opts.NewName != "" {
		if err = h.sc.SetVolumeName(volume, h.opts.NewName); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(err)
		}
	}

	if h.opts.Size != 0 {
		sizeIncrease := h.opts.Size - volume.Size
		if sizeIncrease <= 0 {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("volumes can only be sized up (current size is %d GiB)", volume.Size),
			})
		}
		maxVolumeFromSettings := h.env.Settings().Providers.AWS.MaxVolumeSizePerUser
		if err = checkVolumeLimitExceeded(u.Username(), sizeIncrease, maxVolumeFromSettings); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			})
		}
	}

	if !utility.IsZeroTime(h.opts.Expiration) {
		if h.opts.Expiration.Before(volume.Expiration) {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "can't move expiration time earlier",
			})
		}
		if h.opts.Expiration.Sub(time.Now()) > evergreen.MaxSpawnHostExpirationDurationHours {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    fmt.Sprintf("can't extend expiration past max duration '%s'", time.Now().Add(evergreen.MaxSpawnHostExpirationDurationHours).Format(time.RFC1123)),
			})
		}

		if h.opts.NoExpiration {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "can't specify both expiration and no-expiration",
			})
		}
	}

	if h.opts.NoExpiration {
		if h.opts.HasExpiration {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "can't specify both has expiration and no-expiration",
			})
		}
		var unexpirableVolumesForUser int
		unexpirableVolumesForUser, err = host.CountNoExpirationVolumesForUser(u.Id)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    "can't get no-expire count",
			})
		}
		if h.env.Settings().Spawnhost.UnexpirableVolumesPerUser-unexpirableVolumesForUser <= 0 {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    "user '%s' has no unexpirable volumes remaining",
			})
		}
	}

	mgrOpts := cloud.ManagerOpts{
		Provider: h.provider,
		Region:   cloud.AztoRegion(volume.AvailabilityZone),
	}
	var mgr cloud.Manager
	mgr, err = cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
	}
	if err = mgr.ModifyVolume(ctx, volume, h.opts); err != nil {
		if cloud.ModifyVolumeBadRequest(err) {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusBadRequest,
				Message:    err.Error(),
			})
		}
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    err.Error(),
		})
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/volumes

type getVolumesHandler struct {
	sc data.Connector
}

func makeGetVolumes(sc data.Connector) gimlet.RouteHandler {
	return &getVolumesHandler{
		sc: sc,
	}
}

func (h *getVolumesHandler) Factory() gimlet.RouteHandler {
	return &getVolumesHandler{
		sc: h.sc,
	}
}

func (h *getVolumesHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *getVolumesHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	volumes, err := h.sc.FindVolumesByUser(u.Username())
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}

	volumeDocs := []model.APIVolume{}
	for _, v := range volumes {
		volumeDoc := model.APIVolume{}
		if err = volumeDoc.BuildFromService(v); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "err converting volume '%s' to API model", v.ID))
		}

		// if the volume is attached to a host, also return the host ID and volume device name
		if v.Host != "" {
			h, err := h.sc.FindHostById(v.Host)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error querying for host"))
			}
			if h != nil {
				for _, attachment := range h.Volumes {
					if attachment.VolumeID == v.ID {
						volumeDoc.DeviceName = utility.ToStringPtr(attachment.DeviceName)
					}
				}
			}
		}
		volumeDocs = append(volumeDocs, volumeDoc)
	}
	return gimlet.NewJSONResponse(volumeDocs)
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/volumes

type getVolumeByIDHandler struct {
	volumeID string
	sc       data.Connector
}

func makeGetVolumeByID(sc data.Connector) gimlet.RouteHandler {
	return &getVolumeByIDHandler{
		sc: sc,
	}
}

func (h *getVolumeByIDHandler) Factory() gimlet.RouteHandler {
	return &getVolumeByIDHandler{
		sc: h.sc,
	}
}

func (h *getVolumeByIDHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	if h.volumeID, err = validateID(gimlet.GetVars(r)["volume_id"]); err != nil {
		return err
	}
	return nil
}

func (h *getVolumeByIDHandler) Run(ctx context.Context) gimlet.Responder {
	v, err := h.sc.FindVolumeById(h.volumeID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(err)
	}
	if v == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    "volume not found",
		})
	}
	volumeDoc := &model.APIVolume{}
	if err = volumeDoc.BuildFromService(v); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "err converting volume '%s' to API model", v.ID))
	}
	// if the volume is attached to a host, also return the host ID and volume device name
	if v.Host != "" {
		attachedHost, err := h.sc.FindHostById(v.Host)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "error querying for host"))
		}
		if attachedHost != nil {
			for _, attachment := range attachedHost.Volumes {
				if attachment.VolumeID == v.ID {
					volumeDoc.DeviceName = utility.ToStringPtr(attachment.DeviceName)
				}
			}
		}
	}

	return gimlet.NewJSONResponse(volumeDoc)
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
	if err := utility.ReadJSON(util.NewRequestReader(r), &hostModify); err != nil {
		return err
	}

	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return err
	}

	h.rdpPassword = utility.FromStringPtr(hostModify.RDPPwd)
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
	if err := utility.ReadJSON(util.NewRequestReader(r), &hostModify); err != nil {
		return err
	}

	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return err
	}

	addHours, err := strconv.Atoi(utility.FromStringPtr(hostModify.AddHours))
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
	if h.addHours > evergreen.MaxSpawnHostExpirationDurationHours {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot add more than %s", evergreen.MaxSpawnHostExpirationDurationHours.String()),
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
// POST /rest/v2/host/start_process
//
type hostStartProcesses struct {
	sc  data.Connector
	env evergreen.Environment

	hostIDs []string
	script  string
}

func makeHostStartProcesses(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &hostStartProcesses{
		sc:  sc,
		env: env,
	}
}

func (hs *hostStartProcesses) Factory() gimlet.RouteHandler {
	return &hostStartProcesses{
		sc:  hs.sc,
		env: hs.env,
	}
}

func (hs *hostStartProcesses) Parse(ctx context.Context, r *http.Request) error {
	var err error
	hostScriptOpts := model.APIHostScript{}
	if err = utility.ReadJSON(util.NewRequestReader(r), &hostScriptOpts); err != nil {
		return errors.Wrap(err, "can't read host command from json")
	}
	hs.script = hostScriptOpts.Script
	hs.hostIDs = hostScriptOpts.Hosts

	return nil
}

func (hs *hostStartProcesses) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	response := gimlet.NewResponseBuilder()
	for _, hostID := range hs.hostIDs {
		h, err := hs.sc.FindHostByIdWithOwner(hostID, u)
		if err != nil {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   hostID,
				Complete: true,
				Output:   errors.Wrap(err, "can't get host").Error(),
			}), "can't add data for host '%s'", hostID))
			continue
		}
		if h.Status != evergreen.HostRunning {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   hostID,
				Complete: true,
				Output:   fmt.Sprintf("can't run script on host with status '%s'", h.Status),
			}), "can't add data for host '%s'", hostID))
			continue
		}
		if !h.Distro.JasperCommunication() {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   hostID,
				Complete: true,
				Output:   fmt.Sprintf("can't run script on host of distro '%s' because it doesn't support Jasper communication", h.Distro.Id),
			}), "can't add data for host '%s'", hostID))
			continue
		}

		logger, err := jasper.NewInMemoryLogger(host.OutputBufferSize)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "problem creating new in-memory logger"))
		}
		bashPath := h.Distro.AbsPathNotCygwinCompatible(h.Distro.BootstrapSettings.ShellPath)
		opts := &options.Create{
			Args:   []string{bashPath, "-l", "-c", hs.script},
			Output: options.Output{Loggers: []*options.LoggerConfig{logger}},
		}
		procID, err := h.StartJasperProcess(ctx, hs.env, opts)
		if err != nil {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   hostID,
				Complete: true,
				Output:   errors.Wrap(err, "can't run script with Jasper").Error(),
			}), "can't add data for host '%s'", hostID))
			continue
		}
		grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
			HostID:   hostID,
			Complete: false,
			ProcID:   procID,
		}), "can't add data for host '%s'", hostID))
	}

	return response
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/host/get_process
//
type hostGetProcesses struct {
	sc  data.Connector
	env evergreen.Environment

	hostProcesses []model.APIHostProcess
}

func makeHostGetProcesses(sc data.Connector, env evergreen.Environment) gimlet.RouteHandler {
	return &hostGetProcesses{
		sc:  sc,
		env: env,
	}
}

func (h *hostGetProcesses) Factory() gimlet.RouteHandler {
	return &hostGetProcesses{
		sc:  h.sc,
		env: h.env,
	}
}

func (h *hostGetProcesses) Parse(ctx context.Context, r *http.Request) error {
	var err error
	hostProcesses := []model.APIHostProcess{}
	if err = utility.ReadJSON(util.NewRequestReader(r), &hostProcesses); err != nil {
		return errors.Wrap(err, "can't read host processes from json")
	}
	h.hostProcesses = hostProcesses

	return nil
}

func (h *hostGetProcesses) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	response := gimlet.NewResponseBuilder()
	for _, process := range h.hostProcesses {
		host, err := h.sc.FindHostByIdWithOwner(process.HostID, u)
		if err != nil {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   process.HostID,
				ProcID:   process.ProcID,
				Complete: true,
				Output:   errors.Wrap(err, "can't get host").Error(),
			}), "can't add data for host '%s'", process.HostID))
			continue
		}

		complete, output, err := host.GetJasperProcess(ctx, h.env, process.ProcID)
		if err != nil {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   process.HostID,
				Complete: true,
				Output:   errors.Wrap(err, "can't get process with Jasper").Error(),
			}), "can't add data for host '%s'", process.HostID))
			continue
		}
		grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
			HostID:   process.HostID,
			Complete: complete,
			ProcID:   process.ProcID,
			Output:   output,
		}), "can't add data for host '%s'", process.HostID))
	}

	return response
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

func makeSpawnHostSubscription(hostID, subscriberType string, user *user.DBUser) (model.APISubscription, error) {
	var subscriber model.APISubscriber
	if subscriberType == event.SlackSubscriberType {
		subscriber = model.APISubscriber{
			Type:   utility.ToStringPtr(event.SlackSubscriberType),
			Target: fmt.Sprintf("@%s", user.Settings.SlackUsername),
		}
	} else if subscriberType == event.EmailSubscriberType {
		subscriber = model.APISubscriber{
			Type:   utility.ToStringPtr(event.EmailSubscriberType),
			Target: user.Email(),
		}
	} else {
		return model.APISubscription{}, errors.Errorf("'%s' is not a valid subscriber type", subscriberType)
	}

	return model.APISubscription{
		OwnerType:    utility.ToStringPtr(string(event.OwnerTypePerson)),
		ResourceType: utility.ToStringPtr(event.ResourceTypeHost),
		Trigger:      utility.ToStringPtr(event.TriggerOutcome),
		Selectors: []model.APISelector{
			{
				Type: utility.ToStringPtr(event.SelectorID),
				Data: utility.ToStringPtr(hostID),
			},
		},
		Subscriber: subscriber,
	}, nil
}
