package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostProvisioned, makeHostTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostProvisionError, makeHostTriggers)
}

type hostTriggers struct {
	event    *event.EventLogEntry
	data     *event.HostEventData
	host     *host.Host
	uiConfig evergreen.UIConfig

	base
}

func makeHostTriggers() eventHandler {
	t := &hostTriggers{}
	t.base.triggers = map[string]trigger{
		triggerOutcome: t.hostSpawnOutcome,
	}
	return t
}

func (t *hostTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
	t.host, err = host.FindOneId(e.ResourceId)
	if err != nil {
		return errors.Wrapf(err, "failed to fetch host '%s'", e.ResourceId)
	}
	if t.host == nil {
		return errors.Wrapf(err, "can't find host'%s'", e.ResourceId)
	}

	data, ok := e.Data.(*event.HostEventData)
	if !ok {
		return errors.Wrapf(err, "patch '%s' contains unexpected data with type '%T'", e.ResourceId, e.Data)
	}

	t.data = data
	t.event = e

	return nil
}

func (t *hostTriggers) Selectors() []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: t.host.Id,
		},
		{
			Type: selectorObject,
			Data: "host",
		},
		{
			Type: selectorOwner,
			Data: t.host.StartedBy,
		},
	}
}

func (t *hostTriggers) hostSpawnOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if !t.host.UserHost {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *hostTriggers) slack() *notification.SlackPayload {
	text := fmt.Sprintf("Host with distro '%s' has spawned", t.host.Distro)

	var attachment message.SlackAttachment
	if t.event.EventType == event.EventHostProvisioned {
		attachment = message.SlackAttachment{
			Title:     "Evergreen Host",
			TitleLink: fmt.Sprintf("%s/host/%s", t.uiConfig.Url, t.host.Id),
			Color:     "good",
			Fields: []*message.SlackAttachmentField{
				&message.SlackAttachmentField{
					Title: "distro",
					Value: t.host.Distro.Id,
					Short: true,
				},
				&message.SlackAttachmentField{
					Title: "ssh command",
					Value: fmt.Sprintf("ssh %s@%s", t.host.User, t.host.Host),
				},
			},
		}

	} else if t.event.EventType == event.EventHostProvisionError {
		text = fmt.Sprintf("Host with distro '%s' has failed to spawn", t.host.Distro)
		attachment = message.SlackAttachment{
			Title:     "Click here to spawn another host",
			TitleLink: fmt.Sprintf("%s/spawn", t.uiConfig.Url),
			Color:     "danger",
			Fields: []*message.SlackAttachmentField{
				&message.SlackAttachmentField{
					Title: "distro",
					Value: t.host.Distro.Id,
					Short: true,
				},
			},
		}
	}

	return &notification.SlackPayload{
		Body:        text,
		Attachments: []message.SlackAttachment{attachment},
	}
}

func (t *hostTriggers) email() (*notification.SlackPayload, error) {
	return nil, nil
}

func (t *hostTriggers) makePayload(sub *event.Subscription) interface{} {
	var payload interface{}
	if sub.Type == event.SlackSubscriberType {
		payload = t.slack()

	} else if sub.Type == event.EmailSubscriberType {
		payload, _ = t.email()
	}

	return payload
}

func (t *hostTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	payload := t.makePayload(sub)
	if payload == nil {
		return nil, nil
	}

	return notification.New(t.event, sub.Trigger, &sub.Subscriber, payload)
}
