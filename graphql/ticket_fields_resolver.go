package graphql

import (
	"context"

	"github.com/evergreen-ci/evergreen/thirdparty"
)

// AssignedTeam is the resolver for the assignedTeam field.
func (r *ticketFieldsResolver) AssignedTeam(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.AssignedTeam == nil {
		return nil, nil
	}
	if len(obj.AssignedTeam) != 0 {
		return &obj.AssignedTeam[0].Value, nil
	}
	return nil, nil
}

// AssigneeDisplayName is the resolver for the assigneeDisplayName field.
func (r *ticketFieldsResolver) AssigneeDisplayName(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.Assignee == nil {
		return nil, nil
	}
	return &obj.Assignee.DisplayName, nil
}

// ResolutionName is the resolver for the resolutionName field.
func (r *ticketFieldsResolver) ResolutionName(ctx context.Context, obj *thirdparty.TicketFields) (*string, error) {
	if obj.Resolution == nil {
		return nil, nil
	}
	return &obj.Resolution.Name, nil
}

// TicketFields returns TicketFieldsResolver implementation.
func (r *Resolver) TicketFields() TicketFieldsResolver { return &ticketFieldsResolver{r} }

type ticketFieldsResolver struct{ *Resolver }
