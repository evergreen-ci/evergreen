package notification

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type payloads struct {
	evergreenWebhook *string
	email            *EmailPayload
	jiraIssue        *message.JiraIssue
	jiraComment      *string
	githubStatusAPI  *GithubStatusAPIPayload
	slack            *string
}

func (p *payloads) get(subType string) (interface{}, error) {
	switch subType {
	case event.EvergreenWebhookSubscriberType:
		return p.evergreenWebhook, nil

	case event.EmailSubscriberType:
		return p.email, nil

	case event.JIRAIssueSubscriberType:
		return p.jiraIssue, nil

	case event.JIRACommentSubscriberType:
		return p.jiraComment, nil

	case event.SlackSubscriberType:
		return p.slack, nil

	case event.GithubPullRequestSubscriberType:
		return p.githubStatusAPI, nil

	default:
		return nil, errors.Errorf("unknown type '%s'", subType)
	}
}

func (p *payloads) generateNotifications(e *event.EventLogEntry, triggerName string, selectors []event.Selector) ([]Notification, error) {
	groupedSubs, err := event.FindSubscribers(e.ResourceType, triggerName, selectors)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch subscribers")
	}

	num := 0
	for _, v := range groupedSubs {
		num += len(v)
	}
	if num == 0 {
		return nil, nil
	}
	n := make([]Notification, 0, num)

	catcher := grip.NewSimpleCatcher()
	for subType, subs := range groupedSubs {
		payload, err := p.get(subType)
		catcher.Add(err)
		if err != nil {
			continue
		}

		for i := range subs {
			notification, err := New(e, triggerName, &subs[i], payload)
			if err != nil {
				catcher.Add(err)
				continue
			}
			n = append(n, *notification)
		}
	}

	return n, catcher.Resolve()
}
