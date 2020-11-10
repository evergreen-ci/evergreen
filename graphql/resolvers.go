package graphql

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/evergreen-ci/evergreen/plugin"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mitchellh/mapstructure"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/api"
	"github.com/evergreen-ci/evergreen/apimodels"
	"github.com/evergreen-ci/evergreen/cloud"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/commitqueue"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/pkg/errors"
	"github.com/vektah/gqlparser/v2/gqlerror"
	"gopkg.in/mgo.v2/bson"
)

type Resolver struct {
	sc data.Connector
}

func (r *Resolver) Mutation() MutationResolver {
	return &mutationResolver{r}
}
func (r *Resolver) Patch() PatchResolver {
	return &patchResolver{r}
}
func (r *Resolver) Query() QueryResolver {
	return &queryResolver{r}
}
func (r *Resolver) Task() TaskResolver {
	return &taskResolver{r}
}
func (r *Resolver) Host() HostResolver {
	return &hostResolver{r}
}
func (r *Resolver) Volume() VolumeResolver {
	return &volumeResolver{r}
}
func (r *Resolver) TaskQueueItem() TaskQueueItemResolver {
	return &taskQueueItemResolver{r}
}

type hostResolver struct{ *Resolver }
type mutationResolver struct{ *Resolver }
type taskQueueItemResolver struct{ *Resolver }
type volumeResolver struct{ *Resolver }

func (r *hostResolver) DistroID(ctx context.Context, obj *restModel.APIHost) (*string, error) {
	return obj.Distro.Id, nil
}

func (r *hostResolver) Uptime(ctx context.Context, obj *restModel.APIHost) (*time.Time, error) {
	return obj.CreationTime, nil
}

func (r *hostResolver) Elapsed(ctx context.Context, obj *restModel.APIHost) (*time.Time, error) {
	return obj.RunningTask.StartTime, nil
}

func (r *hostResolver) Volumes(ctx context.Context, obj *restModel.APIHost) ([]*restModel.APIVolume, error) {
	volumes := make([]*restModel.APIVolume, 0, len(obj.AttachedVolumeIDs))
	for _, volId := range obj.AttachedVolumeIDs {
		volume, err := r.sc.FindVolumeById(volId)
		if err != nil {
			return volumes, InternalServerError.Send(ctx, fmt.Sprintf("Error getting volume %s", volId))
		}
		if volume == nil {
			continue
		}
		apiVolume := &restModel.APIVolume{}
		err = apiVolume.BuildFromService(volume)
		if err != nil {
			return nil, errors.Wrapf(err, "error building volume '%s' from service", volId)
		}
		volumes = append(volumes, apiVolume)
	}

	return volumes, nil
}

func (r *volumeResolver) Host(ctx context.Context, obj *restModel.APIVolume) (*restModel.APIHost, error) {
	if obj.HostID == nil || *obj.HostID == "" {
		return nil, nil
	}
	host, err := r.sc.FindHostById(*obj.HostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding host %s: %s", *obj.HostID, err.Error()))
	}
	if host == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find host %s", *obj.HostID))
	}
	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(host)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost %s from service: %s", host.Id, err))
	}
	return &apiHost, nil
}

func (r *queryResolver) MyPublicKeys(ctx context.Context) ([]*restModel.APIPubKey, error) {
	publicKeys := getMyPublicKeys(ctx)
	return publicKeys, nil
}

func (r *taskResolver) AbortInfo(ctx context.Context, at *restModel.APITask) (*AbortInfo, error) {
	if at.Aborted != true {
		return nil, nil
	}

	info := AbortInfo{
		User:   &at.AbortInfo.User,
		TaskID: &at.AbortInfo.TaskID,
	}

	abortedTask, err := task.FindOneId(at.AbortInfo.TaskID)
	if err != nil {
		return &info, InternalServerError.Send(ctx, fmt.Sprintf("Problem getting aborted task %s: %s", *at.Id, err.Error()))
	}
	if abortedTask == nil {
		return &info, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find aborted task %s: %s", at.AbortInfo.TaskID, err.Error()))
	}

	abortedTaskBuild, err := build.FindOneId(abortedTask.BuildId)
	if err != nil {
		return &info, InternalServerError.Send(ctx, fmt.Sprintf("Problem getting build for aborted task %s: %s", abortedTask.BuildId, err.Error()))
	}
	if abortedTaskBuild == nil {
		return &info, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find build %s for aborted task: %s", abortedTask.BuildId, err.Error()))
	}

	info.TaskDisplayName = &abortedTask.DisplayName
	info.BuildVariantDisplayName = &abortedTaskBuild.DisplayName

	return &info, nil
}

func (r *taskResolver) ReliesOn(ctx context.Context, at *restModel.APITask) ([]*Dependency, error) {
	dependencies := []*Dependency{}
	if len(at.DependsOn) == 0 {
		return dependencies, nil
	}
	depIds := []string{}
	for _, dep := range at.DependsOn {
		depIds = append(depIds, dep.TaskId)
	}

	dependencyTasks, err := task.Find(task.ByIds(depIds).WithFields(task.DisplayNameKey, task.StatusKey,
		task.ActivatedKey, task.BuildVariantKey, task.DetailsKey, task.DependsOnKey))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Cannot find dependency tasks for task %s: %s", *at.Id, err.Error()))
	}

	taskMap := map[string]*task.Task{}
	for i := range dependencyTasks {
		taskMap[dependencyTasks[i].Id] = &dependencyTasks[i]
	}

	i, err := at.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *at.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *at.Id))
	}

	for _, dep := range at.DependsOn {
		depTask, ok := taskMap[dep.TaskId]
		if !ok {
			continue
		}
		var metStatus MetStatus
		if !depTask.IsFinished() {
			metStatus = "PENDING"
		} else if t.SatisfiesDependency(depTask) {
			metStatus = "MET"
		} else {
			metStatus = "UNMET"
		}
		var requiredStatus RequiredStatus
		switch dep.Status {
		case model.AllStatuses:
			requiredStatus = "MUST_FINISH"
			break
		case evergreen.TaskFailed:
			requiredStatus = "MUST_FAIL"
			break
		default:
			requiredStatus = "MUST_SUCCEED"
		}

		dependency := Dependency{
			Name:           depTask.DisplayName,
			BuildVariant:   depTask.BuildVariant,
			MetStatus:      metStatus,
			RequiredStatus: requiredStatus,
			UILink:         fmt.Sprintf("/task/%s", depTask.Id),
		}

		dependencies = append(dependencies, &dependency)
	}
	return dependencies, nil
}

func (r *mutationResolver) AddFavoriteProject(ctx context.Context, identifier string) (*restModel.UIProjectFields, error) {
	p, err := model.FindOneProjectRef(identifier)
	if err != nil || p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", identifier))
	}

	usr := MustHaveUser(ctx)

	err = usr.AddFavoritedProject(identifier)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	return &restModel.UIProjectFields{
		DisplayName: p.DisplayName,
		Identifier:  p.Identifier,
		Repo:        p.Repo,
		Owner:       p.Owner,
	}, nil
}

func (r *mutationResolver) RemoveFavoriteProject(ctx context.Context, identifier string) (*restModel.UIProjectFields, error) {
	p, err := model.FindOneProjectRef(identifier)
	if err != nil || p == nil {
		return nil, &gqlerror.Error{
			Message: fmt.Sprintln("Could not find proj", identifier),
			Extensions: map[string]interface{}{
				"code": "RESOURCE_NOT_FOUND",
			},
		}

	}

	usr := MustHaveUser(ctx)

	err = usr.RemoveFavoriteProject(identifier)
	if err != nil {
		return nil, &gqlerror.Error{
			Message: fmt.Sprintln("Error removing project", identifier),
			Extensions: map[string]interface{}{
				"code": "INTERNAL_SERVER_ERROR",
			},
		}
	}

	return &restModel.UIProjectFields{
		DisplayName: p.DisplayName,
		Identifier:  p.Identifier,
		Repo:        p.Repo,
		Owner:       p.Owner,
	}, nil
}

func (r *mutationResolver) SpawnVolume(ctx context.Context, spawnVolumeInput SpawnVolumeInput) (bool, error) {
	err := validateVolumeExpirationInput(ctx, spawnVolumeInput.Expiration, spawnVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}
	success, _, gqlErr, err, vol := RequestNewVolume(ctx, GetVolumeFromSpawnVolumeInput(spawnVolumeInput))
	if err != nil {
		return false, gqlErr.Send(ctx, err.Error())
	}
	if vol == nil {
		return false, InternalServerError.Send(ctx, "Unable to create volume")
	}
	errorTemplate := "Volume %s has been created but an error occurred."
	var additionalOptions restModel.VolumeModifyOptions
	if spawnVolumeInput.Expiration != nil {
		var newExpiration time.Time
		newExpiration, err = restModel.FromTimePtr(spawnVolumeInput.Expiration)
		if err != nil {
			return false, gqlErr.Send(ctx, errors.Wrapf(err, errorTemplate, vol.ID).Error())
		}
		additionalOptions.Expiration = newExpiration
	} else if spawnVolumeInput.NoExpiration != nil && *spawnVolumeInput.NoExpiration == true {
		// this value should only ever be true or nil
		additionalOptions.NoExpiration = true
	}
	err = applyVolumeOptions(ctx, *vol, additionalOptions)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Unable to apply expiration options to volume %s: %s", vol.ID, err.Error()))
	}
	if spawnVolumeInput.Host != nil {
		_, _, gqlErr, err := AttachVolume(ctx, vol.ID, *spawnVolumeInput.Host)
		if err != nil {
			return false, gqlErr.Send(ctx, errors.Wrapf(err, errorTemplate, vol.ID).Error())
		}
	}

	return success, nil
}

