package trigger

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"net/http"
	"net/url"
	ttemplate "text/template"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/notification"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

const (
	evergreenHeaderPrefix = "X-Evergreen-"

	evergreenSuccessColor    = "#4ead4a"
	evergreenFailColor       = "#ce3c3e"
	evergreenSystemFailColor = "#ce3c3e"
	evergreenRunningColor    = "#ffdd49"

	// slackAttachmentsLimit is a limit to the number of extra entries to
	// attach to a Slack message. It does not count the link to Evergreen,
	// or the link back to Github Pull Requests.
	// This number MUST NOT exceed 100, and Slack recommends a limit of 10
	slackAttachmentsLimit = 10
)

type commonTemplateData struct {
	ID              string
	EventID         string
	SubscriptionID  string
	DisplayName     string
	Object          string
	Project         string
	Description     string
	URL             string
	PastTenseStatus string
	Headers         http.Header
	FailedTests     []testresult.TestResult

	Task       *task.Task
	ProjectRef *model.ProjectRef
	Build      *build.Build

	apiModel any
	slack    []message.SlackAttachment

	githubContext     string
	githubState       message.GithubState
	githubDescription string

	emailContent *template.Template
}

type emailTemplateData struct {
	commonTemplateData
	Description template.HTML
}

const emailSubjectTemplateString string = `Evergreen: {{ .Object }} {{.DisplayName}} in '{{ .Project }}' has {{ .PastTenseStatus }}!`

var subjectTmpl = template.Must(template.New("subject").Parse(emailSubjectTemplateString))

const emailBodyTemplateBase string = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
</head>
<body>

{{ template "content" . }}

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

const emailTaskFailTemplate = `
{{ define "content" }}
<table>
<tr><td colspan="3" height="10" bgcolor="#3b291f"></td></tr>
<tr><td colspan="3" height="20"></td></tr>
<tr>
  <td width="20"></td>
  <td align="left">
    <!-- table lvl 2 -->
    <table cellpadding="0" cellspacing="0" width="100%">
      {{ range .FailedTests }}
        <tr>
          <td width="90%">
            <span style="font-family:Arial,sans-serif;font-weight:bold;font-size:10px;color:#999999" class="label">TEST</span>
          </td>
          <td>&nbsp;</td>
        </tr>
        <tr>
          <td width="90%">
            <span style="font-family:Arial,sans-serif;font-weight:bold;font-size:36px;line-height:28px;color:#333333" class="task">
	      {{ .GetDisplayTestName }}
            </span>
          </td>
          {{ if eq $.Task.Details.Type "system" }}
          <td style="padding:0 10px;background-color:#800080;">
          {{ else }}
          <td style="padding:0 10px;background-color:#ed1c24;">
          {{ end }}
            <span style="font-family:Arial,sans-serif;font-weight:bold;font-size:18px;color:#ffffff" class="status">FAILED</span>
          </td>
        </tr>
        <tr><td colspan="2" height="10"></td></tr>
        <tr>
          <td width="90%">
            <a href="{{ .LogURL }}" style="font-family:Arial,sans-serif;font-weight:normal;font-size:13px;color:#006cbc" class="link">view logs</a>
          </td>
          <td>&nbsp;</td>
        </tr>
      {{end}}

      <tr><td colspan="2" height="30"></td></tr>
      <tr>
        <td width="90%"><span style="font-family:Arial,sans-serif;font-weight:bold;font-size:10px;color:#999999" class="label">PROJECT</span></td>
        <td>&nbsp;</td>
      </tr>
      <tr>
        <td width="90%">
          <span style="font-family:Arial,sans-serif;font-weight:bold;font-size:36px;line-height:28px;color:#333333" class="task">
            {{ .ProjectRef.DisplayName }}
          </span>
        </td>
      </tr>
      <tr><td colspan="2" height="30"></td></tr>
      <tr>
        <td width="90%"><span style="font-family:Arial,sans-serif;font-weight:bold;font-size:10px;color:#999999" class="label">TASK</span></td>
        <td>&nbsp;</td>
      </tr>
      <tr>
        <td width="90%">
          <span style="font-family:Arial,sans-serif;font-weight:bold;font-size:36px;line-height:28px;color:#333333" class="task">
            {{ if .Task.DisplayTask }}
            {{ .Task.DisplayTask.DisplayName }}
            {{ else }}
            {{ .Task.DisplayName }}
            {{ end }}
          </span>
        </td>
        {{ if .Task.Details.TimedOut }}
          {{ if eq .Task.Details.Type "system" }}
          <td style="padding:0 10px;background-color:#800080;">
          {{ else }}
          <td style="padding:0 10px;background-color:#ed1c24;">
          {{ end }}
            <span style="font-family:Arial,sans-serif;font-weight:bold;font-size:18px;color:#ffffff" class="status">
              TIMED OUT
            </span>
          </td>
        {{ else }}
          <td>&nbsp;</td>
        {{ end }}
      </tr>
      <tr><td colspan="2" height="10"></td></tr>
      <tr>
        <td width="90%">
          <a href="{{ .URL }}" style="font-family:Arial,sans-serif;font-weight:normal;font-size:13px;color:#006cbc" class="link">view task</a>
        </td>
        <td>&nbsp;</td>
      </tr>

      <tr>
        <td colspan="2" height="30"></td>
      </tr>
      <tr>
        <td colspan="2"><span style="font-family:Arial,sans-serif;font-weight:bold;font-size:10px;color:#999999" class="label">BUILD VARIANT</span></td>
      </tr>
      <tr>
        <td colspan="2">
          <span style="font-family:Arial,sans-serif;font-weight:bold;font-size:36px;color:#333333" class="build">
            {{ .Build.DisplayName }}
          </span>
        </td>
      </tr>
      <tr>
	     <td colspan="2"><span style="font-family:Arial,sans-serif;font-weight:bold;font-size:10px;color:#999999" class="label">SUBSCRIPTION ID</span></td>
	  </tr>
      <tr>
        <td colspan="2">
          <span style="font-family:Arial,sans-serif;font-weight:bold;font-size:36px;color:#333333" class="subscription">
            {{ .SubscriptionID }}
          </span>
        </td>
      </tr>
      <tr>
	     <td colspan="2"><span style="font-family:Arial,sans-serif;font-weight:bold;font-size:10px;color:#999999" class="label">EVENT ID</span></td>
	  </tr>
      <tr>
        <td colspan="2">
          <span style="font-family:Arial,sans-serif;font-weight:bold;font-size:36px;color:#333333" class="event">
            {{ .EventID }}
          </span>
        </td>
      </tr>
    </table>
  </td>
  <td width="20"></td>
</tr>
</table>
{{ end }}`

