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
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

func init() {
	registry.registerEventHandler(event.ResourceTypeHost, event.EventHostExpirationWarningSent, makeHostTriggers)
}

const (
	objectHost        = "host"
	triggerExpiration = "expiration"

	// notification templates
	expiringHostTitle = `{{.Distro}} host termination reminder`
	expiringHostBody  = `Your {{.Distro}} host with id {{.ID}} will be terminated at {{.ExpirationTime}}. Visit {{.URL}} to extend its lifetime.`
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

	if err = t.uiConfig.Get(); err != nil {
		return errors.Wrap(err, "Failed to fetch ui config")
	}

	t.event = e
	return nil
}

func (t *hostBase) Selectors() []event.Selector {
	return []event.Selector{
		{
			Type: selectorID,
			Data: t.host.Id,
		},
		{
			Type: selectorObject,
			Data: objectHost,
		},
		{
			Type: selectorOwner,
			Data: t.host.StartedBy,
		},
	}
}

type hostTemplateData struct {
	ID             string
	Distro         string
	ExpirationTime time.Time
	URL            string
}

func makeHostTriggers() eventHandler {
	t := &hostTriggers{}
	t.hostBase.base.triggers = map[string]trigger{
		triggerExpiration: t.hostExpiration,
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
		ID:             t.host.Id,
		Distro:         t.host.Distro.Id,
		ExpirationTime: t.host.ExpirationTime,
		URL:            fmt.Sprintf("%s/spawn", t.hostBase.uiConfig.Url),
	}
	t.event = e
	return nil
}

func (t *hostTriggers) generate(sub *event.Subscription, subjectTempl, bodyTempl string) (*notification.Notification, error) {
	var payload interface{}
	var err error
	switch sub.Subscriber.Type {
	case event.EmailSubscriberType:
		payload, err = hostExpirationEmailPayload(t.templateData, subjectTempl, bodyTempl, sub.Selectors)
	case event.SlackSubscriberType:
		payload, err = hostExpirationSlackPayload(t.templateData, bodyTempl, sub.Selectors)
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

func hostExpirationSlackPayload(t hostTemplateData, messageString string, selectors []event.Selector) (*notification.SlackPayload, error) {
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
			Title:     "Spawnhost Page",
			TitleLink: t.URL,
			Color:     evergreenSuccessColor,
		}},
	}, nil
}

func (t *hostTriggers) hostExpiration(sub *event.Subscription) (*notification.Notification, error) {
	return t.generate(sub, expiringHostTitle, expiringHostBody)
}
