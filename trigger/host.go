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
	expiringHostSlackAttachmentTitle = "Spawnhost Page"
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
		return errors.Wrap(err, "failed to fetch host")
	}
	if t.host == nil {
		return errors.New("couldn't find host")
	}

	if err = t.uiConfig.Get(evergreen.GetEnvironment()); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	t.event = e
	return nil
}

func (t *hostBase) Selectors() []event.Selector {
	return []event.Selector{
		{
			Type: event.SelectorID,
			Data: t.host.Id,
		},
		{
			Type: event.SelectorObject,
			Data: event.ObjectHost,
		},
		{
			Type: event.SelectorOwner,
			Data: t.host.StartedBy,
		},
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
		return errors.Wrap(err, "failed to fetch host data")
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
		payload, err = hostExpirationEmailPayload(t.templateData, expiringHostEmailSubject, expiringHostEmailBody, sub.Selectors)
	case event.SlackSubscriberType:
		payload, err = hostExpirationSlackPayload(t.templateData, expiringHostSlackBody, expiringHostSlackAttachmentTitle, sub.Selectors)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, errors.Wrap(err, "unable to parse templates")
	}

	return notification.New(t.event.ID, sub.Trigger, &sub.Subscriber, payload)
}

func hostExpirationEmailPayload(t hostTemplateData, subjectString, bodyString string, selectors []event.Selector) (*message.Email, error) {
	subjectTemplate, err := template.New("subject").Parse(subjectString)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse subject template")
	}
	buf := &bytes.Buffer{}
	err = subjectTemplate.Execute(buf, t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute subject template")
	}
	subject := buf.String()

	bodyTemplate, err := template.New("subject").Parse(bodyString)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse body template")
	}
	buf = &bytes.Buffer{}
	err = bodyTemplate.Execute(buf, t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute body template")
	}
	body := buf.String()

	return &message.Email{
		Subject:           subject,
		Body:              body,
		PlainTextContents: false,
		Headers:           makeHeaders(selectors),
	}, nil
}

func hostExpirationSlackPayload(t hostTemplateData, messageString string, linkTitle string, selectors []event.Selector) (*notification.SlackPayload, error) {
	messageTemplate, err := template.New("subject").Parse(messageString)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse slack template")
	}
	buf := &bytes.Buffer{}
	err = messageTemplate.Execute(buf, t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute slack template")
	}
	msg := buf.String()

	return &notification.SlackPayload{
		Body: msg,
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
