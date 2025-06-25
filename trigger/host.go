package trigger

import (
	"bytes"
	"context"
	"fmt"
	"text/template"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostExpirationWarningSent, makeHostTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostTemporaryExemptionExpirationWarningSent, makeHostTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventSpawnHostIdleNotification, makeHostTriggers)
	registry.registerEventHandler(event.ResourceTypeHost, event.EventAlertableInstanceTypeWarningSent, makeHostTriggers)

}

const (
	// notification templates
	expiringHostEmailSubject         = `{{.Distro}} host termination reminder`
	expiringHostEmailBody            = `Your {{.Distro}} host '{{.Name}}' will be terminated at {{.ExpirationTime}}. Visit the <a href={{.URL}}>spawnhost page</a> to extend its lifetime.`
	expiringHostSlackBody            = `Your {{.Distro}} host '{{.Name}}' will be terminated at {{.ExpirationTime}}. Visit the <{{.URL}}|spawnhost page> to extend its lifetime.`
	expiringHostSlackAttachmentTitle = "Spawn Host Page"

	expiringHostTemporaryExemptionEmailSubject         = `{{.Distro}} host temporary exemption reminder`
	expiringHostTemporaryExemptionEmailBody            = `Your {{.Distro}} host '{{.Name}}' has a temporary exemption that will end at {{.ExpirationTime}}. Visit the <a href={{.URL}}>spawnhost page</a> to extend its temporary exemption if needed.`
	expiringHostTemporaryExemptionSlackAttachmentTitle = "Spawn Host Page"
	expiringHostTemporaryExemptionSlackBody            = `Your {{.Distro}} host '{{.Name}}' has a temporary exemption that will end at {{.ExpirationTime}}. Visit the <{{.URL}}|spawnhost page> to extend its temporary exemption if needed.`

	idleHostEmailSubject     = `{{.Distro}} idle stopped host notice`
	idleStoppedHostEmailBody = `Your stopped {{.Distro}} host '{{.Name}}' has been idle for at least three months.
In order to be responsible about resource consumption (as stopped instances still have EBS volumes attached and thus still incur costs),
please consider terminating from the <a href={{.URL}}>spawnhost page</a> if the host is no longer in use.`

	alertableInstanceTypeEmailSubject         = `Large instance type reminder`
	alertableInstanceTypeEmailBody            = `Your host '{{.Name}}' is using a large instance type ({{.InstanceType}}). Please remember to switch to smaller instance types when you're finished with development to reduce costs. Visit the <a href={{.URL}}>spawnhost page</a> to modify your host.`
	alertableInstanceTypeSlackBody            = `Your host '{{.Name}}' is using a large instance type ({{.InstanceType}}). Please remember to switch to smaller instance types when you're finished with development to reduce costs. Visit the <{{.URL}}|spawnhost page> to modify your host.`
	alertableInstanceTypeSlackAttachmentTitle = "Spawn Host Page"
)

type hostBase struct {
	event    *event.EventLogEntry
	data     *event.HostEventData
	host     *host.Host
	uiConfig evergreen.UIConfig

	base
}

func (t *hostBase) Fetch(ctx context.Context, e *event.EventLogEntry) error {
	var ok bool
	var err error
	t.data, ok = e.Data.(*event.HostEventData)
	if !ok {
		return errors.Errorf("expected host event data, got %T", e.Data)
	}

	t.host, err = host.FindOneByIdOrTag(ctx, e.ResourceId)
	if err != nil {
		return errors.Wrapf(err, "finding host '%s'", e.ResourceId)
	}
	if t.host == nil {
		return errors.Errorf("host '%s' not found", e.ResourceId)
	}

	if err = t.uiConfig.Get(ctx); err != nil {
		return errors.Wrap(err, "fetching UI config")
	}

	t.event = e
	return nil
}

func (t *hostBase) Attributes() event.Attributes {
	return event.Attributes{
		ID:     []string{t.host.Id},
		Object: []string{event.ObjectHost},
		Owner:  []string{t.host.StartedBy},
	}
}

type hostTemplateData struct {
	ID             string
	Name           string
	Distro         string
	ExpirationTime string
	URL            string
	InstanceType   string
}

func makeHostTriggers() eventHandler {
	t := &hostTriggers{}
	t.hostBase.base.triggers = map[string]trigger{
		event.TriggerExpiration:            t.hostExpiration,
		event.TriggerSpawnHostIdle:         t.spawnHostIdle,
		event.TriggerAlertableInstanceType: t.alertableInstanceType,
	}

	return t
}

type hostTriggers struct {
	templateData hostTemplateData

	hostBase
}

func (t *hostTriggers) Fetch(ctx context.Context, e *event.EventLogEntry) error {
	err := t.hostBase.Fetch(ctx, e)
	if err != nil {
		return errors.Wrap(err, "fetching host data")
	}

	t.templateData = hostTemplateData{
		ID:           t.host.Id,
		Name:         t.host.DisplayName,
		Distro:       t.host.Distro.Id,
		URL:          fmt.Sprintf("%s/spawn/host", t.uiConfig.UIv2Url),
		InstanceType: t.host.InstanceType,
	}
	if t.host.DisplayName == "" {
		t.templateData.Name = t.host.Id
	}
	t.event = e
	return nil
}

