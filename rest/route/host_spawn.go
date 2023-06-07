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

func makeSpawnHostCreateRoute(settings *evergreen.Settings) gimlet.RouteHandler {
	return &hostPostHandler{
		settings: settings,
	}
}

type hostPostHandler struct {
	settings *evergreen.Settings

	options *model.HostRequestOptions
}

func (hph *hostPostHandler) Factory() gimlet.RouteHandler {
	return &hostPostHandler{
		settings: hph.settings,
	}
}

func (hph *hostPostHandler) Parse(ctx context.Context, r *http.Request) error {
	hph.options = &model.HostRequestOptions{}
	return errors.Wrap(utility.ReadJSON(r.Body, hph.options), "reading host options from JSON request body")
}

func (hph *hostPostHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	if hph.options.NoExpiration {
		if err := CheckUnexpirableHostLimitExceeded(user.Id, hph.settings.Spawnhost.UnexpirableHostsPerUser); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "checking expirable host limit"))
		}
	}

	intentHost, err := data.NewIntentHost(ctx, hph.options, user, hph.settings)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "creating intent host"))
	}

	hostModel := &model.APIHost{}
	hostModel.BuildFromService(intentHost, nil)
	return gimlet.NewJSONResponse(hostModel)
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/hosts/{host_id}

type hostModifyHandler struct {
	hostID string
	env    evergreen.Environment

	options *host.HostModifyOptions
}

func makeHostModifyRouteManager(env evergreen.Environment) gimlet.RouteHandler {
	return &hostModifyHandler{
		env: env,
	}
}

func (h *hostModifyHandler) Factory() gimlet.RouteHandler {
	return &hostModifyHandler{
		env: h.env,
	}
}

func (h *hostModifyHandler) Parse(ctx context.Context, r *http.Request) error {
	h.hostID = gimlet.GetVars(r)["host_id"]
	body := utility.NewRequestReader(r)
	defer body.Close()

	h.options = &host.HostModifyOptions{}
	if err := utility.ReadJSON(body, h.options); err != nil {
		return errors.Wrap(err, "reading host modification options from JSON request body")
	}

	return nil
}

func (h *hostModifyHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	foundHost, err := data.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' with owner '%s'", h.hostID, user.Id))
	}

	if foundHost.Status == evergreen.HostTerminated {
		return gimlet.MakeJSONErrorResponder(errors.New("cannot modify a terminated host"))
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
		return gimlet.MakeJSONErrorResponder(errors.Wrap(catcher.Resolve(), "invalid host modify request"))
	}

	modifyJob := units.NewSpawnhostModifyJob(foundHost, *h.options, utility.RoundPartOfMinute(1).Format(units.TSFormat))
	if err = h.env.RemoteQueue().Put(ctx, modifyJob); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "enqueueing spawn host modification job"))
	}

	if h.options.SubscriptionType != "" {
		subscription, err := makeSpawnHostSubscription(h.hostID, h.options.SubscriptionType, user)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "creating spawn host subscription"))
		}
		if err = data.SaveSubscriptions(user.Username(), []model.APISubscription{subscription}, false); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "saving subscription"))
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
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "counting number of existing non-expiring hosts for user '%s'", userId).Error(),
		}
	}
	if count >= maxHosts {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot exceed user total unexpirable host limit %d", maxHosts),
		}
	}
	return nil
}

func checkVolumeLimitExceeded(user string, newSize int, maxSize int) error {
	totalSize, err := host.FindTotalVolumeSizeByUser(user)
	if err != nil {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    errors.Wrapf(err, "counting total volume size for user '%s'", user).Error(),
		}
	}
	if totalSize+newSize > maxSize {
		return gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("cannot exceed user total volume size limit %d", maxSize),
		}
	}
	return nil
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/stop

type hostStopHandler struct {
	hostID           string
	subscriptionType string
	env              evergreen.Environment
}

func makeHostStopManager(env evergreen.Environment) gimlet.RouteHandler {
	return &hostStopHandler{
		env: env,
	}
}

func (h *hostStopHandler) Factory() gimlet.RouteHandler {
	return &hostStopHandler{
		env: h.env,
	}
}

