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

func (j *jiraClientMock) PostIssue(_ *jira.IssueFields) error {
	if j.failSend {
		return errors.New("mock failed to post issue")
	}

	j.numSent++

	return nil
}

func (j *jiraClientMock) PostComment(issueID string, comment string) error {
	if j.failSend {
		return errors.New("mock failed to post comment")
	}

	j.numSent++

	return nil
}
