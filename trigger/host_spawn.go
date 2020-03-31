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
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostProvisioned, makeSpawnHostProvisioningTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostProvisionFailed, makeSpawnHostProvisioningTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostStarted, makeSpawnHostStateChangeTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostStopped, makeSpawnHostStateChangeTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostModified, makeSpawnHostStateChangeTriggers)
}

type spawnHostProvisioningTriggers struct {
	hostBase
}

func makeSpawnHostProvisioningTriggers() eventHandler {
	t := &spawnHostProvisioningTriggers{}
	t.triggers = map[string]trigger{
		event.TriggerOutcome: t.hostSpawnProvisionOutcome,
	}
	return t
}

func (t *spawnHostProvisioningTriggers) hostSpawnProvisionOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if !t.host.UserHost {
		return nil, nil
	}

	return t.generate(sub)
}

func (t *spawnHostProvisioningTriggers) slack() *notification.SlackPayload {
	text := "Host has failed to spawn"
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
				Value: fmt.Sprintf("<%s|%s>", taskLink(t.uiConfig.Url, t.host.ProvisionOptions.TaskId, -1), t.host.ProvisionOptions.TaskId),
				Short: true,
			})
	}

	if t.host.Provisioned && t.host.Status == evergreen.HostRunning {
		text = "Host has spawned"
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

func (t *spawnHostProvisioningTriggers) email() *message.Email {
	status := "failed to spawn"
	url := spawnHostURL(t.uiConfig.Url)
	cmd := "N/A"
	if t.host.Provisioned && t.host.Status == evergreen.HostRunning {
		status = "spawned"
		cmd = sshCommand(t.host)
	}

	return &message.Email{
		Subject:           fmt.Sprintf(spawnHostEmailSubjectTemplate, t.host.Distro.Id, status),
		Body:              fmt.Sprintf(spawnHostEmailTemplate, url, t.host.Id, t.host.Distro.Id, status, cmd),
		PlainTextContents: false,
	}
}

func (t *spawnHostProvisioningTriggers) makePayload(sub *event.Subscription) interface{} {
	switch sub.Subscriber.Type {
	case event.SlackSubscriberType:
		return t.slack()

	case event.EmailSubscriberType:
		return t.email()

	default:
		return nil
	}
}

func (t *spawnHostProvisioningTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	payload := t.makePayload(sub)
	if payload == nil {
		return nil, errors.Errorf("unsupported subscriber type: %s", sub.ResourceType)
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func spawnHostURL(base string) string {
	return fmt.Sprintf("%s/spawn", base)
}

func hostURL(base, hostID string) string {
	return fmt.Sprintf("%s/host/%s", base, hostID)
}

func sshCommand(h *host.Host) string {
	return fmt.Sprintf("ssh %s@%s", h.Distro.User, h.Host)
}

type spawnHostStateChangeTriggers struct {
	hostBase
}

func makeSpawnHostStateChangeTriggers() eventHandler {
	t := &spawnHostStateChangeTriggers{}
	t.triggers = map[string]trigger{
		event.TriggerOutcome: t.spawnHostStateChangeOutcome,
	}
	return t
}

func (t *spawnHostStateChangeTriggers) spawnHostStateChangeOutcome(sub *event.Subscription) (*notification.Notification, error) {
	if !t.host.UserHost {
		return nil, nil
	}

	payload, err := t.makePayload(sub)
	if err != nil {
		return nil, errors.Wrap(err, "can't make payload")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *spawnHostStateChangeTriggers) makePayload(sub *event.Subscription) (interface{}, error) {
	var action string
	switch t.event.EventType {
	case event.EventHostStarted:
		action = "Starting"
	case event.EventHostStopped:
		action = "Stopping"
	case event.EventHostModified:
		action = "Modifying"
	default:
		return nil, errors.Errorf("unexpected event type '%s'", t.event.EventType)
	}

	var result string
	if t.data.Successful {
		result = "succeeded"
	} else {
		result = "failed"
	}

	switch sub.Subscriber.Type {
	case event.SlackSubscriberType:
		return t.slackPayload(action, result, t.host.Id, spawnHostURL(t.uiConfig.Url), hostURL(t.uiConfig.Url, t.host.Id)), nil

	case event.EmailSubscriberType:
		return t.emailPayload(action, result, t.host.Id, spawnHostURL(t.uiConfig.Url), hostURL(t.uiConfig.Url, t.host.Id)), nil

	default:
		return nil, errors.Errorf("unexpected subscriber type '%s'", sub.Subscriber.Type)
	}
}

func (t *spawnHostStateChangeTriggers) slackPayload(action, result, hostID, url, hostURL string) *notification.SlackPayload {
	color := evergreenSuccessColor
	if !t.data.Successful {
		color = evergreenFailColor
	}

	return &notification.SlackPayload{
		Body: fmt.Sprintf("%s spawn host <%s|%s> has %s", action, hostURL, hostID, result),
		Attachments: []message.SlackAttachment{
			{
				Title:     "Spawn hosts page",
				TitleLink: url,
				Color:     color,
			},
		},
	}
}

func (t *spawnHostStateChangeTriggers) emailPayload(action, result, hostID, url, hostURL string) *message.Email {
	subject := fmt.Sprintf("%s spawn host has %s", action, result)
	body := fmt.Sprintf(`<html>
	<head>
	</head>
	<body>
	<p>Hi,</p>
	<p>%s spawn host <a href="%s">%s</a> has %s.</p>
	<p>Manage spawn hosts from <a href="%s">your hosts page</a> or the Evergreen CLI</p>


	</body>
	</html>
	`, action, hostURL, hostID, result, url)

	return &message.Email{
		Subject:           subject,
		Body:              body,
		PlainTextContents: false,
	}
}
