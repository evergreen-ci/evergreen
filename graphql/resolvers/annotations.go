package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"
	"strconv"

	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/graphql/resolvers/util"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/annotations"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/rest/data"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/thirdparty"
	evgUtil "github.com/evergreen-ci/evergreen/util"
)

func (r *annotationResolver) WebhookConfigured(ctx context.Context, obj *restModel.APITaskAnnotation) (bool, error) {
	t, err := task.FindOneId(*obj.TaskId)
	if err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("error finding task: %s", err.Error()))
	}
	if t == nil {
		return false, gqlError.ResourceNotFound.Send(ctx, "error finding task for the task annotation")
	}
	_, ok, _ := model.IsWebhookConfigured(t.Project, t.Version)
	return ok, nil
}

func (r *issueLinkResolver) JiraTicket(ctx context.Context, obj *restModel.APIIssueLink) (*thirdparty.JiraTicket, error) {
	return restModel.GetJiraTicketFromURL(*obj.URL)
}

func (r *mutationResolver) BbCreateTicket(ctx context.Context, taskID string, execution *int) (bool, error) {
	httpStatus, err := data.BbFileTicket(ctx, taskID, *execution)
	if err != nil {
		return false, util.MapHTTPStatusToGqlError(ctx, httpStatus, err)
	}
	return true, nil
}

func (r *mutationResolver) AddAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	usr := util.MustHaveUser(ctx)
	issue := restModel.APIIssueLinkToService(apiIssue)
	if err := evgUtil.CheckURL(issue.URL); err != nil {
		return false, gqlError.InputValidationError.Send(ctx, fmt.Sprintf("issue does not have valid URL: %s", err.Error()))
	}
	if isIssue {
		if err := annotations.AddIssueToAnnotation(taskID, execution, *issue, usr.Username()); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't add issue: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.AddSuspectedIssueToAnnotation(taskID, execution, *issue, usr.Username()); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't add suspected issue: %s", err.Error()))
		}
		return true, nil
	}
}

func (r *mutationResolver) EditAnnotationNote(ctx context.Context, taskID string, execution int, originalMessage string, newMessage string) (bool, error) {
	usr := util.MustHaveUser(ctx)
	if err := annotations.UpdateAnnotationNote(taskID, execution, originalMessage, newMessage, usr.Username()); err != nil {
		return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't update note: %s", err.Error()))
	}
	return true, nil
}

func (r *mutationResolver) MoveAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	usr := util.MustHaveUser(ctx)
	issue := restModel.APIIssueLinkToService(apiIssue)
	if isIssue {
		if err := annotations.MoveIssueToSuspectedIssue(taskID, execution, *issue, usr.Username()); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't move issue to suspected issues: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.MoveSuspectedIssueToIssue(taskID, execution, *issue, usr.Username()); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't move issue to suspected issues: %s", err.Error()))
		}
		return true, nil
	}
}

func (r *mutationResolver) RemoveAnnotationIssue(ctx context.Context, taskID string, execution int, apiIssue restModel.APIIssueLink, isIssue bool) (bool, error) {
	issue := restModel.APIIssueLinkToService(apiIssue)
	if isIssue {
		if err := annotations.RemoveIssueFromAnnotation(taskID, execution, *issue); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't delete issue: %s", err.Error()))
		}
		return true, nil
	} else {
		if err := annotations.RemoveSuspectedIssueFromAnnotation(taskID, execution, *issue); err != nil {
			return false, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("couldn't delete suspected issue: %s", err.Error()))
		}
		return true, nil
	}
}

func (r *queryResolver) BbGetCreatedTickets(ctx context.Context, taskID string) ([]*thirdparty.JiraTicket, error) {
	createdTickets, err := util.BBGetCreatedTicketsPointers(taskID)
	if err != nil {
		return nil, err
	}

	return createdTickets, nil
}

func (r *queryResolver) BuildBaron(ctx context.Context, taskID string, execution int) (*gqlModel.BuildBaron, error) {
	execString := strconv.Itoa(execution)

	searchReturnInfo, bbConfig, err := model.GetSearchReturnInfo(taskID, execString)
	if err != nil {
		return nil, gqlError.InternalServerError.Send(ctx, err.Error())
	}

	return &gqlModel.BuildBaron{
		SearchReturnInfo:        searchReturnInfo,
		BuildBaronConfigured:    bbConfig.ProjectFound && bbConfig.SearchConfigured,
		BbTicketCreationDefined: bbConfig.TicketCreationDefined,
	}, nil
}

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

// Annotation returns generated.AnnotationResolver implementation.
func (r *Resolver) Annotation() generated.AnnotationResolver { return &annotationResolver{r} }

// IssueLink returns generated.IssueLinkResolver implementation.
func (r *Resolver) IssueLink() generated.IssueLinkResolver { return &issueLinkResolver{r} }

// TicketFields returns generated.TicketFieldsResolver implementation.
func (r *Resolver) TicketFields() generated.TicketFieldsResolver { return &ticketFieldsResolver{r} }

type annotationResolver struct{ *Resolver }
type issueLinkResolver struct{ *Resolver }
type ticketFieldsResolver struct{ *Resolver }
