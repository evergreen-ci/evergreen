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
	host, err := host.FindOneId(ctx, utility.FromStringPtr(obj.Id))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error finding host %s: %s", utility.FromStringPtr(obj.Id), err.Error()))
	}
	if host == nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Could not find host %s", utility.FromStringPtr(obj.Id)))
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
func (r *hostResolver) Events(ctx context.Context, obj *restModel.APIHost, limit *int, page *int, sortDir *SortDirection) (*HostEvents, error) {
	sortAsc := false
	if sortDir != nil {
		sortAsc = *sortDir == SortDirectionAsc
	}
	events, count, err := event.MostRecentPaginatedHostEvents(utility.FromStringPtr(obj.Id), utility.FromStringPtr(obj.Tag), utility.FromIntPtr(limit), utility.FromIntPtr(page), sortAsc)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching host events: %s", err.Error()))
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

// HomeVolume is the resolver for the homeVolume field.
func (r *hostResolver) HomeVolume(ctx context.Context, obj *restModel.APIHost) (*restModel.APIVolume, error) {
	if utility.FromStringPtr(obj.HomeVolumeID) != "" {
		volId := utility.FromStringPtr(obj.HomeVolumeID)
		volume, err := host.FindVolumeByID(volId)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting volume %s: %s", volId, err.Error()))
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
		apiVolume.BuildFromService(*volume)
		return apiVolume, nil
	}
	return nil, nil
}

// SleepSchedule is the resolver for the sleepSchedule field.
func (r *hostResolver) SleepSchedule(ctx context.Context, obj *restModel.APIHost) (*host.SleepScheduleInfo, error) {
	h, err := host.FindOne(ctx, host.ById(utility.FromStringPtr(obj.Id)))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting host %s", utility.FromStringPtr(obj.Id)))
	}
	if h == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("Unable to find host %s", utility.FromStringPtr(obj.Id)))
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
	for _, volId := range obj.AttachedVolumeIDs {
		volume, err := host.FindVolumeByID(volId)
		if err != nil {
			return volumes, InternalServerError.Send(ctx, fmt.Sprintf("Error getting volume %s", volId))
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