func (r *mutationResolver) UpdateVolume(ctx context.Context, updateVolumeInput UpdateVolumeInput) (bool, error) {
	volume, err := r.sc.FindVolumeById(updateVolumeInput.VolumeID)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Error finding volume by id %s: %s", updateVolumeInput.VolumeID, err.Error()))
	}
	if volume == nil {
		return false, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find volume %s", volume.ID))
	}
	err = validateVolumeExpirationInput(ctx, updateVolumeInput.Expiration, updateVolumeInput.NoExpiration)
	if err != nil {
		return false, err
	}
	err = validateVolumeName(ctx, updateVolumeInput.Name)
	if err != nil {
		return false, err
	}
	var updateOptions restModel.VolumeModifyOptions
	if updateVolumeInput.NoExpiration != nil {
		if *updateVolumeInput.NoExpiration == true {
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
			return false, InternalServerError.Send(ctx, fmt.Sprintf("Error parsing time %s", err))
		}
		updateOptions.Expiration = newExpiration
	}
	if updateVolumeInput.Name != nil {
		updateOptions.NewName = *updateVolumeInput.Name
	}
	err = applyVolumeOptions(ctx, *volume, updateOptions)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Unable to update volume %s: %s", volume.ID, err.Error()))
	}

	return true, nil
}

func (r *mutationResolver) SpawnHost(ctx context.Context, spawnHostInput *SpawnHostInput) (*restModel.APIHost, error) {
	usr := MustHaveUser(ctx)
	if spawnHostInput.SavePublicKey {
		err := savePublicKey(ctx, *spawnHostInput.PublicKey)
		if err != nil {
			return nil, err
		}
	}
	dist, err := distro.FindByID(spawnHostInput.DistroID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while trying to find distro with id: %s, err:  `%s`", spawnHostInput.DistroID, err))
	}
	if dist == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find Distro with id: %s", spawnHostInput.DistroID))
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

	if util.IsPtrSetToTrue(spawnHostInput.UseProjectSetupScript) {
		if spawnHostInput.TaskID == nil {
			return nil, ResourceNotFound.Send(ctx, "A valid task id must be supplied when useProjectSetupScript is set to true")
		}
		options.UseProjectSetupScript = *spawnHostInput.UseProjectSetupScript
		options.TaskID = *spawnHostInput.TaskID
	}

	hc := &data.DBConnector{}

	if util.IsPtrSetToTrue(spawnHostInput.SpawnHostsStartedByTask) {
		if spawnHostInput.TaskID == nil {
			return nil, ResourceNotFound.Send(ctx, "A valid task id must be supplied when SpawnHostsStartedByTask is set to true")
		}
		var t *task.Task
		t, err = task.FindOneId(*spawnHostInput.TaskID)
		if err != nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Error finding Task with id: %s : %s", *spawnHostInput.TaskID, err))
		}
		if t == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Task with id: %s was not found", *spawnHostInput.TaskID))
		}
		err = hc.CreateHostsFromTask(t, *usr, spawnHostInput.PublicKey.Key)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error spawning hosts from task: %s : %s", *spawnHostInput.TaskID, err))
		}
	}

	spawnHost, err := hc.NewIntentHost(ctx, options, usr, evergreen.GetEnvironment().Settings())
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error spawning host: %s", err))
	}
	if spawnHost == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("An error occurred Spawn host is nil"))
	}
	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(spawnHost)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *mutationResolver) EditSpawnHost(ctx context.Context, editSpawnHostInput *EditSpawnHostInput) (*restModel.APIHost, error) {
	usr := MustHaveUser(ctx)
	h, err := host.FindOneByIdOrTag(editSpawnHostInput.HostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding host by id: %s", err))
	}

	if !CanUpdateSpawnHost(h, usr) {
		return nil, Forbidden.Send(ctx, "You are not authorized to modify this host")
	}

	opts := host.HostModifyOptions{}
	if editSpawnHostInput.DisplayName != nil {
		opts.NewName = *editSpawnHostInput.DisplayName
	}
	if editSpawnHostInput.NoExpiration != nil {
		opts.NoExpiration = editSpawnHostInput.NoExpiration
	}
	if editSpawnHostInput.Expiration != nil {
		err = h.SetExpirationTime(*editSpawnHostInput.Expiration)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while modifying spawnhost expiration time: %s", err))
		}
	}
	if editSpawnHostInput.InstanceType != nil {
		var config *evergreen.Settings
		config, err = evergreen.GetConfig()
		if err != nil {
			return nil, InternalServerError.Send(ctx, "unable to retrieve server config")
		}
		allowedTypes := config.Providers.AWS.AllowedInstanceTypes

		err = cloud.CheckInstanceTypeValid(ctx, h.Distro, *editSpawnHostInput.InstanceType, allowedTypes)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error validating instance type: %s", err))
		}
		opts.InstanceType = *editSpawnHostInput.InstanceType
	}
	if editSpawnHostInput.AddedInstanceTags != nil || editSpawnHostInput.DeletedInstanceTags != nil {
		addedTags := []host.Tag{}
		deletedTags := []string{}
		for _, tag := range editSpawnHostInput.AddedInstanceTags {
			tag.CanBeModified = true
			addedTags = append(addedTags, *tag)
		}
		for _, tag := range editSpawnHostInput.DeletedInstanceTags {
			deletedTags = append(deletedTags, tag.Key)
		}
		opts.AddInstanceTags = addedTags
		opts.DeleteInstanceTags = deletedTags
	}
	if editSpawnHostInput.Volume != nil {
		opts.AttachVolume = *editSpawnHostInput.Volume
	}
	if err = cloud.ModifySpawnHost(ctx, evergreen.GetEnvironment(), h, opts); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error modifying spawn host: %s", err))
	}

	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(h)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
	}
	return &apiHost, nil
}

func (r *mutationResolver) UpdateSpawnHostStatus(ctx context.Context, hostID string, action SpawnHostStatusActions) (*restModel.APIHost, error) {
	host, err := host.FindOneByIdOrTag(hostID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Error finding host by id: %s", err))
	}
	usr := MustHaveUser(ctx)
	env := evergreen.GetEnvironment()

	if !CanUpdateSpawnHost(host, usr) {
		return nil, Forbidden.Send(ctx, "You are not authorized to modify this host")
	}

	switch action {
	case SpawnHostStatusActionsStart:
		h, httpStatus, err := StartSpawnHost(ctx, env, host, usr, nil)
		if err != nil {
			return nil, mapHTTPStatusToGqlError(ctx, httpStatus, err)
		}
		apiHost := restModel.APIHost{}
		err = apiHost.BuildFromService(h)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
		}
		return &apiHost, nil
	case SpawnHostStatusActionsStop:
		h, httpStatus, err := StopSpawnHost(ctx, env, host, usr, nil)
		if err != nil {
			return nil, mapHTTPStatusToGqlError(ctx, httpStatus, err)
		}
		apiHost := restModel.APIHost{}
		err = apiHost.BuildFromService(h)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
		}
		return &apiHost, nil
	case SpawnHostStatusActionsTerminate:
		h, httpStatus, err := TerminateSpawnHost(ctx, env, host, usr, nil)
		if err != nil {
			return nil, mapHTTPStatusToGqlError(ctx, httpStatus, err)
		}
		apiHost := restModel.APIHost{}
		err = apiHost.BuildFromService(h)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost from service: %s", err))
		}
		return &apiHost, nil
	default:
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find matching status for action : %s", action))
	}

}

type queryResolver struct{ *Resolver }

