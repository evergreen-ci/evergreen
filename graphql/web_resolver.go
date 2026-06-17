package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/rest/model"
)

// BetaFeatures is the resolver for the betaFeatures field.
func (r *uIConfigResolver) BetaFeatures(ctx context.Context, obj *model.APIUIConfig) (*evergreen.BetaFeatures, error) {
	betaFeatures := obj.BetaFeatures.ToService()
	return &betaFeatures, nil
}

// UIConfig returns UIConfigResolver implementation.
func (r *Resolver) UIConfig() UIConfigResolver { return &uIConfigResolver{r} }

type uIConfigResolver struct{ *Resolver }
