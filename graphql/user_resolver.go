package graphql

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
)

// HasTokenExchangePending is the resolver for the hasTokenExchangePending field.
func (r *userResolver) HasTokenExchangePending(ctx context.Context, obj *user.DBUser) (bool, error) {
	return obj.TokenExchangeState != nil, nil
}

// Patches is the resolver for the patches field.
func (r *userResolver) Patches(ctx context.Context, obj *user.DBUser, patchesInput PatchesInput) (*Patches, error) {
	// Return empty Patches - field resolvers will access patchesInput via Parent.Args
	return &Patches{}, nil
}

// Permissions is the resolver for the permissions field.
func (r *userResolver) Permissions(ctx context.Context, obj *user.DBUser) (*Permissions, error) {
	return &Permissions{UserID: obj.Id}, nil
}

// Settings is the resolver for the settings field.
func (r *userResolver) Settings(ctx context.Context, obj *user.DBUser) (*restModel.APIUserSettings, error) {
	settings := restModel.APIUserSettings{}
	settings.BuildFromService(obj.Settings)
	return &settings, nil
}

// Subscriptions is the resolver for the subscriptions field.
func (r *userResolver) Subscriptions(ctx context.Context, obj *user.DBUser) ([]*restModel.APISubscription, error) {
	return getAPISubscriptionsForOwner(ctx, obj.Id, event.OwnerTypePerson)
}

// TokenAccessTokenExpiresAt is the resolver for the tokenAccessTokenExpiresAt field.
func (r *userResolver) TokenAccessTokenExpiresAt(ctx context.Context, obj *user.DBUser) (*time.Time, error) {
	if tok := obj.TokenExchangeToken; tok != nil && !tok.Expiry.IsZero() {
		// Use UTC so GraphQL Time marshaling is stable across server local TZ (RFC3339 Z).
		return restModel.ToTimePtr(tok.Expiry.UTC()), nil
	}
	return nil, nil
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
