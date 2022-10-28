package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// Patches is the resolver for the patches field.
func (r *userResolver) Patches(ctx context.Context, obj *restModel.APIDBUser, patchesInput PatchesInput) (*Patches, error) {
	opts := patch.ByPatchNameStatusesCommitQueuePaginatedOptions{
		Author:             obj.UserID,
		PatchName:          patchesInput.PatchName,
		Statuses:           patchesInput.Statuses,
		Page:               patchesInput.Page,
		Limit:              patchesInput.Limit,
		IncludeCommitQueue: patchesInput.IncludeCommitQueue,
	}
	patches, count, err := patch.ByPatchNameStatusesCommitQueuePaginated(opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error getting patches for user %s: %s", utility.FromStringPtr(obj.UserID), err.Error()))
	}

	apiPatches := []*restModel.APIPatch{}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		if err = apiPatch.BuildFromService(p, nil); err != nil { // Injecting DB info into APIPatch is handled by the resolvers.
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("Error converting patch to APIPatch for patch %s : %s", p.Id, err.Error()))
		}
		apiPatches = append(apiPatches, &apiPatch)
	}
	return &Patches{Patches: apiPatches, FilteredPatchCount: count}, nil
}

// Permissions is the resolver for the permissions field.
func (r *userResolver) Permissions(ctx context.Context, obj *restModel.APIDBUser) (*Permissions, error) {
	return &Permissions{UserID: utility.FromStringPtr(obj.UserID)}, nil
}

// Target is the resolver for the target field.
func (r *subscriberInputResolver) Target(ctx context.Context, obj *restModel.APISubscriber, data string) error {
	panic(fmt.Errorf("not implemented: Target - target"))
}

// User returns UserResolver implementation.
func (r *Resolver) User() UserResolver { return &userResolver{r} }

// SubscriberInput returns SubscriberInputResolver implementation.
func (r *Resolver) SubscriberInput() SubscriberInputResolver { return &subscriberInputResolver{r} }

type userResolver struct{ *Resolver }
type subscriberInputResolver struct{ *Resolver }
