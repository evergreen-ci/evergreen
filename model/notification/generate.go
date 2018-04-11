package notification

import (
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type notificationGenerator struct {
	triggerName string
	selectors   []event.Selector

	// Optional payloads
	evergreenWebhook *evergreenWebhookPayload
	email            *message.Email
	jiraIssue        *message.JiraIssue
	jiraComment      *string
	githubStatusAPI  *message.GithubStatus
	slack            *string
}

func (p *notificationGenerator) get(subType string) (interface{}, error) {
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

func (g *notificationGenerator) generate(e *event.EventLogEntry) ([]Notification, error) {
	if len(g.triggerName) == 0 {
		return nil, errors.New("trigger name is empty")
	}
	if len(g.selectors) == 0 {
		return nil, errors.Errorf("trigger %s has no selectors", g.triggerName)
	}

	groupedSubs, err := event.FindSubscribers(e.ResourceType, g.triggerName, g.selectors)
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
		payload, err := g.get(subType)
		catcher.Add(err)
		if err != nil {
			continue
		}

		for i := range subs {
			notification, err := New(e, g.triggerName, &subs[i], payload)
			if err != nil {
				catcher.Add(err)
				continue
			}
			n = append(n, *notification)
		}
	}

	return n, catcher.Resolve()
}
