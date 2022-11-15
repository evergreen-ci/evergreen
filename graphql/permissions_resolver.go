package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
)

// CanCreateProject is the resolver for the canCreateProject field.
func (r *permissionsResolver) CanCreateProject(ctx context.Context, obj *Permissions) (bool, error) {
	usr, err := user.FindOneById(obj.UserID)
	if err != nil {
		return false, ResourceNotFound.Send(ctx, "user not found")
	}
	return usr.HasPermission(gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionProjectCreate,
		RequiredLevel: evergreen.ProjectCreate.Value,
	}), nil
}

// Permissions returns PermissionsResolver implementation.
func (r *Resolver) Permissions() PermissionsResolver { return &permissionsResolver{r} }

type permissionsResolver struct{ *Resolver }
