package trigger

import (
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
		ID:  t.volume.ID,
		URL: fmt.Sprintf("%s/spawn#?resourcetype=volumes&id=%s", t.uiConfig.Url, t.volume.ID),
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
		payload, err = t.templateData.hostExpirationEmailPayload(expiringVolumeEmailSubject, expiringVolumeEmailBody, t.Attributes())
	case event.SlackSubscriberType:
		payload, err = t.templateData.hostExpirationSlackPayload(expiringVolumeSlackBody, expiringVolumeSlackAttachmentTitle)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse templates")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *volumeTriggers) volumeExpiration(sub *event.Subscription) (*notification.Notification, error) {
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
		return nil, errors.Wrapf(err, "can't get user '%s' for subscription owner", userID)
	}
	if user == nil {
		return nil, errors.Errorf("no user '%s' found", userID)
	}

	userTimeZone, err := time.LoadLocation(user.Settings.Timezone)
	if err != nil {
		return nil, errors.Wrapf(err, "can't parse timezone '%s'", user.Settings.Timezone)
	}

	return userTimeZone, nil
}
