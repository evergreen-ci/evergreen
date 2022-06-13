package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"runtime/debug"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/cloud"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/graphql/resolvers/util"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	werrors "github.com/pkg/errors"
)

func (r *mutationResolver) AttachVolumeToHost(ctx context.Context, volumeAndHost gqlModel.VolumeHost) (bool, error) {
	statusCode, err := cloud.AttachVolume(ctx, volumeAndHost.VolumeID, volumeAndHost.HostID)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	return statusCode == http.StatusOK, nil
}

func (r *mutationResolver) DetachVolumeFromHost(ctx context.Context, volumeID string) (bool, error) {
	statusCode, err := cloud.DetachVolume(ctx, volumeID)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	return statusCode == http.StatusOK, nil
}

func (r *mutationResolver) EditSpawnHost(ctx context.Context, spawnHost *gqlModel.EditSpawnHostInput) (*restModel.APIHost, error) {
	var v *host.Volume
	usr := util.MustHaveUser(ctx)
	h, err := host.FindOneByIdOrTag(spawnHost.HostID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding host by id: %s", err))
	}

	if !host.CanUpdateSpawnHost(h, usr) {
		return nil, gqlError.Forbidden.Send(ctx, "You are not authorized to modify this host")
	}

	opts := host.HostModifyOptions{}
	if spawnHost.DisplayName != nil {
		opts.NewName = *spawnHost.DisplayName
	}
	if spawnHost.NoExpiration != nil {
		opts.NoExpiration = spawnHost.NoExpiration
	}
	if spawnHost.Expiration != nil {
		err = h.SetExpirationTime(*spawnHost.Expiration)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while modifying spawnhost expiration time: %s", err))
		}
	}
	if spawnHost.InstanceType != nil {
		var config *evergreen.Settings
		config, err = evergreen.GetConfig()
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, "unable to retrieve server config")
		}
		allowedTypes := config.Providers.AWS.AllowedInstanceTypes

		err = cloud.CheckInstanceTypeValid(ctx, h.Distro, *spawnHost.InstanceType, allowedTypes)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error validating instance type: %s", err))
		}
		opts.InstanceType = *spawnHost.InstanceType
	}
	if spawnHost.AddedInstanceTags != nil || spawnHost.DeletedInstanceTags != nil {
		addedTags := []host.Tag{}
		deletedTags := []string{}
		for _, tag := range spawnHost.AddedInstanceTags {
			tag.CanBeModified = true
			addedTags = append(addedTags, *tag)
		}
		for _, tag := range spawnHost.DeletedInstanceTags {
			deletedTags = append(deletedTags, tag.Key)
		}
		opts.AddInstanceTags = addedTags
		opts.DeleteInstanceTags = deletedTags
	}
	if spawnHost.Volume != nil {
		v, err = host.FindVolumeByID(*spawnHost.Volume)
		if err != nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Error finding requested volume id: %s", err))
		}
		if v.AvailabilityZone != h.Zone {
			return nil, gqlError.InputValidationError.Send(ctx, "Error mounting volume to spawn host, They must be in the same availability zone.")
		}
		opts.AttachVolume = *spawnHost.Volume
	}
	if spawnHost.PublicKey != nil {
		if utility.FromBoolPtr(spawnHost.SavePublicKey) {
			if err = util.SavePublicKey(ctx, *spawnHost.PublicKey); err != nil {
				return nil, err
			}
		}
		opts.AddKey = spawnHost.PublicKey.Key
		if opts.AddKey == "" {
			opts.AddKey, err = usr.GetPublicKey(spawnHost.PublicKey.Name)
			if err != nil {
				return nil, gqlError.InputValidationError.Send(ctx, fmt.Sprintf("No matching key found for name '%s'", spawnHost.PublicKey.Name))
			}
		}
	}
	if err = cloud.ModifySpawnHost(ctx, evergreen.GetEnvironment(), h, opts); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error modifying spawn host: %s", err))
	}
	if spawnHost.ServicePassword != nil {
		_, err = cloud.SetHostRDPPassword(ctx, evergreen.GetEnvironment(), h, *spawnHost.ServicePassword)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error setting spawn host password: %s", err))
		}
	}

	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(h)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *mutationResolver) SpawnHost(ctx context.Context, spawnHostInput *gqlModel.SpawnHostInput) (*restModel.APIHost, error) {
	usr := util.MustHaveUser(ctx)
	if spawnHostInput.SavePublicKey {
		if err := util.SavePublicKey(ctx, *spawnHostInput.PublicKey); err != nil {
			return nil, err
		}
	}
	dist, err := distro.FindOneId(spawnHostInput.DistroID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while trying to find distro with id: %s, err:  `%s`", spawnHostInput.DistroID, err))
	}
	if dist == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find Distro with id: %s", spawnHostInput.DistroID))
	}

	options := &restModel.HostRequestOptions{
		DistroID:             spawnHostInput.DistroID,
		Region:               spawnHostInput.Region,
		KeyName:              spawnHostInput.PublicKey.Key,
		IsVirtualWorkstation: spawnHostInput.IsVirtualWorkStation,
		NoExpiration:         spawnHostInput.NoExpiration,
	}
	if spawnHostInput.SetUpScript != nil {
		options.SetupScript = *spawnHostInput.SetUpScript
	}
	if spawnHostInput.UserDataScript != nil {
		options.UserData = *spawnHostInput.UserDataScript
	}
	if spawnHostInput.HomeVolumeSize != nil {
		options.HomeVolumeSize = *spawnHostInput.HomeVolumeSize
	}
	if spawnHostInput.VolumeID != nil {
		options.HomeVolumeID = *spawnHostInput.VolumeID
	}
	if spawnHostInput.Expiration != nil {
		options.Expiration = spawnHostInput.Expiration
	}

	// passing an empty string taskId is okay as long as a
	// taskId is not required by other spawnHostInput parameters
	var t *task.Task
	if spawnHostInput.TaskID != nil && *spawnHostInput.TaskID != "" {
		options.TaskID = *spawnHostInput.TaskID
		if t, err = task.FindOneId(*spawnHostInput.TaskID); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error occurred finding task %s: %s", *spawnHostInput.TaskID, err.Error()))
		}
	}

	if utility.FromBoolPtr(spawnHostInput.UseProjectSetupScript) {
		if t == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, "A valid task id must be supplied when useProjectSetupScript is set to true")
		}
		options.UseProjectSetupScript = *spawnHostInput.UseProjectSetupScript
	}
	if utility.FromBoolPtr(spawnHostInput.TaskSync) {
		if t == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, "A valid task id must be supplied when taskSync is set to true")
		}
		options.TaskSync = *spawnHostInput.TaskSync
	}

	if utility.FromBoolPtr(spawnHostInput.SpawnHostsStartedByTask) {
		if t == nil {
			return nil, gqlError.ResourceNotFound.Send(ctx, "A valid task id must be supplied when SpawnHostsStartedByTask is set to true")
		}
		if err = data.CreateHostsFromTask(t, *usr, spawnHostInput.PublicKey.Key); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error spawning hosts from task: %s : %s", *spawnHostInput.TaskID, err))
		}
	}

	spawnHost, err := data.NewIntentHost(ctx, options, usr, evergreen.GetEnvironment().Settings())
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error spawning host: %s", err))
	}
	if spawnHost == nil {
		return nil, gqlError.InternalServerError.Send(ctx, "An error occurred Spawn host is nil")
	}
	apiHost := restModel.APIHost{}
	if err := apiHost.BuildFromService(spawnHost); err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *mutationResolver) SpawnVolume(ctx context.Context, spawnVolumeInput gqlModel.SpawnVolumeInput) (bool, error) {
	err := util.ValidateVolumeExpirationInput(ctx, spawnVolumeInput.Expiration, spawnVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}
	volumeRequest := host.Volume{
		AvailabilityZone: spawnVolumeInput.AvailabilityZone,
		Size:             spawnVolumeInput.Size,
		Type:             spawnVolumeInput.Type,
		CreatedBy:        util.MustHaveUser(ctx).Id,
	}
	vol, statusCode, err := cloud.RequestNewVolume(ctx, volumeRequest)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	if vol == nil {
		return false, gqlError.InternalServerError.Send(ctx, "Unable to create volume")
	}
	errorTemplate := "Volume %s has been created but an error occurred."
	var additionalOptions restModel.VolumeModifyOptions
	if spawnVolumeInput.Expiration != nil {
		var newExpiration time.Time
		newExpiration, err = restModel.FromTimePtr(spawnVolumeInput.Expiration)
		if err != nil {
			return false, gqlError.InternalServerError.Send(ctx, werrors.Wrapf(err, errorTemplate, vol.ID).Error())
		}
		additionalOptions.Expiration = newExpiration
	} else if spawnVolumeInput.NoExpiration != nil && *spawnVolumeInput.NoExpiration {
		// this value should only ever be true or nil
		additionalOptions.NoExpiration = true
	}
	err = util.ApplyVolumeOptions(ctx, *vol, additionalOptions)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to apply expiration options to volume %s: %s", vol.ID, err.Error()))
	}
	if spawnVolumeInput.Host != nil {
		statusCode, err := cloud.AttachVolume(ctx, vol.ID, *spawnVolumeInput.Host)
		if err != nil {
			return false, util.MapHTTPStatusToGqlError(ctx, statusCode, werrors.Wrapf(err, errorTemplate, vol.ID))
		}
	}
	return true, nil
}

