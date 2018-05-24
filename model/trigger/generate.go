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

func (g *notificationGenerator) get(subType string) (interface{}, error) {
	var val interface{}
	switch subType {
	case event.EvergreenWebhookSubscriberType:
		val = g.evergreenWebhook

	case event.EmailSubscriberType:
		val = g.email

	case event.JIRAIssueSubscriberType:
		val = g.jiraIssue

	case event.JIRACommentSubscriberType:
		val = g.jiraComment

	case event.SlackSubscriberType:
		val = g.slack

	case event.GithubPullRequestSubscriberType:
		val = g.githubStatusAPI

	default:
		return nil, errors.Errorf("unknown type '%s'", subType)
	}

	if reflect.ValueOf(val).IsNil() {
		return nil, nil
	}

	return val, nil
}

func (g *notificationGenerator) isEmpty() bool {
	return g.evergreenWebhook == nil && g.email == nil && g.jiraIssue == nil &&
		g.jiraComment == nil && g.slack == nil && g.githubStatusAPI == nil
}

func (g *notificationGenerator) generate(e *event.EventLogEntry, subType string, sub event.Subscription) (*notification.Notification, error) {
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

	payload, err := g.get(subType)
	if err != nil {
		return nil, err
	}
	if payload == nil {
		return nil, errors.New("unable to determine payload")
	}

	// trigger data logic here
	notification, err := notification.New(e, g.triggerName, &sub.Subscriber, payload)
	if err != nil {
		return nil, err
	}

	return notification, nil
}
