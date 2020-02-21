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

	lastIssue       string
	lastSummary     string
	lastDescription string
	lastFields      *jira.IssueFields
	issueKey        string
}

func (j *jiraClientMock) CreateClient(_ *http.Client, _ string) error {
	if j.failCreate {
		return errors.New("mock failed to create client")
	}
	return nil
}

func (j *jiraClientMock) Authenticate(_ string, _ string, _ bool) error {
	if j.failAuth {
		return errors.New("mock failed authentication")
	}
	return nil
}

func (j *jiraClientMock) PostIssue(fields *jira.IssueFields) (string, error) {
	if j.failSend {
		return "", errors.New("mock failed to post issue")
	}

	j.numSent++
	j.lastSummary = fields.Summary
	j.lastDescription = fields.Description
	j.lastFields = fields
	j.issueKey = "ABC-123"

	return j.issueKey, nil
}

func (j *jiraClientMock) PostComment(issueID string, comment string) error {
	if j.failSend {
		return errors.New("mock failed to post comment")
	}

	j.numSent++
	j.lastIssue = issueID

	return nil
}
