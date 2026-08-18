package graphql

import (
	"context"

	model1 "github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/parsley"
	"github.com/evergreen-ci/evergreen/rest/model"
)

// ParsleyFilters is the resolver for the parsleyFilters field.
func (r *repoRefResolver) ParsleyFilters(ctx context.Context, obj *model.APIProjectRef) ([]*parsley.Filter, error) {
	return apiParsleyFiltersToService(obj.ParsleyFilters), nil
}

// TaskOwnership is the resolver for the taskOwnership field.
func (r *repoRefResolver) TaskOwnership(ctx context.Context, obj *model.APIProjectRef) (*model1.TaskOwnershipSettings, error) {
	settings := obj.TaskOwnership.ToService()
	return &settings, nil
}

// TaskOwnership is the resolver for the taskOwnership field.
func (r *repoRefInputResolver) TaskOwnership(ctx context.Context, obj *model.APIProjectRef, data *model1.TaskOwnershipSettings) error {
	if data != nil {
		obj.TaskOwnership.BuildFromService(*data)
	}
	return nil
}

// RepoRef returns RepoRefResolver implementation.
func (r *Resolver) RepoRef() RepoRefResolver { return &repoRefResolver{r} }

// RepoRefInput returns RepoRefInputResolver implementation.
func (r *Resolver) RepoRefInput() RepoRefInputResolver { return &repoRefInputResolver{r} }

type repoRefResolver struct{ *Resolver }
type repoRefInputResolver struct{ *Resolver }
