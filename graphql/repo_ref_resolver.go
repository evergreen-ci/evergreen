package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/rest/model"
)

// ParsleyFilters is the resolver for the parsleyFilters field.
func (r *repoRefResolver) ParsleyFilters(ctx context.Context, obj *model.APIProjectRef) ([]*parsley.Filter, error) {
	return apiParsleyFiltersToService(obj.ParsleyFilters), nil
}

// RepoRef returns RepoRefResolver implementation.
func (r *Resolver) RepoRef() RepoRefResolver { return &repoRefResolver{r} }

type repoRefResolver struct{ *Resolver }
