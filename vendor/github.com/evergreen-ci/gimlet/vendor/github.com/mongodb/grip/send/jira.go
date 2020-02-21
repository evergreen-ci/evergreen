package send

import (
	"context"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"strings"

	jira "github.com/andygrunwald/go-jira"
	"github.com/mongodb/grip/level"
	"github.com/mongodb/grip/message"
	"github.com/trivago/tgo/tcontainer"
)

// jiraIssueKey is the key in a message.Fields that will hold the ID of the issue created
const jiraIssueKey = "jira-key"

type jiraJournal struct {
	opts *JiraOptions
	*Base
}

// JiraOptions include configurations for the JIRA client
type JiraOptions struct {
	Name         string // Name of the journaler
	BaseURL      string // URL of the JIRA instance
	Username     string
	Password     string
	UseBasicAuth bool

	HTTPClient *http.Client
	client     jiraClient
}

// MakeJiraLogger is the same as NewJiraLogger but uses a warning
// level of Trace
func MakeJiraLogger(opts *JiraOptions) (Sender, error) {
	return NewJiraLogger(opts, LevelInfo{level.Trace, level.Trace})
}

// NewJiraLogger constructs a Sender that creates issues to jira, given
// options defined in a JiraOptions struct.
func NewJiraLogger(opts *JiraOptions, l LevelInfo) (Sender, error) {
	if err := opts.Validate(); err != nil {
		return nil, err
	}

	j := &jiraJournal{
		opts: opts,
		Base: NewBase(opts.Name),
	}

	if err := j.opts.client.CreateClient(opts.HTTPClient, opts.BaseURL); err != nil {
		return nil, err
	}

	if err := j.opts.client.Authenticate(opts.Username, opts.Password, opts.UseBasicAuth); err != nil {
		return nil, fmt.Errorf("jira authentication error: %v", err)
	}

	if err := j.SetLevel(l); err != nil {
		return nil, err
	}

	fallback := log.New(os.Stdout, "", log.LstdFlags)
	if err := j.SetErrorHandler(ErrorHandlerFromLogger(fallback)); err != nil {
		return nil, err
	}

	j.reset = func() {
		fallback.SetPrefix(fmt.Sprintf("[%s] ", j.Name()))
	}

	j.SetName(opts.Name)

	return j, nil
}

// Send post issues via jiraJournal with information in the message.Composer
func (j *jiraJournal) Send(m message.Composer) {
	if j.Level().ShouldLog(m) {
		issueFields := getFields(m)
		if len(issueFields.Summary) > 254 {
			issueFields.Summary = issueFields.Summary[:254]
		}
		if len(issueFields.Description) > 32767 {
			issueFields.Description = issueFields.Description[:32767]
		}

		if err := j.opts.client.Authenticate(j.opts.Username, j.opts.Password, j.opts.UseBasicAuth); err != nil {
			j.errHandler(fmt.Errorf("jira authentication error: %v", err), message.NewFormattedMessage(m.Priority(), m.String()))
		}
		issueKey, err := j.opts.client.PostIssue(issueFields)
		if err != nil {
			j.errHandler(err, message.NewFormattedMessage(m.Priority(), m.String()))
			return
		}
		populateKey(m, issueKey)
	}
}

func (j *jiraJournal) Flush(_ context.Context) error { return nil }

