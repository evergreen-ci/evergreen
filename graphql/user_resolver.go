package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// Patches is the resolver for the patches field.
func (r *userResolver) Patches(ctx context.Context, obj *restModel.APIDBUser, patchesInput PatchesInput) (*Patches, error) {
	opts := patch.ByPatchNameStatusesMergeQueuePaginatedOptions{
		Author:        obj.UserID,
		PatchName:     patchesInput.PatchName,
		Statuses:      patchesInput.Statuses,
		Page:          patchesInput.Page,
		Limit:         patchesInput.Limit,
		IncludeHidden: patchesInput.IncludeHidden,
		Requesters:    patchesInput.Requesters,
	}
	patches, count, err := patch.ByPatchNameStatusesMergeQueuePaginated(ctx, opts)
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting patches for user '%s': %s", utility.FromStringPtr(obj.UserID), err.Error()))
	}

	apiPatches := []*restModel.APIPatch{}
	for _, p := range patches {
		apiPatch := restModel.APIPatch{}
		if err = apiPatch.BuildFromService(ctx, p, nil); err != nil { // Injecting DB info into APIPatch is handled by the resolvers.
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting patch to APIPatch for patch '%s': %s", p.Id, err.Error()))
		}
		apiPatches = append(apiPatches, &apiPatch)
	}
	return &Patches{Patches: apiPatches, FilteredPatchCount: count}, nil
}

// Permissions is the resolver for the permissions field.
func (r *userResolver) Permissions(ctx context.Context, obj *restModel.APIDBUser) (*Permissions, error) {
	return &Permissions{UserID: utility.FromStringPtr(obj.UserID)}, nil
}

// Subscriptions is the resolver for the subscriptions field.
func (r *userResolver) Subscriptions(ctx context.Context, obj *restModel.APIDBUser) ([]*restModel.APISubscription, error) {
	return getAPISubscriptionsForOwner(ctx, utility.FromStringPtr(obj.UserID), event.OwnerTypePerson)
}

// Target is the resolver for the target field.
func (r *subscriberInputResolver) Target(ctx context.Context, obj *restModel.APISubscriber, data string) error {
	obj.Target = data
	return nil
}

// User returns UserResolver implementation.
func (r *Resolver) User() UserResolver { return &userResolver{r} }

// SubscriberInput returns SubscriberInputResolver implementation.
func (r *Resolver) SubscriberInput() SubscriberInputResolver { return &subscriberInputResolver{r} }

type userResolver struct{ *Resolver }
type subscriberInputResolver struct{ *Resolver }
