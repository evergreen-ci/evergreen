package thirdparty

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"

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

// JiraTickets marshal to and unmarshal from the json issue
// returned by the rest api at /rest/api/latest/issue/{ticket_id}
type JiraTicket struct {
	Key    string        `json:"key"`
	Expand string        `json:"expand"`
	Fields *TicketFields `json:"fields"`
}

// JiraSearchResults marshal to and unmarshal from the json
// search results returned by the rest api at /rest/api/2/search?jql={jql}
type JiraSearchResults struct {
	Expand     string       `json:"expand"`
	StartAt    int          `json:"startAt"`
	MaxResults int          `json:"maxResults"`
	Total      int          `json:"total"`
	Issues     []JiraTicket `json:"issues"`
}

type TicketFields struct {
	IssueType    *TicketType        `json:"issuetype"`
	Summary      string             `json:"summary"`
	Description  string             `json:"description"`
	Reporter     *User              `json:"reporter"`
	Assignee     *User              `json:"assignee"`
	Project      *JiraProject       `json:"project"`
	Resolution   *JiraResolution    `json:"resolution"`
	Created      string             `json:"created"`
	Updated      string             `json:"updated"`
	Status       *JiraStatus        `json:"status"`
	AssignedTeam []*JiraCustomField `json:"customfield_12751"`
	FailingTasks []string           `json:"customfield_12950"`
}

