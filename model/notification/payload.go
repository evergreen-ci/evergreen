package notification

import (
	"github.com/mongodb/grip/message"
	"github.com/mongodb/grip/send"
)

type EmailPayload struct {
	Headers map[string]string `bson:"headers"`
	Subject string            `bson:"subject"`
	Body    []byte            `bson:"body"`
}

func (e *EmailPayload) GetContents(opts *send.SMTPOptions, _ message.Composer) (string, string) {
	opts.PlainTextContents = false
	return e.Subject, string(e.Body)

}

type EvergreenWebhookPayload struct {
	Headers map[string]string `bson:"headers"`
	Payload []byte            `bson:"payload"`
}

type GithubStatusAPIPayload struct {
	Status      string `bson:"status"`
	Context     string `bson:"context"`
	Description string `bson:"description"`
	URL         string `bson:"url"`
}

type SlackPayload struct {
	URL  string `bson:"string"`
	Text string `bson:"text"`
	//Colour      string              `bson:'colour'`
	//Attachments []map[string]string `bson:"attachments"`
}

// TODO
// This is a copy of message.JiraIssue, but with bson tags.
// Is there a way around this?
type JiraIssuePayload struct {
	Summary     string   `bson:"summary"`
	Description string   `bson:"description"`
	Reporter    string   `bson:"reporter"`
	Assignee    string   `bson:"assignee"`
	Type        string   `bson:"type"`
	Components  []string `bson:"components"`
	Labels      []string `bson:"labels"`
	// ... other fields
	Fields map[string]string `bson:"fields"`
}
