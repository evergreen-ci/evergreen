package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// Patches is the resolver for the patches field.
func (r *userResolver) Patches(ctx context.Context, obj *restModel.APIDBUser, patchesInput PatchesInput) (*Patches, error) {
	// Return empty Patches - field resolvers will access patchesInput via Parent.Args
	return &Patches{}, nil
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

// !!! WARNING !!!
// The code below was going to be deleted when updating resolvers. It has been copied here so you have
// one last chance to move it out of harms way if you want. There are two reasons this happens:
//  - When renaming or deleting a resolver the old code will be put in here. You can safely delete
//    it when you're done.
//  - You have helper methods in this file. Move them out to keep these resolver files clean.
/*
	func (r *Resolver) UserLite() UserLiteResolver { return &userLiteResolver{r} }
type userLiteResolver struct{ *Resolver }
*/