func (r *mutationResolver) RemoveVolume(ctx context.Context, volumeID string) (bool, error) {
	statusCode, err := cloud.DeleteVolume(ctx, volumeID)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, statusCode, err)
	}
	return statusCode == http.StatusOK, nil
}

func (r *mutationResolver) UpdateSpawnHostStatus(ctx context.Context, hostID string, action gqlModel.SpawnHostStatusActions) (*restModel.APIHost, error) {
	h, err := host.FindOneByIdOrTag(hostID)
	if err != nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Error finding host by id: %s", err))
	}
	usr := util.MustHaveUser(ctx)
	env := evergreen.GetEnvironment()

	if !host.CanUpdateSpawnHost(h, usr) {
		return nil, gqlError.Forbidden.Send(ctx, "You are not authorized to modify this host")
	}

	var httpStatus int
	switch action {
	case gqlModel.SpawnHostStatusActionsStart:
		httpStatus, err = data.StartSpawnHost(ctx, env, usr, h)
	case gqlModel.SpawnHostStatusActionsStop:
		httpStatus, err = data.StopSpawnHost(ctx, env, usr, h)
	case gqlModel.SpawnHostStatusActionsTerminate:
		httpStatus, err = data.TerminateSpawnHost(ctx, env, usr, h)
	default:
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find matching status for action : %s", action))
	}
	if err != nil {
		if httpStatus == http.StatusInternalServerError {
			var parsedUrl, _ = url.Parse("/graphql/query")
			grip.Error(message.WrapError(err, message.Fields{
				"method":  "POST",
				"url":     parsedUrl,
				"code":    httpStatus,
				"action":  action,
				"request": gimlet.GetRequestID(ctx),
				"stack":   string(debug.Stack()),
			}))
		}
		return nil, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}
	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(h)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *queryResolver) MyHosts(ctx context.Context) ([]*restModel.APIHost, error) {
	usr := util.MustHaveUser(ctx)
	hosts, err := host.Find(host.ByUserWithRunningStatus(usr.Username()))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx,
			fmt.Sprintf("Error finding running hosts for user %s : %s", usr.Username(), err))
	}
	duration := time.Duration(5) * time.Minute
	timestamp := time.Now().Add(-duration) // within last 5 minutes
	recentlyTerminatedHosts, err := host.Find(host.ByUserRecentlyTerminated(usr.Username(), timestamp))
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx,
			fmt.Sprintf("Error finding recently terminated hosts for user %s : %s", usr.Username(), err))
	}
	hosts = append(hosts, recentlyTerminatedHosts...)

	var apiHosts []*restModel.APIHost
	for _, host := range hosts {
		apiHost := restModel.APIHost{}
		err = apiHost.BuildFromService(host)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building APIHost from service: %s", err.Error()))
		}
		apiHosts = append(apiHosts, &apiHost)
	}
	return apiHosts, nil
}
func (r *queryResolver) MyVolumes(ctx context.Context) ([]*restModel.APIVolume, error) {
	usr := util.MustHaveUser(ctx)
	volumes, err := host.FindSortedVolumesByUser(usr.Username())
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}
	return util.GetAPIVolumeList(volumes)
}

