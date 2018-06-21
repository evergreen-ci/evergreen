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
			TitleLink: spawnHostURL(t.uiConfig.Url, t.host.Id),
			Color:     "good",
			Fields: []*message.SlackAttachmentField{
				&message.SlackAttachmentField{
					Title: "distro",
					Value: t.host.Distro.Id,
					Short: true,
				},
				&message.SlackAttachmentField{
					Title: "ssh command",
					Value: sshCommand(t.host),
				},
			},
		}

	} else if t.event.EventType == event.EventHostProvisionError {
		text = fmt.Sprintf("Host with distro '%s' has failed to spawn", t.host.Distro)
		attachment = message.SlackAttachment{
			Title:     "Click here to spawn another host",
			TitleLink: spawnHostURL(t.uiConfig.Url, ""),
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

const spawnHostEmailSubjectTemplate string = `Evergreen Spawn Host with distro '%s' has %s`
const spawnHostEmailTemplate string = `<html>
<head>
</head>
<body>
<p>Hi,</p>

<p>The Evergreen <a href="%s">Spawn Host</a> in '%s' has %s.</p>
<p>SSH Command: %s</p>


</body>
</html>
`

func (t *hostTriggers) email() *message.Email {
	status := "failed to spawn"
	url := spawnHostURL(t.uiConfig.Url, t.host.Id)
	cmd := "N/A"
	if t.event.EventType == event.EventHostProvisionError {
		status = "spawned"
		url = spawnHostURL(t.uiConfig.Url, "")
		cmd = sshCommand(t.host)
	}

	return &message.Email{
		Subject:           fmt.Sprintf(spawnHostEmailSubjectTemplate, t.host.Distro.Id, status),
		Body:              fmt.Sprintf(spawnHostEmailTemplate, url, t.host.Distro.Id, status, cmd),
		PlainTextContents: false,
	}
}

func (t *hostTriggers) makePayload(sub *event.Subscription) interface{} {
	var payload interface{}
	if sub.Type == event.SlackSubscriberType {
		payload = t.slack()

	} else if sub.Type == event.EmailSubscriberType {
		payload = t.email()
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

func spawnHostURL(base, hostID string) string {
	if len(hostID) == 0 {
		return fmt.Sprintf("%s/spawn", base)
	}
	return fmt.Sprintf("%s/host/%s", base, hostID)
}

func sshCommand(h *host.Host) string {
	return fmt.Sprintf("ssh %s@%s", h.User, h.Host)
}
