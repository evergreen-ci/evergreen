package trigger

import (
	"bytes"
	"context"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"

	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/testresult"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip"
	"github.com/mongodb/grip/message"
	"github.com/pkg/errors"
)

// DescriptionTemplateString defines the content of the alert ticket.
const descriptionTemplateString = `
h2. [{{.Task.DisplayName}} failed on {{.Build.DisplayName}}|{{taskurl .}}]
Host: {{host .}}
Project: [{{.Project.DisplayName}}|{{.UIv2Url}}/project/{{.Project.Id}}/waterfall]
Commit: [diff|https://github.com/{{.Project.Owner}}/{{.Project.Repo}}/commit/{{.Version.Revision}}]: {{.Version.Message}} | {{.Task.CreateTime | formatAsTimestamp}}
Evergreen Subscription: {{.SubscriptionID}}; Evergreen Event: {{.EventID}}
{{range .Tests}}*{{.Name}}* - [Logs|{{.URL}}] {{if (eq .DisplayTaskId "") }} |[History|{{.HistoryURL}}]{{end}}
{{end}}
{{range taskLogURLs . }}[Task Logs ({{.DisplayName}}) | {{.URL}}]
{{end}}`

const (
	jiraMaxTitleLength = 254

	failedTestNamesTmpl = "%%FailedTestNames%%"
)

// descriptionTemplate is filled to create a JIRA alert ticket. Panics at start if invalid.
var descriptionTemplate = template.Must(template.New("Desc").Funcs(template.FuncMap{
	"taskurl":           getTaskURL,
	"formatAsTimestamp": formatAsTimestamp,
	"host":              getHostMetadata,
	"isHostTask":        getIsHostTask,
	"taskLogURLs":       getTaskLogURLs,
}).Parse(descriptionTemplateString))

func formatAsTimestamp(t time.Time) string {
	return t.Format(time.RFC822)
}

func getHostMetadata(data *jiraTemplateData) string {
	if data.Host == nil {
		return "N/A"
	}

	return fmt.Sprintf("[%s|%s/host/%s]", data.Host.Host, data.UIRoot, url.PathEscape(data.Host.Id))
}

// getIsHostTask returns a non-empty string it is a host task and returns an
// empty string if it's not a host task.
func getIsHostTask(data *jiraTemplateData) string {
	if data.Task.IsHostTask() {
		return strconv.FormatBool(true)
	}
	return ""
}

func getTaskURL(data *jiraTemplateData) (string, error) {
	if data.Task == nil {
		return "", errors.New("task is nil")
	}
	id := data.Task.Id
	execution := data.Task.Execution
	if len(data.Task.OldTaskId) != 0 {
		id = data.Task.OldTaskId
	}

	return taskLink(data.UIRoot, id, execution), nil
}

type taskInfo struct {
	DisplayName string
	URL         string
}

func getTaskLogURLs(data *jiraTemplateData) ([]taskInfo, error) {
	if data.Task == nil {
		return nil, errors.New("task is nil")
	}

	if data.Task.DisplayOnly {
		// Task is display only with tests
		if len(data.Tests) > 0 {
			execTaskMap := make(map[string][]jiraTestFailure)
			result := make([]taskInfo, 0, len(execTaskMap))
			for _, test := range data.Tests {
				execTaskMap[test.TaskID] = append(execTaskMap[test.TaskID], test)
			}
			for taskID, tests := range execTaskMap {
				info := taskInfo{URL: taskLogLink(data.UIRoot, taskID, tests[0].Execution)}
				testIDs := make([]string, 0, len(tests))
				for _, test := range tests {
					testIDs = append(testIDs, test.Name)
				}
				info.DisplayName = strings.Join(testIDs, " ")

				result = append(result, info)
			}
			return result, nil
		} else {
			// Task is display only without tests
			result := make([]taskInfo, 0)
			execTasks, err := task.Find(data.Context, task.ByIds(data.Task.ExecutionTasks))
			if err != nil {
				return nil, errors.Wrapf(err, "finding execution tasks for task '%s'", data.Task.Id)
			}

			for _, execTask := range execTasks {
				if execTask.Status == evergreen.TaskFailed {
					id := execTask.Id
					execution := execTask.Execution
					displayName := execTask.DisplayName
					info := taskInfo{DisplayName: displayName, URL: taskLogLink(data.UIRoot, id, execution)}
					result = append(result, info)
				}
			}
			return result, nil
		}
	} else {
		// Task is not display only
		id := data.Task.Id
		execution := data.Task.Execution
		displayName := data.Task.DisplayName
		if len(data.Task.OldTaskId) != 0 {
			id = data.Task.OldTaskId
		}
		return []taskInfo{{DisplayName: displayName, URL: taskLogLink(data.UIRoot, id, execution)}}, nil
	}
}

