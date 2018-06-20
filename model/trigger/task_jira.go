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
	"github.com/evergreen-ci/evergreen/util"
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
	jiraFailingTasksField     = "customfield_12950"
	jiraFailingTestsField     = "customfield_15756"
	jiraFailingVariantField   = "customfield_14277"
	jiraEvergreenProjectField = "customfield_14278"
	jiraFailingRevisionField  = "customfield_14851"
	jiraMaxTitleLength        = 254
)

// supportedJiraProjects are all of the projects, by name that we
// expect to be compatible with the custom fields above.
var supportedJiraProjects = []string{"BFG", "BF", "EVG", "MAKE", "BUILD"}

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
	uiRoot    string

	Task        *task.Task
	Build       *build.Build
	Host        *host.Host
	ProjectRef  *model.ProjectRef
	Version     *version.Version
	FailedTests []task.TestResult
}

// isXgenProjBF is a gross function to figure out if the jira instance
// and project are correctly configured for the specified kind of
// requests/issue metadata.
func isXgenProjBF(host, project string) bool {
	if !strings.Contains(host, "mongodb") {
		return false
	}

	return util.StringSliceContains(supportedJiraProjects, project)
}

type AlertContext struct{}

// Deliver posts the alert defined by the AlertContext to JIRA.
func (j *jiraBuilder) build() (*message.JiraIssue, error) {
	description, err := j.getDescription()
	if err != nil {
		return nil, errors.Wrap(err, "error creating description")
	}
	issue := message.JiraIssue{
		Project:     j.project,
		Type:        j.issueType,
		Summary:     j.getSummary(),
		Description: description,
	}

	if isXgenProjBF("mongodb", j.project) { // TODO
		failedTests := []string{}
		for _, t := range j.FailedTests {
			failedTests = append(failedTests, t.TestFile)
		}
		issue.Fields = map[string]interface{}{}
		issue.Fields[jiraFailingTasksField] = []string{j.Task.DisplayName}
		issue.Fields[jiraFailingTestsField] = failedTests
		issue.Fields[jiraFailingVariantField] = []string{j.Task.BuildVariant}
		issue.Fields[jiraEvergreenProjectField] = []string{j.ProjectRef.Identifier}
		issue.Fields[jiraFailingRevisionField] = []string{j.Task.Revision}
	}

	if err != nil {
		return nil, errors.Wrap(err, "error creating description")
	}
	grip.Info(message.Fields{
		"message":      "creating jira ticket for failure",
		"type":         j.issueType,
		"jira_project": j.project,
		"task":         j.Task.Id,
		"project":      j.ProjectRef.Identifier,
	})

	event.LogJiraIssueCreated(j.Task.Id, j.Task.Execution, j.project)

	return &issue, nil
}

// getSummary creates a JIRA subject for a task failure in the style of
//  Failures: Task_name on Variant (test1, test2) [ProjectName @ githash]
// based on the given AlertContext.
func (j *jiraBuilder) getSummary() string {
	subj := &bytes.Buffer{}
	failed := []string{}

	for _, test := range j.Task.LocalTestResults {
		if test.Status == evergreen.TestFailedStatus {
			failed = append(failed, cleanTestName(test.TestFile))
		}
	}

	switch {
	case j.Task.Details.TimedOut:
		subj.WriteString("Timed Out: ")
	case j.Task.Details.Type == model.SystemCommandType:
		subj.WriteString("System Failure: ")
	case j.Task.Details.Type == model.SetupCommandType:
		subj.WriteString("Setup Failure: ")
	case len(failed) == 1:
		subj.WriteString("Failure: ")
	case len(failed) > 1:
		subj.WriteString("Failures: ")
	default:
		subj.WriteString("Failed: ")
	}

	fmt.Fprintf(subj, "%s on %s ", j.Task.DisplayName, j.Build.DisplayName)
	fmt.Fprintf(subj, "[%s @ %s] ", j.ProjectRef.DisplayName, j.Version.Revision[0:8])

	if len(failed) > 0 {
		// Include an additional 10 characters for overhead, like the
		// parens and number of failures.
		remaining := jiraMaxTitleLength - subj.Len() - 10

		if remaining < len(failed[0]) {
			return subj.String()
		}
		subj.WriteString("(")
		toPrint := []string{}
		for _, fail := range failed {
			if remaining-len(fail) > 0 {
				toPrint = append(toPrint, fail)
			}
			remaining = remaining - len(fail) - 2
		}
		fmt.Fprint(subj, strings.Join(toPrint, ", "))
		if len(failed)-len(toPrint) > 0 {
			fmt.Fprintf(subj, " +%d more", len(failed)-len(toPrint))
		}
		subj.WriteString(")")
	}
	// Truncate string in case we made some mistake above, since it's better
	// to have a truncated title than to miss a Jira ticket.
	if subj.Len() > jiraMaxTitleLength {
		return subj.String()[:jiraMaxTitleLength]
	}
	return subj.String()
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
	for _, test := range j.Task.LocalTestResults {
		if test.Status == evergreen.TestFailedStatus {
			tests = append(tests, jiraTestFailure{
				Name:       cleanTestName(test.TestFile),
				URL:        logURL(test, j.uiRoot),
				HistoryURL: historyURL(j.Task, cleanTestName(test.TestFile), j.uiRoot),
			})
		}
	}

	args := struct {
		Task    *task.Task
		Build   *build.Build
		Host    *host.Host
		Project *model.ProjectRef
		Version *version.Version
		Tests   []jiraTestFailure
		UIRoot  string
	}{j.Task, j.Build, j.Host, j.ProjectRef, j.Version, tests, j.uiRoot}
	buf := &bytes.Buffer{}
	if err := descriptionTemplate.Execute(buf, args); err != nil {
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

func (j *taskTriggers) makeJIRATaskPayload(project string) (*message.JiraIssue, error) {
	buildDoc, err := build.FindOne(build.ById(j.task.BuildId))
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch build while building jira task payload")
	}

	hostDoc, err := host.FindOneId(j.task.HostId)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch host while building jira task payload")
	}

	versionDoc, err := version.FindOneId(j.task.Version)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch version while building jira task payload")
	}

	projectRef, err := model.FindOneProjectRef(j.task.Project)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch project ref while building jira task payload")
	}

	b := jiraBuilder{
		project:    project,
		uiRoot:     j.uiConfig.Url,
		Task:       j.task,
		Version:    versionDoc,
		ProjectRef: projectRef,
		Build:      buildDoc,
		Host:       hostDoc,
	}
	for i := range j.task.LocalTestResults {
		if j.task.LocalTestResults[i].Status == evergreen.TestFailedStatus {
			b.FailedTests = append(b.FailedTests, j.task.LocalTestResults[i])
		}
	}

	return b.build()
}
