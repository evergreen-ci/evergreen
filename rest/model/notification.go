package model

import (
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/message"
)

type APISlack struct {
	Target      *string              `json:"target"`
	Msg         *string              `json:"msg"`
	Attachments []APISlackAttachment `json:"attachments"`
}

///////////////////////////////////////////////////////////////////////

type APISlackAttachment struct {
	Color      *string                   `json:"color"`
	Fallback   *string                   `json:"fallback"`
	AuthorName *string                   `json:"author_name"`
	AuthorIcon *string                   `json:"author_icon"`
	Title      *string                   `json:"title"`
	TitleLink  *string                   `json:"title_link"`
	Text       *string                   `json:"text"`
	Fields     []APISlackAttachmentField `json:"fields"`
	MarkdownIn []string                  `json:"mrkdwn_in"`
	Footer     *string                   `json:"footer"`
}

// BuildFromService converts from service level message.SlackAttachment to APISlackAttachment.
func (a *APISlackAttachment) BuildFromService(attachment message.SlackAttachment) {
	a.Color = utility.ToStringPtr(attachment.Color)
	a.Fallback = utility.ToStringPtr(attachment.Fallback)
	a.AuthorName = utility.ToStringPtr(attachment.AuthorName)
	a.AuthorIcon = utility.ToStringPtr(attachment.AuthorIcon)
	a.Title = utility.ToStringPtr(attachment.Title)
	a.TitleLink = utility.ToStringPtr(attachment.TitleLink)
	a.Text = utility.ToStringPtr(attachment.Text)
	a.Footer = utility.ToStringPtr(attachment.Footer)
	if attachment.Fields != nil {
		a.Fields = []APISlackAttachmentField{}
		for _, f := range attachment.Fields {
			if f != nil {
				field := APISlackAttachmentField{}
				field.BuildFromService(*f)
				a.Fields = append(a.Fields, field)
			}

		}
	}
	if attachment.MarkdownIn != nil {
		a.MarkdownIn = attachment.MarkdownIn
	}
}

// ToService returns a service layer message.SlackAttachment using the data from APISlackAttachment.
func (a *APISlackAttachment) ToService() message.SlackAttachment {
	attachment := message.SlackAttachment{}
	attachment.Color = utility.FromStringPtr(a.Color)
	attachment.Fallback = utility.FromStringPtr(a.Fallback)
	attachment.AuthorName = utility.FromStringPtr(a.AuthorName)
	attachment.AuthorIcon = utility.FromStringPtr(a.AuthorIcon)
	attachment.Title = utility.FromStringPtr(a.Title)
	attachment.TitleLink = utility.FromStringPtr(a.TitleLink)
	attachment.Text = utility.FromStringPtr(a.Text)
	attachment.Footer = utility.FromStringPtr(a.Footer)
	for _, f := range a.Fields {
		field := f.ToService()
		attachment.Fields = append(attachment.Fields, &field)
	}
	attachment.MarkdownIn = a.MarkdownIn

	return attachment
}

///////////////////////////////////////////////////////////////////////

type APISlackAttachmentField struct {
	Title *string `json:"title"`
	Value *string `json:"value"`
	Short bool    `json:"short"`
}

// BuildFromService converts from service level message.SlackAttachmentField to an APISlackAttachmentField.
func (f *APISlackAttachmentField) BuildFromService(field message.SlackAttachmentField) {
	f.Title = utility.ToStringPtr(field.Title)
	f.Value = utility.ToStringPtr(field.Value)
	f.Short = field.Short
}

// ToService returns a service layer message.SlackAttachmentField using the data from APISlackAttachmentField.
func (f *APISlackAttachmentField) ToService() message.SlackAttachmentField {
	return message.SlackAttachmentField{
		Title: utility.FromStringPtr(f.Title),
		Value: utility.FromStringPtr(f.Value),
		Short: f.Short,
	}
}

///////////////////////////////////////////////////////////////////////

type APIEmail struct {
	From              *string             `json:"from"`
	Recipients        []string            `json:"recipients"`
	Subject           *string             `json:"subject"`
	Body              *string             `json:"body"`
	PlainTextContents bool                `json:"is_plain_text"`
	Headers           map[string][]string `json:"headers"`
}

// BuildFromService converts from service level message.Email to an APIEmail.
func (n *APIEmail) BuildFromService(email message.Email) {
	n.From = utility.ToStringPtr(email.From)
	if email.Recipients != nil {
		n.Recipients = email.Recipients
	}
	n.Subject = utility.ToStringPtr(email.Subject)
	n.Body = utility.ToStringPtr(email.Body)
	n.PlainTextContents = email.PlainTextContents
	n.Headers = email.Headers
}

// ToService returns a service layer message.Email using the data from APIEmail.
func (n *APIEmail) ToService() message.Email {
	email := message.Email{}
	email.From = utility.FromStringPtr(n.From)
	email.Recipients = n.Recipients
	email.Subject = utility.FromStringPtr(n.Subject)
	email.Body = utility.FromStringPtr(n.Body)
	email.PlainTextContents = n.PlainTextContents
	email.Headers = n.Headers

	return email
}
