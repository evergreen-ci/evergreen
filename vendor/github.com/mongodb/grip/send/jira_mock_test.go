package send

import (
	"errors"
	"net/http"

	jira "github.com/andygrunwald/go-jira"
)

type jiraClientMock struct {
	failCreate bool
	failAuth   bool
	failSend   bool
	numSent    int

	lastIssue   string
	lastSummary string
	lastFields  *jira.IssueFields
}

func (j *jiraClientMock) CreateClient(_ *http.Client, _ string) error {
	if j.failCreate {
		return errors.New("mock failed to create client")
	}
	return nil
}

func (j *jiraClientMock) Authenticate(_ string, _ string) error {
	if j.failAuth {
		return errors.New("mock failed authentication")
	}
	return nil
}

func (j *jiraClientMock) PostIssue(fields *jira.IssueFields) error {
	if j.failSend {
		return errors.New("mock failed to post issue")
	}

	j.numSent++
	j.lastSummary = fields.Summary
	j.lastFields = fields

	return nil
}

func (j *jiraClientMock) PostComment(issueID string, comment string) error {
	if j.failSend {
		return errors.New("mock failed to post comment")
	}

	j.numSent++
	j.lastIssue = issueID

	return nil
}
