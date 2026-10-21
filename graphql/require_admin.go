package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/gimlet"
)

func RequireAdmin(ctx context.Context, obj any, next graphql.Resolver) (res any, err error) {
	dbUser := mustHaveUser(ctx)

	permissions := gimlet.PermissionOpts{
		Resource:      evergreen.SuperUserPermissionsID,
		ResourceType:  evergreen.SuperUserResourceType,
		Permission:    evergreen.PermissionAdminSettings,
		RequiredLevel: evergreen.AdminSettingsEdit.Value,
	}

	if dbUser.HasPermission(ctx, permissions) {
		return next(ctx)
	}
	return nil, Forbidden.Send(ctx, fmt.Sprintf("User '%s' lacks required admin permissions", dbUser.Username()))
}
