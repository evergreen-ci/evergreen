package data

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/evergreen-ci/evergreen/model/task"
	testselection "github.com/evergreen-ci/test-selection-client"
)

// jiraAssignmentTagPrefix identifies task tags that name the Jira team
// responsible for the task.
const jiraAssignmentTagPrefix = "assigned_to_jira_team_"

// QuarantineMetadata describes who changed a test's manual quarantine state,
// where they did it from, and why. Empty fields are omitted from requests to
// the test selection service.
type QuarantineMetadata struct {
	InitiatedBy               string
	TaskInitiatedFrom         string
	JiraAssignTo              string
	ExistingJiraTicketsToLink []string
	Justification             string
}

// NewTaskQuarantineMetadata returns the metadata for a quarantine change that
// username initiated from task t. uiBaseURL is the base URL of the UI that
// links to the task.
func NewTaskQuarantineMetadata(t *task.Task, username, uiBaseURL string) QuarantineMetadata {
	return QuarantineMetadata{
		InitiatedBy:       username,
		TaskInitiatedFrom: fmt.Sprintf("%s/task/%s/%d", uiBaseURL, url.PathEscape(t.Id), t.Execution),
		JiraAssignTo:      findJiraAssignmentTag(t.Tags),
	}
}

// findJiraAssignmentTag returns the first tag that assigns the task to a Jira
// team, or an empty string if there is none.
func findJiraAssignmentTag(tags []string) string {
	for _, tag := range tags {
		if strings.HasPrefix(tag, jiraAssignmentTagPrefix) {
			return tag
		}
	}
	return ""
}

func (m QuarantineMetadata) toClient() testselection.QuarantineMetadata {
	clientMetadata := testselection.NewQuarantineMetadata()
	if m.InitiatedBy != "" {
		clientMetadata.SetInitiatedBy(m.InitiatedBy)
	}
	if m.TaskInitiatedFrom != "" {
		clientMetadata.SetTaskInitiatedFrom(m.TaskInitiatedFrom)
	}
	if m.JiraAssignTo != "" {
		clientMetadata.SetJiraAssignTo(m.JiraAssignTo)
	}
	if len(m.ExistingJiraTicketsToLink) > 0 {
		clientMetadata.SetJiraTicketsToLinkTo(m.ExistingJiraTicketsToLink)
	}
	if m.Justification != "" {
		clientMetadata.SetJustification(m.Justification)
	}
	return *clientMetadata
}