func (t *hostTriggers) generateExpiration(sub *event.Subscription) (*notification.Notification, error) {
	var payload any
	var err error
	switch sub.Subscriber.Type {
	case event.EmailSubscriberType:
		payload, err = t.templateData.hostEmailPayload(expiringHostEmailSubject, expiringHostEmailBody, t.Attributes())
	case event.SlackSubscriberType:
		payload, err = t.templateData.hostSlackPayload(expiringHostSlackBody, expiringHostSlackAttachmentTitle)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "creating template for event type '%s'", sub.Subscriber.Type)
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *hostTriggers) generateTemporaryExemptionExpiration(sub *event.Subscription) (*notification.Notification, error) {
	var payload any
	var err error
	switch sub.Subscriber.Type {
	case event.EmailSubscriberType:
		payload, err = t.templateData.hostEmailPayload(expiringHostTemporaryExemptionEmailSubject, expiringHostTemporaryExemptionEmailBody, t.Attributes())
	case event.SlackSubscriberType:
		payload, err = t.templateData.hostSlackPayload(expiringHostTemporaryExemptionSlackBody, expiringHostTemporaryExemptionSlackAttachmentTitle)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "creating template for event type '%s'", sub.Subscriber.Type)
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *hostTriggers) generateIdleSpawnHost(sub *event.Subscription, body string) (*notification.Notification, error) {
	payload, err := t.templateData.hostEmailPayload(idleHostEmailSubject, body, t.Attributes())
	if err != nil {
		return nil, errors.Wrapf(err, "creating idle spawn host template for host '%s'", sub.ID)
	}
	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *hostTriggers) generateAlertableInstanceType(sub *event.Subscription) (*notification.Notification, error) {
	var payload any
	var err error
	switch sub.Subscriber.Type {
	case event.EmailSubscriberType:
		payload, err = t.templateData.hostEmailPayload(alertableInstanceTypeEmailSubject, alertableInstanceTypeEmailBody, t.Attributes())
	case event.SlackSubscriberType:
		payload, err = t.templateData.hostSlackPayload(alertableInstanceTypeSlackBody, alertableInstanceTypeSlackAttachmentTitle)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "creating template for event type '%s'", sub.Subscriber.Type)
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *hostTemplateData) hostEmailPayload(subjectString, bodyString string, attributes event.Attributes) (*message.Email, error) {
	subjectBuf := &bytes.Buffer{}
	subjectTemplate, err := template.New("subject").Parse(subjectString)
	if err != nil {
		return nil, errors.Wrap(err, "parsing subject template")
	}
	if err = subjectTemplate.Execute(subjectBuf, t); err != nil {
		return nil, errors.Wrap(err, "executing subject template")
	}

	bodyBuf := &bytes.Buffer{}
	bodyTemplate, err := template.New("body").Parse(bodyString)
	if err != nil {
		return nil, errors.Wrap(err, "parsing body template")
	}
	if err = bodyTemplate.Execute(bodyBuf, t); err != nil {
		return nil, errors.Wrap(err, "executing body template")
	}

	return &message.Email{
		Subject:           subjectBuf.String(),
		Body:              bodyBuf.String(),
		PlainTextContents: false,
		Headers:           makeHeaders(attributes.ToSelectorMap()),
	}, nil
}

func (t *hostTemplateData) hostSlackPayload(messageString string, linkTitle string) (*notification.SlackPayload, error) {
	messageTemplate, err := template.New("subject").Parse(messageString)
	if err != nil {
		return nil, errors.Wrap(err, "parsing Slack template")
	}

	msgBuf := &bytes.Buffer{}
	if err = messageTemplate.Execute(msgBuf, t); err != nil {
		return nil, errors.Wrap(err, "executing Slack template")
	}

	return &notification.SlackPayload{
		Body: msgBuf.String(),
		Attachments: []message.SlackAttachment{{
			Title:     linkTitle,
			TitleLink: t.URL,
			Color:     evergreenSuccessColor,
		}},
	}, nil
}

func (t *hostTriggers) hostExpiration(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	switch t.event.EventType {
	case event.EventHostExpirationWarningSent:
		return t.makeHostExpirationNotification(ctx, sub)
	case event.EventHostTemporaryExemptionExpirationWarningSent:
		return t.makeHostTemporaryExemptionNotification(ctx, sub)
	default:
		return nil, nil
	}
}

func (t *hostTriggers) makeHostExpirationNotification(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	if t.host.NoExpiration {
		return nil, nil
	}

	timeZone := t.getTimeZone(ctx, sub, "host expiration")
	t.templateData.ExpirationTime = t.host.ExpirationTime.In(timeZone).Format(time.RFC1123)

	return t.generateExpiration(sub)
}

func (t *hostTriggers) makeHostTemporaryExemptionNotification(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	timeZone := t.getTimeZone(ctx, sub, "host temporary exemption expiration")
	t.templateData.ExpirationTime = t.host.SleepSchedule.TemporarilyExemptUntil.In(timeZone).Format(time.RFC1123)

	return t.generateTemporaryExemptionExpiration(sub)
}

func (t *hostTriggers) getTimeZone(ctx context.Context, sub *event.Subscription, trigger string) *time.Location {
	if sub.OwnerType == event.OwnerTypePerson {
		userTimeZone, err := getUserTimeZone(ctx, sub.Owner)
		grip.Error(message.WrapError(err, message.Fields{
			"message":    "problem getting user time zone",
			"user":       sub.Owner,
			"event_type": t.event.EventType,
			"trigger":    trigger,
		}))
		if userTimeZone != nil {
			return userTimeZone
		}
	}

	return time.Local
}

func (t *hostTriggers) spawnHostIdle(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	shouldNotify, err := t.host.ShouldNotifyStoppedSpawnHostIdle(ctx)
	if err != nil {
		return nil, err
	}
	if !shouldNotify {
		return nil, nil
	}
	return t.generateIdleSpawnHost(sub, idleStoppedHostEmailBody)
}

func (t *hostTriggers) alertableInstanceType(ctx context.Context, sub *event.Subscription) (*notification.Notification, error) {
	return t.generateAlertableInstanceType(sub)
}