func (r *queryResolver) Hosts(ctx context.Context, hostID *string, distroID *string, currentTaskID *string, statuses []string, startedBy *string, sortBy *HostSortBy, sortDir *SortDirection, page *int, limit *int) (*HostsResponse, error) {
	hostIDParam := ""
	if hostID != nil {
		hostIDParam = *hostID
	}
	distroParam := ""
	if distroID != nil {
		distroParam = *distroID
	}
	currentTaskParam := ""
	if currentTaskID != nil {
		currentTaskParam = *currentTaskID
	}
	startedByParam := ""
	if startedBy != nil {
		startedByParam = *startedBy
	}
	sorter := host.StatusKey
	if sortBy != nil {
		switch *sortBy {
		case HostSortByCurrentTask:
			sorter = host.RunningTaskKey
			break
		case HostSortByDistro:
			sorter = host.DistroKey
			break
		case HostSortByElapsed:
			sorter = "task_full.start_time"
			break
		case HostSortByID:
			sorter = host.IdKey
			break
		case HostSortByIDLeTime:
			sorter = host.TotalIdleTimeKey
			break
		case HostSortByOwner:
			sorter = host.StartedByKey
			break
		case HostSortByStatus:
			sorter = host.StatusKey
			break
		case HostSortByUptime:
			sorter = host.CreateTimeKey
			break
		default:
			sorter = host.StatusKey
			break
		}

	}
	sortDirParam := 1
	if *sortDir == SortDirectionDesc {
		sortDirParam = -1
	}
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}

	hosts, filteredHostsCount, totalHostsCount, err := host.GetPaginatedRunningHosts(hostIDParam, distroParam, currentTaskParam, statuses, startedByParam, sorter, sortDirParam, pageParam, limitParam)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting hosts: %s", err.Error()))
	}

	apiHosts := []*restModel.APIHost{}

	for _, host := range hosts {
		apiHost := restModel.APIHost{}

		err = apiHost.BuildFromService(host)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building API Host from Service: %s", err.Error()))
		}

		if host.RunningTask != "" {
			// Add the task information to the host document.
			if err = apiHost.BuildFromService(host.RunningTaskFull); err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting from host.Host to model.APIHost: %s", err.Error()))
			}
		}

		apiHosts = append(apiHosts, &apiHost)
	}

	return &HostsResponse{
		Hosts:              apiHosts,
		FilteredHostsCount: filteredHostsCount,
		TotalHostsCount:    totalHostsCount,
	}, nil
}

func (r *queryResolver) Host(ctx context.Context, hostID string) (*restModel.APIHost, error) {
	host, err := host.GetHostByIdWithTask(hostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error Fetching host: %s", err.Error()))
	}
	if host == nil {
		return nil, errors.Errorf("unable to find host %s", hostID)
	}

	apiHost := &restModel.APIHost{}
	err = apiHost.BuildFromService(host)
	if err != nil || apiHost == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting from host.Host to model.APIHost: %s", err.Error()))
	}

	if host.RunningTask != "" {
		// Add the task information to the host document.
		if err = apiHost.BuildFromService(host.RunningTaskFull); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting from host.Host to model.APIHost: %s", err.Error()))
		}
	}

	return apiHost, nil
}

func (r *queryResolver) MyVolumes(ctx context.Context) ([]*restModel.APIVolume, error) {
	volumes, err := GetMyVolumes(MustHaveUser(ctx))
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	volumePointers := make([]*restModel.APIVolume, 0, len(volumes))
	for i, _ := range volumes {
		volumePointers = append(volumePointers, &volumes[i])
	}
	return volumePointers, nil
}

func (r *queryResolver) MyHosts(ctx context.Context) ([]*restModel.APIHost, error) {
	usr := MustHaveUser(ctx)
	hosts, err := host.Find(host.ByUserWithRunningStatus(usr.Username()))
	if err != nil {
		return nil, InternalServerError.Send(ctx,
			fmt.Sprintf("Error finding running hosts for user %s : %s", usr.Username(), err))
	}
	var apiHosts []*restModel.APIHost

	for _, host := range hosts {
		apiHost := restModel.APIHost{}
		err = apiHost.BuildFromService(host)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIHost from service: %s", err.Error()))
		}
		apiHosts = append(apiHosts, &apiHost)
	}
	return apiHosts, nil
}

func (r *mutationResolver) AttachVolumeToHost(ctx context.Context, volumeAndHost VolumeHost) (bool, error) {
	success, _, gqlErr, err := AttachVolume(ctx, volumeAndHost.VolumeID, volumeAndHost.HostID)
	if err != nil {
		return false, gqlErr.Send(ctx, err.Error())
	}
	return success, nil
}

func (r *mutationResolver) DetachVolumeFromHost(ctx context.Context, volumeID string) (bool, error) {
	success, _, gqlErr, err := DetachVolume(ctx, volumeID)
	if err != nil {
		return false, gqlErr.Send(ctx, err.Error())
	}
	return success, nil
}

type patchResolver struct{ *Resolver }

func (r *patchResolver) CommitQueuePosition(ctx context.Context, apiPatch *restModel.APIPatch) (*int, error) {
	var commitQueuePosition *int
	if *apiPatch.Alias == evergreen.CommitQueueAlias {
		cq, err := commitqueue.FindOneId(*apiPatch.ProjectId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting commit queue position for patch %s: %s", *apiPatch.Id, err.Error()))
		}
		if cq != nil {
			position := cq.FindItem(*apiPatch.Id)
			commitQueuePosition = &position
		}
	}
	return commitQueuePosition, nil
}

func (r *patchResolver) TaskStatuses(ctx context.Context, obj *restModel.APIPatch) ([]string, error) {
	tasks, _, err := r.sc.FindTasksByVersion(*obj.Id, task.DisplayNameKey, []string{}, "", "", 1, 0, 0, []string{task.DisplayStatusKey})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version tasks: %s", err.Error()))
	}
	return getAllTaskStatuses(tasks), nil
}

func (r *patchResolver) BaseTaskStatuses(ctx context.Context, obj *restModel.APIPatch) ([]string, error) {
	baseTasks, err := getVersionBaseTasks(r.sc, *obj.Id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting version base tasks: %s", err.Error()))
	}
	return getAllTaskStatuses(baseTasks), nil
}

func (r *patchResolver) Builds(ctx context.Context, obj *restModel.APIPatch) ([]*restModel.APIBuild, error) {
	builds, err := build.FindBuildsByVersions([]string{*obj.Version})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding build by version %s: %s", *obj.Version, err.Error()))
	}
	var apiBuilds []*restModel.APIBuild
	for _, build := range builds {
		apiBuild := restModel.APIBuild{}
		err = apiBuild.BuildFromService(build)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIBuild from service: %s", err.Error()))
		}
		apiBuilds = append(apiBuilds, &apiBuild)
	}
	return apiBuilds, nil
}

func (r *patchResolver) Duration(ctx context.Context, obj *restModel.APIPatch) (*PatchDuration, error) {
	tasks, err := task.FindAllFirstExecution(task.ByVersion(*obj.Id).WithFields(task.TimeTakenKey, task.StartTimeKey, task.FinishTimeKey, task.DisplayOnlyKey, task.ExecutionKey))
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	if tasks == nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	timeTaken, makespan := task.GetTimeSpent(tasks)

	// return nil if rounded timeTaken/makespan == 0s
	t := timeTaken.Round(time.Second).String()
	var tPointer *string
	if t != "0s" {
		tFormated := formatDuration(t)
		tPointer = &tFormated
	}
	m := makespan.Round(time.Second).String()
	var mPointer *string
	if m != "0s" {
		mFormated := formatDuration(m)
		mPointer = &mFormated
	}

	return &PatchDuration{
		Makespan:  mPointer,
		TimeTaken: tPointer,
	}, nil
}

