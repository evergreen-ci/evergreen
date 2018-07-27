package trigger

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// DescriptionTemplateString defines the content of the alert ticket.
const descriptionTemplateString = `
h2. [{{.Task.DisplayName}} failed on {{.Build.DisplayName}}|{{.UIRoot}}/task/{{.Task.Id}}/{{.Task.Execution}}]
Host: [{{.Host.Host}}|{{.UIRoot}}/host/{{.Host.Id}}]
Project: [{{.Project.DisplayName}}|{{.UIRoot}}/waterfall/{{.Project.Identifier}}]
Commit: [diff|https://github.com/{{.Project.Owner}}/{{.Project.Repo}}/commit/{{.Version.Revision}}]: {{.Version.Message}}
{{range .Tests}}*{{.Name}}* - [Logs|{{.URL}}] | [History|{{.HistoryURL}}]
{{end}}
`
const (
	jiraMaxTitleLength = 254

	failedTestNamesTmpl = "%%FailedTestNames%%"
)

// descriptionTemplate is filled to create a JIRA alert ticket. Panics at start if invalid.
var descriptionTemplate = template.Must(template.New("Desc").Parse(descriptionTemplateString))

// jiraTestFailure contains the required fields for generating a failure report.
type jiraTestFailure struct {
	Name       string
	URL        string
	HistoryURL string
}

type jiraBuilder struct {
	project   string
	issueType string
	mappings  *evergreen.JIRANotificationsConfig

	data jiraTemplateData
}

type jiraTemplateData struct {
	UIRoot          string
	Task            *task.Task
	Build           *build.Build
	Host            *host.Host
	Project         *model.ProjectRef
	Version         *version.Version
	FailedTests     []task.TestResult
	FailedTestNames []string
	Tests           []jiraTestFailure
}

func (j *jiraBuilder) build() (*message.JiraIssue, error) {
	description, err := j.getDescription()
	if err != nil {
		return nil, errors.Wrap(err, "error creating description")
	}
	summary, err := j.getSummary()
	if err != nil {
		return nil, errors.Wrap(err, "error creating summary")
	}

	issue := message.JiraIssue{
		Project:     j.project,
		Type:        j.issueType,
		Summary:     summary,
		Description: description,
		Fields:      j.makeCustomFields(),
	}

	if err != nil {
		return nil, errors.Wrap(err, "error creating description")
	}
	grip.Info(message.Fields{
		"message":      "creating jira ticket for failure",
		"type":         j.issueType,
		"jira_project": j.project,
		"task":         j.data.Task.Id,
		"project":      j.data.Project.Identifier,
	})

	event.LogJiraIssueCreated(j.data.Task.Id, j.data.Task.Execution, j.project)

	return &issue, nil
}

// getSummary creates a JIRA subject for a task failure in the style of
//  Failures: Task_name on Variant (test1, test2) [ProjectName @ githash]
// based on the given AlertContext.
func (j *jiraBuilder) getSummary() (string, error) {
	subj := &bytes.Buffer{}
	failed := []string{}

	for _, test := range j.data.Task.LocalTestResults {
		if test.Status == evergreen.TestFailedStatus {
			failed = append(failed, cleanTestName(test.TestFile))
		}
	}

	switch {
	case j.data.Task.Details.TimedOut:
		subj.WriteString("Timed Out: ")
	case j.data.Task.Details.Type == evergreen.CommandTypeSystem:
		subj.WriteString("System Failure: ")
	case j.data.Task.Details.Type == evergreen.CommandTypeSetup:
		subj.WriteString("Setup Failure: ")
	case len(failed) == 1:
		subj.WriteString("Failure: ")
	case len(failed) > 1:
		subj.WriteString("Failures: ")
	default:
		subj.WriteString("Failed: ")
	}

	catcher := grip.NewSimpleCatcher()
	_, err := fmt.Fprintf(subj, "%s on %s ", j.data.Task.DisplayName, j.data.Build.DisplayName)
	catcher.Add(err)
	_, err = fmt.Fprintf(subj, "[%s @ %s] ", j.data.Project.DisplayName, j.data.Version.Revision[0:8])
	catcher.Add(err)

	if len(failed) > 0 {
		// Include an additional 10 characters for overhead, like the
		// parens and number of failures.
		remaining := jiraMaxTitleLength - subj.Len() - 10

		if remaining < len(failed[0]) {
			return subj.String(), catcher.Resolve()
		}
		subj.WriteString("(")
		toPrint := []string{}
		for _, fail := range failed {
			if remaining-len(fail) > 0 {
				toPrint = append(toPrint, fail)
			}
			remaining = remaining - len(fail) - 2
		}
		_, err = fmt.Fprint(subj, strings.Join(toPrint, ", "))
		catcher.Add(err)
		if len(failed)-len(toPrint) > 0 {
			_, err := fmt.Fprintf(subj, " +%d more", len(failed)-len(toPrint))
			catcher.Add(err)
		}
		subj.WriteString(")")
	}
	// Truncate string in case we made some mistake above, since it's better
	// to have a truncated title than to miss a Jira ticket.
	if subj.Len() > jiraMaxTitleLength {
		return subj.String()[:jiraMaxTitleLength], catcher.Resolve()
	}
	return subj.String(), catcher.Resolve()
}

