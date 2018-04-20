package notification

import (
	"bytes"
	"html/template"

	"github.com/evergreen-ci/evergreen/model/event"
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
