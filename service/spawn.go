package service

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	graphql "github.com/evergreen-ci/evergreen/graphql"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/rest/route"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

var (
	HostRename                 = "changeHostDisplayName"
	HostPasswordUpdate         = "updateRDPPassword"
	HostInstanceTypeUpdate     = "updateInstanceType"
	HostTagUpdate              = "updateHostTags"
	HostExpirationExtension    = "extendHostExpiration"
	HostTerminate              = "terminate"
	HostStop                   = "stop"
	HostStart                  = "start"
	VolumeRename               = "changeVolumeDisplayName"
	VolumeExtendExpiration     = "extendVolumeExpiration"
	VolumeSetNoExpiration      = "setVolumeNoExpiration"
	VolumeSetHasExpiration     = "setVolumeHasExpiration"
	VolumeAttach               = "attachVolume"
	VolumeDetach               = "detachVolume"
	VolumeDelete               = "deleteVolume"
	MaxExpirationDurationHours = 24 * 7 // 7 days
)

func (uis *UIServer) spawnPage(w http.ResponseWriter, r *http.Request) {
	var spawnDistro distro.Distro
	var spawnTask *task.Task
	var err error

	currentUser := MustHaveUser(r)

	hasQueryParams := false
	spruceQueryParams := "/host?spawnHost=True"
	if len(r.FormValue("distro_id")) > 0 {
		spruceQueryParams += fmt.Sprintf("&distroId=%s", r.FormValue("distro_id"))
		hasQueryParams = true
	}
	if len(r.FormValue("task_id")) > 0 {
		spruceQueryParams += fmt.Sprintf("&taskId=%s", r.FormValue("task_id"))
		hasQueryParams = true
	}
	spruceLink := fmt.Sprintf("%s/spawn", uis.Settings.Ui.UIv2Url)
	if hasQueryParams {
		spruceLink += spruceQueryParams
	}
	// Redirect the user to spruce only if they aren't visiting this page from spruce already and have spruce enabled
	if currentUser.Settings.UseSpruceOptions.SpruceV1 && !strings.Contains(r.Referer(), uis.Settings.Ui.UIv2Url) {
		http.Redirect(w, r, spruceLink, http.StatusTemporaryRedirect)
		return
	}

	if len(r.FormValue("distro_id")) > 0 {
		var dat distro.AliasLookupTable
		dat, err = distro.NewDistroAliasesLookupTable()
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error getting distro lookup table"))
			return
		}
		// Make a best-effort attempt to find a matching distro, but don't error
		// if we can't find one.
		for _, distroID := range dat.Expand([]string{r.FormValue("distro_id")}) {
			spawnDistro, err = distro.FindOne(distro.ById(distroID))
			if err == nil {
				break
			}
		}
	}
	var setupScriptPath string
	if len(r.FormValue("task_id")) > 0 {
		spawnTask, err = task.FindOneId(r.FormValue("task_id"))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError,
				errors.Wrapf(err, "Error finding task '%s'", r.FormValue("task_id")))
			return
		}
		if spawnTask == nil {
			uis.LoggedError(w, r, http.StatusBadRequest,
				errors.Errorf("can't find task '%s'", r.FormValue("task_id")))
			return
		}
		// if we can't find the setup script path, don't fail the request
		var pRef *model.ProjectRef
		pRef, err = model.GetProjectRefForTask(spawnTask.Id)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message":    "project can't be found",
				"project_id": spawnTask.Project,
				"task_id":    spawnTask.Id,
			}))
		} else {
			setupScriptPath = pRef.SpawnHostScriptPath
		}
	}
	maxHosts := evergreen.DefaultMaxSpawnHostsPerUser
	settings, err := evergreen.GetConfig()
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error retrieving settings"))
		return
	}
	if settings.Spawnhost.SpawnHostsPerUser >= 0 {
		maxHosts = settings.Spawnhost.SpawnHostsPerUser
	}

	newUILink := ""
	if len(uis.Settings.Ui.UIv2Url) > 0 {
		newUILink = spruceLink
	}

	uis.render.WriteResponse(w, http.StatusOK, struct {
		Distro                       distro.Distro
		Task                         *task.Task
		MaxHostsPerUser              int
		MaxUnexpirableHostsPerUser   int
		MaxUnexpirableVolumesPerUser int
		MaxVolumeSizePerUser         int
		SetupScriptPath              string
		NewUILink                    string
		ViewData
	}{spawnDistro, spawnTask, maxHosts, settings.Spawnhost.UnexpirableHostsPerUser, settings.Spawnhost.UnexpirableVolumesPerUser, settings.Providers.AWS.MaxVolumeSizePerUser,
		setupScriptPath, newUILink, uis.GetCommonViewData(w, r, false, true)}, "base", "spawned_hosts.html", "base_angular.html", "menu.html")
}

