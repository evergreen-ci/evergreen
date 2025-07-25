package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
)

// GithubTriggerAliases is the resolver for the githubTriggerAliases field.
func (r *repoRefInputResolver) GithubTriggerAliases(ctx context.Context, obj *model.APIProjectRef, data []string) error {
	obj.GithubPRTriggerAliases = utility.ToStringPtrSlice(data)
	return nil
}

// RepoRefInput returns RepoRefInputResolver implementation.
func (r *Resolver) RepoRefInput() RepoRefInputResolver { return &repoRefInputResolver{r} }

type repoRefInputResolver struct{ *Resolver }
