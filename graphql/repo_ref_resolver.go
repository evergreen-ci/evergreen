package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/rest/model"
)

// Private is the resolver for the private field.
func (r *repoRefResolver) Private(ctx context.Context, obj *model.APIProjectRef) (bool, error) {
	panic(fmt.Errorf("not implemented: Private - private"))
}

// Private is the resolver for the private field.
func (r *repoRefInputResolver) Private(ctx context.Context, obj *model.APIProjectRef, data *bool) error {
	panic(fmt.Errorf("not implemented: Private - private"))
}

// RepoRef returns RepoRefResolver implementation.
func (r *Resolver) RepoRef() RepoRefResolver { return &repoRefResolver{r} }

// RepoRefInput returns RepoRefInputResolver implementation.
func (r *Resolver) RepoRefInput() RepoRefInputResolver { return &repoRefInputResolver{r} }

type repoRefResolver struct{ *Resolver }
type repoRefInputResolver struct{ *Resolver }
