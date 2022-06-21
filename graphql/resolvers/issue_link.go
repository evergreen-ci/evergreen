package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/evergreen-ci/evergreen/graphql/generated"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
)

func (r *issueLinkResolver) JiraTicket(ctx context.Context, obj *restModel.APIIssueLink) (*thirdparty.JiraTicket, error) {
	return restModel.GetJiraTicketFromURL(*obj.URL)
}

// IssueLink returns generated.IssueLinkResolver implementation.
func (r *Resolver) IssueLink() generated.IssueLinkResolver { return &issueLinkResolver{r} }

type issueLinkResolver struct{ *Resolver }