func (h *hostStopHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return errors.Wrap(err, "invalid host ID")
	}

	body := utility.NewRequestReader(r)
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
	host, err := data.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' with owner '%s'", h.hostID, user.Id))
	}

	statusCode, err := data.StopSpawnHost(ctx, h.env, user, host)
	if err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: statusCode,
			Message:    errors.Wrap(err, "stopping spawn host").Error(),
		})
	}

	if h.subscriptionType != "" {
		subscription, err := makeSpawnHostSubscription(h.hostID, h.subscriptionType, user)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "creating spawn host subscription"))
		}
		if err = data.SaveSubscriptions(user.Username(), []model.APISubscription{subscription}, false); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "saving subscription"))
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
	env              evergreen.Environment
}

func makeHostStartManager(env evergreen.Environment) gimlet.RouteHandler {
	return &hostStartHandler{
		env: env,
	}
}

func (h *hostStartHandler) Factory() gimlet.RouteHandler {
	return &hostStartHandler{
		env: h.env,
	}
}

func (h *hostStartHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return errors.Wrap(err, "invalid host ID")
	}

	body := utility.NewRequestReader(r)
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
	host, err := data.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' with owner '%s'", h.hostID, user.Id))
	}

	statusCode, err := data.StartSpawnHost(ctx, h.env, user, host)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(gimlet.ErrorResponse{
			StatusCode: statusCode,
			Message:    errors.Wrap(err, "stopping spawn host").Error(),
		})
	}

	if h.subscriptionType != "" {
		subscription, err := makeSpawnHostSubscription(h.hostID, h.subscriptionType, user)
		if err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "creating spawn host subscription"))
		}
		if err = data.SaveSubscriptions(user.Username(), []model.APISubscription{subscription}, false); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "saving subscription"))
		}
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/attach

type attachVolumeHandler struct {
	env    evergreen.Environment
	hostID string

	attachment *host.VolumeAttachment
}

func makeAttachVolume(env evergreen.Environment) gimlet.RouteHandler {
	return &attachVolumeHandler{
		env: env,
	}
}

func (h *attachVolumeHandler) Factory() gimlet.RouteHandler {
	return &attachVolumeHandler{
		env: h.env,
	}
}

func (h *attachVolumeHandler) Parse(ctx context.Context, r *http.Request) error {
	h.attachment = &host.VolumeAttachment{}
	if err := utility.ReadJSON(r.Body, h.attachment); err != nil {
		return errors.Wrap(err, "reading volume attachment from JSON request body")
	}

	if h.attachment.VolumeID == "" {
		return errors.New("attachment must provide a volume ID")
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
	targetHost, err := data.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "getting host '%s' with owner '%s'", h.hostID, user.Id))
	}

	if utility.StringSliceContains(evergreen.DownHostStatus, targetHost.Status) {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("cannot attach volume to host '%s' whose status is '%s'", targetHost.Id, targetHost.Status))
	}
	if h.attachment.DeviceName != "" {
		if utility.StringSliceContains(targetHost.HostVolumeDeviceNames(), h.attachment.DeviceName) {
			return gimlet.MakeJSONErrorResponder(errors.Errorf("host '%s' already has a volume with device name '%s'", h.hostID, h.attachment.DeviceName))
		}
	}

	// Check whether attachment already attached to a host
	attachedHost, err := host.FindHostWithVolume(h.attachment.VolumeID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "checking whether attachment '%s' is already attached to host", h.attachment.VolumeID))
	}
	if attachedHost != nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("attachment '%s' is already attached to a host", h.attachment.VolumeID))
	}

	v, err := host.FindVolumeByID(h.attachment.VolumeID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "checking whether attachment '%s' exists", h.attachment.VolumeID))
	}
	if v == nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("attachment '%s' does not exist", h.attachment.VolumeID))
	}

	if v.AvailabilityZone != targetHost.Zone {
		return gimlet.MakeJSONErrorResponder(errors.New("host and volume must have same availability zone"))
	}

	mgrOpts, err := cloud.GetManagerOptions(targetHost.Distro)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting cloud manager options"))
	}
	mgr, err := cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting cloud manager"))
	}

	grip.Info(message.Fields{
		"message": "attaching volume to spawnhost",
		"host_id": h.hostID,
		"volume":  h.attachment,
	})

	if err = mgr.AttachVolume(ctx, targetHost, h.attachment); err != nil {
		if cloud.ModifyVolumeBadRequest(err) {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "attaching volume '%s' to spawn host '%s'", h.attachment.VolumeID, h.hostID))
		}
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "attaching volume '%s' to spawn host '%s'", h.attachment.VolumeID, h.hostID))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/hosts/{host_id}/detach

