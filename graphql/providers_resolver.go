package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// Port is the resolver for the port field.
func (r *containerPoolResolver) Port(ctx context.Context, obj *model.APIContainerPool) (int, error) {
	return int(obj.Port), nil
}

// Port is the resolver for the port field.
func (r *containerPoolInputResolver) Port(ctx context.Context, obj *model.APIContainerPool, data int) error {
	obj.Port = uint16(data)
	return nil
}

// ContainerPool returns ContainerPoolResolver implementation.
func (r *Resolver) ContainerPool() ContainerPoolResolver { return &containerPoolResolver{r} }

// ContainerPoolInput returns ContainerPoolInputResolver implementation.
func (r *Resolver) ContainerPoolInput() ContainerPoolInputResolver {
	return &containerPoolInputResolver{r}
}

type containerPoolResolver struct{ *Resolver }
type containerPoolInputResolver struct{ *Resolver }
