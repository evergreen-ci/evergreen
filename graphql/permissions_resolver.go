package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.
// Code generated by github.com/99designs/gqlgen version v0.17.38

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
)

// CanCreateDistro is the resolver for the canCreateDistro field.
func (r *permissionsResolver) CanCreateDistro(ctx context.Context, obj *Permissions) (bool, error) {
	usr, err := user.FindOneById(obj.UserID)
	if err != nil {
		return false, ResourceNotFound.Send(ctx, "user not found")
	}
	opts := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionDistroCreate,
		RequiredLevel: evergreen.DistroCreate.Value,
	}
	return usr.HasPermission(opts), nil
}

// CanCreateProject is the resolver for the canCreateProject field.
func (r *permissionsResolver) CanCreateProject(ctx context.Context, obj *Permissions) (bool, error) {
	usr, err := user.FindOneById(obj.UserID)
	if err != nil {
		return false, ResourceNotFound.Send(ctx, "user not found")
	}
	canCreate, err := usr.HasProjectCreatePermission()
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("Error checking user permission: %s", err.Error()))
	}
	return canCreate, nil
}

// CanEditAdminSettings is the resolver for the canEditAdminSettings field.
func (r *permissionsResolver) CanEditAdminSettings(ctx context.Context, obj *Permissions) (bool, error) {
	usr, err := user.FindOneById(obj.UserID)
	if err != nil {
		return false, ResourceNotFound.Send(ctx, "user not found")
	}
	opts := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionAdminSettings,
		RequiredLevel: evergreen.AdminSettingsEdit.Value,
	}
	return usr.HasPermission(opts), nil
}

// DistroPermissions is the resolver for the distroPermissions field.
func (r *permissionsResolver) DistroPermissions(ctx context.Context, obj *Permissions, options DistroPermissionsOptions) (*DistroPermissions, error) {
	usr, err := user.FindOneById(obj.UserID)
	if err != nil {
		return nil, ResourceNotFound.Send(ctx, "user not found")
	}
	return &DistroPermissions{
		Admin: userHasDistroPermission(usr, options.DistroID, evergreen.DistroSettingsAdmin.Value),
		Edit:  userHasDistroPermission(usr, options.DistroID, evergreen.DistroSettingsEdit.Value),
		View:  userHasDistroPermission(usr, options.DistroID, evergreen.DistroSettingsView.Value),
	}, nil
}

// Permissions returns PermissionsResolver implementation.
func (r *Resolver) Permissions() PermissionsResolver { return &permissionsResolver{r} }

type permissionsResolver struct{ *Resolver }