type detachVolumeHandler struct {
	env    evergreen.Environment
	hostID string

	attachment *host.VolumeAttachment
}

func makeDetachVolume(env evergreen.Environment) gimlet.RouteHandler {
	return &detachVolumeHandler{
		env: env,
	}
}

func (h *detachVolumeHandler) Factory() gimlet.RouteHandler {
	return &detachVolumeHandler{
		env: h.env,
	}
}

func (h *detachVolumeHandler) Parse(ctx context.Context, r *http.Request) error {
	h.attachment = &host.VolumeAttachment{}
	if err := utility.ReadJSON(r.Body, h.attachment); err != nil {
		return errors.Wrap(err, "reading volume attachment from JSON request body")
	}

	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return errors.Wrap(err, "invalid host ID")
	}

	return nil
}

func (h *detachVolumeHandler) Run(ctx context.Context) gimlet.Responder {
	user := MustHaveUser(ctx)
	targetHost, err := data.FindHostByIdWithOwner(h.hostID, user)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' with owner '%s'", h.hostID, user.Id))
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
			Message:    fmt.Sprintf("attachment '%s' is not attached to host '%s", h.attachment.VolumeID, h.hostID),
		})
	}

	grip.Info(message.Fields{
		"message": "detaching volume from spawn host",
		"host_id": h.hostID,
		"volume":  h.attachment.VolumeID,
	})
	mgrOpts, err := cloud.GetManagerOptions(targetHost.Distro)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting cloud manager options"))
	}
	mgr, err := cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting cloud manager"))
	}

	if err = mgr.DetachVolume(ctx, targetHost, h.attachment.VolumeID); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "detaching volume '%s' from spawn host '%s'", h.attachment.VolumeID, h.hostID))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/volumes

type createVolumeHandler struct {
	env evergreen.Environment

	volume   *host.Volume
	provider string
}

func makeCreateVolume(env evergreen.Environment) gimlet.RouteHandler {
	return &createVolumeHandler{
		env: env,
	}
}

func (h *createVolumeHandler) Factory() gimlet.RouteHandler {
	return &createVolumeHandler{
		env: h.env,
	}
}

func (h *createVolumeHandler) Parse(ctx context.Context, r *http.Request) error {
	h.volume = &host.Volume{}
	if err := utility.ReadJSON(r.Body, h.volume); err != nil {
		return errors.Wrap(err, "reading volume from JSON request body")
	}
	if h.volume.Size == 0 {
		return errors.New("volume size is required")
	}
	h.provider = evergreen.ProviderNameEc2OnDemand
	return nil
}

func (h *createVolumeHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)

	h.volume.CreatedBy = u.Id

	if h.volume.Type == "" {
		h.volume.Type = evergreen.DefaultEBSType
		h.volume.IOPS = cloud.Gp2EquivalentIOPSForGp3(h.volume.Size)
		h.volume.Throughput = cloud.Gp2EquivalentThroughputForGp3(h.volume.Size)
	}
	if h.volume.AvailabilityZone == "" {
		h.volume.AvailabilityZone = evergreen.DefaultEBSAvailabilityZone
	}

	if err := cloud.ValidVolumeOptions(h.volume, h.env.Settings()); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "invalid volume options"))
	}

	maxVolumeFromSettings := h.env.Settings().Providers.AWS.MaxVolumeSizePerUser
	if err := checkVolumeLimitExceeded(u.Username(), int(h.volume.Size), maxVolumeFromSettings); err != nil {
		return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "checking volume limit"))
	}

	res, err := cloud.CreateVolume(ctx, h.env, h.volume, h.provider)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "creating new volume"))
	}
	if res == nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Errorf("no volume created"))
	}
	volumeModel := &model.APIVolume{}
	volumeModel.BuildFromService(*res)

	return gimlet.NewJSONResponse(volumeModel)
}

