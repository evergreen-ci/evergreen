package trigger

import (
	"context"
	"fmt"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/user"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeHost, event.EventVolumeExpirationWarningSent, makeVolumeTriggers)
}

const (
	// notification templates
	expiringVolumeEmailSubject         = `Volume termination reminder`
	expiringVolumeEmailBody            = `Your volume with id {{.ID}} is unattached and will be terminated at {{.ExpirationTime}}. Visit the <a href={{.URL}}>volume page</a> to extend its lifetime.`
	expiringVolumeSlackBody            = `Your volume with id {{.ID}} is unattached and will be terminated at {{.ExpirationTime}}. Visit the <{{.URL}}|volume page> to extend its lifetime.`
	expiringVolumeSlackAttachmentTitle = "Volume Page"
)

type volumeTriggers struct {
	event        *event.EventLogEntry
	volume       *host.Volume
	templateData hostTemplateData
	uiConfig     evergreen.UIConfig

	base
}

func (t *volumeTriggers) Fetch(ctx context.Context, e *event.EventLogEntry) error {
	var err error
	t.volume, err = host.FindVolumeByID(e.ResourceId)
	if err != nil {
		return errors.Wrapf(err, "finding volume '%s'", e.ResourceId)
	}
	if t.volume == nil {
		return errors.Errorf("volume '%s' not found", e.ResourceId)
	}

	if err = t.uiConfig.Get(ctx); err != nil {
		return errors.Wrap(err, "fetching UI config")
	}

	t.templateData = hostTemplateData{
		ID:  t.volume.ID,
		URL: fmt.Sprintf("%s/spawn/volume", t.uiConfig.UIv2Url),
	}

	t.event = e
	return nil
}

func (t *volumeTriggers) Attributes() event.Attributes {
	return event.Attributes{
		ID:     []string{t.volume.ID},
		Object: []string{event.ObjectHost},
		Owner:  []string{t.volume.CreatedBy},
	}
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
		payload, err = t.templateData.hostEmailPayload(expiringVolumeEmailSubject, expiringVolumeEmailBody, t.Attributes())
	case event.SlackSubscriberType:
		payload, err = t.templateData.hostSlackPayload(expiringVolumeSlackBody, expiringVolumeSlackAttachmentTitle)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "creating template for event type '%s'", sub.Subscriber.Type)
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *volumeTriggers) volumeExpiration(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	timeZone := time.Local
	if sub.OwnerType == event.OwnerTypePerson {
		userTimeZone, err := getUserTimeZone(sub.Owner)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem getting time zone",
				"user":    sub.Owner,
				"trigger": "volumeExpiration",
			}))
		} else {
			timeZone = userTimeZone
		}
	}
	t.templateData.ExpirationTime = t.volume.Expiration.In(timeZone).Format(time.RFC1123)
	return t.generate(sub)
}

func getUserTimeZone(userID string) (*time.Location, error) {
	user, err := user.FindOneById(userID)
	if err != nil {
		return nil, errors.Wrapf(err, "finding user '%s' for subscription owner", userID)
	}
	if user == nil {
		return nil, errors.Errorf("user '%s' not found", userID)
	}

	userTimeZone, err := time.LoadLocation(user.Settings.Timezone)
	if err != nil {
		return nil, errors.Wrapf(err, "parsing user timezone '%s'", user.Settings.Timezone)
	}

	return userTimeZone, nil
}
