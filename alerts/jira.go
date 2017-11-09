package alerts

import (
	"bytes"
	"fmt"
	"strings"
	"text/template"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model"
	"github.com/evergreen-ci/evergreen/model/build"
	"github.com/evergreen-ci/evergreen/model/host"
	"github.com/evergreen-ci/evergreen/model/task"
	"github.com/evergreen-ci/evergreen/model/version"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/evergreen/util"
	"github.com/mongodb/grip"
	"github.com/pkg/errors"
)

// DescriptionTemplateString defines the content of the alert ticket.
const DescriptionTemplateString = `
h2. [{{.Task.DisplayName}} failed on {{.Build.DisplayName}}|{{.UIRoot}}/task/{{.Task.Id}}/{{.Task.Execution}}]
Host: [{{.Host.Host}}|{{.UIRoot}}/host/{{.Host.Id}}]
Project: [{{.Project.DisplayName}}|{{.UIRoot}}/waterfall/{{.Project.Identifier}}]
Commit: [diff|https://github.com/{{.Project.Owner}}/{{.Project.Repo}}/commit/{{.Version.Revision}}]: {{.Version.Message}}
{{range .Tests}}*{{.Name}}* - [Logs|{{.URL}}] | [History|{{.HistoryURL}}]
{{end}}
`
const (
	jiraFailingTasksField     = "customfield_12950"
	jiraFailingVariantField   = "customfield_14277"
	jiraEvergreenProjectField = "customfield_14278"
	jiraFailingRevisionField  = "customfield_14851"
)

// supportedJiraProjects are all of the projects, by name that we
// expect to be compatible with the custom fields above.
var supportedJiraProjects = []string{"BFG", "BF", "EVG", "MAKE", "BUILD"}

// DescriptionTemplate is filled to create a JIRA alert ticket. Panics at start if invalid.
var DescriptionTemplate = template.Must(template.New("Desc").Parse(DescriptionTemplateString))

// jiraTestFailure contains the required fields for generating a failure report.
type jiraTestFailure struct {
	Name       string
	URL        string
	HistoryURL string
}

// jiraCreator is an interface for types that can create JIRA tickets.
type jiraCreator interface {
	CreateTicket(fields map[string]interface{}) (*thirdparty.JiraCreateTicketResponse, error)
	JiraHost() string
}

// jiraDeliverer is an implementation of Deliverer that files JIRA tickets
type jiraDeliverer struct {
	project   string
	issueType string
	uiRoot    string
	handler   jiraCreator
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

// Deliver posts the alert defined by the AlertContext to JIRA.
func (jd *jiraDeliverer) Deliver(ctx AlertContext, alertConf model.AlertConfig) error {
	var err error
	request := map[string]interface{}{}
	request["project"] = map[string]string{"key": jd.project}
	request["issuetype"] = map[string]string{"name": jd.issueType}
	request["summary"] = getSummary(ctx)
	request["description"], err = getDescription(ctx, jd.uiRoot)

	if isXgenProjBF(jd.handler.JiraHost(), jd.project) {
		request[jiraFailingTasksField] = []string{ctx.Task.DisplayName}
		request[jiraFailingVariantField] = []string{ctx.Task.BuildVariant}
		request[jiraEvergreenProjectField] = []string{ctx.ProjectRef.Identifier}
		request[jiraFailingRevisionField] = []string{ctx.Task.Revision}
	}

	if err != nil {
		return errors.Wrap(err, "error creating description")
	}
	grip.Infof("Creating '%v' JIRA ticket in %v for failure %v in project %s",
		jd.issueType, jd.project, ctx.Task.Id, ctx.ProjectRef.Identifier)
	result, err := jd.handler.CreateTicket(request)
	if err != nil {
		return errors.Wrap(err, "error creating JIRA ticket")
	}
	grip.Infof("Created JIRA ticket %v successfully", result.Key)
	return nil
}

// getSummary creates a JIRA subject for a task failure in the style of
//  Failures: Task_name on Variant (test1, test2) [ProjectName @ githash]
// based on the given AlertContext.
func getSummary(ctx AlertContext) string {
	subj := &bytes.Buffer{}
	failed := []string{}

	for _, test := range ctx.Task.TestResults {
		if test.Status == evergreen.TestFailedStatus {
			failed = append(failed, cleanTestName(test.TestFile))
		}
	}

	switch {
	case ctx.Task.Details.TimedOut:
		subj.WriteString("Timed Out: ")
	case len(failed) == 1:
		subj.WriteString("Failure: ")
	case len(failed) > 1:
		subj.WriteString("Failures: ")
	case ctx.Task.Details.Description == task.AgentHeartbeat:
		subj.WriteString("System Failure: ")
	case ctx.Task.Details.Type == model.SystemCommandType:
		subj.WriteString("System Failure: ")
	default:
		subj.WriteString("Failed: ")
	}

	fmt.Fprintf(subj, "%s on %s ", ctx.Task.DisplayName, ctx.Build.DisplayName)

	// include test names if <= 4 failed, otherwise print two plus the number remaining
	if len(failed) > 0 {
		subj.WriteString("(")
		if len(failed) <= 4 {
			subj.WriteString(strings.Join(failed, ", "))
		} else {
			fmt.Fprintf(subj, "%s, %s, +%v more", failed[0], failed[1], len(failed)-2)
		}
		subj.WriteString(") ")
	}

	fmt.Fprintf(subj, "[%s @ %s]", ctx.ProjectRef.DisplayName, ctx.Version.Revision[0:8])
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
func getDescription(ctx AlertContext, uiRoot string) (string, error) {
	// build a list of all failed tests to include
	tests := []jiraTestFailure{}
	for _, test := range ctx.Task.TestResults {
		if test.Status == evergreen.TestFailedStatus {
			tests = append(tests, jiraTestFailure{
				Name:       cleanTestName(test.TestFile),
				URL:        logURL(test, uiRoot),
				HistoryURL: historyURL(ctx.Task, cleanTestName(test.TestFile), uiRoot),
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
	}{ctx.Task, ctx.Build, ctx.Host, ctx.ProjectRef, ctx.Version, tests, uiRoot}
	buf := &bytes.Buffer{}
	if err := DescriptionTemplate.Execute(buf, args); err != nil {
		return "", err
	}
	return buf.String(), nil
}