var emailBodyTemplate = template.Must(template.New("emailbody").Parse(emailBodyTemplateBase))

const emailDefaultContentTemplateString = `{{ define "content"}}
<p>Hi,</p>

<p>Your Evergreen {{ .Object }} in '{{ .Project }}' <a href="{{ .URL }}">{{ .DisplayName }}</a> has {{ .PastTenseStatus }}.</p>
<p>{{ .Description }}</p>

<p>Subscription: {{ .SubscriptionID }}; Event: {{ .EventID }} </p>
{{ end }}`

var emailDefaultContentTemplate = template.Must(template.New("content").Parse(emailDefaultContentTemplateString))
var emailTaskContentTemplate = template.Must(template.New("content").Parse(emailTaskFailTemplate))

const jiraCommentTemplate string = `Evergreen {{ .Object }} [{{ .DisplayName }}|{{ .URL }}] in '{{ .Project }}' has {{ .PastTenseStatus }}!`

const jiraIssueTitle string = "Evergreen {{ .Object }} '{{ .DisplayName }}' in '{{ .Project }}' has {{ .PastTenseStatus }}"

const slackTemplate string = `The {{ .Object }} <{{ .URL }}|{{ .DisplayName }}> in '{{ .Project }}' has {{ .PastTenseStatus }}!`

func makeHeaders(headerMap map[string][]string) http.Header {
	headers := http.Header{}
	for headerField, headerData := range headerMap {
		headers[evergreenHeaderPrefix+headerField] = headerData
	}

	return headers
}

func emailPayload(t *commonTemplateData) (*message.Email, error) {
	emailData := &emailTemplateData{
		commonTemplateData: *t,
		Description:        template.HTML(t.Description),
	}
	bodyTmpl, err := emailBodyTemplate.Clone()
	if err != nil {
		return nil, errors.Wrap(err, "cloning email body template")
	}
	if emailData.emailContent == nil {
		_, err = bodyTmpl.AddParseTree("content", emailDefaultContentTemplate.Tree)
	} else {
		_, err = bodyTmpl.AddParseTree("content", emailData.emailContent.Tree)
	}
	if err != nil {
		return nil, errors.Wrap(err, "adding email content")
	}
	buf := &bytes.Buffer{}
	err = bodyTmpl.ExecuteTemplate(buf, "emailbody", emailData)
	if err != nil {
		return nil, errors.Wrap(err, "executing email template")
	}
	body := buf.String()

	buf = &bytes.Buffer{}
	err = subjectTmpl.Execute(buf, emailData)
	if err != nil {
		return nil, errors.Wrap(err, "executing email subject template")
	}
	subject := buf.String()

	m := message.Email{
		Subject:           subject,
		Body:              body,
		PlainTextContents: false,
		Headers:           emailData.Headers,
	}

	// prevent Gmail from threading notifications with similar subjects
	m.Headers["X-Entity-Ref-Id"] = []string{fmt.Sprintf("%s-%s-%s", emailData.Object, emailData.SubscriptionID, emailData.EventID)}
	m.Headers["X-Evergreen-Event-Id"] = []string{emailData.EventID}
	m.Headers["X-Evergreen-Subscription-Id"] = []string{emailData.SubscriptionID}

	return &m, nil
}

func webhookPayload(api any, headers http.Header) (*util.EvergreenWebhook, error) {
	bytes, err := json.Marshal(api)
	if err != nil {
		return nil, errors.Wrap(err, "building JSON model")
	}

	return &util.EvergreenWebhook{
		Body:    bytes,
		Headers: headers,
	}, nil
}