func (r *patchResolver) Time(ctx context.Context, obj *restModel.APIPatch) (*PatchTime, error) {
	usr := MustHaveUser(ctx)

	started, err := GetFormattedDate(obj.StartTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	finished, err := GetFormattedDate(obj.FinishTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	submittedAt, err := GetFormattedDate(obj.CreateTime, usr.Settings.Timezone)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}

	return &PatchTime{
		Started:     started,
		Finished:    finished,
		SubmittedAt: *submittedAt,
	}, nil
}

func (r *patchResolver) TaskCount(ctx context.Context, obj *restModel.APIPatch) (*int, error) {
	taskCount, err := task.Count(task.ByVersion(*obj.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting task count for patch %s: %s", *obj.Id, err.Error()))
	}
	return &taskCount, nil
}

func (r *patchResolver) BaseVersionID(ctx context.Context, obj *restModel.APIPatch) (*string, error) {
	baseVersion, err := model.VersionFindOne(model.BaseVersionByProjectIdAndRevision(*obj.ProjectId, *obj.Githash).Project(bson.M{model.VersionIdentifierKey: 1}))
	if baseVersion == nil || err != nil {
		return nil, nil
	}
	return &baseVersion.Id, nil
}

func (r *patchResolver) Project(ctx context.Context, apiPatch *restModel.APIPatch) (*PatchProject, error) {
	patchProject, err := GetPatchProjectVariantsAndTasksForUI(ctx, apiPatch)
	if err != nil {
		return nil, err
	}
	return patchProject, nil
}

func (r *patchResolver) ID(ctx context.Context, obj *restModel.APIPatch) (string, error) {
	return *obj.Id, nil
}

func (r *queryResolver) Patch(ctx context.Context, id string) (*restModel.APIPatch, error) {
	patch, err := r.sc.FindPatchById(id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return patch, nil
}

func (r *queryResolver) UserSettings(ctx context.Context) (*restModel.APIUserSettings, error) {
	usr := MustHaveUser(ctx)
	userSettings := restModel.APIUserSettings{}
	err := userSettings.BuildFromService(usr.Settings)
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return &userSettings, nil
}

func (r *queryResolver) UserPatches(ctx context.Context, limit *int, page *int, patchName *string, statuses []string, userID *string, includeCommitQueue *bool) (*UserPatches, error) {
	usr := MustHaveUser(ctx)
	userIdParam := usr.Username()
	if userID != nil {
		userIdParam = *userID
	}
	patches, count, err := r.sc.FindPatchesByUserPatchNameStatusesCommitQueue(userIdParam, *patchName, statuses, *includeCommitQueue, *page, *limit)
	patchPointers := []*restModel.APIPatch{}
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	for i := range patches {
		patchPointers = append(patchPointers, &patches[i])
	}
	userPatches := UserPatches{
		Patches:            patchPointers,
		FilteredPatchCount: *count,
	}
	return &userPatches, nil
}

func (r *queryResolver) Task(ctx context.Context, taskID string, execution *int) (*restModel.APITask, error) {
	dbTask, err := task.FindByIdExecution(taskID, execution)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if dbTask == nil {
		return nil, errors.Errorf("unable to find task %s", taskID)
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *dbTask)
	if err != nil {
		return nil, InternalServerError.Send(ctx, "error converting task")
	}
	start, err := model.GetEstimatedStartTime(*dbTask)
	if err != nil {
		return nil, InternalServerError.Send(ctx, "error getting estimated start time")
	}
	apiTask.EstimatedStart = restModel.NewAPIDuration(start)
	return apiTask, err
}

func (r *queryResolver) TaskAllExecutions(ctx context.Context, taskID string) ([]*restModel.APITask, error) {
	latestTask, err := task.FindOneId(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}
	if latestTask == nil {
		return nil, errors.Errorf("unable to find task %s", taskID)
	}
	allTasks := []*restModel.APITask{}
	for i := 0; i < latestTask.Execution; i++ {
		var dbTask *task.Task
		dbTask, err = task.FindByIdExecution(taskID, &i)
		if err != nil {
			return nil, ResourceNotFound.Send(ctx, err.Error())
		}
		if dbTask == nil {
			return nil, errors.Errorf("unable to find task %s", taskID)
		}
		var apiTask *restModel.APITask
		apiTask, err = GetAPITaskFromTask(ctx, r.sc, *dbTask)
		if err != nil {
			return nil, InternalServerError.Send(ctx, "error converting task")
		}
		allTasks = append(allTasks, apiTask)
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *latestTask)
	if err != nil {
		return nil, InternalServerError.Send(ctx, "error converting task")
	}
	allTasks = append(allTasks, apiTask)
	return allTasks, nil
}

func (r *queryResolver) Projects(ctx context.Context) (*Projects, error) {
	allProjs, err := model.FindAllTrackedProjectRefs()
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	usr := MustHaveUser(ctx)
	groupsMap := make(map[string][]*restModel.UIProjectFields)
	favorites := []*restModel.UIProjectFields{}

	for _, p := range allProjs {
		groupName := strings.Join([]string{p.Owner, p.Repo}, "/")

		uiProj := restModel.UIProjectFields{
			DisplayName: p.DisplayName,
			Identifier:  p.Identifier,
			Repo:        p.Repo,
			Owner:       p.Owner,
		}

		// favorite projects are filtered out and appended to their own array
		if utility.StringSliceContains(usr.FavoriteProjects, p.Identifier) {
			favorites = append(favorites, &uiProj)
			continue
		}
		if projs, ok := groupsMap[groupName]; ok {
			groupsMap[groupName] = append(projs, &uiProj)
		} else {
			groupsMap[groupName] = []*restModel.UIProjectFields{&uiProj}
		}
	}

	groupsArr := []*GroupedProjects{}

	for groupName, groupedProjects := range groupsMap {
		gp := GroupedProjects{
			Name:     groupName,
			Projects: groupedProjects,
		}
		groupsArr = append(groupsArr, &gp)
	}

	sort.SliceStable(groupsArr, func(i, j int) bool {
		return groupsArr[i].Name < groupsArr[j].Name
	})

	pjs := Projects{
		Favorites:     favorites,
		OtherProjects: groupsArr,
	}

	return &pjs, nil
}

func (r *queryResolver) PatchTasks(ctx context.Context, patchID string, sortBy *TaskSortCategory, sortDir *SortDirection, page *int, limit *int, statuses []string, baseStatuses []string, variant *string, taskName *string) (*PatchTasks, error) {
	sorter := ""
	if sortBy != nil {
		switch *sortBy {
		case TaskSortCategoryStatus:
			sorter = task.DisplayStatusKey
			break
		case TaskSortCategoryName:
			sorter = task.DisplayNameKey
			break
		case TaskSortCategoryBaseStatus:
			// base status is not a field on the task db model; therefore sorting by base status
			// cannot be done in the mongo query. sorting by base status is done in the resolver.
			break
		case TaskSortCategoryVariant:
			sorter = task.BuildVariantKey
			break
		default:
			break
		}
	}
	sortDirParam := 1
	if *sortDir == SortDirectionDesc {
		sortDirParam = -1
	}
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}
	statusesParam := []string{}
	if statuses != nil {
		statusesParam = statuses
	}
	variantParam := ""
	if variant != nil {
		variantParam = *variant
	}
	taskNameParam := ""
	if taskName != nil {
		taskNameParam = *taskName
	}
	tasks, count, err := r.sc.FindTasksByVersion(patchID, sorter, statusesParam, variantParam, taskNameParam, sortDirParam, pageParam, limitParam, []string{})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting patch tasks for %s: %s", patchID, err.Error()))
	}
	baseTaskStatuses, _ := GetBaseTaskStatusesFromPatchID(r.sc, patchID)
	taskResults := ConvertDBTasksToGqlTasks(tasks, baseTaskStatuses)

	if *sortBy == TaskSortCategoryBaseStatus {
		sort.SliceStable(taskResults, func(i, j int) bool {
			if sortDirParam == 1 {
				return taskResults[i].BaseStatus < taskResults[j].BaseStatus
			}
			return taskResults[i].BaseStatus > taskResults[j].BaseStatus
		})
	}

	if len(baseStatuses) > 0 {
		// tasks cannot be filtered by base status through a DB query. tasks are filtered by base status here.
		allTasks, err := task.Find(task.ByVersion(patchID))
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting tasks for patch %s: %s", patchID, err.Error()))
		}
		taskResults = FilterTasksByBaseStatuses(taskResults, baseStatuses, baseTaskStatuses)
		if count > 0 {
			// calculate filtered task count by base status
			allTasksGql := ConvertDBTasksToGqlTasks(allTasks, baseTaskStatuses)
			count = len(FilterTasksByBaseStatuses(allTasksGql, baseStatuses, baseTaskStatuses))
		}
	}

	patchTasks := PatchTasks{
		Count: count,
		Tasks: taskResults,
	}
	return &patchTasks, nil
}

func (r *queryResolver) TaskTests(ctx context.Context, taskID string, execution *int, sortCategory *TestSortCategory, sortDirection *SortDirection, page *int, limit *int, testName *string, statuses []string) (*TaskTestResult, error) {
	dbTask, err := task.FindByIdExecution(taskID, execution)
	if dbTask == nil || err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	baseTask, err := dbTask.FindTaskOnBaseCommit()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding base task with id %s: %s", taskID, err))
	}

	baseTestStatusMap := make(map[string]string)
	if baseTask != nil {
		baseTestResults, _ := r.sc.FindTestsByTaskId(baseTask.Id, "", "", "", 0, 0)
		for _, t := range baseTestResults {
			baseTestStatusMap[t.TestFile] = t.Status
		}
	}

	sortBy := ""
	if sortCategory != nil {
		switch *sortCategory {
		case TestSortCategoryStatus:
			sortBy = testresult.StatusKey
			break
		case TestSortCategoryDuration:
			sortBy = "duration"
			break
		case TestSortCategoryTestName:
			sortBy = testresult.TestFileKey
		}
	}

	sortDir := 1
	if sortDirection != nil {
		switch *sortDirection {
		case SortDirectionDesc:
			sortDir = -1
			break
		}
	}

	if *sortDirection == SortDirectionDesc {
		sortDir = -1
	}

	testNameParam := ""
	if testName != nil {
		testNameParam = *testName
	}
	pageParam := 0
	if page != nil {
		pageParam = *page
	}
	limitParam := 0
	if limit != nil {
		limitParam = *limit
	}
	statusesParam := []string{}
	if statuses != nil {
		statusesParam = statuses
	}
	paginatedFilteredTests, err := r.sc.FindTestsByTaskIdFilterSortPaginate(taskID, testNameParam, statusesParam, sortBy, sortDir, pageParam, limitParam, dbTask.Execution)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, err.Error())
	}

	testPointers := []*restModel.APITest{}
	for _, t := range paginatedFilteredTests {
		apiTest := restModel.APITest{}
		buildErr := apiTest.BuildFromService(&t)
		if buildErr != nil {
			return nil, InternalServerError.Send(ctx, buildErr.Error())
		}
		if apiTest.Logs.HTMLDisplayURL != nil && IsURL(*apiTest.Logs.HTMLDisplayURL) == false {
			formattedURL := fmt.Sprintf("%s%s", r.sc.GetURL(), *apiTest.Logs.HTMLDisplayURL)
			apiTest.Logs.HTMLDisplayURL = &formattedURL
		}
		if apiTest.Logs.RawDisplayURL != nil && IsURL(*apiTest.Logs.RawDisplayURL) == false {
			formattedURL := fmt.Sprintf("%s%s", r.sc.GetURL(), *apiTest.Logs.RawDisplayURL)
			apiTest.Logs.RawDisplayURL = &formattedURL
		}
		baseTestStatus := baseTestStatusMap[*apiTest.TestFile]
		apiTest.BaseStatus = &baseTestStatus
		testPointers = append(testPointers, &apiTest)
	}

	totalTestCount, err := r.sc.GetTestCountByTaskIdAndFilters(taskID, "", []string{}, dbTask.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting total test count: %s", err.Error()))
	}
	filteredTestCount, err := r.sc.GetTestCountByTaskIdAndFilters(taskID, testNameParam, statusesParam, dbTask.Execution)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting filtered test count: %s", err.Error()))
	}

	taskTestResult := TaskTestResult{
		TestResults:       testPointers,
		TotalTestCount:    totalTestCount,
		FilteredTestCount: filteredTestCount,
	}

	return &taskTestResult, nil
}

