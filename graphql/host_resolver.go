package graphql

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
)

// Ami is the resolver for the ami field.
func (r *hostResolver) Ami(ctx context.Context, obj *restModel.APIHost) (*string, error) {
	hostID := utility.FromStringPtr(obj.Id)
	host, err := host.FindOneId(ctx, hostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding host '%s': %s", hostID, err.Error()))
	}
	if host == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("host '%s' not found", hostID))
	}
	return utility.ToStringPtr(host.GetAMI()), nil
}

// DistroID is the resolver for the distroId field.
func (r *hostResolver) DistroID(ctx context.Context, obj *restModel.APIHost) (*string, error) {
	return obj.Distro.Id, nil
}

// Elapsed is the resolver for the elapsed field.
func (r *hostResolver) Elapsed(ctx context.Context, obj *restModel.APIHost) (*time.Time, error) {
	return obj.RunningTask.StartTime, nil
}

// Events is the resolver for the events field.
func (r *hostResolver) Events(ctx context.Context, obj *restModel.APIHost, opts HostEventsInput) (*HostEvents, error) {
	sortAsc := false
	if opts.SortDir != nil {
		sortAsc = *opts.SortDir == SortDirectionAsc
	}
	hostQueryOpts := event.PaginatedHostEventsOpts{
		ID:         utility.FromStringPtr(obj.Id),
		Tag:        utility.FromStringPtr(obj.Tag),
		Limit:      utility.FromIntPtr(opts.Limit),
		Page:       utility.FromIntPtr(opts.Page),
		SortAsc:    sortAsc,
		EventTypes: opts.EventTypes,
	}
	events, count, err := event.GetPaginatedHostEvents(hostQueryOpts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching events for host '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
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
func (r *hostResolver) EventTypes(ctx context.Context, obj *restModel.APIHost) ([]string, error) {
	eventTypes, err := event.GetEventTypesForHost(utility.FromStringPtr(obj.Id), utility.FromStringPtr(obj.Tag))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting event types for host '%s': %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	return eventTypes, nil
}

// HomeVolume is the resolver for the homeVolume field.
func (r *hostResolver) HomeVolume(ctx context.Context, obj *restModel.APIHost) (*restModel.APIVolume, error) {
	if utility.FromStringPtr(obj.HomeVolumeID) != "" {
		volumeID := utility.FromStringPtr(obj.HomeVolumeID)
		volume, err := host.FindVolumeByID(volumeID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding volume '%s': %s", volumeID, err.Error()))
		}
		if volume == nil {
			grip.Error(message.Fields{
				"message":   "could not find the volume associated with this host",
				"host_id":   obj.Id,
				"volume_id": volumeID,
			})
			return nil, nil
		}
		apiVolume := &restModel.APIVolume{}
		apiVolume.BuildFromService(*volume)
		return apiVolume, nil
	}
	return nil, nil
}

// SleepSchedule is the resolver for the sleepSchedule field.
func (r *hostResolver) SleepSchedule(ctx context.Context, obj *restModel.APIHost) (*host.SleepScheduleInfo, error) {
	hostID := utility.FromStringPtr(obj.Id)
	h, err := host.FindOne(ctx, host.ById(hostID))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting host '%s': %s", hostID, err.Error()))
	}
	if h == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("host '%s' not found", hostID))
	}
	return &h.SleepSchedule, nil
}

// Uptime is the resolver for the uptime field.
func (r *hostResolver) Uptime(ctx context.Context, obj *restModel.APIHost) (*time.Time, error) {
	return obj.CreationTime, nil
}

// Volumes is the resolver for the volumes field.
func (r *hostResolver) Volumes(ctx context.Context, obj *restModel.APIHost) ([]*restModel.APIVolume, error) {
	volumes := make([]*restModel.APIVolume, 0, len(obj.AttachedVolumeIDs))
	for _, volumeID := range obj.AttachedVolumeIDs {
		volume, err := host.FindVolumeByID(volumeID)
		if err != nil {
			return volumes, InternalServerError.Send(ctx, fmt.Sprintf("getting volume '%s': %s", volumeID, err.Error()))
		}
		if volume == nil {
			continue
		}
		apiVolume := &restModel.APIVolume{}
		apiVolume.BuildFromService(*volume)
		volumes = append(volumes, apiVolume)
	}

	return volumes, nil
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
