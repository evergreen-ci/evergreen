package graphql

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"

	"github.com/evergreen-ci/evergreen/thirdparty"
)

func (r *ticketFieldsResolver) AssignedTeam(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.AssignedTeam == nil {
		return nil, nil
	}
	if len(obj.AssignedTeam) != 0 {
		return &obj.AssignedTeam[0].Value, nil
	}
	return nil, nil
}

func (r *ticketFieldsResolver) AssigneeDisplayName(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.Assignee == nil {
		return nil, nil
	}
	return &obj.Assignee.DisplayName, nil
}

func (r *ticketFieldsResolver) ResolutionName(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.Resolution == nil {
		return nil, nil
	}
	return &obj.Resolution.Name, nil
}

// TicketFields returns TicketFieldsResolver implementation.
func (r *Resolver) TicketFields() TicketFieldsResolver { return &ticketFieldsResolver{r} }

type ticketFieldsResolver struct{ *Resolver }
