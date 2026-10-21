package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model/patch"
)

// RequirePatchScheduleAccess requires project patch-submit permission and, beyond that, lets non-owners authorize a
// pending GitHub PR patch. All other patches still require the user to be the owner or an admin.
func RequirePatchScheduleAccess(ctx context.Context, obj any, next graphql.Resolver) (any, error) {
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
	requiredPermission, permissionInfo, err := getProjectPermissionLevel(ProjectPermissionPatches, AccessLevelEdit)
	if err != nil {
		return nil, InputValidationError.Send(ctx, fmt.Sprintf("invalid permission and access level configuration: %s", err.Error()))
	}
	if err = checkProjectPermission(ctx, p.Project, requiredPermission, permissionInfo); err != nil {
		return nil, err
	}
	// A PR patch that hasn't been finalized is still awaiting authorization, so any user who can submit patches
	// to the project may authorize it, even if the PR author is someone else (or no longer has an account).
	isPendingPR := p.IsGithubPRPatch() && p.Version == ""
	if !isPendingPR && !userCanModifyPatch(ctx, usr, *p) {
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to modify patch '%s'", usr.Username(), patchID))
	}
	return next(ctx)
}
