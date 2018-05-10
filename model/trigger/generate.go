package trigger

import (
	"reflect"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type notificationGenerator struct {
	triggerName string
	selectors   []event.Selector

	// Optional payloads
	evergreenWebhook *util.EvergreenWebhook
	email            *message.Email
	jiraIssue        *message.JiraIssue
	jiraComment      *string
	githubStatusAPI  *message.GithubStatus
	slack            *notification.SlackPayload
}

func (p *notificationGenerator) get(subType string) (interface{}, error) {
	var val interface{}
	switch subType {
	case event.EvergreenWebhookSubscriberType:
		val = p.evergreenWebhook

	case event.EmailSubscriberType:
		val = p.email

	case event.JIRAIssueSubscriberType:
		val = p.jiraIssue

	case event.JIRACommentSubscriberType:
		val = p.jiraComment

	case event.SlackSubscriberType:
		val = p.slack

	case event.GithubPullRequestSubscriberType:
		val = p.githubStatusAPI

	default:
		return nil, errors.Errorf("unknown type '%s'", subType)
	}

	if reflect.ValueOf(val).IsNil() {
		return nil, nil
	}

	return val, nil
}

func (p *notificationGenerator) isEmpty() bool {
	return p.evergreenWebhook == nil && p.email == nil && p.jiraIssue == nil &&
		p.jiraComment == nil && p.slack == nil && p.githubStatusAPI == nil
}

func (g *notificationGenerator) generate(e *event.EventLogEntry) ([]notification.Notification, error) {
	if len(g.triggerName) == 0 {
		return nil, errors.New("trigger name is empty")
	}
	if len(g.selectors) == 0 {
		return nil, errors.Errorf("trigger %s has no selectors", g.triggerName)
	}
	if g.isEmpty() {
		grip.Warning(message.Fields{
			"event_id": e.ID.Hex(),
			"trigger":  g.triggerName,
			"cause":    "programmer error",
			"message":  "a trigger created an empty generator; it should've just returned nil",
		})
		return nil, errors.New("generator has no payloads, and cannot yield any notifications")
	}

	groupedSubs, err := event.FindSubscriptions(e.ResourceType, g.triggerName, g.selectors)
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
	n := make([]notification.Notification, 0, num)

	catcher := grip.NewSimpleCatcher()
	for subType, subs := range groupedSubs {
		payload, err := g.get(subType)
		catcher.Add(err)
		if err != nil || payload == nil {
			continue
		}

		for i := range subs {
			notification, err := notification.New(e, g.triggerName, &subs[i].Subscriber, payload)
			if err != nil {
				catcher.Add(err)
				continue
			}
			n = append(n, *notification)
		}
	}

	return n, catcher.Resolve()
}
