package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model/patch"
)

// RequirePatchSubmitAccess requires project patch submit permission for configuration and scheduling through schedulePatch.
// Pending GitHub PR patches may be authorized without ownership; other patches require modification access.
// The ownership exception is specific to authorization and scheduling, so this directive should not guard standalone edits.
func RequirePatchSubmitAccess(ctx context.Context, obj any, next graphql.Resolver) (any, error) {
	return RequireProjectAccess(ctx, obj, func(ctx context.Context) (any, error) {
		usr := mustHaveUser(ctx)
		args, ok := obj.(map[string]any)
		if !ok {
			return nil, ResourceNotFound.Send(ctx, "patch ID not specified")
		}
		patchID, ok := args["patchId"].(string)
		if !ok {
			return nil, ResourceNotFound.Send(ctx, "patch ID not specified")
		}
		p, err := patch.FindOneId(ctx, patchID)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch '%s': %s", patchID, err.Error()))
		}
		if p == nil {
			return nil, ResourceNotFound.Send(ctx, fmt.Sprintf("patch '%s' not found", patchID))
		}
		if !p.IsPendingGithubPRAuthorization() && !userCanModifyPatch(ctx, usr, *p) {
			return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to modify patch '%s'", usr.Username(), patchID))
		}
		return next(ctx)
	}, ProjectPermissionPatches, AccessLevelEdit)
}
