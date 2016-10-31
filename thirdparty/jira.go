package thirdparty

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"math"
	"net/url"
)

// JiraTickets marshal to and unmarshal from the json issue
// returned by the rest api at /rest/api/latest/issue/{ticket_id}
type JiraTicket struct {
	jiraBase
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
	IssueType   *TicketType     `json:"issuetype"`
	Summary     string          `json:"summary"`
	Description string          `json:"description"`
	Reporter    *User           `json:"reporter"`
	Assignee    *User           `json:"assignee"`
	Project     *JiraProject    `json:"project"`
	Resolution  *JiraResolution `json:"resolution"`
	Created     string          `json:"created"`
	Updated     string          `json:"updated"`
	Status      *JiraStatus     `json:"status"`
}

// JiraCreateTicketResponse contains the results of a JIRA create ticket API call.
type JiraCreateTicketResponse struct {
	Id   string `json:"id"`
	Key  string `json:"key"`
	Self string `json:"self"`
}

type jiraBase struct {
	Id   string `json:"id"`
	Self string `json:"self"`
}

type JiraStatus struct {
	jiraBase
	Name string `json:"name"`
}

type JiraResolution struct {
	jiraBase
	Name        string `json:"name"`
	Description string `json:"description"`
}

type TicketType struct {
	jiraBase
	Description string `json:"description"`
	IconUrl     string `json:"iconUrl"`
	Name        string `json:"name"`
	Subtask     bool   `json:"subtask"`
}

type JiraProject struct {
	jiraBase
	Key        string            `json:"key"`
	Name       string            `json:"name"`
	AvatarUrls map[string]string `json:"avatarUrls"`
}

type User struct {
	Self         string            `json:"self"`
	Name         string            `json:"name"`
	EmailAddress string            `json:"emailAddress"`
	DisplayName  string            `json:"displayName"`
	Active       bool              `json:"active"`
	TimeZone     string            `json:"timeZone"`
	AvatarUrls   map[string]string `json:"avatarUrls"`
}

type JiraHandler struct {
	MyHttp     httpClient
	JiraServer string
	UserName   string
	Password   string
}

// CreateTicket takes a map of fields to initialize a JIRA ticket with. Returns a response containing the
// new ticket's key, id, and API URL. See the JIRA API documentation for help.
func (jiraHandler *JiraHandler) CreateTicket(fields map[string]interface{}) (*JiraCreateTicketResponse, error) {
	postArgs := struct {
		Fields map[string]interface{} `json:"fields"`
	}{fields}
	apiEndpoint := fmt.Sprintf("https://%v/rest/api/2/issue", jiraHandler.JiraServer)
	res, err := jiraHandler.MyHttp.doPost(apiEndpoint, jiraHandler.UserName, jiraHandler.Password, postArgs)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, err
	}
	if res.StatusCode >= 300 || res.StatusCode < 200 {
		msg, _ := ioutil.ReadAll(res.Body)
		return nil, fmt.Errorf("HTTP request returned unexpected status `%v`: %v", res.Status, string(msg))
	}

	ticketInfo := &JiraCreateTicketResponse{}
	if err := json.NewDecoder(res.Body).Decode(ticketInfo); err != nil {
		return nil, fmt.Errorf("Unable to decode http body: %v", err.Error())
	}
	return ticketInfo, nil
}

// UpdateTicket sets the given fields of the ticket with the given key. Returns any errors JIRA returns.
func (jiraHandler *JiraHandler) UpdateTicket(key string, fields map[string]interface{}) error {
	apiEndpoint := fmt.Sprintf("https://%v/rest/api/2/issue/%v", jiraHandler.JiraServer, url.QueryEscape(key))
	putArgs := struct {
		Fields map[string]interface{} `json:"fields"`
	}{fields}
	res, err := jiraHandler.MyHttp.doPut(apiEndpoint, jiraHandler.UserName, jiraHandler.Password, putArgs)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return err
	}
	if res.StatusCode >= 300 || res.StatusCode < 200 {
		msg, _ := ioutil.ReadAll(res.Body)
		return fmt.Errorf("HTTP request returned unexpected status `%v`: %v", res.Status, string(msg))
	}

	return nil
}

// GetJIRATicket returns the ticket with the given key.
func (jiraHandler *JiraHandler) GetJIRATicket(key string) (*JiraTicket, error) {
	apiEndpoint := fmt.Sprintf("https://%v/rest/api/latest/issue/%v", jiraHandler.JiraServer, url.QueryEscape(key))

	res, err := jiraHandler.MyHttp.doGet(apiEndpoint, jiraHandler.UserName, jiraHandler.Password)
	if res != nil {
		defer res.Body.Close()
	}
	if err != nil {
		return nil, err
	}

	if res == nil {
		return nil, fmt.Errorf("HTTP results are nil even though err was nil")
	}

	if res.StatusCode >= 300 || res.StatusCode < 200 {
		return nil, fmt.Errorf("HTTP request returned unexpected status `%v`", res.Status)
	}

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Unable to read http body: %v", err.Error())
	}

	ticket := &JiraTicket{}
	err = json.Unmarshal(body, ticket)
	if err != nil {
		return nil, err
	}

	return ticket, nil
}

// JQLSearch runs the given JQL query against the given jira instance and returns
// the results in a JiraSearchResults
func (jiraHandler *JiraHandler) JQLSearch(query string, startAt, maxResults int) (*JiraSearchResults, error) {
	apiEndpoint := fmt.Sprintf("https://%v/rest/api/latest/search?jql=%v&startAt=%d&maxResults=%d", jiraHandler.JiraServer, url.QueryEscape(query), startAt, maxResults)

	res, err := jiraHandler.MyHttp.doGet(apiEndpoint, jiraHandler.UserName, jiraHandler.Password)
	if err != nil {
		return nil, err
	}
	if res == nil {
		return nil, fmt.Errorf("HTTP results are nil even though err was not nil")
	}

	if res.StatusCode >= 300 || res.StatusCode < 200 {
		return nil, fmt.Errorf("HTTP request returned unexpected status `%v`", res.Status)
	}

	defer res.Body.Close()

	body, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, fmt.Errorf("Unable to read http body: %v", err.Error())
	}

	results := &JiraSearchResults{}
	err = json.Unmarshal(body, results)
	if err != nil {
		return nil, err
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
			return []JiraTicket{}, err
		}

		numReturned := nextResult.MaxResults
		ticketsLeft = nextResult.Total - (index + numReturned)
		index = numReturned + index + 1

		allIssues = append(allIssues, nextResult.Issues...)
	}

	return allIssues, nil

}

func NewJiraHandler(server string, user string, password string) JiraHandler {
	return JiraHandler{
		liveHttp{},
		server,
		user,
		password,
	}
}
