package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/patch"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// ParsleyFilters is the resolver for the parsleyFilters field.
func (r *userResolver) ParsleyFilters(ctx context.Context, obj *restModel.APIDBUser) ([]*restModel.APIParsleyFilter, error) {
	res := []*restModel.APIParsleyFilter{}

	usr, err := user.FindOneById(utility.FromStringPtr(obj.UserID))
	if err != nil {
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("finding user with ID '%s': %s", utility.FromStringPtr(obj.UserID), err.Error()))
	}
	if usr == nil {
		return nil, ResourceNotFound.Send(ctx, "user not found")
	}

	for _, p := range usr.ParsleyFilters {
		parsleyFilter := restModel.APIParsleyFilter{}
		parsleyFilter.BuildFromService(p)
		res = append(res, &parsleyFilter)
	}
	return res, nil
}

// Patches is the resolver for the patches field.
func (r *userResolver) Patches(ctx context.Context, obj *restModel.APIDBUser, patchesInput PatchesInput) (*Patches, error) {
	opts := patch.ByPatchNameStatusesCommitQueuePaginatedOptions{
		Author:             obj.UserID,
		PatchName:          patchesInput.PatchName,
		Statuses:           patchesInput.Statuses,
		Page:               patchesInput.Page,
		Limit:              patchesInput.Limit,
		IncludeCommitQueue: patchesInput.IncludeCommitQueue,
		IncludeHidden:      patchesInput.IncludeHidden,
	}
	patches, count, err := patch.ByPatchNameStatusesCommitQueuePaginated(ctx, opts)
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