func (uis *UIServer) getSpawnedHosts(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)

	hosts, err := host.Find(host.ByUserWithRunningStatus(user.Username()))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "Error finding running hosts for user %v", user.Username()))
		return
	}

	gimlet.WriteJSON(w, hosts)
}

func (uis *UIServer) getUserPublicKeys(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	gimlet.WriteJSON(w, user.PublicKeys())
}

func (uis *UIServer) getAllowedInstanceTypes(w http.ResponseWriter, r *http.Request) {
	hostId := r.FormValue("host_id")
	h, err := host.FindOneByIdOrTag(hostId)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError,
			errors.Wrapf(err, "Error finding host '%s'", hostId))
		return
	}
	if h == nil {
		uis.LoggedError(w, r, http.StatusBadRequest,
			errors.Errorf("Host '%s' not found", hostId))
		return
	}
	if cloud.IsEc2Provider(h.Provider) {
		allowedTypes := uis.Settings.Providers.AWS.AllowedInstanceTypes
		// add the original instance type to the list if applicable
		if len(h.Distro.ProviderSettingsList) > 0 {
			originalInstanceType, ok := h.Distro.ProviderSettingsList[0].Lookup("instance_type").StringValueOK()
			if ok && originalInstanceType != "" && !utility.StringSliceContains(allowedTypes, originalInstanceType) {
				allowedTypes = append(allowedTypes, originalInstanceType)
			}
		}

		gimlet.WriteJSON(w, allowedTypes)
		return
	}
	gimlet.WriteJSON(w, []string{})
}

func (uis *UIServer) listSpawnableDistros(w http.ResponseWriter, r *http.Request) {
	// load in the distros
	distros, err := distro.Find(distro.BySpawnAllowed().WithFields(distro.IdKey, distro.IsVirtualWorkstationKey, distro.IsClusterKey, distro.ProviderSettingsListKey))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error loading distros"))
		return
	}

	distroList := []map[string]interface{}{}
	for _, d := range distros {
		regions := d.GetRegionsList(uis.Settings.Providers.AWS.AllowedRegions)
		distroList = append(distroList, map[string]interface{}{
			"name":                        d.Id,
			"virtual_workstation_allowed": d.IsVirtualWorkstation,
			"is_cluster":                  d.IsCluster,
			"regions":                     regions,
		})
	}
	gimlet.WriteJSON(w, distroList)
}

func (uis *UIServer) getVolumes(w http.ResponseWriter, r *http.Request) {
	user := MustHaveUser(r)
	volumes, err := graphql.GetMyVolumes(user)

	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "error getting volumes for '%s'", user.Username()))
		return
	}

	gimlet.WriteJSON(w, volumes)
}

