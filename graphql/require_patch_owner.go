package graphql

import (
	"context"
	"fmt"
	"strings"

	"github.com/99designs/gqlgen/graphql"
	"github.com/evergreen-ci/evergreen/model/patch"
)

// RequirePatchOwner restricts access to the patch(es) given by the patchId or patchIds argument
// to users who can modify them (such as the patch author, a patch admin, a project admin, or a superuser).
func RequirePatchOwner(ctx context.Context, obj any, next graphql.Resolver) (any, error) {
	user := mustHaveUser(ctx)
	args, isStringMap := obj.(map[string]any)
	if !isStringMap {
		return nil, ResourceNotFound.Send(ctx, "patch ID(s) not specified")
	}
	var patchIds []string
	if rawPatchIds, ok := args["patchIds"].([]any); ok {
		patchIds = make([]string, len(rawPatchIds))
		for i, v := range rawPatchIds {
			patchIds[i] = v.(string)
		}
	} else if singlePatchId, ok := args["patchId"].(string); ok {
		patchIds = []string{singlePatchId}
	} else {
		return nil, ResourceNotFound.Send(ctx, "patch ID not specified")
	}

	patches, err := patch.Find(ctx, patch.ByStringIds(ctx, patchIds))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patches '%s': %s", patchIds, err.Error()))
	}

	forbiddenPatches := []string{}
	for _, p := range patches {
		if !userCanModifyPatch(ctx, user, p) {
			forbiddenPatches = append(forbiddenPatches, p.Id.Hex())
		}
	}
	if len(forbiddenPatches) == 1 {
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to modify patch '%s'", user.Username(), forbiddenPatches[0]))
	} else if len(forbiddenPatches) > 1 {
		patchString := strings.Join(forbiddenPatches, ", ")
		return nil, Forbidden.Send(ctx, fmt.Sprintf("user '%s' does not have permission to modify patches: '%s'", user.Username(), patchString))
	}

	return next(ctx)
}
