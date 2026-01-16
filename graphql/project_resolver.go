package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// IsFavorite is the resolver for the isFavorite field.
func (r *projectResolver) IsFavorite(ctx context.Context, obj *restModel.APIProjectRef) (bool, error) {
	projectIdentifier := utility.FromStringPtr(obj.Identifier)
	p, err := model.FindBranchProjectRef(ctx, projectIdentifier)
	if err != nil {
		return false, InternalServerError.Send(ctx, fmt.Sprintf("fetching project '%s': %s", projectIdentifier, err.Error()))
	}
	if p == nil {
		return false, ResourceNotFound.Send(ctx, fmt.Sprintf("project '%s' not found", projectIdentifier))
	}
	usr := mustHaveUser(ctx)
	if utility.StringSliceContains(usr.FavoriteProjects, *obj.Identifier) {
		return true, nil
	}
	return false, nil
}

// Patches is the resolver for the patches field.
func (r *projectResolver) Patches(ctx context.Context, obj *restModel.APIProjectRef, patchesInput PatchesInput) (*Patches, error) {
	opts := patch.ByPatchNameStatusesMergeQueuePaginatedOptions{
		Project:        obj.Id,
		PatchName:      patchesInput.PatchName,
		Statuses:       patchesInput.Statuses,
		Page:           patchesInput.Page,
		Limit:          patchesInput.Limit,
		OnlyMergeQueue: patchesInput.OnlyMergeQueue,
		IncludeHidden:  patchesInput.IncludeHidden,
		Requesters:     patchesInput.Requesters,
	}

	patches, count, err := patch.ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patches for project '%s': %s", utility.FromStringPtr(opts.Project), err.Error()))
	}
	apiPatches := []*restModel.APIPatch{}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		err = apiPatch.BuildFromService(ctx, p, nil) // Injecting DB info into APIPatch is handled by the resolvers.
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting patch '%s' to APIPatch: %s", p.Id.Hex(), err.Error()))
		}
		apiPatches = append(apiPatches, &apiPatch)
	}
	return &Patches{Patches: apiPatches, FilteredPatchCount: count}, nil
}

// Project returns ProjectResolver implementation.
func (r *Resolver) Project() ProjectResolver { return &projectResolver{r} }

type projectResolver struct{ *Resolver }