func (r *queryResolver) TaskFiles(ctx context.Context, taskID string, execution *int) (*TaskFiles, error) {
	emptyTaskFiles := TaskFiles{
		FileCount:    0,
		GroupedFiles: []*GroupedFiles{},
	}
	t, err := task.FindByIdExecution(taskID, execution)
	if t == nil {
		return &emptyTaskFiles, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	if err != nil {
		return &emptyTaskFiles, ResourceNotFound.Send(ctx, err.Error())
	}
	groupedFilesList := []*GroupedFiles{}
	fileCount := 0
	if t.DisplayOnly {
		execTasks, err := task.Find(task.ByIds(t.ExecutionTasks))
		if err != nil {
			return &emptyTaskFiles, ResourceNotFound.Send(ctx, err.Error())
		}
		for _, execTask := range execTasks {
			groupedFiles, err := GetGroupedFiles(ctx, execTask.DisplayName, execTask.Id, t.Execution)
			if err != nil {
				return &emptyTaskFiles, err
			}
			fileCount += len(groupedFiles.Files)
			groupedFilesList = append(groupedFilesList, groupedFiles)
		}
	} else {
		groupedFiles, err := GetGroupedFiles(ctx, t.DisplayName, taskID, t.Execution)
		if err != nil {
			return &emptyTaskFiles, err
		}
		fileCount += len(groupedFiles.Files)
		groupedFilesList = append(groupedFilesList, groupedFiles)
	}
	taskFiles := TaskFiles{
		FileCount:    fileCount,
		GroupedFiles: groupedFilesList,
	}
	return &taskFiles, nil
}

func (r *queryResolver) TaskLogs(ctx context.Context, taskID string) (*RecentTaskLogs, error) {
	const logMessageCount = 100
	var loggedEvents []event.EventLogEntry
	// loggedEvents is ordered ts descending
	loggedEvents, err := event.Find(event.AllLogCollection, event.MostRecentTaskEvents(taskID, logMessageCount))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to find EventLogs for task %s: %s", taskID, err.Error()))
	}

	// remove all scheduled events except the youngest and push to filteredEvents
	filteredEvents := []event.EventLogEntry{}
	foundScheduled := false
	for i := 0; i < len(loggedEvents); i++ {
		if foundScheduled == false || loggedEvents[i].EventType != event.TaskScheduled {
			filteredEvents = append(filteredEvents, loggedEvents[i])
		}
		if loggedEvents[i].EventType == event.TaskScheduled {
			foundScheduled = true
		}
	}

	// reverse order so ts is ascending
	for i := len(filteredEvents)/2 - 1; i >= 0; i-- {
		opp := len(filteredEvents) - 1 - i
		filteredEvents[i], filteredEvents[opp] = filteredEvents[opp], filteredEvents[i]
	}

	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.TaskAPIEventLogEntry{}
	for _, e := range filteredEvents {
		apiEventLog := restModel.TaskAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(&e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}

	// need to task to get project id
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	// need project to get default logger
	p, err := r.sc.FindProjectById(t.Project)
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", t.Project))
	}

	defaultLogger := p.DefaultLogger
	if defaultLogger == "" {
		defaultLogger = evergreen.GetEnvironment().Settings().LoggerConfig.DefaultLogger
	}

	taskLogs := []apimodels.LogMessage{}
	systemLogs := []apimodels.LogMessage{}
	agentLogs := []apimodels.LogMessage{}
	// get logs from cedar
	if defaultLogger == model.BuildloggerLogSender {
		opts := apimodels.GetBuildloggerLogsOptions{
			BaseURL:       evergreen.GetEnvironment().Settings().Cedar.BaseURL,
			TaskID:        taskID,
			Execution:     t.Execution,
			PrintPriority: true,
			Tail:          logMessageCount,
			LogType:       apimodels.TaskLogPrefix,
		}
		// task logs
		taskLogReader, blErr := apimodels.GetBuildloggerLogs(ctx, opts)
		if blErr != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		taskLogs = apimodels.ReadBuildloggerToSlice(ctx, taskID, taskLogReader)
		// system logs
		opts.LogType = apimodels.SystemLogPrefix
		systemLogReader, blErr := apimodels.GetBuildloggerLogs(ctx, opts)
		if blErr != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		systemLogs = apimodels.ReadBuildloggerToSlice(ctx, taskID, systemLogReader)
		// agent logs
		opts.LogType = apimodels.AgentLogPrefix
		agentLogReader, blErr := apimodels.GetBuildloggerLogs(ctx, opts)
		if blErr != nil {
			return nil, InternalServerError.Send(ctx, err.Error())
		}
		agentLogs = apimodels.ReadBuildloggerToSlice(ctx, taskID, agentLogReader)
	} else {
		// task logs
		taskLogs, err = model.FindMostRecentLogMessages(taskID, t.Execution, logMessageCount, []string{},
			[]string{apimodels.TaskLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task logs for task %s: %s", taskID, err.Error()))
		}
		// system logs
		systemLogs, err = model.FindMostRecentLogMessages(taskID, t.Execution, logMessageCount, []string{},
			[]string{apimodels.SystemLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding system logs for task %s: %s", taskID, err.Error()))
		}
		// agent logs
		agentLogs, err = model.FindMostRecentLogMessages(taskID, t.Execution, logMessageCount, []string{},
			[]string{apimodels.AgentLogPrefix})
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding agent logs for task %s: %s", taskID, err.Error()))
		}
	}
	taskLogPointers := []*apimodels.LogMessage{}
	systemLogPointers := []*apimodels.LogMessage{}
	agentLogPointers := []*apimodels.LogMessage{}
	for i := range taskLogs {
		taskLogPointers = append(taskLogPointers, &taskLogs[i])
	}
	for i := range systemLogs {
		systemLogPointers = append(systemLogPointers, &systemLogs[i])
	}
	for i := range agentLogs {
		agentLogPointers = append(agentLogPointers, &agentLogs[i])
	}
	return &RecentTaskLogs{EventLogs: apiEventLogPointers, TaskLogs: taskLogPointers, AgentLogs: agentLogPointers, SystemLogs: systemLogPointers}, nil
}

func (r *queryResolver) PatchBuildVariants(ctx context.Context, patchID string) ([]*PatchBuildVariant, error) {
	patch, err := r.sc.FindPatchById(patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding patch `%s`: %s", patchID, err))
	}

	var tasksByVariant map[string][]*PatchBuildVariantTask = map[string][]*PatchBuildVariantTask{}
	for _, variant := range patch.Variants {
		tasksByVariant[*variant] = []*PatchBuildVariantTask{}
	}
	tasks, _, err := r.sc.FindTasksByVersion(patchID, task.DisplayNameKey, []string{}, "", "", 1, 0, 0, []string{})
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting tasks for patch `%s`: %s", patchID, err))
	}
	variantDisplayName := make(map[string]string)
	for _, task := range tasks {
		t := PatchBuildVariantTask{
			ID:     task.Id,
			Name:   task.DisplayName,
			Status: task.GetDisplayStatus(),
		}
		tasksByVariant[task.BuildVariant] = append(tasksByVariant[task.BuildVariant], &t)
		if _, ok := variantDisplayName[task.BuildVariant]; !ok {
			build, err := r.sc.FindBuildById(task.BuildId)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting build for task `%s`: %s", task.BuildId, err))
			}
			variantDisplayName[task.BuildVariant] = build.DisplayName
		}

	}

	result := []*PatchBuildVariant{}
	for variant, tasks := range tasksByVariant {
		pbv := PatchBuildVariant{
			Variant:     variant,
			DisplayName: variantDisplayName[variant],
			Tasks:       tasks,
		}
		result = append(result, &pbv)
	}
	// sort variants by name
	sort.SliceStable(result, func(i, j int) bool {
		return result[i].DisplayName < result[j].DisplayName
	})
	return result, nil
}

