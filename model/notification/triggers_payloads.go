package notification

import (
	"bytes"
	"encoding/json"
	"html/template"
	ttemplate "text/template"

	"github.com/evergreen-ci/evergreen/model/event"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	evergreenHeaderPrefix = "X-Evergreen-"
)

type emailTemplateData struct {
	Object          string
	ID              string
	URL             string
	PastTenseStatus string
	Headers         map[string][]string
}

const emailSubjectTemplate string = `Evergreen {{ .Object }} has {{ .PastTenseStatus }}!`
const emailTemplate string = `<html>
<head>
</head>
<body>
<p>Hi,</p>

<p>your Evergreen {{ .Object }} <a href="{{ .URL }}">'{{ .ID }}'</a> has {{ .PastTenseStatus }}.</p>

<span style="overflow:hidden; float:left; display:none !important; line-height:0px;">
{{ range $key, $value := .Headers }}
{{ range $i, $header := $value }}
{{ $key }}:{{ $header}}\n
{{ end }}
{{ end }}
</span>

</body>
</html>
`

func emailPayload(id, objectName, url, status string, selectors []event.Selector) (*message.Email, error) {
	t := emailTemplateData{
		Object:          objectName,
		ID:              id,
		URL:             url,
		PastTenseStatus: status,
		Headers:         map[string][]string{},
	}

	for i := range selectors {
		t.Headers[evergreenHeaderPrefix+selectors[i].Type] = append(t.Headers[evergreenHeaderPrefix+selectors[i].Type], selectors[i].Data)
	}

	bodyTmpl, err := template.New("emailBody").Parse(emailTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse body template")
	}
	buf := new(bytes.Buffer)
	bodyTmpl.Execute(buf, t)
	body := buf.String()

	subjectTmpl, err := template.New("subject").Parse(emailSubjectTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse subject template")
	}
	buf = new(bytes.Buffer)
	subjectTmpl.Execute(buf, t)
	subject := buf.String()

	m := message.Email{
		Subject:           subject,
		Body:              body,
		PlainTextContents: false,
		Headers:           t.Headers,
	}

	return &m, nil
}

func webhookPayload(api restModel.Model, selectors []event.Selector) (*util.EvergreenWebhook, error) {
	bytes, err := json.Marshal(api)
	if err != nil {
		return nil, errors.Wrap(err, "error building json model")
	}

	return &util.EvergreenWebhook{
		Body: bytes,
	}, nil
}

const jiraCommentTemplate string = `Evergreen {{ .Object }} [{{ .ID }}|{{ .URL }}] has {{ .PastTenseStatus }}!`

func jiraComment(id, objectName, url, status string) (*string, error) {
	commentTmpl, err := ttemplate.New("jira-comment").Parse(jiraCommentTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse jira comment template")
	}

	t := emailTemplateData{
		Object:          objectName,
		ID:              id,
		URL:             url,
		PastTenseStatus: status,
	}

	buf := new(bytes.Buffer)
	if err = commentTmpl.Execute(buf, t); err != nil {
		return nil, errors.Wrap(err, "failed to make jira comment")
	}
	comment := buf.String()

	return &comment, nil
}

const jiraIssueTitle string = "Evergreen {{ .Object }} has {{ .PastTenseStatus }}"

func jiraIssue(id, objectName, url, status string) (*message.JiraIssue, error) {
	// TODO cleanup
	comment, err := jiraComment(id, objectName, url, status)
	if err != nil {
		return nil, errors.Wrap(err, "failed to make jira issue")
	}

	t := emailTemplateData{
		Object:          objectName,
		ID:              id,
		URL:             url,
		PastTenseStatus: status,
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

func slack(id, objectName, url, status string) (*SlackPayload, error) {
	t := emailTemplateData{
		Object:          objectName,
		ID:              id,
		URL:             url,
		PastTenseStatus: status,
	}

	issueTmpl, err := ttemplate.New("slack").Parse(slackTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "failed to parse slack template")
	}

	buf := new(bytes.Buffer)
	if err = issueTmpl.Execute(buf, t); err != nil {
		return nil, errors.Wrap(err, "failed to make slack message")
	}
	msg := buf.String()

	return &SlackPayload{
		Body: msg,
	}, nil
}
