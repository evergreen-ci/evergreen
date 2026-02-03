package route

import (
	"context"
	"net/http"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

///////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/notifications/slack

type slackNotificationPostHandler struct {
	APISlack    *model.APISlack
	composer    message.Composer
	sender      send.Sender
	environment evergreen.Environment
}

func makeSlackNotification(environment evergreen.Environment) gimlet.RouteHandler {
	return &slackNotificationPostHandler{
		environment: environment,
	}
}

func (h *slackNotificationPostHandler) Factory() gimlet.RouteHandler {
	return &slackNotificationPostHandler{
		environment: h.environment,
	}
}

// Parse fetches the JSON payload from the and unmarshals it to an APISlack.
func (h *slackNotificationPostHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	h.APISlack = &model.APISlack{}
	if err := gimlet.GetJSON(body, h.APISlack); err != nil {
		return errors.Wrap(err, "reading Slack payload from JSON request body")
	}

	return nil
}

// Run dispatches the notification.
func (h *slackNotificationPostHandler) Run(ctx context.Context) gimlet.Responder {
	attachments := []message.SlackAttachment{}
	for _, a := range h.APISlack.Attachments {
		attachments = append(attachments, a.ToService())
	}
	target := utility.FromStringPtr(h.APISlack.Target)
	formattedTarget, err := notification.FormatSlackTarget(ctx, target)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "formatting slack target"))
	}
	msg := utility.FromStringPtr(h.APISlack.Msg)

	h.composer = message.NewSlackMessage(level.Notice, formattedTarget, msg, attachments)
	s, err := h.environment.GetSender(evergreen.SenderSlack)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting Slack sender"))
	}
	if s == nil {
		return gimlet.MakeJSONErrorResponder(errors.New("slack sender is not configured"))
	}

	h.sender = s
	h.sender.Send(h.composer)

	return gimlet.NewJSONResponse(struct{}{})
}

///////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/notifications/email

type emailNotificationPostHandler struct {
	APIEmail    *model.APIEmail
	composer    message.Composer
	sender      send.Sender
	environment evergreen.Environment
}

func makeEmailNotification(environment evergreen.Environment) gimlet.RouteHandler {
	return &emailNotificationPostHandler{
		environment: environment,
	}
}

func (h *emailNotificationPostHandler) Factory() gimlet.RouteHandler {
	return &emailNotificationPostHandler{
		environment: h.environment,
	}
}

// Parse fetches the JSON payload from the and unmarshals it to an APIEmail.
func (h *emailNotificationPostHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	h.APIEmail = &model.APIEmail{}
	if err := gimlet.GetJSON(body, h.APIEmail); err != nil {
		return errors.Wrap(err, "reading email payload from JSON request body")
	}

	return nil
}

// Run dispatches the notification.
func (h *emailNotificationPostHandler) Run(ctx context.Context) gimlet.Responder {
	var err error
	email := h.APIEmail.ToService()
	h.composer = message.NewEmailMessage(level.Notice, email)
	h.sender, err = h.environment.GetSender(evergreen.SenderEmail)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "getting email sender"))
	}

	h.sender.Send(h.composer)

	return gimlet.NewJSONResponse(struct{}{})
}
