package graphql

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/graphql/loaders"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// Ami is the resolver for the ami field.
func (r *hostResolver) Ami(ctx context.Context, obj *host.Host) (*string, error) {
	return utility.ToStringPtr(obj.GetAMI()), nil
}

// Distro is the resolver for the distro field.
func (r *hostResolver) Distro(ctx context.Context, obj *host.Host) (*restModel.APIDistro, error) {
	apiDistro := &restModel.APIDistro{}
	apiDistro.BuildFromService(obj.Distro)
	return apiDistro, nil
}

// Elapsed is the resolver for the elapsed field.
func (r *hostResolver) Elapsed(ctx context.Context, obj *host.Host) (*time.Time, error) {
	taskId := obj.RunningTask
	if taskId == "" {
		return nil, nil
	}

	runningTask, err := loaders.GetTask(ctx, taskId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s': %s", taskId, err.Error()))
	}

	return utility.ToTimePtr(runningTask.StartTime), nil
}

// Events is the resolver for the events field.
func (r *hostResolver) Events(ctx context.Context, obj *host.Host, opts HostEventsInput) (*HostEvents, error) {
	sortAsc := false
	if opts.SortDir != nil {
		sortAsc = *opts.SortDir == SortDirectionAsc
	}
	hostQueryOpts := event.PaginatedHostEventsOpts{
		ID:         obj.Id,
		Tag:        obj.Tag,
		Limit:      utility.FromIntPtr(opts.Limit),
		Page:       utility.FromIntPtr(opts.Page),
		SortAsc:    sortAsc,
		EventTypes: opts.EventTypes,
	}
	events, count, err := event.GetPaginatedHostEvents(ctx, hostQueryOpts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching events for host '%s': %s", obj.Id, err.Error()))
	}
	apiEventLogPointers := []*restModel.HostAPIEventLogEntry{}
	for _, e := range events {
		apiEventLog := restModel.HostAPIEventLogEntry{}
		if err = apiEventLog.BuildFromService(e); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("building APIEventLogEntry from EventLog: %s", err.Error()))
		}
		apiEventLogPointers = append(apiEventLogPointers, &apiEventLog)
	}
	hostEvents := HostEvents{
		EventLogEntries: apiEventLogPointers,
		Count:           count,
	}
	return &hostEvents, nil
}

// EventTypes is the resolver for the eventTypes field.
func (r *hostResolver) EventTypes(ctx context.Context, obj *host.Host) ([]string, error) {
	eventTypes, err := event.GetEventTypesForHost(ctx, obj.Id, obj.Tag)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting event types for host '%s': %s", obj.Id, err.Error()))
	}
	return eventTypes, nil
}

// HomeVolume is the resolver for the homeVolume field.
func (r *hostResolver) HomeVolume(ctx context.Context, obj *host.Host) (*host.Volume, error) {
	if obj.HomeVolumeID != "" {
		volumeID := obj.HomeVolumeID
		volume, err := host.FindVolumeByID(ctx, volumeID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding volume '%s': %s", volumeID, err.Error()))
		}
		if volume == nil {
			grip.Error(ctx, message.Fields{
				"message":   "could not find the volume associated with this host",
				"host_id":   obj.Id,
				"volume_id": volumeID,
			})
			return nil, nil
		}
		return volume, nil
	}
	return nil, nil
}

// RunningTask is the resolver for the runningTask field.
func (r *hostResolver) RunningTask(ctx context.Context, obj *host.Host) (*TaskInfo, error) {
	taskId := obj.RunningTask
	if taskId == "" {
		return nil, nil
	}

	runningTask, err := loaders.GetTask(ctx, taskId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding task '%s': %s", taskId, err.Error()))
	}

	// TODO DEVPROD-38056: Ideally can return Task type here once off REST
	return &TaskInfo{
		ID:   runningTask.Id,
		Name: runningTask.DisplayName,
	}, nil
}

// TotalIdleTime is the resolver for the totalIdleTime field.
func (r *hostResolver) TotalIdleTime(ctx context.Context, obj *host.Host) (*restModel.APIDuration, error) {
	idleTime := restModel.NewAPIDuration(obj.TotalIdleTime)
	return &idleTime, nil
}

// Volumes is the resolver for the volumes field.
func (r *hostResolver) Volumes(ctx context.Context, obj *host.Host) ([]*host.Volume, error) {
	volumeIds := make([]string, 0, len(obj.Volumes))
	for _, v := range obj.Volumes {
		volumeIds = append(volumeIds, v.VolumeID)
	}

	volumes, err := host.FindVolumesByIDs(ctx, volumeIds)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting volumes", err.Error()))
	}

	volumePtrs := make([]*host.Volume, 0, len(volumes))
	for _, vol := range volumes {
		vCopy := vol
		volumePtrs = append(volumePtrs, &vCopy)
	}

	return volumePtrs, nil
}

// WholeWeekdaysOff is the resolver for the wholeWeekdaysOff field.
func (r *sleepScheduleResolver) WholeWeekdaysOff(ctx context.Context, obj *host.SleepScheduleInfo) ([]int, error) {
	weekdayInts := []int{}
	for _, day := range obj.WholeWeekdaysOff {
		weekdayInts = append(weekdayInts, int(day))
	}
	return weekdayInts, nil
}

// Host returns HostResolver implementation.
func (r *Resolver) Host() HostResolver { return &hostResolver{r} }

// SleepSchedule returns SleepScheduleResolver implementation.
func (r *Resolver) SleepSchedule() SleepScheduleResolver { return &sleepScheduleResolver{r} }

type hostResolver struct{ *Resolver }
type sleepScheduleResolver struct{ *Resolver }
