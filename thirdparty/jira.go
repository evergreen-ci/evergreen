package thirdparty

import (
	"context"
	"encoding/json"

	"github.com/andygrunwald/go-jira"
	"github.com/evergreen-ci/utility"
	"github.com/mongodb/grip/send"
	"github.com/pkg/errors"
)

type SearchReturnInfo struct {
	Issues      []JiraTicket `json:"issues"`
	Search      string       `json:"search"`
	Source      string       `json:"source"`
	FeaturesURL string       `json:"features_url"`
}

// JiraTicket marshals to and unmarshals from the json issue
// returned by the rest api at /rest/api/latest/issue/{ticket_id}
type JiraTicket struct {
	Key    string        `json:"key"`
	Fields *TicketFields `json:"fields"`
}

// JiraSearchResults marshal to and unmarshal from the json
// search results returned by the rest api at /rest/api/2/search?jql={jql}
type JiraSearchResults struct {
	StartAt    int          `json:"startAt"`
	MaxResults int          `json:"maxResults"`
	Total      int          `json:"total"`
	Issues     []JiraTicket `json:"issues"`
}

type TicketFields struct {
	Summary      string             `json:"summary"`
	Assignee     *User              `json:"assignee"`
	Resolution   *JiraResolution    `json:"resolution"`
	Created      string             `json:"created"`
	Updated      string             `json:"updated"`
	Status       *JiraStatus        `json:"status"`
	AssignedTeam []*JiraCustomField `json:"customfield_12751"`
	FailingTasks []string           `json:"customfield_12950"`
}

type JiraCustomField struct {
	Value string `json:"value"`
}

type JiraStatus struct {
	Id   string `json:"id"`
	Name string `json:"name"`
}

type JiraResolution struct {
	Name string `json:"name"`
}

type User struct {
	DisplayName string `json:"displayName"`
}

type JiraHandler struct {
	client *jira.Client
}

// GetIssue returns the ticket with the given key using the go-jira client.
func (jiraHandler *JiraHandler) GetIssue(ctx context.Context, key string) (*JiraTicket, error) {
	if jiraHandler.client == nil {
		return nil, errors.New("jira client is not initialized")
	}
	issue, _, err := jiraHandler.client.Issue.GetWithContext(ctx, key, nil)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return jiraIssueToJiraTicket(issue)
}

// JQLSearch runs the given JQL query against the given jira instance and returns
// the results in a JiraSearchResults.
func (jiraHandler *JiraHandler) JQLSearch(ctx context.Context, query string, startAt, maxResults int) (*JiraSearchResults, error) {
	if jiraHandler.client == nil {
		return nil, errors.New("jira client is not initialized")
	}
	opts := &jira.SearchOptions{StartAt: startAt}
	if maxResults > 0 {
		opts.MaxResults = maxResults
	}
	issues, resp, err := jiraHandler.client.Issue.SearchWithContext(ctx, query, opts)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	out := &JiraSearchResults{}
	if resp != nil {
		out.StartAt = resp.StartAt
		out.MaxResults = resp.MaxResults
		out.Total = resp.Total
	}
	for i := range issues {
		t, err := jiraIssueToJiraTicket(&issues[i])
		if err != nil {
			return nil, err
		}
		if t != nil {
			out.Issues = append(out.Issues, *t)
		}
	}
	return out, nil
}

func NewJiraHandler(opts send.JiraOptions) (JiraHandler, error) {
	httpClient := utility.GetHTTPClient()
	if opts.PersonalAccessTokenOpts.Token != "" {
		transport := jira.BearerAuthTransport{
			Token:     opts.PersonalAccessTokenOpts.Token,
			Transport: httpClient.Transport,
		}
		httpClient = transport.Client()
	}
	jiraClient, err := jira.NewClient(httpClient, opts.BaseURL)
	if err != nil {
		return JiraHandler{}, errors.Wrap(err, "creating jira client")
	}
	return JiraHandler{client: jiraClient}, nil
}

// jiraIssueToJiraTicket converts a go-jira Issue into our API shape. go-jira's IssueFields.MarshalJSON
// merges Unknowns (custom fields) into the same JSON the Jira REST API returns, so we rely on that
// instead of hand-mapping fields.
func jiraIssueToJiraTicket(issue *jira.Issue) (*JiraTicket, error) {
	if issue == nil {
		return nil, nil
	}
	data, err := json.Marshal(issue)
	if err != nil {
		return nil, errors.Wrap(err, "marshaling jira issue")
	}
	var ticket JiraTicket
	if err := json.Unmarshal(data, &ticket); err != nil {
		return nil, errors.Wrap(err, "unmarshaling Jira ticket")
	}
	return &ticket, nil
}