func (uis *UIServer) requestNewHost(w http.ResponseWriter, r *http.Request) {
	authedUser := MustHaveUser(r)

	putParams := struct {
		Task                  string     `json:"task_id"`
		TaskSync              bool       `json:"task_sync"`
		Distro                string     `json:"distro"`
		KeyName               string     `json:"key_name"`
		PublicKey             string     `json:"public_key"`
		SaveKey               bool       `json:"save_key"`
		UserData              string     `json:"userdata"`
		SetupScript           string     `json:"setup_script"`
		UseProjectSetupScript bool       `json:"use_project_setup_script"`
		UseTaskConfig         bool       `json:"use_task_config"`
		IsVirtualWorkstation  bool       `json:"is_virtual_workstation"`
		IsCluster             bool       `json:"is_cluster"`
		NoExpiration          bool       `json:"no_expiration"`
		HomeVolumeSize        int        `json:"home_volume_size"`
		HomeVolumeID          string     `json:"home_volume_id"`
		InstanceTags          []host.Tag `json:"instance_tags"`
		InstanceType          string     `json:"instance_type"`
		Region                string     `json:"region"`
	}{}

	err := utility.ReadJSON(util.NewRequestReader(r), &putParams)
	if err != nil {
		http.Error(w, fmt.Sprintf("Bad json in request: %v", err), http.StatusBadRequest)
		return
	}

	if putParams.IsVirtualWorkstation && putParams.Task != "" {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New("cannot request a spawn host as a virtual workstation and load task data"))
		return
	}

	// save the supplied public key if needed
	if putParams.SaveKey {
		if err = authedUser.AddPublicKey(putParams.KeyName, putParams.PublicKey); err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error saving public key"))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Public key successfully saved."))
	}
	hc := &data.DBConnector{}
	options := &restModel.HostRequestOptions{
		DistroID:              putParams.Distro,
		Region:                putParams.Region,
		KeyName:               putParams.PublicKey,
		TaskID:                putParams.Task,
		TaskSync:              putParams.TaskSync,
		SetupScript:           putParams.SetupScript,
		UseProjectSetupScript: putParams.UseProjectSetupScript,
		UserData:              putParams.UserData,
		InstanceTags:          putParams.InstanceTags,
		InstanceType:          putParams.InstanceType,
		IsVirtualWorkstation:  putParams.IsVirtualWorkstation,
		IsCluster:             putParams.IsCluster,
		NoExpiration:          putParams.NoExpiration,
		HomeVolumeSize:        putParams.HomeVolumeSize,
		HomeVolumeID:          putParams.HomeVolumeID,
	}
	ctx, cancel := uis.env.Context()
	defer cancel()
	ctx = gimlet.AttachUser(ctx, authedUser)
	spawnHost, err := hc.NewIntentHost(ctx, options, authedUser, &uis.Settings)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error spawning host"))
		return
	}
	if spawnHost == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("Spawned host is nil"))
		return
	}
	if putParams.UseTaskConfig {
		t, err := task.FindOne(task.ById(putParams.Task))
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("Error finding task"))
			return
		}
		if t == nil {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("task not found"))
			return
		}
		err = hc.CreateHostsFromTask(t, *authedUser, putParams.PublicKey)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "error creating hosts from task",
				"task":    t.Id,
			}))
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.New("Error creating hosts from task"))
			return
		}
	}

	PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host spawned"))
	gimlet.WriteJSON(w, "Host successfully spawned")
}

