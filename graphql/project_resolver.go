package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

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
	p, err := model.FindBranchProjectRef(*obj.Identifier)
	if err != nil || p == nil {
		return false, ResourceNotFound.Send(ctx, fmt.Sprintf("Could not find project: %s : %s", *obj.Identifier, err))
	}
	usr := mustHaveUser(ctx)
	if utility.StringSliceContains(usr.FavoriteProjects, *obj.Identifier) {
		return true, nil
	}
	return false, nil
}

// Patches is the resolver for the patches field.
func (r *projectResolver) Patches(ctx context.Context, obj *restModel.APIProjectRef, patchesInput PatchesInput) (*Patches, error) {
	opts := patch.ByPatchNameStatusesCommitQueuePaginatedOptions{
		Project:         obj.Id,
		PatchName:       patchesInput.PatchName,
		Statuses:        patchesInput.Statuses,
		Page:            patchesInput.Page,
		Limit:           patchesInput.Limit,
		OnlyCommitQueue: patchesInput.OnlyCommitQueue,
	}

	patches, count, err := patch.ByPatchNameStatusesCommitQueuePaginated(opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error while fetching patches for this project : %s", err.Error()))
	}
	apiPatches := []*restModel.APIPatch{}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		err = apiPatch.BuildFromService(p, nil) // Injecting DB info into APIPatch is handled by the resolvers.
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building APIPatch from service for patch: %s : %s", p.Id.Hex(), err.Error()))
		}
		apiPatches = append(apiPatches, &apiPatch)
	}
	return &Patches{Patches: apiPatches, FilteredPatchCount: count}, nil
}

// Private is the resolver for the private field.
func (r *projectResolver) Private(ctx context.Context, obj *restModel.APIProjectRef) (*bool, error) {
	panic(fmt.Errorf("not implemented: Private - private"))
}

// Project returns ProjectResolver implementation.
func (r *Resolver) Project() ProjectResolver { return &projectResolver{r} }

type projectResolver struct{ *Resolver }
