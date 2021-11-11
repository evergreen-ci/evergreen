package route

import (
	"context"
	"fmt"
	"net/http"

	"github.com/evergreen-ci/evergreen"
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
// POST /rest/v2/notifications/{type}

type notificationPostHandler struct {
	handler     gimlet.RouteHandler
	environment evergreen.Environment
}

func makeNotification(environment evergreen.Environment) gimlet.RouteHandler {
	return &notificationPostHandler{
		environment: environment,
	}
}

func (h *notificationPostHandler) Factory() gimlet.RouteHandler {
	return &notificationPostHandler{
		environment: h.environment,
	}
}

// Parse fetches the notification type from the http request.
func (h *notificationPostHandler) Parse(ctx context.Context, r *http.Request) error {
	t := gimlet.GetVars(r)["type"]
	switch t {
	case "jira_comment":
		h.handler = makeJiraCommentNotification(h.environment)
	case "jira_issue":
		h.handler = makeJiraIssueNotification(h.environment)
	case "slack":
		h.handler = makeSlackNotification(h.environment)
	case "email":
		h.handler = makeEmailNotification(h.environment)
	default:
		return errors.Errorf("'%s' is not a supported {type}", t)
	}

	return h.handler.Parse(ctx, r)
}

// Run dispatches the notification.
func (h *notificationPostHandler) Run(ctx context.Context) gimlet.Responder {
	return h.handler.Run(ctx)
}

///////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/notifications/jira_comment

type jiraCommentNotificationPostHandler struct {
	APIJiraComment *model.APIJiraComment
	composer       message.Composer
	sender         send.Sender
	environment    evergreen.Environment
}

func makeJiraCommentNotification(environment evergreen.Environment) gimlet.RouteHandler {
	return &jiraCommentNotificationPostHandler{
		environment: environment,
	}
}

func (h *jiraCommentNotificationPostHandler) Factory() gimlet.RouteHandler {
	return &jiraCommentNotificationPostHandler{
		environment: h.environment,
	}
}

// Parse fetches the JSON payload from the and unmarshals it to an APIJiraComment.
func (h *jiraCommentNotificationPostHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	h.APIJiraComment = &model.APIJiraComment{}
	if err := gimlet.GetJSON(body, h.APIJiraComment); err != nil {
		return errors.Wrap(err, "API error while unmarshalling JSON to model.APIJiraComment")
	}

	return nil
}

// Run dispatches the notification.
func (h *jiraCommentNotificationPostHandler) Run(ctx context.Context) gimlet.Responder {
	i, err := h.APIJiraComment.ToService()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.APIJiraComment to message.JIRAComment"))
	}
	comment, ok := i.(*message.JIRAComment)
	if !ok {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("Unexpected type %T for message.JIRAComment", i),
		})
	}

	h.composer = message.NewJIRACommentMessage(level.Notice, comment.IssueID, comment.Body)
	h.sender, err = h.environment.GetSender(evergreen.SenderJIRAComment)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Error fetching sender key for evergreen.SenderJIRAComment"))
	}

	h.sender.Send(h.composer)

	return gimlet.NewJSONResponse(struct{}{})
}

///////////////////////////////////////////////////////////////////////
//
// POST /rest/v2/notifications/jira_issue

type jiraIssueNotificationPostHandler struct {
	APIJiraIssue *model.APIJiraIssue
	composer     message.Composer
	sender       send.Sender
	environment  evergreen.Environment
}

func makeJiraIssueNotification(environment evergreen.Environment) gimlet.RouteHandler {
	return &jiraIssueNotificationPostHandler{
		environment: environment,
	}
}

func (h *jiraIssueNotificationPostHandler) Factory() gimlet.RouteHandler {
	return &jiraIssueNotificationPostHandler{
		environment: h.environment,
	}
}

// Parse fetches the JSON payload from the and unmarshals it to an APIJiraIssue.
func (h *jiraIssueNotificationPostHandler) Parse(ctx context.Context, r *http.Request) error {
	body := utility.NewRequestReader(r)
	h.APIJiraIssue = &model.APIJiraIssue{}
	if err := gimlet.GetJSON(body, h.APIJiraIssue); err != nil {
		return errors.Wrap(err, "API error while unmarshalling JSON to model.APIJiraIssue")
	}

	return nil
}

// Run dispatches the notification.
func (h *jiraIssueNotificationPostHandler) Run(ctx context.Context) gimlet.Responder {
	i, err := h.APIJiraIssue.ToService()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.APIJiraIssue to message.JiraIssue"))
	}
	issue, ok := i.(*message.JiraIssue)
	if !ok {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("Unexpected type %T for message.JiraIssue", i),
		})
	}

	h.composer = message.MakeJiraMessage(issue)
	if err = h.composer.SetPriority(level.Notice); err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Error setting priority level on jira message"))
	}
	h.sender, err = h.environment.GetSender(evergreen.SenderJIRAIssue)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Error fetching sender key for evergreen.SenderJIRAIssue"))
	}

	h.sender.Send(h.composer)

	return gimlet.NewJSONResponse(struct{}{})
}

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
		return errors.Wrap(err, "API error while unmarshalling JSON to model.APISlack")
	}

	return nil
}

// Run dispatches the notification.
func (h *slackNotificationPostHandler) Run(ctx context.Context) gimlet.Responder {
	attachments := []message.SlackAttachment{}
	for _, a := range h.APISlack.Attachments {
		i, err := a.ToService()
		if err != nil {
			return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.APISlackAttachment to message.SlackAttachment"))
		}
		attachment, ok := i.(*message.SlackAttachment)
		if !ok {
			return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
				StatusCode: http.StatusInternalServerError,
				Message:    fmt.Sprintf("Unexpected type %T for message.SlackAttachment", i),
			})
		}
		attachments = append(attachments, *attachment)
	}
	target := utility.FromStringPtr(h.APISlack.Target)
	msg := utility.FromStringPtr(h.APISlack.Msg)

	h.composer = message.NewSlackMessage(level.Notice, target, msg, attachments)
	s, err := h.environment.GetSender(evergreen.SenderSlack)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Error fetching sender key for evergreen.SenderSlack"))
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
		return errors.Wrap(err, "API error while unmarshalling JSON to model.APIEmail")
	}

	return nil
}

// Run dispatches the notification.
func (h *emailNotificationPostHandler) Run(ctx context.Context) gimlet.Responder {
	i, err := h.APIEmail.ToService()
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "API error converting from model.APIEmail to message.Email"))
	}
	email, ok := i.(*message.Email)
	if !ok {
		return gimlet.MakeJSONErrorResponder(gimlet.ErrorResponse{
			StatusCode: http.StatusInternalServerError,
			Message:    fmt.Sprintf("Unexpected type %T for message.Email", i),
		})
	}

	h.composer = message.NewEmailMessage(level.Notice, *email)
	h.sender, err = h.environment.GetSender(evergreen.SenderEmail)
	if err != nil {
		return gimlet.MakeJSONInternalErrorResponder(errors.Wrap(err, "Error fetching sender key for evergreen.SenderEmail"))
	}

	h.sender.Send(h.composer)

	return gimlet.NewJSONResponse(struct{}{})
}