func (uis *UIServer) modifySpawnHost(w http.ResponseWriter, r *http.Request) {
	u := MustHaveUser(r)
	updateParams := restModel.APISpawnHostModify{}
	ctx := r.Context()

	if err := utility.ReadJSON(util.NewRequestReader(r), &updateParams); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	hostId := utility.FromStringPtr(updateParams.HostID)
	h, err := host.FindOne(host.ById(hostId))
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "error finding host with id %v", hostId))
		return
	}
	if h == nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Errorf("No host with id %v found", hostId))
		return
	}

	if !graphql.CanUpdateSpawnHost(h, u) {
		uis.LoggedError(w, r, http.StatusUnauthorized, errors.New("not authorized to modify this host"))
		return
	}

	if updateParams.Action == nil {
		http.Error(w, "no action specified", http.StatusBadRequest)
		return
	}
	// determine what action needs to be taken
	switch *updateParams.Action {
	case HostTerminate:
		var cancel func()
		ctx, cancel = context.WithCancel(r.Context())
		defer cancel()
		_, _, err = graphql.TerminateSpawnHost(ctx, evergreen.GetEnvironment(), h, u, r)
		if err != nil {
			gimlet.WriteJSONError(w, err)
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host terminated"))
		gimlet.WriteJSON(w, "Host terminated")
		return

	case HostStop:
		_, _, err = graphql.StopSpawnHost(ctx, evergreen.GetEnvironment(), h, u, r)
		if err != nil {
			gimlet.WriteJSONError(w, err)
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host stopping"))
		gimlet.WriteJSON(w, "Host stopping")
		return

	case HostStart:
		_, _, err = graphql.StartSpawnHost(ctx, evergreen.GetEnvironment(), h, u, r)
		if err != nil {
			gimlet.WriteJSONError(w, err)
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host starting"))
		gimlet.WriteJSON(w, "Host starting")
		return

	case HostPasswordUpdate:
		pwd := utility.FromStringPtr(updateParams.RDPPwd)
		_, _, err = graphql.UpdateHostPassword(ctx, evergreen.GetEnvironment(), h, u, pwd, r)
		if err != nil {
			gimlet.WriteJSONError(w, err)
		}
		gimlet.WriteJSON(w, "Successfully updated host password")
		return

	case HostInstanceTypeUpdate:
		instanceType := utility.FromStringPtr(updateParams.InstanceType)
		if err = cloud.ModifySpawnHost(ctx, uis.env, h, host.HostModifyOptions{
			InstanceType: instanceType,
		}); err != nil {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash("Error modifying host instance type"))
			uis.LoggedError(w, r, http.StatusInternalServerError, err)
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Instance type successfully set to '%s'", instanceType)))
		gimlet.WriteJSON(w, "Successfully update host instance type")
		return

	case HostExpirationExtension:
		if updateParams.Expiration == nil || updateParams.Expiration.IsZero() { // set expiration to never expire
			var settings *evergreen.Settings
			settings, err = evergreen.GetConfig()
			if err != nil {
				PushFlash(uis.CookieStore, r, w, NewErrorFlash("Error updating host expiration"))
				uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error retrieving settings"))
				return
			}
			if err = route.CheckUnexpirableHostLimitExceeded(u.Id, settings.Spawnhost.UnexpirableHostsPerUser); err != nil {
				PushFlash(uis.CookieStore, r, w, NewErrorFlash(err.Error()))
				uis.LoggedError(w, r, http.StatusBadRequest, err)
				return
			}
			noExpiration := true
			if err = cloud.ModifySpawnHost(ctx, uis.env, h, host.HostModifyOptions{NoExpiration: &noExpiration}); err != nil {
				PushFlash(uis.CookieStore, r, w, NewErrorFlash("Error updating host expiration"))
				uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error extending host expiration"))
				return
			}
			PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Host expiration successfully set to never expire"))
			gimlet.WriteJSON(w, "Successfully updated host to never expire")
			return
		}
		// use now as a base for how far we're extending if there is currently no expiration
		if h.NoExpiration {
			h.ExpirationTime = time.Now()
		}
		if updateParams.Expiration.Before(h.ExpirationTime) {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash("Expiration can only be extended."))
			uis.LoggedError(w, r, http.StatusBadRequest, errors.New("expiration can only be extended"))
			return
		}

		addtTime := updateParams.Expiration.Sub(h.ExpirationTime)
		var futureExpiration time.Time
		futureExpiration, err = cloud.MakeExtendedSpawnHostExpiration(h, addtTime)
		if err != nil {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash(err.Error()))
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}
		if err = h.SetExpirationTime(futureExpiration); err != nil {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash("Error updating host expiration time"))
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrap(err, "Error extending host expiration time"))
			return
		}

		var loc *time.Location
		loc, err = time.LoadLocation(u.Settings.Timezone)
		if err != nil || loc == nil {
			loc = time.UTC
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprintf("Host expiration successfully set to %s",
			futureExpiration.In(loc).Format(time.RFC822))))
		gimlet.WriteJSON(w, "Successfully extended host expiration time")
		return
	case HostTagUpdate:
		if len(updateParams.AddTags) <= 0 && len(updateParams.DeleteTags) <= 0 {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash("Nothing to update."))
			uis.LoggedError(w, r, http.StatusBadRequest, err)
			return
		}

		deleteTags := utility.FromStringPtrSlice(updateParams.DeleteTags)
		addTagPairs := utility.FromStringPtrSlice(updateParams.AddTags)
		var addTags []host.Tag
		addTags, err = host.MakeHostTags(addTagPairs)
		if err != nil {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash("Error creating tags to add: "+err.Error()))
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Wrapf(err, "Error creating tags to add"))
			return
		}

		opts := host.HostModifyOptions{
			AddInstanceTags:    addTags,
			DeleteInstanceTags: deleteTags,
		}
		if err = cloud.ModifySpawnHost(ctx, uis.env, h, opts); err != nil {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash("Problem modifying spawn host"))
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "Problem modifying spawn host"))
			return
		}
		PushFlash(uis.CookieStore, r, w, NewSuccessFlash(fmt.Sprint("Host tags successfully modified.")))
		gimlet.WriteJSON(w, "Successfully updated host tags.")
		return
	case HostRename:
		if err = h.SetDisplayName(utility.FromStringPtr(updateParams.NewName)); err != nil {
			PushFlash(uis.CookieStore, r, w, NewErrorFlash("Error updating display name"))
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "Problem renaming spawn host"))
			return
		}
	default:
		http.Error(w, fmt.Sprintf("Unrecognized action: %v", updateParams.Action), http.StatusBadRequest)
		return
	}
}

func (uis *UIServer) requestNewVolume(w http.ResponseWriter, r *http.Request) {
	volume := &host.Volume{}
	if err := utility.ReadJSON(util.NewRequestReader(r), volume); err != nil {
		http.Error(w, fmt.Sprintf("Bad json in request: %v", err), http.StatusBadRequest)
		return
	}
	if volume.Size == 0 {
		http.Error(w, "Size is required", http.StatusBadRequest)
		return
	}
	if volume.AvailabilityZone == "" {
		volume.AvailabilityZone = evergreen.DefaultEBSAvailabilityZone
	}
	if volume.Type == "" {
		volume.Type = evergreen.DefaultEBSType
	}
	_, httpStatusCode, _, err, _ := graphql.RequestNewVolume(r.Context(), *volume)
	if err != nil {
		uis.LoggedError(w, r, httpStatusCode, err)
		return
	}
	PushFlash(uis.CookieStore, r, w, NewSuccessFlash("Volume Created"))
	gimlet.WriteJSON(w, "Volume successfully created")
	return
}

