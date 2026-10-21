package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model/host"
)

func RequireVolumeAccess(ctx context.Context, obj any, next graphql.Resolver) (any, error) {
	args, isStringMap := obj.(map[string]any)
	if !isStringMap {
		return nil, ResourceNotFound.Send(ctx, "converting args into map")
	}

	volumeId, hasVolumeId := args["volumeId"].(string)
	// If no volume ID is present, the field is optional and null, so skip the access check.
	if !hasVolumeId {
		return next(ctx)
	}
	if volumeId == "" {
		return nil, InputValidationError.Send(ctx, "must specify volume ID")
	}

	dbUser := mustHaveUser(ctx)
	v, err := host.FindVolumeByID(ctx, volumeId)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding volume '%s': %s", volumeId, err.Error()))
	}
	if v == nil {
		return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("volume '%s' not found", volumeId))
	}
	if userHasVolumePermission(ctx, dbUser, volumeId, v.CreatedBy) {
		return next(ctx)
	}
	return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to access volume '%s'", dbUser.Username(), volumeId))
}
