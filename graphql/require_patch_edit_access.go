package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model/patch"
)

// RequirePatchEditAccess requires the user to be the patch owner or an admin, except for GitHub PR patches that are
// awaiting authorization, which any user may authorize. It does not check project permissions.
func RequirePatchEditAccess(ctx context.Context, obj any, next graphql.Resolver) (any, error) {
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
	// Users who can submit patches to the project may authorize a pending PR patch even if the PR author is someone
	// else (or no longer has an account).
	if !p.IsPendingGithubPRAuthorization() && !userCanModifyPatch(ctx, usr, *p) {
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to modify patch '%s'", usr.Username(), patchID))
	}
	return next(ctx)
}
