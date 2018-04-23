package trigger

import (
	"bytes"
	"encoding/json"
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
	Object          string
	ID              string
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

const jiraCommentTemplate string = `Evergreen {{ .Object }} [{{ .ID }}|{{ .URL }}] has {{ .PastTenseStatus }}!`

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

const jiraIssueTitle string = "Evergreen {{ .Object }} has {{ .PastTenseStatus }}"

func jiraIssue(t commonTemplateData) (*message.JiraIssue, error) {
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
	title := buf.String()

	issue := message.JiraIssue{
		Summary:     title,
		Description: *comment,
	}

	return &issue, nil
}

const slackTemplate string = `Evergreen {{ .Object }} <{{ .URL }}|{{ .ID }}> has {{ .PastTenseStatus }}!`

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