// Validate inspects the contents of JiraOptions struct and returns an error in case of
// missing any required fields.
func (o *JiraOptions) Validate() error {
	if o == nil {
		return errors.New("jira options cannot be nil")
	}

	errs := []string{}

	if o.Name == "" {
		errs = append(errs, "no name specified")
	}

	if o.BaseURL == "" {
		errs = append(errs, "no baseURL specified")
	}

	if o.Username == "" {
		errs = append(errs, "no username specified")
	}

	if o.Password == "" {
		errs = append(errs, "no password specified")
	}

	if o.client == nil {
		o.client = &jiraClientImpl{}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func getFields(m message.Composer) *jira.IssueFields {
	var issueFields *jira.IssueFields

	switch msg := m.Raw().(type) {
	case *message.JiraIssue:
		issueFields = &jira.IssueFields{
			Project:     jira.Project{Key: msg.Project},
			Summary:     msg.Summary,
			Description: msg.Description,
		}
		if len(msg.Fields) != 0 {
			unknownsMap := tcontainer.NewMarshalMap()
			for key, value := range msg.Fields {
				unknownsMap[key] = value
			}
			issueFields.Unknowns = unknownsMap
		}
		if msg.Reporter != "" {
			issueFields.Reporter = &jira.User{Name: msg.Reporter}
		}
		if msg.Assignee != "" {
			issueFields.Assignee = &jira.User{Name: msg.Assignee}
		}
		if msg.Type != "" {
			issueFields.Type = jira.IssueType{Name: msg.Type}
		}
		if len(msg.Labels) > 0 {
			issueFields.Labels = msg.Labels
		}
		if len(msg.Components) > 0 {
			issueFields.Components = make([]*jira.Component, 0, len(msg.Components))
			for _, component := range msg.Components {
				issueFields.Components = append(issueFields.Components,
					&jira.Component{
						Name: component,
					})
			}
		}
		if len(msg.FixVersions) > 0 {
			issueFields.FixVersions = make([]*jira.FixVersion, 0, len(msg.FixVersions))
			for _, version := range msg.FixVersions {
				issueFields.FixVersions = append(issueFields.FixVersions,
					&jira.FixVersion{
						Name: version,
					})
			}
		}

	case message.Fields:
		issueFields = &jira.IssueFields{
			Summary: fmt.Sprintf("%s", msg[message.FieldsMsgName]),
		}
		for k, v := range msg {
			if k == message.FieldsMsgName {
				continue
			}

			issueFields.Description += fmt.Sprintf("*%s*: %s\n", k, v)
		}

	default:
		issueFields = &jira.IssueFields{
			Summary:     m.String(),
			Description: fmt.Sprintf("%+v", msg),
		}
	}
	return issueFields
}

func populateKey(m message.Composer, issueKey string) {
	switch msg := m.Raw().(type) {
	case *message.JiraIssue:
		msg.IssueKey = issueKey
		if msg.Callback != nil {
			msg.Callback(issueKey)
		}
	case message.Fields:
		msg[jiraIssueKey] = issueKey
	}
}

////////////////////////////////////////////////////////////////////////
//
// interface wrapper for the slack client so that we can mock things out
//
////////////////////////////////////////////////////////////////////////

type jiraClient interface {
	CreateClient(*http.Client, string) error
	Authenticate(string, string, bool) error
	PostIssue(*jira.IssueFields) (string, error)
	PostComment(string, string) error
}

type jiraClientImpl struct {
	*jira.Client
}

func (c *jiraClientImpl) CreateClient(client *http.Client, baseURL string) error {
	var err error
	c.Client, err = jira.NewClient(client, baseURL)
	return err
}

func (c *jiraClientImpl) Authenticate(username string, password string, useBasic bool) error {
	if useBasic {
		c.Client.Authentication.SetBasicAuth(username, password)

	} else {
		authed, err := c.Client.Authentication.AcquireSessionCookie(username, password)
		if err != nil {
			return fmt.Errorf("problem authenticating to jira as '%s' [%s]", username, err.Error())
		}

		if !authed {
			return fmt.Errorf("problem authenticating to jira as '%s'", username)
		}
	}
	return nil
}

func (c *jiraClientImpl) PostIssue(issueFields *jira.IssueFields) (string, error) {
	i := jira.Issue{Fields: issueFields}
	issue, resp, err := c.Client.Issue.Create(&i)
	if err != nil {
		if resp != nil {
			defer resp.Body.Close()
			data, _ := ioutil.ReadAll(resp.Body)
			return "", fmt.Errorf("encountered error logging to jira: %s [%s]",
				err.Error(), string(data))
		}

		return "", err
	}
	if issue == nil {
		return "", errors.New("no issue returned from Jira")
	}

	return issue.Key, nil
}

// todo: allow more parameters than just body?
func (c *jiraClientImpl) PostComment(issueID string, commentToPost string) error {
	_, _, err := c.Client.Issue.AddComment(issueID, &jira.Comment{Body: commentToPost})
	return err
}
