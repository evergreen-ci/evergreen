package model

import (
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

type APIJiraComment struct {
	IssueID APIString `json:"issue_id"`
	Body    APIString `json:"body"`
}

// BuildFromService converts from service level message.JIRAComment to APIJiraComment.
func (c *APIJiraComment) BuildFromService(h interface{}) error {
	var comment message.JIRAComment
	switch v := h.(type) {
	case *message.JIRAComment:
		comment = *v
	default:
		return errors.Errorf("%T is not a supported type", h)
	}

	c.IssueID = ToAPIString(comment.IssueID)
	c.Body = ToAPIString(comment.Body)

	return nil
}

// ToService returns a service layer message.JIRAComment using the data from APIJiraComment.
func (c *APIJiraComment) ToService() (interface{}, error) {
	comment := message.JIRAComment{}
	comment.IssueID = FromAPIString(c.IssueID)
	comment.Body = FromAPIString(c.Body)

	return &comment, nil
}

///////////////////////////////////////////////////////////////////////

type APIJiraIssue struct {
	IssueKey    APIString              `json:"issue_key"`
	Project     APIString              `json:"project"`
	Summary     APIString              `json:"summary"`
	Description APIString              `json:"description"`
	Reporter    APIString              `json:"reporter"`
	Assignee    APIString              `json:"assignee"`
	Type        APIString              `json:"type"`
	Components  []string               `json:"components"`
	Labels      []string               `json:"labels"`
	Fields      map[string]interface{} `json:"fields"`
}

// BuildFromService converts from service level message.JiraIssue to APIJiraIssue.
func (i *APIJiraIssue) BuildFromService(h interface{}) error {
	var issue message.JiraIssue
	switch v := h.(type) {
	case *message.JiraIssue:
		issue = *v
	default:
		return errors.Errorf("%T is not a supported type", h)
	}

	i.IssueKey = ToAPIString(issue.IssueKey)
	i.Project = ToAPIString(issue.Project)
	i.Summary = ToAPIString(issue.Summary)
	i.Description = ToAPIString(issue.Description)
	i.Reporter = ToAPIString(issue.Reporter)
	i.Assignee = ToAPIString(issue.Assignee)
	i.Type = ToAPIString(issue.Type)
	if issue.Components != nil {
		i.Components = issue.Components
	}
	if issue.Labels != nil {
		i.Labels = issue.Labels
	}
	i.Fields = issue.Fields

	return nil
}

// ToService returns a service layer message.JiraIssue using the data from APIJiraIssue.
func (i *APIJiraIssue) ToService() (interface{}, error) {
	issue := message.JiraIssue{}
	issue.IssueKey = FromAPIString(i.IssueKey)
	issue.Project = FromAPIString(i.Project)
	issue.Summary = FromAPIString(i.Summary)
	issue.Description = FromAPIString(i.Description)
	issue.Reporter = FromAPIString(i.Reporter)
	issue.Assignee = FromAPIString(i.Assignee)
	issue.Type = FromAPIString(i.Type)
	issue.Components = i.Components
	issue.Labels = i.Labels
	issue.Fields = i.Fields

	return &issue, nil
}

///////////////////////////////////////////////////////////////////////

type APISlack struct {
	Target      APIString            `json:"target"`
	Msg         APIString            `json:"msg"`
	Attachments []APISlackAttachment `json:"attachments"`
}

// BuildFromService converts from service level message.Slack to APISlack.
func (n *APISlack) BuildFromService(h interface{}) error {
	var slack message.Slack
	switch v := h.(type) {
	case *message.Slack:
		slack = *v
	default:
		return errors.Errorf("%T is not a supported type", h)
	}

	n.Target = ToAPIString(slack.Target)
	n.Msg = ToAPIString(slack.Msg)
	if slack.Attachments != nil {
		n.Attachments = []APISlackAttachment{}
		for _, a := range slack.Attachments {
			attachment := &APISlackAttachment{}
			if err := attachment.BuildFromService(a); err != nil {
				return errors.Wrap(err, "Error converting from message.Slack to model.APISlack")
			}
			n.Attachments = append(n.Attachments, *attachment)
		}
	}

	return nil
}

// ToService is not implemented
func (n *APISlack) ToService() (interface{}, error) {
	return nil, errors.New("ToService() is not implemented for model.APISlack")
}

///////////////////////////////////////////////////////////////////////

type APISlackAttachment struct {
	Color      APIString                 `json:"color"`
	Fallback   APIString                 `json:"fallback"`
	AuthorName APIString                 `json:"author_name"`
	AuthorIcon APIString                 `json:"author_icon"`
	Title      APIString                 `json:"title"`
	TitleLink  APIString                 `json:"title_link"`
	Text       APIString                 `json:"text"`
	Fields     []APISlackAttachmentField `json:"fields"`
	MarkdownIn []string                  `json:"mrkdwn_in"`
	Footer     APIString                 `json:"footer"`
}

// BuildFromService converts from service level message.SlackAttachment to APISlackAttachment.
func (a *APISlackAttachment) BuildFromService(h interface{}) error {
	var attachment message.SlackAttachment
	switch v := h.(type) {
	case *message.SlackAttachment:
		attachment = *v
	default:
		return errors.Errorf("%T is not a supported type", h)
	}

	a.Color = ToAPIString(attachment.Color)
	a.Fallback = ToAPIString(attachment.Fallback)
	a.AuthorName = ToAPIString(attachment.AuthorName)
	a.AuthorIcon = ToAPIString(attachment.AuthorIcon)
	a.Title = ToAPIString(attachment.Title)
	a.TitleLink = ToAPIString(attachment.TitleLink)
	a.Text = ToAPIString(attachment.Text)
	a.Footer = ToAPIString(attachment.Footer)
	if attachment.Fields != nil {
		a.Fields = []APISlackAttachmentField{}
		for _, f := range attachment.Fields {
			field := &APISlackAttachmentField{}
			if err := field.BuildFromService(f); err != nil {
				return errors.Wrap(err, "Error converting from slack.Attachment to model.APISlackAttachment")
			}
			a.Fields = append(a.Fields, *field)
		}
	}
	if attachment.MarkdownIn != nil {
		a.MarkdownIn = attachment.MarkdownIn
	}

	return nil
}

// ToService returns a service layer message.SlackAttachment using the data from APISlackAttachment.
func (a *APISlackAttachment) ToService() (interface{}, error) {
	attachment := message.SlackAttachment{}
	attachment.Color = FromAPIString(a.Color)
	attachment.Fallback = FromAPIString(a.Fallback)
	attachment.AuthorName = FromAPIString(a.AuthorName)
	attachment.AuthorIcon = FromAPIString(a.AuthorIcon)
	attachment.Title = FromAPIString(a.Title)
	attachment.TitleLink = FromAPIString(a.TitleLink)
	attachment.Text = FromAPIString(a.Text)
	attachment.Footer = FromAPIString(a.Footer)
	for _, f := range a.Fields {
		i, err := f.ToService()
		if err != nil {
			return nil, errors.Wrap(err, "Error converting from model.APISlackAttachment to message.SlackAttachment")
		}
		attachment.Fields = append(attachment.Fields, i.(*message.SlackAttachmentField))
	}
	attachment.MarkdownIn = a.MarkdownIn

	return &attachment, nil
}

///////////////////////////////////////////////////////////////////////

type APISlackAttachmentField struct {
	Title APIString `json:"title"`
	Value APIString `json:"value"`
	Short bool      `json:"short"`
}

// BuildFromService converts from service level message.SlackAttachmentField to an APISlackAttachmentField.
func (f *APISlackAttachmentField) BuildFromService(h interface{}) error {
	var field message.SlackAttachmentField
	switch v := h.(type) {
	case *message.SlackAttachmentField:
		field = *v
	default:
		return errors.Errorf("%T is not a supported type", h)
	}

	f.Title = ToAPIString(field.Title)
	f.Value = ToAPIString(field.Value)
	f.Short = field.Short

	return nil
}

// ToService returns a service layer message.SlackAttachmentField using the data from APISlackAttachmentField.
func (f *APISlackAttachmentField) ToService() (interface{}, error) {
	field := message.SlackAttachmentField{}
	field.Title = FromAPIString(f.Title)
	field.Value = FromAPIString(f.Value)
	field.Short = f.Short

	return &field, nil
}

///////////////////////////////////////////////////////////////////////

type APIEmail struct {
	From              APIString           `json:"from"`
	Recipients        []string            `json:"recipients"`
	Subject           APIString           `json:"subject"`
	Body              APIString           `json:"body"`
	PlainTextContents bool                `json:"is_plain_text"`
	Headers           map[string][]string `json:"headers"`
}

// BuildFromService converts from service level message.Email to an APIEmail.
func (n *APIEmail) BuildFromService(h interface{}) error {
	var email message.Email
	switch v := h.(type) {
	case *message.Email:
		email = *v
	default:
		return errors.Errorf("%T is not a supported type", h)
	}

	n.From = ToAPIString(email.From)
	if email.Recipients != nil {
		n.Recipients = email.Recipients
	}
	n.Subject = ToAPIString(email.Subject)
	n.Body = ToAPIString(email.Body)
	n.PlainTextContents = email.PlainTextContents
	n.Headers = email.Headers

	return nil
}

// ToService returns a service layer message.Email using the data from APIEmail.
func (n *APIEmail) ToService() (interface{}, error) {
	email := message.Email{}
	email.From = FromAPIString(n.From)
	email.Recipients = n.Recipients
	email.Subject = FromAPIString(n.Subject)
	email.Body = FromAPIString(n.Body)
	email.PlainTextContents = n.PlainTextContents
	email.Headers = n.Headers

	return &email, nil
}
