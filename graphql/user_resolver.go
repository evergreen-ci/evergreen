package graphql

import (
	"context"
	"fmt"

	"github.com/99designs/gqlgen/graphql"
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
		CountLimit:    utility.FromIntPtr(patchesInput.CountLimit),
	}

	// Check which fields are requested in the query
	selectedFields := graphql.CollectAllFields(ctx)
	queriedPatches := false
	queriedCount := false
	for _, field := range selectedFields {
		if field == "patches" {
			queriedPatches = true
		}
		if field == "filteredPatchCount" {
			queriedCount = true
		}
	}

	// Fetch patches only if requested
	var err error
	apiPatches := []*restModel.APIPatch{}
	if queriedPatches {
		patches, err := patch.ByPatchNameStatusesMergeQueuePaginatedResults(ctx, opts)
		if err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("getting patches for user '%s': %s", utility.FromStringPtr(obj.UserID), err.Error()))
		}

		// Build API patches
		for _, p := range patches {
			apiPatch := restModel.APIPatch{}
			if err = apiPatch.BuildFromService(ctx, p, nil); err != nil { // Injecting DB info into APIPatch is handled by the resolvers.
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("converting patch to APIPatch for patch '%s': %s", p.Id, err.Error()))
			}
			apiPatches = append(apiPatches, &apiPatch)
		}
	}

	// Fetch count only if requested
	count := 0
	if queriedCount {
		// Optimization: if we fetched patches and got fewer than the limit, we know the total count
		if queriedPatches && len(apiPatches) < patchesInput.Limit {
			count = len(apiPatches)
		} else {
			// Either we didn't fetch patches (only count requested) or we hit the limit (might be more)
			count, err = patch.ByPatchNameStatusesMergeQueuePaginatedCount(ctx, opts)
			if err != nil {
				return nil, InternalServerError.Send(ctx, fmt.Sprintf("fetching patch count for user '%s': %s", utility.FromStringPtr(obj.UserID), err.Error()))
			}
		}
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
