package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// Oauth is the resolver for the oauth field.
func (r *authConfigResolver) Oauth(ctx context.Context, obj *model.APIAuthConfig) (*OAuthConfig, error) {
	panic(fmt.Errorf("not implemented: Oauth - oauth"))
}

// Oauth is the resolver for the oauth field.
func (r *authConfigInputResolver) Oauth(ctx context.Context, obj *model.APIAuthConfig, data *OAuthConfigInput) error {
	panic(fmt.Errorf("not implemented: Oauth - oauth"))
}

// AuthConfig returns AuthConfigResolver implementation.
func (r *Resolver) AuthConfig() AuthConfigResolver { return &authConfigResolver{r} }

// AuthConfigInput returns AuthConfigInputResolver implementation.
func (r *Resolver) AuthConfigInput() AuthConfigInputResolver { return &authConfigInputResolver{r} }

type authConfigResolver struct{ *Resolver }
type authConfigInputResolver struct{ *Resolver }
