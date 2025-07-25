package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// GithubTriggerAliases is the resolver for the githubTriggerAliases field.
func (r *repoRefResolver) GithubTriggerAliases(ctx context.Context, obj *model.APIProjectRef) ([]string, error) {
	return utility.FromStringPtrSlice(obj.GithubPRTriggerAliases), nil
}

// GithubTriggerAliases is the resolver for the githubTriggerAliases field.
func (r *repoRefInputResolver) GithubTriggerAliases(ctx context.Context, obj *model.APIProjectRef, data []string) error {
	obj.GithubPRTriggerAliases = utility.ToStringPtrSlice(data)
	return nil
}

// RepoRef returns RepoRefResolver implementation.
func (r *Resolver) RepoRef() RepoRefResolver { return &repoRefResolver{r} }

// RepoRefInput returns RepoRefInputResolver implementation.
func (r *Resolver) RepoRefInput() RepoRefInputResolver { return &repoRefInputResolver{r} }

type repoRefResolver struct{ *Resolver }
type repoRefInputResolver struct{ *Resolver }
