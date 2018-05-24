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
	registry.AddTrigger(event.ResourceTypeHost,
		hostValidator(hostSpawnOutcome),
	)
	registry.AddPrefetch(event.ResourceTypeHost, hostFetch)
}

func hostFetch(e *event.EventLogEntry) (interface{}, error) {
	p, err := host.FindOneId(e.ResourceId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch host")
	}
	if p == nil {
		return nil, errors.New("couldn't find host")
	}

	return p, nil
}

func hostValidator(t func(e *event.HostEventData, h *host.Host) (*notificationGenerator, error)) trigger {
	return func(e *event.EventLogEntry, object interface{}) (*notificationGenerator, error) {
		h, ok := object.(*host.Host)
		if !ok {
			return nil, errors.New("expected a host, received unknown type")
		}
		if h == nil {
			return nil, errors.New("expected a host, received nil data")
		}

		return t(e, h)
	}
}

func hostSelectors(h *host.Host) []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: h.Id,
		},
		{
			Type: selectorObject,
			Data: "host",
		},
	}
}

func hostSpawnOutcome(e *event.EventLogEntry, h *host.Host) (*notificationGenerator, error) {
	const name = "spawn-outcome"

	if !h.UserHost || e.EventType == event.EventHostProvisioned || e.EventType == event.EventHostProvisionError {
		return nil, nil
	}

	uiConfig := evergreen.UIConfig{}
	if err := uiConfig.Get(); err != nil {
		return nil, errors.Wrap(err, "failed to process spawn-outcome trigger")
	}

	text := fmt.Sprintf("Host with distro '%s' has spawned", h.Distro)

	var attachment message.SlackAttachment
	if e.EventType == event.EventHostProvisioned {
		attachment = message.SlackAttachment{
			Title:     "Requested host has succesfully spawned",
			TitleLink: fmt.Sprintf("%s/host/%s", uiConfig.Url, h.Id),
			Color:     "good",
			Fields: []*message.SlackAttachmentField{
				&message.SlackAttachmentField{
					Title: "distro",
					Value: h.Distro,
					Short: true,
				},
				&message.SlackAttachmentField{
					Title: "ssh command",
					Value: fmt.Sprintf("ssh %s@%s", h.User, h.Host),
				},
			},
		}

	} else if e.EventType == event.EventHostProvisionError {
		attachment = message.SlackAttachment{
			Title:     "Requested host has failed to provision",
			TitleLink: fmt.Sprintf("%s/spawn", uiConfig.Url),
			Color:     "danger",
			Fields: []*message.SlackAttachmentField{
				&message.SlackAttachmentField{
					Title: "distro",
					Value: h.Distro,
					Short: true,
				},
			},
		}
	}

	gen := &notificationGenerator{
		triggerName: name,
		selectors:   hostSelectors(h),
		slack: &notification.SlackPayload{
			Attachments: []message.SlackAttachment{
				attachment,
			},
		},
	}

	return gen, nil
}
