package trigger

import (
	"bytes"
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
}

const (
	// notification templates
	expiringHostEmailSubject         = `{{.Distro}} host termination reminder`
	expiringHostEmailBody            = `Your {{.Distro}} host '{{.Name}}' will be terminated at {{.ExpirationTime}}. Visit the <a href={{.URL}}>spawnhost page</a> to extend its lifetime.`
	expiringHostSlackBody            = `Your {{.Distro}} host '{{.Name}}' will be terminated at {{.ExpirationTime}}. Visit the <{{.URL}}|spawnhost page> to extend its lifetime.`
	expiringHostSlackAttachmentTitle = "Spawn Host Page"
)

type hostBase struct {
	event    *event.EventLogEntry
	data     *event.HostEventData
	host     *host.Host
	uiConfig evergreen.UIConfig

	base
}

func (t *hostBase) Fetch(e *event.EventLogEntry) error {
	var ok bool
	var err error
	t.data, ok = e.Data.(*event.HostEventData)
	if !ok {
		return errors.Errorf("expected host event data, got %T", e.Data)
	}

	t.host, err = host.FindOneId(e.ResourceId)
	if err != nil {
		return errors.Wrapf(err, "finding host '%s'", e.ResourceId)
	}
	if t.host == nil {
		return errors.Errorf("host '%s' not found", e.ResourceId)
	}

	if err = t.uiConfig.Get(evergreen.GetEnvironment()); err != nil {
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
}

func makeHostTriggers() eventHandler {
	t := &hostTriggers{}
	t.hostBase.base.triggers = map[string]trigger{
		event.TriggerExpiration: t.hostExpiration,
	}

	return t
}

type hostTriggers struct {
	templateData hostTemplateData

	hostBase
}

func (t *hostTriggers) Fetch(e *event.EventLogEntry) error {
	err := t.hostBase.Fetch(e)
	if err != nil {
		return errors.Wrap(err, "fetching host data")
	}

	t.templateData = hostTemplateData{
		ID:     t.host.Id,
		Name:   t.host.DisplayName,
		Distro: t.host.Distro.Id,
		URL:    fmt.Sprintf("%s/spawn#?resourcetype=hosts&id=%s", t.uiConfig.Url, t.host.Id),
	}
	if t.host.DisplayName == "" {
		t.templateData.Name = t.host.Id
	}
	t.event = e
	return nil
}

func (t *hostTriggers) generate(sub *event.Subscription) (*notification.Notification, error) {
	var payload interface{}
	var err error
	switch sub.Subscriber.Type {
	case event.EmailSubscriberType:
		payload, err = t.templateData.hostExpirationEmailPayload(expiringHostEmailSubject, expiringHostEmailBody, t.Attributes())
	case event.SlackSubscriberType:
		payload, err = t.templateData.hostExpirationSlackPayload(expiringHostSlackBody, expiringHostSlackAttachmentTitle)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrapf(err, "creating template for event type '%s'", sub.Subscriber.Type)
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func (t *hostTemplateData) hostExpirationEmailPayload(subjectString, bodyString string, attributes event.Attributes) (*message.Email, error) {
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

func (t *hostTemplateData) hostExpirationSlackPayload(messageString string, linkTitle string) (*notification.SlackPayload, error) {
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

func (t *hostTriggers) hostExpiration(sub *event.Subscription) (*notification.Notification, error) {
	timeZone := time.Local
	if sub.OwnerType == event.OwnerTypePerson {
		userTimeZone, err := getUserTimeZone(sub.Owner)
		if err != nil {
			grip.Error(message.WrapError(err, message.Fields{
				"message": "problem getting time zone",
				"user":    sub.Owner,
				"trigger": "hostExpiration",
			}))
		} else {
			timeZone = userTimeZone
		}
	}
	t.templateData.ExpirationTime = t.host.ExpirationTime.In(timeZone).Format(time.RFC1123)
	return t.generate(sub)
}