func (uis *UIServer) modifyVolume(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	updateParams := restModel.APIVolumeModify{}
	if err := utility.ReadJSON(util.NewRequestReader(r), &updateParams); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
	}

	volumeID := gimlet.GetVars(r)["volume_id"]
	vol, err := host.FindVolumeByID(volumeID)
	if err != nil {
		uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "error finding volume '%s'", volumeID))
		return
	}
	if vol == nil {
		uis.LoggedError(w, r, http.StatusNotFound, errors.Wrapf(err, "no volume '%s' exists", volumeID))
		return
	}

	u := MustHaveUser(r)
	if u.Username() != vol.CreatedBy {
		uis.LoggedError(w, r, http.StatusUnauthorized, errors.New("not authorized to modify this volume"))
		return
	}

	if updateParams.Action == nil {
		uis.LoggedError(w, r, http.StatusBadRequest, errors.New("no action specified"))
		return
	}

	// take the specified action
	switch *updateParams.Action {
	case VolumeRename:
		uis.LoggedError(w, r, http.StatusUnauthorized, errors.Wrapf(vol.SetDisplayName(*updateParams.NewName), "can't set display name of '%s' to '%s'", vol.ID, *updateParams.NewName))

	case VolumeExtendExpiration:
		if updateParams.Expiration == nil {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "must specify an expiration time"))
			return
		}
		mgrOpts := cloud.ManagerOpts{
			Provider: evergreen.ProviderNameEc2OnDemand,
			Region:   cloud.AztoRegion(vol.AvailabilityZone),
		}
		var mgr cloud.Manager
		mgr, err = cloud.GetManager(ctx, uis.env, mgrOpts)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "can't get manager for volume '%s'", vol.ID))
			return
		}

		var newExpiration time.Time
		newExpiration, err = restModel.FromTimePtr(updateParams.Expiration)
		if err != nil {
			uis.LoggedError(w, r, http.StatusBadRequest, errors.Wrap(err, "can't parse new expiration time"))
			return
		}
		err = mgr.ModifyVolume(ctx, vol, &restModel.VolumeModifyOptions{
			Expiration: newExpiration,
		})
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "can't update volume '%s' expiration", vol.ID))
			return
		}

	case VolumeSetNoExpiration:
		mgrOpts := cloud.ManagerOpts{
			Provider: evergreen.ProviderNameEc2OnDemand,
			Region:   cloud.AztoRegion(vol.AvailabilityZone),
		}
		mgr, err := cloud.GetManager(ctx, uis.env, mgrOpts)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "can't get manager for volume '%s'", vol.ID))
			return
		}
		err = mgr.ModifyVolume(ctx, vol, &restModel.VolumeModifyOptions{
			NoExpiration: true,
		})
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "can't update volume '%s' to no expiration", vol.ID))
			return
		}

	case VolumeSetHasExpiration:
		mgrOpts := cloud.ManagerOpts{
			Provider: evergreen.ProviderNameEc2OnDemand,
			Region:   cloud.AztoRegion(vol.AvailabilityZone),
		}
		mgr, err := cloud.GetManager(ctx, uis.env, mgrOpts)
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "can't get manager for volume '%s'", vol.ID))
			return
		}
		err = mgr.ModifyVolume(ctx, vol, &restModel.VolumeModifyOptions{
			HasExpiration: true,
		})
		if err != nil {
			uis.LoggedError(w, r, http.StatusInternalServerError, errors.Wrapf(err, "can't update volume '%s' to have expiration", vol.ID))
			return
		}

	case VolumeAttach:
		_, httpStatusCode, _, err := graphql.AttachVolume(ctx, vol.ID, *updateParams.HostID)
		if err != nil {
			uis.LoggedError(w, r, httpStatusCode, err)
			return
		}

	case VolumeDetach:
		_, httpStatusCode, _, err := graphql.DetachVolume(ctx, vol.ID)
		if err != nil {
			uis.LoggedError(w, r, httpStatusCode, err)
			return
		}

	case VolumeDelete:
		_, httpStatusCode, _, err := graphql.DeleteVolume(ctx, vol.ID)
		if err != nil {
			uis.LoggedError(w, r, httpStatusCode, err)
			return
		}
	}
}