func (r *queryResolver) CommitQueue(ctx context.Context, id string) (*restModel.APICommitQueue, error) {
	commitQueue, err := r.sc.FindCommitQueueForProject(id)
	if err != nil {
		if errors.Cause(err) == err {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding commit queue for %s: %s", id, err.Error()))
		}
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding commit queue for %s: %s", id, err.Error()))
	}
	project, err := r.sc.FindProjectById(id)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding project %s: %s", id, err.Error()))
	}
	if project.CommitQueue.Message != "" {
		commitQueue.Message = &project.CommitQueue.Message
	}

	if project.CommitQueue.PatchType == commitqueue.PRPatchType {
		if len(commitQueue.Queue) > 0 {
			versionId := restModel.FromStringPtr(commitQueue.Queue[0].Version)
			if versionId != "" {
				p, err := r.sc.FindPatchById(versionId)
				if err != nil {
					return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding patch: %s", err.Error()))
				}
				commitQueue.Queue[0].Patch = p
			}
		}
		commitQueue.Owner = restModel.ToStringPtr(project.Owner)
		commitQueue.Repo = restModel.ToStringPtr(project.Repo)
	} else {
		patchIds := []string{}
		for _, item := range commitQueue.Queue {
			issue := *item.Issue
			patchIds = append(patchIds, issue)
		}
		patches, err := r.sc.FindPatchesByIds(patchIds)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding patches: %s", err.Error()))
		}
		for i := range commitQueue.Queue {
			for j := range patches {
				if *commitQueue.Queue[i].Issue == *patches[j].Id {
					commitQueue.Queue[i].Patch = &patches[j]
				}
			}
		}
	}

	return commitQueue, nil
}

func (r *queryResolver) UserConfig(ctx context.Context) (*UserConfig, error) {
	usr := MustHaveUser(ctx)
	settings := evergreen.GetEnvironment().Settings()
	config := &UserConfig{
		User:          usr.Username(),
		APIKey:        usr.GetAPIKey(),
		UIServerHost:  settings.Ui.Url,
		APIServerHost: settings.ApiUrl + "/api",
	}

	return config, nil
}

func (r *queryResolver) ClientConfig(ctx context.Context) (*restModel.APIClientConfig, error) {
	envClientConfig := evergreen.GetEnvironment().ClientConfig()
	clientConfig := restModel.APIClientConfig{}
	err := clientConfig.BuildFromService(*envClientConfig)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building APIClientConfig from service: %s", err.Error()))
	}

	return &clientConfig, nil
}

func (r *queryResolver) AwsRegions(ctx context.Context) ([]string, error) {
	return evergreen.GetEnvironment().Settings().Providers.AWS.AllowedRegions, nil
}

func (r *queryResolver) SubnetAvailabilityZones(ctx context.Context) ([]string, error) {
	zones := []string{}
	for _, subnet := range evergreen.GetEnvironment().Settings().Providers.AWS.Subnets {
		zones = append(zones, subnet.AZ)
	}
	return zones, nil
}

func (r *queryResolver) SpruceConfig(ctx context.Context) (*restModel.APIAdminSettings, error) {
	config, err := evergreen.GetConfig()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error Fetching evergreen settings: %s", err.Error()))
	}

	spruceConfig := restModel.APIAdminSettings{}
	err = spruceConfig.BuildFromService(config)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error building api admin settings from service: %s", err.Error()))
	}
	return &spruceConfig, nil
}

func (r *queryResolver) HostEvents(ctx context.Context, hostID string, hostTag *string, limit *int, page *int) (*HostEvents, error) {
	events, count, err := event.FindPaginated(hostID, *hostTag, event.AllLogCollection, *limit, *page)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error Fetching host events: %s", err.Error()))
	}
	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.HostAPIEventLogEntry{}
	for _, e := range events {
		apiEventLog := restModel.HostAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(&e)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}
	hostevents := HostEvents{
		EventLogEntries: apiEventLogPointers,
		Count:           count,
	}
	return &hostevents, nil
}

func (r *queryResolver) Distros(ctx context.Context, onlySpawnable bool) ([]*restModel.APIDistro, error) {
	apiDistros := []*restModel.APIDistro{}

	var distros []distro.Distro
	if onlySpawnable {
		d, err := distro.Find(distro.BySpawnAllowed())
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while fetching spawnable distros: %s", err.Error()))
		}
		distros = d
	} else {
		d, err := distro.FindAll()
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while fetching distros: %s", err.Error()))
		}
		distros = d
	}
	for _, d := range distros {
		apiDistro := restModel.APIDistro{}
		err := apiDistro.BuildFromService(d)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIDistro from distro: %s", err.Error()))
		}
		apiDistros = append(apiDistros, &apiDistro)
	}
	return apiDistros, nil
}

func (r *queryResolver) DistroTaskQueue(ctx context.Context, distroID string) ([]*restModel.APITaskQueueItem, error) {
	distroQueue, err := model.LoadTaskQueue(distroID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting task queue for distro %v: %v", distroID, err.Error()))
	}
	if distroQueue == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find queue with distro ID `%s`", distroID))
	}

	taskQueue := []*restModel.APITaskQueueItem{}

	for _, taskQueueItem := range distroQueue.Queue {
		apiTaskQueueItem := restModel.APITaskQueueItem{}

		err := apiTaskQueueItem.BuildFromService(taskQueueItem)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting task queue item db model to api model: %v", err.Error()))
		}

		taskQueue = append(taskQueue, &apiTaskQueueItem)
	}

	return taskQueue, nil
}

func (r *queryResolver) TaskQueueDistros(ctx context.Context) ([]*TaskQueueDistro, error) {
	queues, err := model.FindAllTaskQueues()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting all task queues: %v", err.Error()))
	}

	distros := []*TaskQueueDistro{}

	for _, distro := range queues {
		tqd := TaskQueueDistro{
			ID:         distro.Distro,
			QueueCount: len(distro.Queue),
		}
		distros = append(distros, &tqd)
	}

	// sort distros by queue count in descending order
	sort.SliceStable(distros, func(i, j int) bool {
		return distros[i].QueueCount > distros[j].QueueCount
	})

	return distros, nil
}

func (r *taskQueueItemResolver) Requester(ctx context.Context, obj *restModel.APITaskQueueItem) (TaskQueueItemType, error) {
	if *obj.Requester != evergreen.RepotrackerVersionRequester {
		return TaskQueueItemTypePatch, nil
	}
	return TaskQueueItemTypeCommit, nil
}

func (r *mutationResolver) SetTaskPriority(ctx context.Context, taskID string, priority int) (*restModel.APITask, error) {
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	authUser := gimlet.GetUser(ctx)
	if priority > evergreen.MaxTaskPriority {
		requiredPermission := gimlet.PermissionOpts{
			Resource:      t.Project,
			ResourceType:  "project",
			Permission:    evergreen.PermissionTasks,
			RequiredLevel: evergreen.TasksAdmin.Value,
		}
		isTaskAdmin := authUser.HasPermission(requiredPermission)
		if !isTaskAdmin {
			return nil, Forbidden.Send(ctx, fmt.Sprintf("Insufficient access to set priority %v, can only set priority less than or equal to %v", priority, evergreen.MaxTaskPriority))
		}
	}
	if err = model.SetTaskPriority(*t, int64(priority), authUser.Username()); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error setting task priority %v: %v", taskID, err.Error()))
	}

	t, err = r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *t)
	return apiTask, err
}

func (r *mutationResolver) SchedulePatch(ctx context.Context, patchID string, reconfigure PatchReconfigure) (*restModel.APIPatch, error) {
	patchUpdateReq := PatchVariantsTasksRequest{}
	patchUpdateReq.BuildFromGqlInput(reconfigure)
	version, err := r.sc.FindVersionById(patchID)
	if err != nil {
		// FindVersionById does not distinguish between nil version err and db err; therefore must check that err
		// does not contain nil version err values before sending InternalServerError
		if !strings.Contains(err.Error(), strconv.Itoa(http.StatusNotFound)) {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error occurred fetching patch `%s`: %s", patchID, err.Error()))
		}
	}
	err, _, _, versionID := SchedulePatch(ctx, patchID, version, patchUpdateReq)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error scheduling patch `%s`: %s", patchID, err))
	}
	scheduledPatch, err := r.sc.FindPatchById(versionID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting scheduled patch `%s`: %s", patchID, err))
	}
	return scheduledPatch, nil
}

