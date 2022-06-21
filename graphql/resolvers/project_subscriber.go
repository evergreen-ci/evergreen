package resolvers

// This file will be automatically regenerated based on the schema, any resolver implementations
// will be copied through when generating and any unknown code will be moved to the end.

import (
	"context"
	"fmt"

	gqlError "github.com/evergreen-ci/evergreen/graphql/errors"
	"github.com/evergreen-ci/evergreen/graphql/generated"
	gqlModel "github.com/evergreen-ci/evergreen/graphql/model"
	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
	werrors "github.com/pkg/errors"
)

func (r *projectSubscriberResolver) Subscriber(ctx context.Context, obj *restModel.APISubscriber) (*gqlModel.Subscriber, error) {
	res := &gqlModel.Subscriber{}
	subscriberType := utility.FromStringPtr(obj.Type)

	switch subscriberType {
	case event.GithubPullRequestSubscriberType:
		sub := restModel.APIGithubPRSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("problem converting %s subscriber: %s",
				event.GithubPullRequestSubscriberType, err.Error()))
		}
		res.GithubPRSubscriber = &sub
	case event.GithubCheckSubscriberType:
		sub := restModel.APIGithubCheckSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.GithubCheckSubscriberType, err.Error()))
		}
		res.GithubCheckSubscriber = &sub

	case event.EvergreenWebhookSubscriberType:
		sub := restModel.APIWebhookSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.EvergreenWebhookSubscriberType, err.Error()))
		}
		res.WebhookSubscriber = &sub

	case event.JIRAIssueSubscriberType:
		sub := &restModel.APIJIRAIssueSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, gqlError.InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.JIRAIssueSubscriberType, err.Error()))
		}
		res.JiraIssueSubscriber = sub
	case event.JIRACommentSubscriberType:
		res.JiraCommentSubscriber = obj.Target.(*string)
	case event.EmailSubscriberType:
		res.EmailSubscriber = obj.Target.(*string)
	case event.SlackSubscriberType:
		res.SlackSubscriber = obj.Target.(*string)
	case event.EnqueuePatchSubscriberType:
		// We don't store information in target for this case, so do nothing.
	default:
		return nil, werrors.Errorf("unknown subscriber type: '%s'", subscriberType)
	}

	return res, nil
}

// ProjectSubscriber returns generated.ProjectSubscriberResolver implementation.
func (r *Resolver) ProjectSubscriber() generated.ProjectSubscriberResolver {
	return &projectSubscriberResolver{r}
}

type projectSubscriberResolver struct{ *Resolver }
