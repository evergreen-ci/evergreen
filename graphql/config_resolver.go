package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.36

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/model"
)

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

// SpruceConfig returns SpruceConfigResolver implementation.
func (r *Resolver) SpruceConfig() SpruceConfigResolver { return &spruceConfigResolver{r} }

type spruceConfigResolver struct{ *Resolver }