func (r *mutationResolver) SchedulePatchTasks(ctx context.Context, patchID string) (*string, error) {
	modifications := VersionModifications{
		Action: SetActive,
		Active: true,
		Abort:  false,
	}
	err := ModifyVersionHandler(ctx, r.sc, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) UnschedulePatchTasks(ctx context.Context, patchID string, abort bool) (*string, error) {
	modifications := VersionModifications{
		Action: SetActive,
		Active: false,
		Abort:  abort,
	}
	err := ModifyVersionHandler(ctx, r.sc, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) RestartPatch(ctx context.Context, patchID string, abort bool, taskIds []string) (*string, error) {
	if len(taskIds) == 0 {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("`taskIds` array is empty. You must provide at least one task id"))
	}
	modifications := VersionModifications{
		Action:  Restart,
		Abort:   abort,
		TaskIds: taskIds,
	}
	err := ModifyVersionHandler(ctx, r.sc, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) SetPatchPriority(ctx context.Context, patchID string, priority int) (*string, error) {
	modifications := VersionModifications{
		Action:   SetPriority,
		Priority: int64(priority),
	}
	err := ModifyVersionHandler(ctx, r.sc, patchID, modifications)
	if err != nil {
		return nil, err
	}
	return &patchID, nil
}

func (r *mutationResolver) EnqueuePatch(ctx context.Context, patchID string) (*restModel.APIPatch, error) {
	user := MustHaveUser(ctx)
	hasPermission, err := r.hasEnqueuePatchPermission(user, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error getting permissions: %s", err.Error()))
	}
	if !hasPermission {
		return nil, Forbidden.Send(ctx, "can't enqueue another user's patch")
	}

	newPatch, err := r.sc.CreatePatchForMerge(ctx, patchID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error creating new patch: %s", err.Error()))
	}

	_, err = r.sc.EnqueueItem(restModel.FromStringPtr(newPatch.Project), restModel.APICommitQueueItem{Issue: newPatch.Id}, false)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error enqueuing new patch: %s", err.Error()))
	}

	return newPatch, nil
}

func (r *mutationResolver) hasEnqueuePatchPermission(u *user.DBUser, patchID string) (bool, error) {
	// patch owner
	existingPatch, err := r.sc.FindPatchById(patchID)
	if err != nil {
		return false, err
	}
	if restModel.FromStringPtr(existingPatch.Author) == u.Username() {
		return true, nil
	}

	// superuser
	permissions := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionAdminSettings,
		RequiredLevel: evergreen.AdminSettingsEdit.Value,
	}
	if u != nil && u.HasPermission(permissions) {
		return true, nil
	}

	// project admin
	projectRef, err := r.sc.FindProjectById(patchID)
	if err != nil {
		return false, err
	}
	isProjectAdmin := utility.StringSliceContains(projectRef.Admins, u.Username()) || u.HasPermission(gimlet.PermissionOpts{
		Resource:      projectRef.Id,
		ResourceType:  evergreen.ProjectResourceType,
		Permission:    evergreen.PermissionProjectSettings,
		RequiredLevel: evergreen.ProjectSettingsEdit.Value,
	})
	return isProjectAdmin, nil
}

func (r *mutationResolver) ScheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := SetScheduled(ctx, r.sc, taskID, true)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *mutationResolver) UnscheduleTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	task, err := SetScheduled(ctx, r.sc, taskID, false)
	if err != nil {
		return nil, err
	}
	return task, nil
}

func (r *mutationResolver) AbortTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	p, err := r.sc.FindProjectById(t.Project)
	if p == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find project '%s'", t.Project))
	}
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding project by id: %s: %s", t.Project, err.Error()))
	}
	user := gimlet.GetUser(ctx).DisplayName()
	err = model.AbortTask(taskID, user)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error aborting task %s: %s", taskID, err.Error()))
	}
	if t.Requester == evergreen.MergeTestRequester {
		_, err = commitqueue.RemoveCommitQueueItemForVersion(t.Project, p.CommitQueue.PatchType, t.Version, user)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to remove commit queue item for project %s, version %s: %s", taskID, t.Version, err.Error()))
		}
	}
	t, err = r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *t)
	return apiTask, err
}

func (r *mutationResolver) RestartTask(ctx context.Context, taskID string) (*restModel.APITask, error) {
	usr := MustHaveUser(ctx)
	username := usr.Username()
	if err := model.TryResetTask(taskID, username, evergreen.UIPackage, nil); err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error restarting task %s: %s", taskID, err.Error()))
	}
	t, err := r.sc.FindTaskById(taskID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("error finding task %s: %s", taskID, err.Error()))
	}
	if t == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", taskID))
	}
	apiTask, err := GetAPITaskFromTask(ctx, r.sc, *t)
	return apiTask, err
}

func (r *mutationResolver) RemoveItemFromCommitQueue(ctx context.Context, commitQueueID string, issue string) (*string, error) {
	result, err := r.sc.CommitQueueRemoveItem(commitQueueID, issue, gimlet.GetUser(ctx).DisplayName())
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error removing item %s from commit queue %s: %s",
			issue, commitQueueID, err.Error()))
	}
	if result != true {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("couldn't remove item %s from commit queue %s", issue, commitQueueID))
	}

	return &issue, nil
}

func (r *mutationResolver) ClearMySubscriptions(ctx context.Context) (int, error) {
	usr := MustHaveUser(ctx)
	username := usr.Username()
	subs, err := r.sc.GetSubscriptions(username, event.OwnerTypePerson)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("Error retreiving subscriptions %s", err.Error()))
	}
	subIds := []string{}
	for _, sub := range subs {
		if sub.ID != nil {
			subIds = append(subIds, *sub.ID)
		}
	}
	err = r.sc.DeleteSubscriptions(username, subIds)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("Error deleting subscriptions %s", err.Error()))
	}

	return len(subIds), nil
}

func (r *mutationResolver) SaveSubscription(ctx context.Context, subscription restModel.APISubscription) (bool, error) {
	usr := MustHaveUser(ctx)
	username := usr.Username()
	idType, id, err := getResourceTypeAndIdFromSubscriptionSelectors(ctx, subscription.Selectors)
	if err != nil {
		return false, err
	}
	switch idType {
	case "task":
		t, taskErr := r.sc.FindTaskById(id)
		if taskErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding task by id %s: %s", id, taskErr.Error()))
		}
		if t == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find task with id %s", id))
		}
		break
	case "build":
		b, buildErr := r.sc.FindBuildById(id)
		if buildErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding build by id %s: %s", id, buildErr.Error()))
		}
		if b == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find build with id %s", id))
		}
		break
	case "version":
		v, versionErr := r.sc.FindVersionById(id)
		if versionErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding version by id %s: %s", id, versionErr.Error()))
		}
		if v == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find version with id %s", id))
		}
		break
	case "project":
		p, projectErr := r.sc.FindProjectById(id)
		if projectErr != nil {
			return false, InternalServerError.Send(ctx, fmt.Sprintf("error finding project by id %s: %s", id, projectErr.Error()))
		}
		if p == nil {
			return false, ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find project with id %s", id))
		}
		break
	default:
		return false, InputValidationError.Send(ctx, "Selectors do not indicate a target version, build, project, or task ID")
	}
	err = r.sc.SaveSubscriptions(username, []restModel.APISubscription{subscription})
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("error saving subscription: %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) UpdateUserSettings(ctx context.Context, userSettings *restModel.APIUserSettings) (bool, error) {
	usr := MustHaveUser(ctx)

	updatedUserSettings, err := restModel.UpdateUserSettings(ctx, usr, *userSettings)
	if err != nil {
		return false, InternalServerError.Send(ctx, err.Error())
	}
	err = r.sc.UpdateSettings(usr, *updatedUserSettings)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Error saving userSettings : %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) RestartJasper(ctx context.Context, hostIds []string) (int, error) {
	user := MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetRestartJasperCallback(user.Username()))
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, errors.Errorf("error marking selected hosts as needing Jasper service restarted: %s", err.Error()))
	}

	return hostsUpdated, nil
}

func (r *mutationResolver) UpdateHostStatus(ctx context.Context, hostIds []string, status string, notes *string) (int, error) {
	user := MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	rq := evergreen.GetEnvironment().RemoteQueue()

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetUpdateHostStatusCallback(rq, status, *notes, user))
	if err != nil {
		return 0, mapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	return hostsUpdated, nil
}

func (r *mutationResolver) CreatePublicKey(ctx context.Context, publicKeyInput PublicKeyInput) ([]*restModel.APIPubKey, error) {
	err := savePublicKey(ctx, publicKeyInput)
	if err != nil {
		return nil, err
	}
	myPublicKeys := getMyPublicKeys(ctx)
	return myPublicKeys, nil
}

func (r *mutationResolver) RemovePublicKey(ctx context.Context, keyName string) ([]*restModel.APIPubKey, error) {
	if !doesPublicKeyNameAlreadyExist(ctx, keyName) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("Error deleting public key. Provided key name, %s, does not exist.", keyName))
	}
	err := MustHaveUser(ctx).DeletePublicKey(keyName)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error deleting public key: %s", err.Error()))
	}
	myPublicKeys := getMyPublicKeys(ctx)
	return myPublicKeys, nil
}

func (r *mutationResolver) RemoveVolume(ctx context.Context, volumeID string) (bool, error) {
	success, _, gqlErr, err := DeleteVolume(ctx, volumeID)
	if err != nil {
		return false, gqlErr.Send(ctx, err.Error())
	}
	return success, nil
}

