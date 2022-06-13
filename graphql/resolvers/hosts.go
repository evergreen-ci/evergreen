package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"sort"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/api"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/graphql/resolvers/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/distro"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	werrors "github.com/pkg/errors"
)

func (r *hostResolver) DistroID(ctx context.Context, obj *restModel.APIHost) (*string, error) {
	return obj.Distro.Id, nil
}

func (r *hostResolver) Elapsed(ctx context.Context, obj *restModel.APIHost) (*time.Time, error) {
	return obj.RunningTask.StartTime, nil
}

func (r *hostResolver) HomeVolume(ctx context.Context, obj *restModel.APIHost) (*restModel.APIVolume, error) {
	if utility.FromStringPtr(obj.HomeVolumeID) != "" {
		volId := utility.FromStringPtr(obj.HomeVolumeID)
		volume, err := host.FindVolumeByID(volId)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting volume %s: %s", volId, err.Error()))
		}
		if volume == nil {
			grip.Error(message.Fields{
				"message":   "could not find the volume associated with this host",
				"ticket":    "EVG-16149",
				"host_id":   obj.Id,
				"volume_id": volId,
			})
			return nil, nil
		}
		apiVolume := &restModel.APIVolume{}
		err = apiVolume.BuildFromService(volume)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building volume '%s' from service: %s", volId, err.Error()))
		}
		return apiVolume, nil
	}
	return nil, nil
}

func (r *hostResolver) Uptime(ctx context.Context, obj *restModel.APIHost) (*time.Time, error) {
	return obj.CreationTime, nil
}

func (r *hostResolver) Volumes(ctx context.Context, obj *restModel.APIHost) ([]*restModel.APIVolume, error) {
	volumes := make([]*restModel.APIVolume, 0, len(obj.AttachedVolumeIDs))
	for _, volId := range obj.AttachedVolumeIDs {
		volume, err := host.FindVolumeByID(volId)
		if err != nil {
			return volumes, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting volume %s", volId))
		}
		if volume == nil {
			continue
		}
		apiVolume := &restModel.APIVolume{}
		err = apiVolume.BuildFromService(volume)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, werrors.Wrapf(err, "error building volume '%s' from service", volId).Error())
		}
		volumes = append(volumes, apiVolume)
	}

	return volumes, nil
}

func (r *mutationResolver) ReprovisionToNew(ctx context.Context, hostIds []string) (int, error) {
	user := util.MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetReprovisionToNewCallback(ctx, evergreen.GetEnvironment(), user.Username()))
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, werrors.Errorf("Error marking selected hosts as needing to reprovision: %s", err.Error()))
	}

	return hostsUpdated, nil
}

func (r *mutationResolver) RestartJasper(ctx context.Context, hostIds []string) (int, error) {
	user := util.MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetRestartJasperCallback(ctx, evergreen.GetEnvironment(), user.Username()))
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, werrors.Errorf("Error marking selected hosts as needing Jasper service restarted: %s", err.Error()))
	}

	return hostsUpdated, nil
}

func (r *mutationResolver) UpdateHostStatus(ctx context.Context, hostIds []string, status string, notes *string) (int, error) {
	user := util.MustHaveUser(ctx)

	hosts, permissions, httpStatus, err := api.GetHostsAndUserPermissions(user, hostIds)
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	rq := evergreen.GetEnvironment().RemoteQueue()
	hostsUpdated, httpStatus, err := api.ModifyHostsWithPermissions(hosts, permissions, api.GetUpdateHostStatusCallback(ctx, evergreen.GetEnvironment(), rq, status, *notes, user))
	if err != nil {
		return 0, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}

	return hostsUpdated, nil
}

func (r *queryResolver) Distros(ctx context.Context, onlySpawnable bool) ([]*restModel.APIDistro, error) {
	apiDistros := []*restModel.APIDistro{}

	var distros []distro.Distro
	if onlySpawnable {
		d, err := distro.Find(distro.BySpawnAllowed())
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while fetching spawnable distros: %s", err.Error()))
		}
		distros = d
	} else {
		d, err := distro.FindAll()
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error while fetching distros: %s", err.Error()))
		}
		distros = d
	}
	for _, d := range distros {
		apiDistro := restModel.APIDistro{}
		err := apiDistro.BuildFromService(d)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIDistro from distro: %s", err.Error()))
		}
		apiDistros = append(apiDistros, &apiDistro)
	}
	return apiDistros, nil
}

func (r *queryResolver) DistroTaskQueue(ctx context.Context, distroID string) ([]*restModel.APITaskQueueItem, error) {
	distroQueue, err := model.LoadTaskQueue(distroID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting task queue for distro %v: %v", distroID, err.Error()))
	}
	if distroQueue == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("cannot find queue with distro ID `%s`", distroID))
	}

	taskQueue := []*restModel.APITaskQueueItem{}

	for _, taskQueueItem := range distroQueue.Queue {
		apiTaskQueueItem := restModel.APITaskQueueItem{}
		err := apiTaskQueueItem.BuildFromService(taskQueueItem)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error converting task queue item db model to api model: %v", err.Error()))
		}
		taskQueue = append(taskQueue, &apiTaskQueueItem)
	}

	return taskQueue, nil
}

func (r *queryResolver) Host(ctx context.Context, hostID string) (*restModel.APIHost, error) {
	host, err := host.GetHostByIdOrTagWithTask(hostID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error Fetching host: %s", err.Error()))
	}
	if host == nil {
		return nil, werrors.Errorf("unable to find host %s", hostID)
	}

	apiHost := &restModel.APIHost{}
	err = apiHost.BuildFromService(host)
	if err != nil || apiHost == nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error converting from host.Host to model.APIHost: %s", err.Error()))
	}

	if host.RunningTask != "" {
		// Add the task information to the host document.
		if err = apiHost.BuildFromService(host.RunningTaskFull); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error converting from host.Host to model.APIHost: %s", err.Error()))
		}
	}

	return apiHost, nil
}

