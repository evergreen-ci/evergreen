package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/evergreen-ci/gimlet"
)

func (r *permissionsResolver) CanCreateProject(ctx context.Context, obj *gqlModel.Permissions) (bool, error) {
	usr, err := user.FindOneById(obj.UserID)
	if err != nil {
		return false, gqlError.ResourceNotFound.Send(ctx, "user not found")
	}
	return usr.HasPermission(gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionProjectCreate,
		RequiredLevel: evergreen.ProjectCreate.Value,
	}), nil
}

// Permissions returns generated.PermissionsResolver implementation.
func (r *Resolver) Permissions() generated.PermissionsResolver { return &permissionsResolver{r} }

type permissionsResolver struct{ *Resolver }
