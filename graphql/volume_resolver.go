package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/host"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// Host is the resolver for the host field.
func (r *volumeResolver) Host(ctx context.Context, obj *restModel.APIVolume) (*restModel.APIHost, error) {
	if obj.HostID == nil || *obj.HostID == "" {
		return nil, nil
	}
	hostId := utility.FromStringPtr(obj.HostID)
	h, err := host.FindOneId(ctx, hostId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding host '%s': %s", hostId, err.Error()))
	}
	if h == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("host '%s' not found", hostId))
	}
	apiHost := restModel.APIHost{}
	apiHost.BuildFromService(h, nil)
	return &apiHost, nil
}

// Volume returns VolumeResolver implementation.
func (r *Resolver) Volume() VolumeResolver { return &volumeResolver{r} }

type volumeResolver struct{ *Resolver }
