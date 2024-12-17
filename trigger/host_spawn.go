package trigger

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostProvisioned, makeSpawnHostProvisioningTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostProvisionFailed, makeSpawnHostProvisioningTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventSpawnHostCreatedError, makeSpawnHostStateChangeTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostStarted, makeSpawnHostStateChangeTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostStopped, makeSpawnHostStateChangeTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostModified, makeSpawnHostStateChangeTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostScriptExecuted, makeSpawnHostSetupScriptTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostScriptExecuteFailed, makeSpawnHostSetupScriptTriggers)
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

func (t *spawnHostProvisioningTriggers) hostSpawnProvisionOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
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
			{
				Title: "Distro",
				Value: t.host.Distro.Id,
				Short: true,
			},
		},
	}
	if t.host.ProvisionOptions != nil && t.host.ProvisionOptions.TaskId != "" {
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

		if t.host.IsVirtualWorkstation {
			attachment.Fields = append(attachment.Fields,
				&message.SlackAttachmentField{
					Title: "IDE",
					Value: fmt.Sprintf("<%s/host/%s/ide|IDE>", t.uiConfig.Url, t.host.Id),
				})
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
		return nil, errors.Errorf("unsupported subscriber type '%s'", sub.Subscriber.Type)
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
	cmd := []string{"ssh"}
	if port := h.GetSSHPort(); port != host.DefaultSSHPort {
		cmd = append(cmd, "-p", strconv.Itoa(port))
	}
	cmd = append(cmd, fmt.Sprintf("%s@%s", h.User, h.Host))
	return strings.Join(cmd, " ")
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

func (t *spawnHostStateChangeTriggers) spawnHostStateChangeOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !t.host.UserHost {
		return nil, nil
	}
	if t.event.EventType == event.EventHostStarted && t.data.Successful && t.host.Status != evergreen.HostStarting {
		// When the host is starting up, send a notification only if:
		// * There was an error starting the host or
		// * If it successfully started the host and it's still starting up now.
		//   There will be a notification later on when the host is up and
		//   running.
		return nil, nil
	}
	if t.data.Successful && t.data.Source == string(evergreen.ModifySpawnHostSleepSchedule) {
		// Skip notifying for host modifications due to the sleep schedule. They
		// can be noisy if users regularly receive them.
		return nil, nil
	}

	payload, err := t.makePayload(sub)
	if err != nil {
		return nil, errors.Wrap(err, "making payload")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *spawnHostStateChangeTriggers) makePayload(sub *event.Subscription) (interface{}, error) {
	var action string
	switch t.event.EventType {
	case event.EventSpawnHostCreatedError, event.EventHostCreatedError:
		action = "Creating"
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

type spawnHostSetupScriptTriggers struct {
	hostBase
}

func makeSpawnHostSetupScriptTriggers() eventHandler {
	t := &spawnHostSetupScriptTriggers{}
	t.triggers = map[string]trigger{
		event.TriggerOutcome: t.spawnHostSetupScriptOutcome,
	}
	return t
}

func (t *spawnHostSetupScriptTriggers) spawnHostSetupScriptOutcome(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if !t.host.UserHost {
		return nil, nil
	}
	payload, err := t.makePayload(sub)
	if err != nil {
		return nil, errors.Wrap(err, "making payload")
	}
	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *spawnHostSetupScriptTriggers) makePayload(sub *event.Subscription) (interface{}, error) {
	var result string
	switch t.event.EventType {
	case event.EventHostScriptExecuted:
		result = "succeeded"
	case event.EventHostScriptExecuteFailed:
		result = "failed"
		if strings.Contains(t.data.Logs, evergreen.FetchingTaskDataUnfinishedError) {
			result = "failed to start"
		}
	default:
		return nil, errors.Errorf("unexpected event type '%s'", t.event.EventType)
	}
	var spawnHostScriptPath string
	if t.host.ProvisionOptions != nil && t.host.ProvisionOptions.TaskId != "" {
		pRef, err := model.GetProjectRefForTask(t.host.ProvisionOptions.TaskId)
		if err != nil {
			return "", errors.Wrap(err, "getting project")
		}
		if pRef != nil {
			spawnHostScriptPath = pRef.SpawnHostScriptPath
		}
	}

	switch sub.Subscriber.Type {
	case event.SlackSubscriberType:
		return t.slackPayload(spawnHostScriptPath, result, t.host.Id, spawnHostURL(t.uiConfig.Url), hostURL(t.uiConfig.Url, t.host.Id)), nil
	case event.EmailSubscriberType:
		return t.emailPayload(spawnHostScriptPath, result, t.host.Id, spawnHostURL(t.uiConfig.Url), hostURL(t.uiConfig.Url, t.host.Id)), nil
	default:
		return nil, errors.Errorf("unexpected subscriber type '%s'", sub.Subscriber.Type)
	}
}

func (t *spawnHostSetupScriptTriggers) slackPayload(spawnHostScriptPath, result, hostID, url, hostURL string) *notification.SlackPayload {
	color := evergreenSuccessColor
	if !t.data.Successful {
		color = evergreenFailColor
	}

	attachment := message.SlackAttachment{
		Title:     "Spawn hosts page",
		TitleLink: url,
		Color:     color,
		Fields: []*message.SlackAttachmentField{
			{
				Title: "SSH Command",
				Value: fmt.Sprintf("`%s`", sshCommand(t.host)),
			},
		},
	}
	if spawnHostScriptPath != "" {
		attachment.Fields = append(attachment.Fields,
			&message.SlackAttachmentField{
				Title: "With data from task",
				Value: fmt.Sprintf("<%s|%s>", taskLink(t.uiConfig.Url, t.host.ProvisionOptions.TaskId, -1), t.host.ProvisionOptions.TaskId),
				Short: true,
			})
		attachment.Fields = append(attachment.Fields,
			&message.SlackAttachmentField{
				Title: "Setup Script Path",
				Value: spawnHostScriptPath,
			})
	}
	payload := &notification.SlackPayload{
		Body:        fmt.Sprintf("The setup script for spawn host <%s|%s> has %s", hostURL, hostID, result),
		Attachments: []message.SlackAttachment{attachment},
	}
	return payload
}

func (t *spawnHostSetupScriptTriggers) emailPayload(spawnHostScriptPath, result, hostID, url, hostURL string) *message.Email {
	subject := fmt.Sprintf("The setup script for spawn host has %s", result)
	body := fmt.Sprintf(`<html>
	<head>
	</head>
	<body>
	<p>Hi,</p>
	<p>The setup script '%s' for spawn host <a href="%s">%s</a> has %s.</p>
	<p>Manage spawn hosts from <a href="%s">your hosts page</a> or the Evergreen CLI</p>


	</body>
	</html>
	`, spawnHostScriptPath, hostURL, hostID, result, url)

	return &message.Email{
		Subject:           subject,
		Body:              body,
		PlainTextContents: false,
	}
}
