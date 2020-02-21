package message

import (
	"fmt"
	"net/mail"

	"github.com/mongodb/grip/level"
)

// Email represents the parameters of an email message
type Email struct {
	From       string   `bson:"from" json:"from" yaml:"from"`
	Recipients []string `bson:"recipients" json:"recipients" yaml:"recipients"`
	Subject    string   `bson:"subject" json:"subject" yaml:"subject"`
	Body       string   `bson:"body" json:"body" yaml:"body"`
	// PlainTextContents dictates the Content-Type of the email. If true,
	// it will text/plain; otherwise, it is text/html. This value is overridden
	// by the presence of a "Content-Type" header in Headers.
	PlainTextContents bool `bson:"is_plain_text" json:"is_plain_text" yaml:"is_plain_text"`

	// Headers adds additional headers to the email body, ignoring any
	// named "To", "From", "Subject", or "Content-Transfer-Encoding"
	// (which should be set with the above fields)
	Headers map[string][]string `bson:"headers" json:"headers" yaml:"headers"`
}

type emailMessage struct {
	data Email
	Base `bson:"metadata" json:"metadata" yaml:"metadata"`
}

// NewEmailMessage returns a composer for emails
func NewEmailMessage(l level.Priority, e Email) Composer {
	m := MakeEmailMessage(e)
	_ = m.SetPriority(l)

	return m
}

// MakeEmailMessage creates a composer for emails without a priority set
func MakeEmailMessage(e Email) Composer {
	return &emailMessage{
		data: e,
	}
}

func (e *emailMessage) Loggable() bool {
	if len(e.data.From) != 0 {
		if _, err := mail.ParseAddress(e.data.From); err != nil {
			return false
		}
	}

	if len(e.data.Recipients) == 0 {
		return false
	}
	for i := range e.data.Recipients {
		_, err := mail.ParseAddress(e.data.Recipients[i])
		if err != nil {
			return false
		}
	}
	if len(e.data.Subject) == 0 {
		return false
	}
	if len(e.data.Body) == 0 {
		return false
	}

	// reject empty headers
	for _, v := range e.data.Headers {
		if len(v) == 0 {
			return false
		}
	}

	return true
}

func (e *emailMessage) Raw() interface{} {
	return &e.data
}

func (e *emailMessage) String() string {
	const (
		tmpl       = `To: %s; %sBody: %s`
		headerTmpl = "%s: %s\n"
	)

	headers := "Headers: "
	for k, v := range e.data.Headers {
		headers += fmt.Sprintf(headerTmpl, k, v)
	}
	if len(e.data.Headers) == 0 {
		headers = ""
	} else {
		headers += "; "
	}

	to := ""
	for i, recp := range e.data.Recipients {
		to += recp
		if i != (len(e.data.Recipients) - 1) {
			to += ", "
		}
	}

	return fmt.Sprintf(tmpl, to, headers, e.data.Body)
}
