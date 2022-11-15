package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen/model/host"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
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

func (r *hostResolver) Uptime(ctx context.Context, obj *restModel.APIHost) (*time.Time, error) {
	return obj.CreationTime, nil
}

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

// Host returns HostResolver implementation.
func (r *Resolver) Host() HostResolver { return &hostResolver{r} }

type hostResolver struct{ *Resolver }
