package graphql

import (
	"context"
	"fmt"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/utility"
	"github.com/mitchellh/mapstructure"
)

// Subscriber is the resolver for the subscriber field.
func (r *subscriberWrapperResolver) Subscriber(ctx context.Context, obj *model.APISubscriber) (*Subscriber, error) {
	res := &Subscriber{}
	subscriberType := utility.FromStringPtr(obj.Type)

	switch subscriberType {
	case event.GithubPullRequestSubscriberType:
		sub := model.APIGithubPRSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem converting %s subscriber: %s",
				event.GithubPullRequestSubscriberType, err.Error()))
		}
		res.GithubPRSubscriber = &sub
	case event.GithubCheckSubscriberType:
		sub := model.APIGithubCheckSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.GithubCheckSubscriberType, err.Error()))
		}
		res.GithubCheckSubscriber = &sub

	case event.EvergreenWebhookSubscriberType:
		sub := model.APIWebhookSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.EvergreenWebhookSubscriberType, err.Error()))
		}
		res.WebhookSubscriber = &sub

	case event.JIRAIssueSubscriberType:
		sub := &model.APIJIRAIssueSubscriber{}
		if err := mapstructure.Decode(obj.Target, &sub); err != nil {
			return nil, InternalServerError.Send(ctx, fmt.Sprintf("problem building %s subscriber from service: %s",
				event.JIRAIssueSubscriberType, err.Error()))
		}
		res.JiraIssueSubscriber = sub
	case event.JIRACommentSubscriberType:
		res.JiraCommentSubscriber = obj.Target.(*string)
	case event.EmailSubscriberType:
		res.EmailSubscriber = obj.Target.(*string)
	case event.SlackSubscriberType:
		res.SlackSubscriber = obj.Target.(*string)
	default:
		return nil, InternalServerError.Send(ctx, fmt.Sprintf("encountered unknown subscriber type '%s'", subscriberType))
	}

	return res, nil
}

// SubscriberWrapper returns SubscriberWrapperResolver implementation.
func (r *Resolver) SubscriberWrapper() SubscriberWrapperResolver {
	return &subscriberWrapperResolver{r}
}

type subscriberWrapperResolver struct{ *Resolver }
