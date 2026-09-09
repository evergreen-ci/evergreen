package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model/host"
)

// Host is the resolver for the host field.
func (r *volumeResolver) Host(ctx context.Context, obj *host.Volume) (*host.Host, error) {
	if obj.Host == "" {
		return nil, nil
	}

	// If only id is requested, we can return it without a database call.
	requestedFields := graphql.CollectAllFields(ctx)
	if len(requestedFields) == 1 && requestedFields[0] == "id" {
		return &host.Host{Id: obj.Host}, nil
	}

	hostID := obj.Host
	h, err := host.FindOneId(ctx, hostID)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding host '%s': %s", hostID, err.Error()))
	}
	if h == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("host '%s' not found", hostID))
	}
	return h, nil
}

// Volume returns VolumeResolver implementation.
func (r *Resolver) Volume() VolumeResolver { return &volumeResolver{r} }

type volumeResolver struct{ *Resolver }