////////////////////////////////////////////////////////////////////////
//
// DELETE /rest/v2/volumes/{volume_id}

type deleteVolumeHandler struct {
	env evergreen.Environment

	VolumeID string
	provider string
}

func makeDeleteVolume(env evergreen.Environment) gimlet.RouteHandler {
	return &deleteVolumeHandler{
		env: env,
	}
}

func (h *deleteVolumeHandler) Factory() gimlet.RouteHandler {
	return &deleteVolumeHandler{
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

	attachedHost, err := host.FindHostWithVolume(h.VolumeID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host with volume '%s'", h.VolumeID))
	}
	if attachedHost != nil {
		return gimlet.MakeJSONErrorResponder(errors.Errorf("host with volume '%s' not found", h.VolumeID))
	}

	mgrOpts := cloud.ManagerOpts{
		Provider: h.provider,
		Region:   cloud.AztoRegion(volume.AvailabilityZone),
	}
	mgr, err := cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting cloud manager"))
	}
	if err = mgr.DeleteVolume(ctx, volume); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "deleting volume"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// PATCH /rest/v2/volumes/{volume_id}

type modifyVolumeHandler struct {
	env evergreen.Environment

	provider string
	volumeID string
	opts     *model.VolumeModifyOptions
}

func makeModifyVolume(env evergreen.Environment) gimlet.RouteHandler {
	return &modifyVolumeHandler{
		env: env,
	}
}

func (h *modifyVolumeHandler) Factory() gimlet.RouteHandler {
	return &modifyVolumeHandler{
		env:  h.env,
		opts: &model.VolumeModifyOptions{},
	}
}

func (h *modifyVolumeHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	if err = utility.ReadJSON(r.Body, h.opts); err != nil {
		return errors.Wrap(err, "reading volume modification options from JSON request body")
	}
	if h.volumeID, err = validateID(gimlet.GetVars(r)["volume_id"]); err != nil {
		return errors.Wrap(err, "invalid volume ID")
	}

	h.provider = evergreen.ProviderNameEc2OnDemand

	return nil
}

func (h *modifyVolumeHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	volume, err := host.FindVolumeByID(h.volumeID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding volume '%s'", h.volumeID))
	}
	if volume == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("volume '%s' not found", h.volumeID),
		})
	}

	// Only allow users to modify their own volumes
	if u.Id != volume.CreatedBy {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusUnauthorized,
			Message:    fmt.Sprintf("not authorized to modify volume '%s'", volume.ID),
		})
	}

	if h.opts.NewName != "" {
		if err = volume.SetDisplayName(h.opts.NewName); err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "setting new volume name '%s'", h.opts.NewName))
		}
	}

	if h.opts.Size != 0 {
		sizeIncrease := h.opts.Size - volume.Size
		if sizeIncrease <= 0 {
			return gimlet.MakeJSONErrorResponder(errors.Errorf("volumes can only be sized up (current size is %d GiB)", volume.Size))
		}
		maxVolumeFromSettings := h.env.Settings().Providers.AWS.MaxVolumeSizePerUser
		if err = checkVolumeLimitExceeded(u.Username(), int(sizeIncrease), maxVolumeFromSettings); err != nil {
			return gimlet.MakeJSONErrorResponder(errors.Wrap(err, "checking volume limit"))
		}
	}

	if !utility.IsZeroTime(h.opts.Expiration) {
		if h.opts.Expiration.Before(volume.Expiration) {
			return gimlet.MakeJSONErrorResponder(errors.Errorf("cannot make expiration time earlier than current expiration %s", volume.Expiration.Format(time.RFC1123)))
		}
		if time.Until(h.opts.Expiration) > evergreen.MaxSpawnHostExpirationDurationHours {
			return gimlet.MakeJSONErrorResponder(errors.Errorf("cannot extend expiration past max expiration %s", time.Now().Add(evergreen.MaxSpawnHostExpirationDurationHours).Format(time.RFC1123)))
		}

		if h.opts.NoExpiration {
			return gimlet.MakeJSONErrorResponder(errors.New("cannot specify both an expiration time and also no expiration"))
		}
	}

	if h.opts.NoExpiration {
		if h.opts.HasExpiration {
			return gimlet.MakeJSONErrorResponder(errors.New("cannot specify both having an expiration and no expiration"))
		}
		var unexpirableVolumesForUser int
		unexpirableVolumesForUser, err = host.CountNoExpirationVolumesForUser(u.Id)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "counting number of unexpirable volumes already owned by user '%s'", u.Id))
		}
		if h.env.Settings().Spawnhost.UnexpirableVolumesPerUser-unexpirableVolumesForUser <= 0 {
			return gimlet.MakeJSONErrorResponder(errors.Errorf("user '%s' has no unexpirable volumes remaining", u.Id))
		}
	}

	mgrOpts := cloud.ManagerOpts{
		Provider: h.provider,
		Region:   cloud.AztoRegion(volume.AvailabilityZone),
	}
	var mgr cloud.Manager
	mgr, err = cloud.GetManager(ctx, h.env, mgrOpts)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting cloud manager"))
	}
	if err = mgr.ModifyVolume(ctx, volume, h.opts); err != nil {
		if cloud.ModifyVolumeBadRequest(err) {
			return gimlet.MakeJSONErrorResponder(errors.Wrapf(err, "modifying volume '%s'", volume.ID))
		}
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "modifying volume '%s'", volume.ID))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

