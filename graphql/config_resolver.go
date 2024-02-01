package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// Port is the resolver for the port field.
func (r *containerPoolResolver) Port(ctx context.Context, obj *model.APIContainerPool) (int, error) {
	return int(obj.Port), nil
}

// Keys is the resolver for the keys field.
func (r *spruceConfigResolver) Keys(ctx context.Context, obj *model.APIAdminSettings) ([]*SSHKey, error) {
	sshKeys := []*SSHKey{}
	for name, location := range obj.Keys {
		sshKeys = append(sshKeys, &SSHKey{
			Location: location,
			Name:     name,
		})
	}
	return sshKeys, nil
}

// ContainerPool returns ContainerPoolResolver implementation.
func (r *Resolver) ContainerPool() ContainerPoolResolver { return &containerPoolResolver{r} }

// SpruceConfig returns SpruceConfigResolver implementation.
func (r *Resolver) SpruceConfig() SpruceConfigResolver { return &spruceConfigResolver{r} }

type containerPoolResolver struct{ *Resolver }
type spruceConfigResolver struct{ *Resolver }