// jiraTestFailure contains the required fields for generating a failure report.
type jiraTestFailure struct {
	Name          string
	URL           string
	HistoryURL    string
	TaskID        string
	Execution     int
	DisplayTaskId string
}

type jiraBuilder struct {
	project   string
	issueType string
	mappings  *evergreen.JIRANotificationsConfig

	data jiraTemplateData
}

type jiraTemplateData struct {
	Context            context.Context
	UIRoot             string
	UIv2Url            string
	SubscriptionID     string
	EventID            string
	Task               *task.Task
	Build              *build.Build
	Host               *host.Host
	Project            *model.ProjectRef
	Version            *model.Version
	FailedTests        []testresult.TestResult
	FailedTestNames    []string
	Tests              []jiraTestFailure
	SpecificTaskStatus string
	TaskDisplayName    string
}

func makeSummaryPrefix(t *task.Task, failed int) string {
	s := t.GetDisplayStatus()
	switch {
	case s == evergreen.TaskSucceeded:
		return "Succeeded: "
	case s == evergreen.TaskSystemTimedOut:
		return "System Timed Out: "
	case s == evergreen.TaskTimedOut:
		return "Timed Out: "
	case s == evergreen.TaskSystemUnresponse:
		return "System Unresponsive: "
	case s == evergreen.TaskSystemFailed:
		return "System Failure: "
	case s == evergreen.TaskSetupFailed:
		return "Setup Failure: "
	case failed == 1:
		return "Failure: "
	case failed > 1:
		return "Failures: "
	default:
		return "Failed: "
	}
}

func (j *jiraBuilder) build(ctx context.Context) (*message.JiraIssue, error) {
	if err := j.data.Task.PopulateTestResults(ctx); err != nil {
		return nil, errors.Wrap(err, "populating test results")
	}

	j.data.SpecificTaskStatus = j.data.Task.GetDisplayStatus()
	description, err := j.getDescription()
	if err != nil {
		return nil, errors.Wrap(err, "creating description")
	}
	summary, err := j.getSummary()
	if err != nil {
		return nil, errors.Wrap(err, "creating summary")
	}

	fields := map[string]any{}
	components := []string{}
	labels := []string{}
	for _, project := range j.mappings.CustomFields {
		if project.Project == j.project {
			fields = j.makeCustomFields(project.Fields)
			components = project.Components
			labels = project.Labels
		}
	}

	issue := message.JiraIssue{
		Project:     j.project,
		Type:        j.issueType,
		Summary:     summary,
		Description: description,
		Fields:      fields,
		Components:  components,
		Labels:      labels,
	}

	grip.Info(message.Fields{
		"message":      "creating Jira ticket for failure",
		"type":         j.issueType,
		"jira_project": j.project,
		"task":         j.data.Task.Id,
		"project":      j.data.Project.Id,
		"issue":        issue,
	})

	return &issue, nil
}

