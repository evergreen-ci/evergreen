package graphql

import (
	"context"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/model/user"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// BetaFeatures is the resolver for the betaFeatures field.
func (r *userResolver) BetaFeatures(ctx context.Context, obj *restModel.APIDBUser) (*evergreen.BetaFeatures, error) {
	betaFeatures := obj.BetaFeatures.ToService()
	return &betaFeatures, nil
}

// ParsleyFilters is the resolver for the parsleyFilters field.
func (r *userResolver) ParsleyFilters(ctx context.Context, obj *restModel.APIDBUser) ([]*parsley.Filter, error) {
	return apiParsleyFiltersToService(obj.ParsleyFilters), nil
}

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

// User returns UserResolver implementation.
func (r *Resolver) User() UserResolver { return &userResolver{r} }

// UserLite returns UserLiteResolver implementation.
func (r *Resolver) UserLite() UserLiteResolver { return &userLiteResolver{r} }

// SubscriberInput returns SubscriberInputResolver implementation.
func (r *Resolver) SubscriberInput() SubscriberInputResolver { return &subscriberInputResolver{r} }

type userResolver struct{ *Resolver }
type userLiteResolver struct{ *Resolver }
type subscriberInputResolver struct{ *Resolver }
