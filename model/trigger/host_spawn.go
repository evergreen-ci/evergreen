package trigger

import (
	"fmt"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostProvisioned, makeSpawnHostTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostProvisionFailed, makeSpawnHostTriggers)
}

type spawnHostTriggers struct {
	event    *event.EventLogEntry
	data     *event.HostEventData
	host     *host.Host
	uiConfig evergreen.UIConfig

	base
}

func makeSpawnHostTriggers() eventHandler {
	t := &spawnHostTriggers{}
	t.base.triggers = map[string]trigger{
		triggerOutcome: t.hostSpawnOutcome,
	}
	return t
}

func (t *spawnHostTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
	if err = t.uiConfig.Get(); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

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

func (t *spawnHostTriggers) Selectors() []event.Selector {
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

func (t *spawnHostTriggers) hostSpawnOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if t.host.StartedBy == evergreen.User {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *spawnHostTriggers) slack() *notification.SlackPayload {
	text := fmt.Sprintf("Host has failed to spawn", t.host.Distro.Id)
	attachment := message.SlackAttachment{
		Title:     "Click here to spawn another host",
		TitleLink: spawnHostURL(t.uiConfig.Url),
		Color:     evergreenFailColor,
		Fields: []*message.SlackAttachmentField{
			&message.SlackAttachmentField{
				Title: "Distro",
				Value: t.host.Distro.Id,
				Short: true,
			},
		},
	}
	if t.host.ProvisionOptions != nil && len(t.host.ProvisionOptions.TaskId) != 0 {
		attachment.Fields = append(attachment.Fields,
			&message.SlackAttachmentField{
				Title: "With data from task",
				Value: fmt.Sprintf("<%s|%s>", taskLink(&t.uiConfig, t.host.ProvisionOptions.TaskId, -1), t.host.ProvisionOptions.TaskId),
				Short: true,
			})
	}

	if t.host.Provisioned {
		text = fmt.Sprintf("Host has spawned", t.host.Distro.Id)
		attachment.Title = fmt.Sprintf("Evergreen Host: %s", t.host.Id)
		attachment.TitleLink = spawnHostURL(t.uiConfig.Url)
		attachment.Color = evergreenSuccessColor

		attachment.Fields = append(attachment.Fields,
			&message.SlackAttachmentField{
				Title: "SSH Command",
				Value: fmt.Sprintf("`%s`", sshCommand(t.host)),
			})
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

<p>The Evergreen Spawn Host <a href="%s">%s</a> with distro '%s' has %s.</p>
<p>SSH Command: %s</p>


</body>
</html>
`

func (t *spawnHostTriggers) email() *message.Email {
	status := "failed to spawn"
	url := spawnHostURL(t.uiConfig.Url)
	cmd := "N/A"
	if t.host.Provisioned {
		status = "spawned"
		url = spawnHostURL(t.uiConfig.Url)
		cmd = sshCommand(t.host)
	}

	return &message.Email{
		Subject:           fmt.Sprintf(spawnHostEmailSubjectTemplate, t.host.Distro.Id, status),
		Body:              fmt.Sprintf(spawnHostEmailTemplate, url, t.host.Id, t.host.Distro.Id, status, cmd),
		PlainTextContents: false,
	}
}

func (t *spawnHostTriggers) makePayload(sub *event.Subscription) interface{} {
	var payload interface{}
	if sub.Subscriber.Type == event.SlackSubscriberType {
		payload = t.slack()

	} else if sub.Subscriber.Type == event.EmailSubscriberType {
		payload = t.email()
	}

	return payload
}

func (t *spawnHostTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	payload := t.makePayload(sub)
	grip.Info(message.Fields{
		"look-here":   "foidsjfhkafhaiwf",
		"pl":          payload == nil,
		"e":           t.event.EventType,
		"provisioned": t.host.Provisioned,
	})
	if payload == nil {
		return nil, nil
	}

	return notification.New(t.event, sub.Trigger, &sub.Subscriber, payload)
}

func spawnHostURL(base string) string {
	return fmt.Sprintf("%s/spawn", base)
}

func sshCommand(h *host.Host) string {
	return fmt.Sprintf("ssh %s@%s", h.Distro.User, h.Host)
}
