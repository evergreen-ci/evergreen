package graphql

import (
	"context"

	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
)

// JiraTicket is the resolver for the jiraTicket field.
func (r *issueLinkResolver) JiraTicket(ctx context.Context, obj *restModel.APIIssueLink) (*thirdparty.JiraTicket, error) {
	return restModel.GetJiraTicketFromURL(*obj.URL)
}

// IssueLink returns IssueLinkResolver implementation.
func (r *Resolver) IssueLink() IssueLinkResolver { return &issueLinkResolver{r} }

type issueLinkResolver struct{ *Resolver }