func (r *queryResolver) HostEvents(ctx context.Context, hostID string, hostTag *string, limit *int, page *int) (*gqlModel.HostEvents, error) {
	h, err := host.FindOneByIdOrTag(hostID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding host %s: %s", hostID, err.Error()))
	}
	if h == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Host %s not found", hostID))
	}
	events, err := event.FindPaginated(h.Id, h.Tag, event.AllLogCollection, *limit, *page)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error Fetching host events: %s", err.Error()))
	}
	// populate eventlogs pointer arrays
	apiEventLogPointers := []*restModel.HostAPIEventLogEntry{}
	for _, e := range events {
		apiEventLog := restModel.HostAPIEventLogEntry{}
		err = apiEventLog.BuildFromService(&e)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Unable to build APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}
	hostevents := gqlModel.HostEvents{
		EventLogEntries: apiEventLogPointers,
		Count:           len(events),
	}
	return &hostevents, nil
}

func (r *queryResolver) Hosts(ctx context.Context, hostID *string, distroID *string, currentTaskID *string, statuses []string, startedBy *string, sortBy *gqlModel.HostSortBy, sortDir *gqlModel.SortDirection, page *int, limit *int) (*gqlModel.HostsResponse, error) {
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
		case gqlModel.HostSortByCurrentTask:
			sorter = host.RunningTaskKey
		case gqlModel.HostSortByDistro:
			sorter = host.DistroKey
		case gqlModel.HostSortByElapsed:
			sorter = "task_full.start_time"
		case gqlModel.HostSortByID:
			sorter = host.IdKey
		case gqlModel.HostSortByIDLeTime:
			sorter = host.TotalIdleTimeKey
		case gqlModel.HostSortByOwner:
			sorter = host.StartedByKey
		case gqlModel.HostSortByStatus:
			sorter = host.StatusKey
		case gqlModel.HostSortByUptime:
			sorter = host.CreateTimeKey
		default:
			sorter = host.StatusKey
		}

	}
	sortDirParam := 1
	if *sortDir == gqlModel.SortDirectionDesc {
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
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting hosts: %s", err.Error()))
	}

	apiHosts := []*restModel.APIHost{}

	for _, host := range hosts {
		apiHost := restModel.APIHost{}

		err = apiHost.BuildFromService(host)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building API Host from Service: %s", err.Error()))
		}

		if host.RunningTask != "" {
			// Add the task information to the host document.
			if err = apiHost.BuildFromService(host.RunningTaskFull); err != nil {
				return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error converting from host.Host to model.APIHost: %s", err.Error()))
			}
		}

		apiHosts = append(apiHosts, &apiHost)
	}

	return &gqlModel.HostsResponse{
		Hosts:              apiHosts,
		FilteredHostsCount: filteredHostsCount,
		TotalHostsCount:    totalHostsCount,
	}, nil
}

func (r *queryResolver) TaskQueueDistros(ctx context.Context) ([]*gqlModel.TaskQueueDistro, error) {
	queues, err := model.FindAllTaskQueues()
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting all task queues: %v", err.Error()))
	}

	distros := []*gqlModel.TaskQueueDistro{}

	for _, distro := range queues {
		numHosts, err := host.CountRunningHosts(distro.Distro)
		if err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error getting associated hosts: %s", err.Error()))
		}
		tqd := gqlModel.TaskQueueDistro{
			ID:        distro.Distro,
			TaskCount: len(distro.Queue),
			HostCount: numHosts,
		}
		distros = append(distros, &tqd)
	}

	// sort distros by task count in descending order
	sort.SliceStable(distros, func(i, j int) bool {
		return distros[i].TaskCount > distros[j].TaskCount
	})

	return distros, nil
}

func (r *taskQueueItemResolver) Requester(ctx context.Context, obj *restModel.APITaskQueueItem) (gqlModel.TaskQueueItemType, error) {
	if *obj.Requester != evergreen.RepotrackerVersionRequester {
		return gqlModel.TaskQueueItemTypePatch, nil
	}
	return gqlModel.TaskQueueItemTypeCommit, nil
}

func (r *volumeResolver) Host(ctx context.Context, obj *restModel.APIVolume) (*restModel.APIHost, error) {
	if obj.HostID == nil || *obj.HostID == "" {
		return nil, nil
	}
	host, err := host.FindOneId(*obj.HostID)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error finding host %s: %s", *obj.HostID, err.Error()))
	}
	if host == nil {
		return nil, gqlError.ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find host %s", *obj.HostID))
	}
	apiHost := restModel.APIHost{}
	err = apiHost.BuildFromService(host)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("Error building apiHost %s from service: %s", host.Id, err))
	}
	return &apiHost, nil
}

// Host returns generated.HostResolver implementation.
func (r *Resolver) Host() generated.HostResolver { return &hostResolver{r} }

// TaskQueueItem returns generated.TaskQueueItemResolver implementation.
func (r *Resolver) TaskQueueItem() generated.TaskQueueItemResolver { return &taskQueueItemResolver{r} }

// Volume returns generated.VolumeResolver implementation.
func (r *Resolver) Volume() generated.VolumeResolver { return &volumeResolver{r} }

type hostResolver struct{ *Resolver }
type taskQueueItemResolver struct{ *Resolver }
type volumeResolver struct{ *Resolver }