////////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/volumes

type getVolumesHandler struct{}

func makeGetVolumes() gimlet.RouteHandler {
	return &getVolumesHandler{}
}

func (h *getVolumesHandler) Factory() gimlet.RouteHandler {
	return &getVolumesHandler{}
}

func (h *getVolumesHandler) Parse(ctx context.Context, r *http.Request) error {
	return nil
}

func (h *getVolumesHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	volumes, err := host.FindVolumesByUser(u.Username())
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding volumes for user '%s'", u.Username()))
	}

	volumeDocs := []model.APIVolume{}
	for _, v := range volumes {
		volumeDoc := model.APIVolume{}
		volumeDoc.BuildFromService(v)

		// if the volume is attached to a host, also return the host ID and volume device name
		if v.Host != "" {
			h, err := host.FindOneId(v.Host)
			if err != nil {
				return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' associated with volume '%s'", v.Host, v.ID))
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
}

func makeGetVolumeByID() gimlet.RouteHandler {
	return &getVolumeByIDHandler{}
}

func (h *getVolumeByIDHandler) Factory() gimlet.RouteHandler {
	return &getVolumeByIDHandler{}
}

func (h *getVolumeByIDHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error
	if h.volumeID, err = validateID(gimlet.GetVars(r)["volume_id"]); err != nil {
		return err
	}
	return nil
}

func (h *getVolumeByIDHandler) Run(ctx context.Context) gimlet.Responder {
	v, err := host.FindVolumeByID(h.volumeID)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding volume '%s'", h.volumeID))
	}
	if v == nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusNotFound,
			Message:    fmt.Sprintf("volume '%s' not found", h.volumeID),
		})
	}
	volumeDoc := &model.APIVolume{}
	volumeDoc.BuildFromService(*v)
	// if the volume is attached to a host, also return the host ID and volume device name
	if v.Host != "" {
		attachedHost, err := host.FindOneId(v.Host)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' for attached volume", v.Host))
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
}

func makeTerminateHostRoute() gimlet.RouteHandler {
	return &hostTerminateHandler{}
}

func (h *hostTerminateHandler) Factory() gimlet.RouteHandler {
	return &hostTerminateHandler{}
}

func (h *hostTerminateHandler) Parse(ctx context.Context, r *http.Request) error {
	var err error

	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])

	return err
}