func (j *jiraBuilder) makeCustomFields() map[string]interface{} {
	fields := map[string]interface{}{}
	m, err := j.mappings.CustomFields.ToMap()
	if err != nil {
		grip.Error(message.WrapError(err, message.Fields{
			"message": "failed to build custom fields",
			"task_id": j.data.Task.Id,
		}))
		return nil
	}
	customFields, ok := m[j.project]
	if !ok || len(customFields) == 0 {
		return nil
	}

	for i := range j.data.Task.LocalTestResults {
		if j.data.Task.LocalTestResults[i].Status == evergreen.TestFailedStatus {
			j.data.FailedTests = append(j.data.FailedTests, j.data.Task.LocalTestResults[i])
			j.data.FailedTestNames = append(j.data.FailedTestNames, j.data.Task.LocalTestResults[i].TestFile)
		}
	}

	for fieldName, fieldTmpl := range customFields {
		if fieldTmpl == failedTestNamesTmpl {
			fields[fieldName] = j.data.FailedTestNames
			continue
		}

		tmpl, err := template.New(fmt.Sprintf("%s-%s", j.project, fieldName)).Parse(fieldTmpl)
		if err != nil {
			// Admins should be notified of misconfiguration, but we shouldn't block
			// ticket generation
			grip.Alert(message.WrapError(err, message.Fields{
				"message":      "invalid custom field template",
				"jira_project": j.project,
				"jira_field":   fieldName,
				"template":     fieldTmpl,
			}))
			continue
		}

		buf := &bytes.Buffer{}
		if err = tmpl.Execute(buf, &j.data); err != nil {
			grip.Alert(message.WrapError(err, message.Fields{
				"message":      "template execution failed",
				"jira_project": j.project,
				"jira_field":   fieldName,
				"template":     fieldTmpl,
			}))
			continue
		}

		fields[fieldName] = []string{buf.String()}
	}
	return fields
}

// historyURL provides a full URL to the test's task history page.
func historyURL(t *task.Task, testName, uiRoot string) string {
	return fmt.Sprintf("%v/task_history/%v/%v#%v=fail",
		uiRoot, t.Project, t.DisplayName, testName)
}

// logURL returns the full URL for linking to a test's logs.
// Returns the empty string if no internal or external log is referenced.
func logURL(test task.TestResult, root string) string {
	if test.LogId != "" {
		return root + "/test_log/" + test.LogId
	}
	return test.URL
}

// getDescription returns the body of the JIRA ticket, with links.
func (j *jiraBuilder) getDescription() (string, error) {
	// build a list of all failed tests to include
	tests := []jiraTestFailure{}
	for _, test := range j.data.Task.LocalTestResults {
		if test.Status == evergreen.TestFailedStatus {
			tests = append(tests, jiraTestFailure{
				Name:       cleanTestName(test.TestFile),
				URL:        logURL(test, j.data.UIRoot),
				HistoryURL: historyURL(j.data.Task, cleanTestName(test.TestFile), j.data.UIRoot),
			})
		}
	}

	buf := &bytes.Buffer{}
	j.data.Tests = tests
	if err := descriptionTemplate.Execute(buf, &j.data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

// cleanTestName returns the last item of a test's path.
//   TODO: stop accommodating this.
func cleanTestName(path string) string {
	if unixIdx := strings.LastIndex(path, "/"); unixIdx != -1 {
		// if the path ends in a slash, remove it and try again
		if unixIdx == len(path)-1 {
			return cleanTestName(path[:len(path)-1])
		}
		return path[unixIdx+1:]
	}
	if windowsIdx := strings.LastIndex(path, `\`); windowsIdx != -1 {
		// if the path ends in a slash, remove it and try again
		if windowsIdx == len(path)-1 {
			return cleanTestName(path[:len(path)-1])
		}
		return path[windowsIdx+1:]
	}
	return path
}