func (r *mutationResolver) UpdatePublicKey(ctx context.Context, targetKeyName string, updateInfo PublicKeyInput) ([]*restModel.APIPubKey, error) {
	if !doesPublicKeyNameAlreadyExist(ctx, targetKeyName) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("Error updating public key. The target key name, %s, does not exist.", targetKeyName))
	}
	if updateInfo.Name != targetKeyName && doesPublicKeyNameAlreadyExist(ctx, updateInfo.Name) {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("Error updating public key. The updated key name, %s, already exists.", targetKeyName))
	}
	err := verifyPublicKey(ctx, updateInfo)
	if err != nil {
		return nil, err
	}
	usr := MustHaveUser(ctx)
	err = usr.UpdatePublicKey(targetKeyName, updateInfo.Name, updateInfo.Key)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error updating public key, %s: %s", targetKeyName, err.Error()))
	}
	myPublicKeys := getMyPublicKeys(ctx)
	return myPublicKeys, nil
}

func (r *queryResolver) User(ctx context.Context, userIdParam *string) (*restModel.APIDBUser, error) {
	usr := MustHaveUser(ctx)
	var err error
	if userIdParam != nil {
		usr, err = model.FindUserByID(*userIdParam)
		if err != nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Error getting user from user ID: %s", err.Error()))
		}
	}
	displayName := usr.DisplayName()
	userID := usr.Username()
	email := usr.Email()
	user := restModel.APIDBUser{
		DisplayName:  &displayName,
		UserID:       &userID,
		EmailAddress: &email,
	}
	return &user, nil
}

func (r *queryResolver) InstanceTypes(ctx context.Context) ([]string, error) {
	config, err := evergreen.GetConfig()
	if err != nil {
		return nil, InternalServerError.Send(ctx, "unable to retrieve server config")
	}
	return config.Providers.AWS.AllowedInstanceTypes, nil
}

type taskResolver struct{ *Resolver }

func (r *taskResolver) TotalTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	tests, err := r.sc.GetTestCountByTaskIdAndFilters(*obj.Id, "", nil, obj.Execution)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("Error getting test count: %s", err.Error()))
	}
	return tests, nil
}

func (r *taskResolver) FailedTestCount(ctx context.Context, obj *restModel.APITask) (int, error) {
	failedTestCount, err := r.sc.GetTestCountByTaskIdAndFilters(*obj.Id, "", []string{evergreen.TestFailedStatus}, obj.Execution)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("Error getting tests for failedTestCount: %s", err.Error()))
	}
	return failedTestCount, nil
}

func (r *taskResolver) PatchMetadata(ctx context.Context, obj *restModel.APITask) (*PatchMetadata, error) {
	version, err := r.sc.FindVersionById(*obj.Version)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error retrieving version %s: %s", *obj.Version, err.Error()))
	}
	patchMetadata := PatchMetadata{
		Author: version.Author,
	}
	return &patchMetadata, nil
}

func (r *taskResolver) BaseTaskMetadata(ctx context.Context, at *restModel.APITask) (*BaseTaskMetadata, error) {
	i, err := at.ToService()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting service model for APITask %s: %s", *at.Id, err.Error()))
	}
	t, ok := i.(*task.Task)
	if !ok {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Unable to convert APITask %s to Task", *at.Id))
	}
	baseTask, err := t.FindTaskOnBaseCommit()
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding task %s on base commit", *at.Id))
	}
	if baseTask == nil {
		return nil, nil
	}
	config, err := evergreen.GetConfig()
	if err != nil {
		return nil, InternalServerError.Send(ctx, "unable to retrieve server config")
	}

	dur := restModel.NewAPIDuration(baseTask.TimeTaken)
	baseTaskMetadata := BaseTaskMetadata{
		BaseTaskLink:     fmt.Sprintf("%s/task/%s", config.Ui.Url, baseTask.Id),
		BaseTaskDuration: &dur,
	}
	if baseTask.TimeTaken == 0 {
		baseTaskMetadata.BaseTaskDuration = nil
	}
	return &baseTaskMetadata, nil
}

func (r *taskResolver) SpawnHostLink(ctx context.Context, at *restModel.APITask) (*string, error) {
	host, err := host.FindOne(host.ById(*at.HostId))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("error finding host for task %s", *at.Id))
	}
	if host == nil {
		return nil, nil
	}
	if host.Distro.SpawnAllowed && utility.StringSliceContains(evergreen.ProviderUserSpawnable, host.Distro.Provider) {
		link := fmt.Sprintf("%s/spawn?distro_id=%s&task_id=%s", evergreen.GetEnvironment().Settings().Ui.Url, host.Distro.Id, *at.Id)
		return &link, nil
	}
	return nil, nil
}

func (r *taskResolver) PatchNumber(ctx context.Context, obj *restModel.APITask) (*int, error) {
	order := obj.Order
	return &order, nil
}

func (r *taskResolver) CanRestart(ctx context.Context, obj *restModel.APITask) (bool, error) {
	canRestart, err := canRestartTask(ctx, obj)
	if err != nil {
		return false, err
	}
	return *canRestart, nil
}

func (r *taskResolver) CanAbort(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return *obj.Status == evergreen.TaskDispatched || *obj.Status == evergreen.TaskStarted, nil
}

func (r *taskResolver) CanSchedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	canRestart, err := canRestartTask(ctx, obj)
	if err != nil {
		return false, err
	}
	return *canRestart == false && !obj.Aborted, nil
}

func (r *taskResolver) CanUnschedule(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return obj.Activated && *obj.Status == evergreen.TaskUndispatched, nil
}

func (r *taskResolver) CanSetPriority(ctx context.Context, obj *restModel.APITask) (bool, error) {
	return *obj.Status == evergreen.TaskUndispatched, nil
}

func (r *taskResolver) Status(ctx context.Context, obj *restModel.APITask) (string, error) {
	return *obj.DisplayStatus, nil
}

func (r *taskResolver) LatestExecution(ctx context.Context, obj *restModel.APITask) (int, error) {
	return task.GetLatestExecution(*obj.Id)
}

func (r *taskResolver) GeneratedByName(ctx context.Context, obj *restModel.APITask) (*string, error) {
	if obj.GeneratedBy == "" {
		return nil, nil
	}
	generator, err := task.FindOneIdWithFields(obj.GeneratedBy, task.DisplayNameKey)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("unable to find generator: %s", err.Error()))
	}
	if generator == nil {
		return nil, nil
	}
	name := generator.DisplayName

	return &name, nil
}

func (r *taskResolver) IsPerfPluginEnabled(ctx context.Context, obj *restModel.APITask) (bool, error) {
	var perfPlugin *plugin.PerfPlugin
	if perfPluginSettings, exists := evergreen.GetEnvironment().Settings().Plugins[perfPlugin.Name()]; exists {
		err := mapstructure.Decode(perfPluginSettings, &perfPlugin)
		if err != nil {
			return false, err
		}
		for _, projectName := range perfPlugin.Projects {
			if projectName == *obj.ProjectId {
				return true, nil
			}
		}
	}
	return false, nil
}

func (r *taskResolver) MinQueuePosition(ctx context.Context, obj *restModel.APITask) (int, error) {
	position, err := model.FindMinimumQueuePositionForTask(*obj.Id)
	if err != nil {
		return 0, InternalServerError.Send(ctx, fmt.Sprintf("error queue position for task: %s", err.Error()))
	}
	if position < 0 {
		return 0, nil
	}
	return position, nil
}

func (r *queryResolver) BuildBaron(ctx context.Context, taskId string, exec int) (*BuildBaron, error) {
	execString := strconv.Itoa(exec)

	searchReturnInfo, bbConfig, err := GetSearchReturnInfo(taskId, execString)
	if !bbConfig.ProjectFound || !bbConfig.SearchConfigured {
		return &BuildBaron{
			SearchReturnInfo:     searchReturnInfo,
			BuildBaronConfigured: false,
		}, nil
	}
	if err != nil {
		return nil, InternalServerError.Send(ctx, err.Error())
	}
	return &BuildBaron{
		SearchReturnInfo:     searchReturnInfo,
		BuildBaronConfigured: true,
	}, nil
}

func (r *mutationResolver) BbCreateTicket(ctx context.Context, taskId string) (bool, error) {
	taskNotFound, err := BbFileTicket(ctx, taskId)
	successful := true

	if err != nil {
		return !successful, InternalServerError.Send(ctx, err.Error())
	}
	if taskNotFound {
		return !successful, ResourceNotFound.Send(ctx, fmt.Sprintf("could not find task '%s'", taskId))
	}

	return successful, nil
}

func (r *queryResolver) BbGetCreatedTickets(ctx context.Context, taskId string) ([]*thirdparty.JiraTicket, error) {
	createdTickets, err := BbGetCreatedTicketsPointers(taskId)
	if err != nil {
		return nil, err
	}

	return createdTickets, nil
}

type ticketFieldsResolver struct{ *Resolver }

func (r *ticketFieldsResolver) AssigneeDisplayName(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.Assignee == nil {
		return nil, nil
	}
	return &obj.Assignee.DisplayName, nil
}

func (r *ticketFieldsResolver) ResolutionName(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.Resolution == nil {
		return nil, nil
	}
	return &obj.Resolution.Name, nil
}

func (r *Resolver) TicketFields() TicketFieldsResolver { return &ticketFieldsResolver{r} }

// New injects resources into the resolvers, such as the data connector
func New(apiURL string) Config {
	return Config{
		Resolvers: &Resolver{
			sc: &data.DBConnector{URL: apiURL},
		},
	}
}
