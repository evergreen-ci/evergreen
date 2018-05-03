package trigger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	ttemplate "text/template"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	evergreenHeaderPrefix = "X-Evergreen-"
)

type commonTemplateData struct {
	ID              string
	Object          string
	Project         string
	URL             string
	PastTenseStatus string
	Headers         http.Header
}

const emailSubjectTemplate string = `Evergreen {{ .Object }} has {{ .PastTenseStatus }}!`
const emailTemplate string = `<html>
<head>
</head>
<body>
<p>Hi,</p>

<p>Your Evergreen {{ .Object }} <a href="{{ .URL }}">'{{ .ID }}'</a> has {{ .PastTenseStatus }}.</p>

<span style="overflow:hidden; float:left; display:none !important; line-height:0px;">
{{ range $key, $value := .Headers }}
{{ range $i, $header := $value }}
{{ $key }}:{{ $header}}
{{ end }}
{{ end }}
</span>

</body>
</html>
`

const jiraCommentTemplate string = `Evergreen {{ .Object }} [{{ .ID }}|{{ .URL }}] in '{{ .Project }}' has {{ .PastTenseStatus }}!`

const jiraIssueTitle string = "Evergreen {{ .Object }} '{{ .ID }}' in '{{ .Project }}' has {{ .PastTenseStatus }}"

const slackTemplate string = `Evergreen {{ .Object }} <{{ .URL }}|{{ .ID }}> in '{{ .Project }}' has {{ .PastTenseStatus }}!`

func makeHeaders(selectors []event.Selector) http.Header {
	headers := http.Header{}
	for i := range selectors {
		headers[evergreenHeaderPrefix+selectors[i].Type] = append(headers[evergreenHeaderPrefix+selectors[i].Type], selectors[i].Data)
	}

	return headers
}

func emailPayload(t commonTemplateData) (*message.Email, error) {
	bodyTmpl, err := template.New("emailBody").Parse(emailTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse body template")
	}
	buf := new(bytes.Buffer)
	err = bodyTmpl.Execute(buf, t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute email template")
	}
	body := buf.String()

	subjectTmpl, err := template.New("subject").Parse(emailSubjectTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse subject template")
	}
	buf = new(bytes.Buffer)
	err = subjectTmpl.Execute(buf, t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to execute subject template")
	}
	subject := buf.String()

	m := message.Email{
		Subject:           subject,
		Body:              body,
		PlainTextContents: false,
		Headers:           t.Headers,
	}

	return &m, nil
}

func webhookPayload(api restModel.Model, headers http.Header) (*util.EvergreenWebhook, error) {
	bytes, err := json.Marshal(api)
	if err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	return &util.EvergreenWebhook{
		Body:    bytes,
		Headers: headers,
	}, nil
}

func jiraComment(t commonTemplateData) (*string, error) {
	commentTmpl, err := ttemplate.New("jira-comment").Parse(jiraCommentTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse jira comment template")
	}

	buf := new(bytes.Buffer)
	if err = commentTmpl.Execute(buf, t); err != nil {
		return nil, errors.Wrap(err, "failed to make jira comment")
	}
	comment := buf.String()

	return &comment, nil
}

func jiraIssue(t commonTemplateData) (*message.JiraIssue, error) {
	const maxSummary = 254

	comment, err := jiraComment(t)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make jira issue")
	}

	issueTmpl, err := ttemplate.New("jira-issue").Parse(jiraIssueTitle)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse jira issue template")
	}

	buf := new(bytes.Buffer)
	if err = issueTmpl.Execute(buf, t); err != nil {
		return nil, errors.Wrap(err, "failed to make jira issue")
	}
	title, remainder := truncateString(buf.String(), maxSummary)
	desc := *comment
	if len(remainder) != 0 {
		desc = fmt.Sprintf("...\n%s\n%s", remainder, desc)
	}

	issue := message.JiraIssue{
		Summary:     title,
		Description: desc,
	}

	return &issue, nil
}

// truncateString splits a string into two parts, with the following behavior:
// If the entire string is <= capacity, it's returned unchanged.
// Otherwise, the string is split at the (capacity-3)'th byte. The first string
// returned will be this string + '...', and the second string will be the
// remaining characters, if any
func truncateString(s string, capacity int) (string, string) {
	if len(s) <= capacity {
		return s, ""
	}
	if capacity <= 0 {
		return "", s
	}

	head := s[0:capacity-3] + "..."
	tail := s[capacity-3:]

	return head, tail
}

func slack(t commonTemplateData) (*notification.SlackPayload, error) {
	issueTmpl, err := ttemplate.New("slack").Parse(slackTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse slack template")
	}

	buf := new(bytes.Buffer)
	if err = issueTmpl.Execute(buf, t); err != nil {
		return nil, errors.Wrap(err, "failed to make slack message")
	}
	msg := buf.String()

	return &notification.SlackPayload{
		Body: msg,
	}, nil
}