func (r *mutationResolver) UpdateVolume(ctx context.Context, updateVolumeInput gqlModel.UpdateVolumeInput) (bool, error) {
	volume, err := host.FindVolumeByID(updateVolumeInput.VolumeID)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding volume by id %s: %s", updateVolumeInput.VolumeID, err.Error()))
	}
	if volume == nil {
		return false, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find volume %s", volume.ID))
	}
	err = util.ValidateVolumeExpirationInput(ctx, updateVolumeInput.Expiration, updateVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}
	err = util.ValidateVolumeName(ctx, updateVolumeInput.Name)
	if err != nil {
		return false, err
	}
	var updateOptions restModel.VolumeModifyOptions
	if updateVolumeInput.NoExpiration != nil {
		if *updateVolumeInput.NoExpiration {
			// this value should only ever be true or nil
			updateOptions.NoExpiration = true
		} else {
			// this value should only ever be true or nil
			updateOptions.HasExpiration = true
		}
	}
	if updateVolumeInput.Expiration != nil {
		var newExpiration time.Time
		newExpiration, err = restModel.FromTimePtr(updateVolumeInput.Expiration)
		if err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error parsing time %s", err))
		}
		updateOptions.Expiration = newExpiration
	}
	if updateVolumeInput.Name != nil {
		updateOptions.NewName = *updateVolumeInput.Name
	}
	err = util.ApplyVolumeOptions(ctx, *volume, updateOptions)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to update volume %s: %s", volume.ID, err.Error()))
	}

	return true, nil
}