// JiraCreateTicketResponse contains the results of a JIRA create ticket API call.
type JiraCreateTicketResponse struct {
	Id   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

type JiraCustomField struct {
	Id    string `json:"id"`
	Value string `json:"value"`
	Self  string `json:"self"`
}

type JiraStatus struct {
	Id   string `json:"id"`
	Self string `json:"self"`
	Name string `json:"name"`
}

type JiraResolution struct {
	Id          string `json:"id"`
	Self        string `json:"self"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TicketType struct {
	Id          string `json:"id"`
	Self        string `json:"self"`
	Description string `json:"description"`
	IconUrl     string `json:"iconUrl"`
	Name        string `json:"name"`
	Subtask     bool   `json:"subtask"`
}

type JiraProject struct {
	Id         string            `json:"id"`
	Self       string            `json:"self"`
	Key        string            `json:"key"`
	Name       string            `json:"name"`
	AvatarUrls map[string]string `json:"avatarUrls"`
}

type User struct {
	Id           string            `json:"id"`
	Self         string            `json:"self"`
	Name         string            `json:"name"`
	EmailAddress string            `json:"emailAddress"`
	DisplayName  string            `json:"displayName"`
	Active       bool              `json:"active"`
	TimeZone     string            `json:"timeZone"`
	AvatarUrls   map[string]string `json:"avatarUrls"`
}

type JiraHandler struct {
	client *http.Client
	opts   send.JiraOptions
}

// JiraHost returns the hostname of the jira service as configured.
func (jiraHandler *JiraHandler) JiraHost() string { return jiraHandler.opts.BaseURL }

// CreateTicket takes a map of fields to initialize a JIRA ticket with. Returns a response containing the
// new ticket's key, id, and API URL. See the JIRA API documentation for help.
func (jiraHandler *JiraHandler) CreateTicket(fields map[string]any) (*JiraCreateTicketResponse, error) {
	postArgs := struct {
		Fields map[string]any `json:"fields"`
	}{fields}
	apiEndpoint := fmt.Sprintf("%s/rest/api/2/issue", jiraHandler.JiraHost())
	body := &bytes.Buffer{}
	if err := json.NewEncoder(body).Encode(postArgs); err != nil {
		return nil, errors.Wrap(err, "unable to serialize ticket body")
	}
	req, err := http.NewRequest(http.MethodPost, apiEndpoint, body)
	if err != nil {
		return nil, errors.Wrap(err, "unable to form create ticket request")
	}
	req.Header.Add("Content-Type", "application/json")
	res, err := jiraHandler.client.Do(req)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if res != nil && (res.StatusCode >= 300 || res.StatusCode < 200) {
		msg, _ := io.ReadAll(res.Body)
		return nil, errors.Errorf("HTTP request returned unexpected status `%v`: %v", res.Status, string(msg))
	}

	ticketInfo := &JiraCreateTicketResponse{}
	if err := json.NewDecoder(res.Body).Decode(ticketInfo); err != nil {
		return nil, errors.Wrap(err, "Unable to decode http body")
	}
	return ticketInfo, nil
}

// UpdateTicket sets the given fields of the ticket with the given key. Returns any errors JIRA returns.
func (jiraHandler *JiraHandler) UpdateTicket(key string, fields map[string]any) error {
	apiEndpoint := fmt.Sprintf("%s/rest/api/2/issue/%v", jiraHandler.JiraHost(), url.QueryEscape(key))
	putArgs := struct {
		Fields map[string]any `json:"fields"`
	}{fields}
	body := &bytes.Buffer{}
	if err := json.NewEncoder(body).Encode(putArgs); err != nil {
		return errors.Wrap(err, "unable to serialize ticket body")
	}
	req, err := http.NewRequest(http.MethodPut, apiEndpoint, body)
	if err != nil {
		return errors.Wrap(err, "unable to form update ticket request")
	}
	req.Header.Add("Content-Type", "application/json")
	res, err := jiraHandler.client.Do(req)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return errors.WithStack(err)
	}
	if res != nil && (res.StatusCode >= 300 || res.StatusCode < 200) {
		msg, _ := io.ReadAll(res.Body)
		return errors.Errorf("HTTP request returned unexpected status `%v`: %v", res.Status, string(msg))
	}

	return nil
}

// GetJIRATicket returns the ticket with the given key.
func (jiraHandler *JiraHandler) GetJIRATicket(key string) (*JiraTicket, error) {
	apiEndpoint := fmt.Sprintf("%s/rest/api/latest/issue/%v", jiraHandler.JiraHost(), url.QueryEscape(key))
	req, err := http.NewRequest(http.MethodGet, apiEndpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to form get ticket request")
	}
	res, err := jiraHandler.client.Do(req)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, errors.WithStack(err)
	}

	if res == nil {
		return nil, errors.Errorf("HTTP results are nil even though err was nil")
	}

	if res.StatusCode >= 300 || res.StatusCode < 200 {
		return nil, errors.Errorf("HTTP request returned unexpected status `%v`", res.Status)
	}

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to read http body")
	}

	ticket := &JiraTicket{}
	err = json.Unmarshal(body, ticket)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return ticket, nil
}

// JQLSearch runs the given JQL query against the given jira instance and returns
// the results in a JiraSearchResults
func (jiraHandler *JiraHandler) JQLSearch(query string, startAt, maxResults int) (*JiraSearchResults, error) {
	// kim: NOTE: this is the main Jira glue logic to execute the JQL search to
	// populate task annotations UI.
	// kim: NOTE: the special characters (which were already escaped for Jira
	// rules) are then query-escaped here. I would imagine that would be
	// sufficient to fix any escaping issues.
	// kim: TODO: test out this URL with Postman. See what happens special
	// characters + query escaping.
	apiEndpoint := fmt.Sprintf("%s/rest/api/latest/search?jql=%v&startAt=%d&maxResults=%d", jiraHandler.JiraHost(), url.QueryEscape(query), startAt, maxResults)
	req, err := http.NewRequest(http.MethodGet, apiEndpoint, nil)
	if err != nil {
		return nil, errors.Wrap(err, "unable to form JQL request")
	}
	res, err := jiraHandler.client.Do(req)
	if err != nil {
		return nil, errors.WithStack(err)
	}
	if res == nil {
		return nil, errors.Errorf("HTTP results are nil even though err was not nil")
	}

	if res.StatusCode >= 300 || res.StatusCode < 200 {
		return nil, errors.Errorf("HTTP request returned unexpected status `%v`", res.Status)
	}

	defer res.Body.Close()

	body, err := io.ReadAll(res.Body)
	if err != nil {
		return nil, errors.Wrap(err, "Unable to read http body")
	}

	results := &JiraSearchResults{}
	err = json.Unmarshal(body, results)
	if err != nil {
		return nil, errors.WithStack(err)
	}

	return results, nil
}

// JQLSearchAll performs repeated JQL searches until the query has been exhausted
func (jiraHandler *JiraHandler) JQLSearchAll(query string) ([]JiraTicket, error) {
	allIssues := []JiraTicket{}

	index := 0
	ticketsLeft := math.MaxInt32

	for ticketsLeft > 0 {
		nextResult, err := jiraHandler.JQLSearch(query, index, -1)
		if err != nil {
			return []JiraTicket{}, errors.WithStack(err)
		}

		numReturned := nextResult.MaxResults
		ticketsLeft = nextResult.Total - (index + numReturned)
		index = numReturned + index + 1

		allIssues = append(allIssues, nextResult.Issues...)
	}

	return allIssues, nil

}

func (jiraHandler *JiraHandler) HttpClient() *http.Client {
	return jiraHandler.client
}

func NewJiraHandler(opts send.JiraOptions) JiraHandler {
	httpClient := utility.GetHTTPClient()
	if opts.PersonalAccessTokenOpts.Token != "" {
		transport := jira.BearerAuthTransport{
			Token:     opts.PersonalAccessTokenOpts.Token,
			Transport: httpClient.Transport,
		}
		httpClient = transport.Client()
	}
	return JiraHandler{
		opts:   opts,
		client: httpClient,
	}
}