// getSummary creates a JIRA subject for a task failure in the style of
//
//	Failures: Task_name on Variant (test1, test2) [ProjectName @ githash]
//
// based on the given AlertContext.
func (j *jiraBuilder) getSummary() (string, error) {
	subj := &bytes.Buffer{}
	failed := []string{}

	for _, test := range j.data.Task.LocalTestResults {
		if test.Status == evergreen.TestFailedStatus {
			failed = append(failed, cleanTestName(test.GetDisplayTestName()))
		}
	}

	subj.WriteString(makeSummaryPrefix(j.data.Task, len(failed)))

	catcher := grip.NewSimpleCatcher()
	if j.data.Task.DisplayTask != nil {
		_, err := fmt.Fprint(subj, j.data.Task.DisplayTask.DisplayName)
		catcher.Add(err)
	} else {
		_, err := fmt.Fprint(subj, j.data.Task.DisplayName)
		catcher.Add(err)
	}
	_, err := fmt.Fprintf(subj, " on %s ", j.data.Build.DisplayName)
	catcher.Add(err)
	_, err = fmt.Fprintf(subj, "[%s @ %s] ", j.data.Project.DisplayName, j.data.Version.Revision[0:8])
	catcher.Add(err)

	if len(failed) > 0 {
		// include an additional 10 characters for overhead, like the
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

func (j *jiraBuilder) makeCustomFields(customFields []evergreen.JIRANotificationsCustomField) map[string]any {
	fields := map[string]any{}
	for i := range j.data.Task.LocalTestResults {
		if j.data.Task.LocalTestResults[i].Status == evergreen.TestFailedStatus {
			j.data.FailedTests = append(j.data.FailedTests, j.data.Task.LocalTestResults[i])
			j.data.FailedTestNames = append(j.data.FailedTestNames, j.data.Task.LocalTestResults[i].GetDisplayTestName())
		}
	}

	for _, field := range customFields {
		if field.Template == failedTestNamesTmpl {
			fields[field.Field] = j.data.FailedTestNames
			continue
		}

		tmpl, err := template.New(fmt.Sprintf("%s-%s", j.project, field.Field)).Parse(field.Template)
		if err != nil {
			// Admins should be notified of misconfiguration, but we shouldn't block
			// ticket generation
			grip.Alert(message.WrapError(err, message.Fields{
				"message":      "invalid custom field template",
				"jira_project": j.project,
				"jira_field":   field.Field,
				"template":     field.Template,
			}))
			continue
		}

		buf := &bytes.Buffer{}
		if err = tmpl.Execute(buf, &j.data); err != nil {
			grip.Alert(message.WrapError(err, message.Fields{
				"message":      "template execution failed",
				"jira_project": j.project,
				"jira_field":   field.Field,
				"template":     field.Template,
			}))
			continue
		}

		fields[field.Field] = []string{buf.String()}
	}
	return fields
}

// historyURL provides a full URL to the test's task history page.
func historyURL(t *task.Task, testName, uiRoot string) string {
	return fmt.Sprintf("%s/task_history/%s/%s?revision=%s#/%s=fail",
		uiRoot, url.PathEscape(t.Project), url.PathEscape(t.DisplayName), t.Revision, url.QueryEscape(testName))
}

// getDescription returns the body of the JIRA ticket, with links.
func (j *jiraBuilder) getDescription() (string, error) {
	const jiraMaxDescLength = 32767
	// build a list of all failed tests to include
	tests := []jiraTestFailure{}
	for _, test := range j.data.Task.LocalTestResults {
		if test.Status == evergreen.TestFailedStatus {
			env := evergreen.GetEnvironment()
			url := test.GetLogURL(env, evergreen.LogViewerParsley)
			if url == "" {
				url = test.GetLogURL(env, evergreen.LogViewerHTML)
			}

			tests = append(tests, jiraTestFailure{
				Name:          cleanTestName(test.GetDisplayTestName()),
				URL:           url,
				HistoryURL:    historyURL(j.data.Task, cleanTestName(test.TestName), j.data.UIRoot),
				TaskID:        test.TaskID,
				Execution:     test.Execution,
				DisplayTaskId: utility.FromStringPtr(j.data.Task.DisplayTaskId),
			})
		}
	}

	buf := &bytes.Buffer{}
	j.data.Tests = tests

	if err := descriptionTemplate.Execute(buf, &j.data); err != nil {
		return "", err
	}

	// Jira description length maximum
	if buf.Len() > jiraMaxDescLength {
		buf.Truncate(jiraMaxDescLength)
	}
	return buf.String(), nil
}

// cleanTestName returns the last item of a test's path.
// TODO: stop accommodating this.
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