func (h *hostTerminateHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	host, err := data.FindHostByIdWithOwner(h.hostID, u)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' with user '%s'", h.hostID, u.Id))
	}

	if host.Status == evergreen.HostTerminated {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusBadRequest,
			Message:    fmt.Sprintf("Host %s is already terminated", host.Id),
		})

	} else if host.Status == evergreen.HostUninitialized {
		if err := host.SetStatus(evergreen.HostTerminated, u.Id, fmt.Sprintf("changed by %s from API", u.Id)); err != nil {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    err.Error(),
			})
		}

	} else {
		if err := errors.WithStack(cloud.TerminateSpawnHost(ctx, evergreen.GetEnvironment(), host, u.Id, "terminated via REST API")); err != nil {
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
	env         evergreen.Environment
}

func makeHostChangePassword(env evergreen.Environment) gimlet.RouteHandler {
	return &hostChangeRDPPasswordHandler{
		env: env,
	}

}

func (h *hostChangeRDPPasswordHandler) Factory() gimlet.RouteHandler {
	return &hostChangeRDPPasswordHandler{
		env: h.env,
	}
}

func (h *hostChangeRDPPasswordHandler) Parse(ctx context.Context, r *http.Request) error {
	hostModify := model.APISpawnHostModify{}
	if err := utility.ReadJSON(utility.NewRequestReader(r), &hostModify); err != nil {
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
	host, err := data.FindHostByIdWithOwner(h.hostID, u)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' with user '%s'", h.hostID, u.Id))
	}

	if statusCode, err := cloud.SetHostRDPPassword(ctx, h.env, host, h.rdpPassword); err != nil {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: statusCode,
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
}

func makeExtendHostExpiration() gimlet.RouteHandler {
	return &hostExtendExpirationHandler{}
}

func (h *hostExtendExpirationHandler) Factory() gimlet.RouteHandler {
	return &hostExtendExpirationHandler{}
}

func (h *hostExtendExpirationHandler) Parse(ctx context.Context, r *http.Request) error {
	hostModify := model.APISpawnHostModify{}
	if err := utility.ReadJSON(utility.NewRequestReader(r), &hostModify); err != nil {
		return err
	}

	var err error
	h.hostID, err = validateID(gimlet.GetVars(r)["host_id"])
	if err != nil {
		return err
	}

	addHours, err := strconv.Atoi(utility.FromStringPtr(hostModify.AddHours))
	if err != nil {
		return errors.Wrapf(err, "additional hours to expiration '%s' is not a valid integer", utility.FromStringPtr(hostModify.AddHours))
	}
	h.addHours = time.Duration(addHours) * time.Hour

	if h.addHours <= 0 {
		return errors.New("must add a positive number of hours to the expiration")
	}
	if h.addHours > evergreen.MaxSpawnHostExpirationDurationHours {
		return errors.Errorf("cannot add more than %s to expiration", evergreen.MaxSpawnHostExpirationDurationHours)
	}

	return nil
}

func (h *hostExtendExpirationHandler) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	host, err := data.FindHostByIdWithOwner(h.hostID, u)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrapf(err, "finding host '%s' with user '%s'", h.hostID, u.Id))
	}
	if host.Status == evergreen.HostTerminated {
		return gimlet.MakeJSONErrorResponder(errors.New("cannot extend expiration of a terminated host"))
	}

	var newExp time.Time
	newExp, err = cloud.MakeExtendedSpawnHostExpiration(host, h.addHours)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "extending cloud host expiration"))
	}

	if err := host.SetExpirationTime(newExp); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "extending host expiration"))
	}

	return gimlet.NewJSONResponse(struct{}{})
}

// //////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/host/start_process
type hostStartProcesses struct {
	env evergreen.Environment

	hostIDs []string
	script  string
}

func makeHostStartProcesses(env evergreen.Environment) gimlet.RouteHandler {
	return &hostStartProcesses{
		env: env,
	}
}

func (hs *hostStartProcesses) Factory() gimlet.RouteHandler {
	return &hostStartProcesses{
		env: hs.env,
	}
}

