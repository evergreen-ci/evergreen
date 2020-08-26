package trigger

import (
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeHost, event.EventVolumeExpirationWarningSent, makeVolumeTriggers)
}

const (
	// notification templates
	expiringVolumeEmailSubject         = `Volume termination reminder`
	expiringVolumeEmailBody            = `Your volume with id {{.ID}} will be terminated at {{.ExpirationTime}}. Visit the <a href={{.URL}}>volume page</a> to extend its lifetime.`
	expiringVolumeSlackBody            = `Your volume with id {{.ID}} will be terminated at {{.ExpirationTime}}. Visit the <{{.URL}}|volume page> to extend its lifetime.`
	expiringVolumeSlackAttachmentTitle = "Volume Page"
)

type volumeTriggers struct {
	event        *event.EventLogEntry
	volume       *host.Volume
	templateData hostTemplateData
	uiConfig     evergreen.UIConfig

	base
}

func (t *volumeTriggers) Fetch(e *event.EventLogEntry) error {
	var err error
	t.volume, err = host.FindVolumeByID(e.ResourceId)
	if err != nil {
		return errors.Wrap(err, "failed to fetch volume")
	}
	if t.volume == nil {
		return errors.Errorf("volume '%s' doesn't exist", e.ResourceId)
	}

	if err = t.uiConfig.Get(evergreen.GetEnvironment()); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	t.templateData = hostTemplateData{
		ID:             t.volume.ID,
		ExpirationTime: t.volume.Expiration.Format(time.RFC1123),
		URL:            fmt.Sprintf("%s/spawn#?resourcetype=volumes&id=%s", t.uiConfig.Url, t.volume.ID),
	}

	t.event = e
	return nil
}

func (t *volumeTriggers) Selectors() []event.Selector {
	return []event.Selector{
		{
			Type: event.SelectorID,
			Data: t.volume.ID,
		},
		{
			Type: event.SelectorObject,
			Data: event.ObjectHost,
		},
		{
			Type: event.SelectorOwner,
			Data: t.volume.CreatedBy,
		},
	}
}

type volumeTemplateData struct {
	ID             string
	ExpirationTime time.Time
}

func makeVolumeTriggers() eventHandler {
	t := &volumeTriggers{}
	t.base.triggers = map[string]trigger{
		event.TriggerExpiration: t.volumeExpiration,
	}

	return t
}

func (t *volumeTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	var payload interface{}
	var err error
	switch sub.Subscriber.Type {
	case event.EmailSubscriberType:
		payload, err = hostExpirationEmailPayload(t.templateData, expiringVolumeEmailSubject, expiringVolumeEmailBody, sub.Selectors)
	case event.SlackSubscriberType:
		payload, err = hostExpirationSlackPayload(t.templateData, expiringVolumeSlackBody, expiringVolumeSlackAttachmentTitle, sub.Selectors)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse templates")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *volumeTriggers) volumeExpiration(sub *event.Subscription) (*notification.Notification, error) {
	return t.generate(sub)
}
