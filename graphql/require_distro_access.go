package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen"
)

// RequireDistroAccess restricts admin, create, edit, or view access to the distro given by the distroId (or name) argument.
func RequireDistroAccess(ctx context.Context, obj any, next graphql.Resolver, access DistroSettingsAccess) (any, error) {
	user := mustHaveUser(ctx)

	// If directive is checking for create permissions, no distro ID is required.
	if access == DistroSettingsAccessCreate {
		if user.HasDistroCreatePermission(ctx) {
			return next(ctx)
		}
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have create distro permissions", user.Username()))
	}

	args, isStringMap := obj.(map[string]any)
	if !isStringMap {
		return nil, ResourceNotFound.Send(ctx, "distro not specified")
	}
	distroId, hasDistroId := args["distroId"].(string)
	if !hasDistroId {
		name, hasName := args["name"].(string)

		if !hasName {
			return nil, ResourceNotFound.Send(ctx, "Distro not specified")
		}
		distroId = name
	}

	var requiredLevel int
	if access == DistroSettingsAccessAdmin {
		requiredLevel = evergreen.DistroSettingsAdmin.Value
	} else if access == DistroSettingsAccessEdit {
		requiredLevel = evergreen.DistroSettingsEdit.Value
	} else if access == DistroSettingsAccessView {
		requiredLevel = evergreen.DistroSettingsView.Value
	} else {
		return nil, Forbidden.Send(ctx, "Permission not specified")
	}

	if userHasDistroPermission(ctx, user, distroId, requiredLevel) {
		return next(ctx)
	}
	return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to access settings for the distro '%s'", user.Username(), distroId))
}
