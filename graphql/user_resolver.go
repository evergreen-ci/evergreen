package graphql

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
)

// HasTokenExchangePending is the resolver for the hasTokenExchangePending field.
func (r *userLiteResolver) HasTokenExchangePending(ctx context.Context, obj *user.DBUser) (bool, error) {
	return obj.TokenExchangeState != nil, nil
}

// Patches is the resolver for the patches field.
func (r *userLiteResolver) Patches(ctx context.Context, obj *user.DBUser, patchesInput PatchesInput) (*Patches, error) {
	// Return empty Patches - field resolvers will access patchesInput via Parent.Args
	return &Patches{}, nil
}

// Permissions is the resolver for the permissions field.
func (r *userLiteResolver) Permissions(ctx context.Context, obj *user.DBUser) (*Permissions, error) {
	return &Permissions{UserID: obj.Id}, nil
}

// Settings is the resolver for the settings field.
func (r *userLiteResolver) Settings(ctx context.Context, obj *user.DBUser) (*restModel.APIUserSettings, error) {
	settings := restModel.APIUserSettings{}
	settings.BuildFromService(obj.Settings)
	return &settings, nil
}

// Subscriptions is the resolver for the subscriptions field.
func (r *userLiteResolver) Subscriptions(ctx context.Context, obj *user.DBUser) ([]*restModel.APISubscription, error) {
	return getAPISubscriptionsForOwner(ctx, obj.Id, event.OwnerTypePerson)
}

// TokenAccessTokenExpiresAt is the resolver for the tokenAccessTokenExpiresAt field.
func (r *userLiteResolver) TokenAccessTokenExpiresAt(ctx context.Context, obj *user.DBUser) (*time.Time, error) {
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

// UserLite returns UserLiteResolver implementation.
func (r *Resolver) UserLite() UserLiteResolver { return &userLiteResolver{r} }

// SubscriberInput returns SubscriberInputResolver implementation.
func (r *Resolver) SubscriberInput() SubscriberInputResolver { return &subscriberInputResolver{r} }

type userLiteResolver struct{ *Resolver }
type subscriberInputResolver struct{ *Resolver }