func (hs *hostStartProcesses) Parse(ctx context.Context, r *http.Request) error {
	hostScriptOpts := model.APIHostScript{}
	if err := utility.ReadJSON(utility.NewRequestReader(r), &hostScriptOpts); err != nil {
		return errors.Wrap(err, "reading script from JSON request body")
	}
	hs.script = hostScriptOpts.Script
	hs.hostIDs = hostScriptOpts.Hosts

	return nil
}

func (hs *hostStartProcesses) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	response := gimlet.NewResponseBuilder()
	for _, hostID := range hs.hostIDs {
		h, err := data.FindHostByIdWithOwner(hostID, u)
		if err != nil {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   hostID,
				Complete: true,
				Output:   errors.Wrapf(err, "finding host '%s'", hostID).Error(),
			}), "adding data for host '%s'", hostID))
			continue
		}
		if h.Status != evergreen.HostRunning {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   hostID,
				Complete: true,
				Output:   fmt.Sprintf("can't run script on host with status '%s' because it is not running", h.Status),
			}), "adding data for host '%s'", hostID))
			continue
		}
		if !h.Distro.JasperCommunication() {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   hostID,
				Complete: true,
				Output:   fmt.Sprintf("can't run script on host of distro '%s' because it doesn't support Jasper communication", h.Distro.Id),
			}), "adding data for host '%s'", hostID))
			continue
		}

		logger, err := jasper.NewInMemoryLogger(host.OutputBufferSize)
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "creating new in-memory logger for process output"))
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
				Output:   errors.Wrap(err, "running script with Jasper").Error(),
			}), "adding data for host '%s'", hostID))
			continue
		}
		grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
			HostID:   hostID,
			Complete: false,
			ProcID:   procID,
		}), "adding data for host '%s'", hostID))
	}

	return response
}

// //////////////////////////////////////////////////////////////////////
//
// GET /rest/v2/host/get_process
type hostGetProcesses struct {
	env evergreen.Environment

	hostProcesses []model.APIHostProcess
}

func makeHostGetProcesses(env evergreen.Environment) gimlet.RouteHandler {
	return &hostGetProcesses{
		env: env,
	}
}

func (h *hostGetProcesses) Factory() gimlet.RouteHandler {
	return &hostGetProcesses{
		env: h.env,
	}
}

func (h *hostGetProcesses) Parse(ctx context.Context, r *http.Request) error {
	var err error
	hostProcesses := []model.APIHostProcess{}
	if err = utility.ReadJSON(utility.NewRequestReader(r), &hostProcesses); err != nil {
		return errors.Wrap(err, "reading host processes from JSON request body")
	}
	h.hostProcesses = hostProcesses

	return nil
}

func (h *hostGetProcesses) Run(ctx context.Context) gimlet.Responder {
	u := MustHaveUser(ctx)
	response := gimlet.NewResponseBuilder()
	for _, process := range h.hostProcesses {
		host, err := data.FindHostByIdWithOwner(process.HostID, u)
		if err != nil {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   process.HostID,
				ProcID:   process.ProcID,
				Complete: true,
				Output:   errors.Wrapf(err, "getting host '%s'", process.HostID).Error(),
			}), "adding data for process on host '%s'", process.HostID))
			continue
		}

		complete, output, err := host.GetJasperProcess(ctx, h.env, process.ProcID)
		if err != nil {
			grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
				HostID:   process.HostID,
				Complete: true,
				Output:   errors.Wrapf(err, "getting output for process '%s'", process.ProcID).Error(),
			}), "adding data for process on host '%s'", process.HostID))
			continue
		}
		grip.Error(errors.Wrapf(response.AddData(model.APIHostProcess{
			HostID:   process.HostID,
			Complete: complete,
			ProcID:   process.ProcID,
			Output:   output,
		}), "adding data for process on host '%s'", process.HostID))
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

		target := fmt.Sprintf("@%s", user.Settings.SlackUsername)
		if user.Settings.SlackMemberId != "" {
			target = user.Settings.SlackMemberId
		}

		subscriber = model.APISubscriber{
			Type:   utility.ToStringPtr(event.SlackSubscriberType),
			Target: target,
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