func jiraComment(t *commonTemplateData) (*string, error) {
	commentTmpl, err := ttemplate.New("jira-comment").Parse(jiraCommentTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "parsing Jira comment template")
	}

	buf := &bytes.Buffer{}
	if err = commentTmpl.Execute(buf, t); err != nil {
		return nil, errors.Wrap(err, "generating Jira comment text from template")
	}
	comment := buf.String()

	return &comment, nil
}

func jiraIssue(t *commonTemplateData) (*message.JiraIssue, error) {
	const maxSummary = 254

	comment, err := jiraComment(t)
	if err != nil {
		return nil, errors.Wrap(err, "making Jira issue")
	}

	issueTmpl, err := ttemplate.New("jira-issue").Parse(jiraIssueTitle)
	if err != nil {
		return nil, errors.Wrap(err, "parsing Jira issue template")
	}

	buf := &bytes.Buffer{}
	if err = issueTmpl.Execute(buf, t); err != nil {
		return nil, errors.Wrap(err, "generating Jira ticket text from template")
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

func slack(t *commonTemplateData) (*notification.SlackPayload, error) {
	issueTmpl, err := ttemplate.New("slack").Parse(slackTemplate)
	if err != nil {
		return nil, errors.Wrap(err, "parsing Slack template")
	}

	buf := &bytes.Buffer{}
	if err = issueTmpl.Execute(buf, t); err != nil {
		return nil, errors.Wrap(err, "generating Slack message text from template")
	}
	msg := buf.String()

	if len(t.slack) > 0 {
		t.slack[len(t.slack)-1].Footer = fmt.Sprintf("Subscription: %s; Event: %s", t.SubscriptionID, t.EventID)
	}

	return &notification.SlackPayload{
		Body:        msg,
		Attachments: t.slack,
	}, nil
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

func makeCommonPayload(sub *event.Subscription, eventAttributes event.Attributes,
	data *commonTemplateData) (any, error) {
	var err error
	headerMap := eventAttributes.ToSelectorMap()
	headerMap["trigger"] = append(headerMap["trigger"], sub.Trigger)
	headerMap[event.SelectorStatus] = append(headerMap[event.SelectorStatus], data.PastTenseStatus)
	data.Headers = makeHeaders(headerMap)
	data.SubscriptionID = sub.ID
	if data.Task != nil {
		data.FailedTests, err = getFailedTestsFromTemplate(*data.Task)
		if err != nil {
			return nil, errors.Wrap(err, "getting failed tests")
		}
	}

	switch sub.Subscriber.Type {
	case event.GithubPullRequestSubscriberType, event.GithubCheckSubscriberType, event.GithubMergeSubscriberType:
		if len(data.githubDescription) == 0 {
			return nil, errors.Errorf("GitHub subscriber not supported for trigger '%s'", sub.Trigger)
		}
		msg := &message.GithubStatus{
			Context:     data.githubContext,
			State:       data.githubState,
			URL:         data.URL,
			Description: data.githubDescription,
		}
		if len(data.githubContext) != 0 {
			msg.Context = data.githubContext
		}
		return msg, nil

	case event.JIRAIssueSubscriberType:
		return jiraIssue(data)

	case event.JIRACommentSubscriberType:
		return jiraComment(data)

	case event.EvergreenWebhookSubscriberType:
		return webhookPayload(data.apiModel, data.Headers)

	case event.EmailSubscriberType:
		return emailPayload(data)

	case event.SlackSubscriberType:
		return slack(data)
	case event.RunChildPatchSubscriberType:
		return nil, nil
	}

	return nil, errors.Errorf("unknown subscriber type '%s'", sub.Subscriber.Type)
}

func getFailedTestsFromTemplate(t task.Task) ([]testresult.TestResult, error) {
	results := []testresult.TestResult{}
	for i := range t.LocalTestResults {
		if t.LocalTestResults[i].Status == evergreen.TestFailedStatus {
			testResult := t.LocalTestResults[i]
			testResult.LogURL = testResult.GetLogURL(evergreen.GetEnvironment(), evergreen.LogViewerHTML)
			results = append(results, testResult)
		}
	}

	return results, nil
}

func taskLink(uiBase string, taskID string, execution int) string {
	return fmt.Sprintf("%s/task/%s/%d", uiBase, url.PathEscape(taskID), execution)
}

func taskLogLink(uiBase string, taskID string, execution int) string {
	return fmt.Sprintf("%s/task_log_raw/%s/%d?type=T", uiBase, url.PathEscape(taskID), execution)
}

type versionLinkInput struct {
	uiBase    string
	versionID string
	hasPatch  bool
	isChild   bool
}

func versionLink(i versionLinkInput) string {
	url := fmt.Sprintf("%s/version/%s", i.uiBase, url.PathEscape(i.versionID))
	if i.isChild {
		url += "/downstream-projects"
	}
	return url
}

func hostLink(uiBase, hostID string) string {
	return fmt.Sprintf("%s/host/%s", uiBase, hostID)
}
