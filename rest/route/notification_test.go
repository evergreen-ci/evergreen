package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	restModel "github.com/evergreen-ci/evergreen/rest/model"
	"github.com/evergreen-ci/gimlet"
	"github.com/stretchr/testify/suite"
)

///////////////////////////////////////////////////////////////////////
//
// Tests for POST /rest/v2/notifications/jira_comment

type JiraCommentNotificationSuite struct {
	rm          gimlet.RouteHandler
	environment evergreen.Environment

	suite.Suite
}

func TestJiraCommentNotificationSuite(t *testing.T) {
	suite.Run(t, new(JiraCommentNotificationSuite))
}

func (s *JiraCommentNotificationSuite) SetupSuite() {
	s.rm = makeJiraCommentNotification(evergreen.GetEnvironment())
}

func (s *JiraCommentNotificationSuite) TestParseValidJSON() {
	ctx := context.Background()
	json := []byte(`{
		"issue_id": "This is the JIRA issue's ID",
		"body": "This is the JIRA comment's body"
  }`)
	req, _ := http.NewRequest("POST", "http://example.com/api/rest/v2/notifications/jira_comment", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)

	apiJiraComment := s.rm.(*jiraCommentNotificationPostHandler).APIJiraComment
	s.Equal(restModel.ToStringPtr("This is the JIRA issue's ID"), apiJiraComment.IssueID)
	s.Equal(restModel.ToStringPtr("This is the JIRA comment's body"), apiJiraComment.Body)
}

///////////////////////////////////////////////////////////////////////
//
// Tests for POST /rest/v2/notifications/jira_issue

type JiraIssueNotificationSuite struct {
	rm          gimlet.RouteHandler
	environment evergreen.Environment

	suite.Suite
}

func TestJiraIssueNotificationSuite(t *testing.T) {
	suite.Run(t, new(JiraIssueNotificationSuite))
}

func (s *JiraIssueNotificationSuite) SetupSuite() {
	s.rm = makeJiraIssueNotification(evergreen.GetEnvironment())
}

func (s *JiraIssueNotificationSuite) TestParseValidJSON() {
	ctx := context.Background()
	json := []byte(`{
		"issue_key": "This is the JIRA issue's key",
		"project": "This is the JIRA issue's project",
		"summary": "This is the JIRA issue's summary",
		"description": "This is the JIRA issue's description",
		"reporter": "This is the JIRA issue's reporter",
		"assignee": "This is the JIRA issue's assignee",
		"type": "This is the JIRA issue's type",
		"components": ["c1", "c2"],
		"labels": ["l1", "l2"],
		"fields": {"f1": true, "f2": 12.34, "f3": "string"}
  }`)
	req, _ := http.NewRequest("POST", "http://example.com/api/rest/v2/notifications/jira_issue", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)

	apiJiraIssue := s.rm.(*jiraIssueNotificationPostHandler).APIJiraIssue
	s.Equal(restModel.ToStringPtr("This is the JIRA issue's key"), apiJiraIssue.IssueKey)
	s.Equal(restModel.ToStringPtr("This is the JIRA issue's project"), apiJiraIssue.Project)
	s.Equal(restModel.ToStringPtr("This is the JIRA issue's summary"), apiJiraIssue.Summary)
	s.Equal(restModel.ToStringPtr("This is the JIRA issue's description"), apiJiraIssue.Description)
	s.Equal(restModel.ToStringPtr("This is the JIRA issue's reporter"), apiJiraIssue.Reporter)
	s.Equal(restModel.ToStringPtr("This is the JIRA issue's assignee"), apiJiraIssue.Assignee)
	s.Equal(restModel.ToStringPtr("This is the JIRA issue's type"), apiJiraIssue.Type)
	s.Equal([]string{"c1", "c2"}, apiJiraIssue.Components)
	s.Equal([]string{"l1", "l2"}, apiJiraIssue.Labels)
	mock := map[string]interface{}{"f1": true, "f2": 12.34, "f3": "string"}
	s.Equal(len(mock), len(apiJiraIssue.Fields))
	s.Equal(mock["f1"], apiJiraIssue.Fields["f1"])
	s.Equal(mock["f2"], apiJiraIssue.Fields["f2"])
	s.Equal(mock["f3"], apiJiraIssue.Fields["f3"])
}

///////////////////////////////////////////////////////////////////////
//
// Tests for POST /rest/v2/notifications/slack

type SlackNotificationSuite struct {
	rm          gimlet.RouteHandler
	environment evergreen.Environment

	suite.Suite
}

func TestSlackNotificationSuite(t *testing.T) {

	suite.Run(t, new(SlackNotificationSuite))
}

func (s *SlackNotificationSuite) SetupSuite() {
	s.rm = makeSlackNotification(evergreen.GetEnvironment())
}

func (s *SlackNotificationSuite) TestParseValidJSON() {
	ctx := context.Background()
	json := []byte(`{
		"target": "This is the Slack's target",
		"msg": "This the Slack's message",
		"attachments": [
			{
				"color": "I'm the first attachment's color",
				"fallback": "I'm the first attachment's fallback",
				"author_name": "I'm the first attachment's author's name",
				"author_icon": "I'm the first attachment's author's icon",
				"title": "I'm the first attachment's title",
				"text": "I'm the first attachment's text",
				"fields":	[
										{
											"title": "I'm the first attachment's first field title",
											"value": "I'm the first attachment's first field value",
											"short": true
										}
				],
				"mrkdwn_in": ["m11", "m12"],
				"footer": "I'm the first attachment's footer"
			},
			{
				"color": "I'm the second attachment's color",
				"fallback": "I'm the second attachment's fallback",
				"author_name": "I'm the second attachment's author's name",
				"author_icon": "I'm the second attachment's author's icon",
				"title": "I'm the second attachment's title",
				"text": "I'm the second attachment's text",
				"fields":	[
										{
											"title": "I'm the second attachment's first field title",
											"value": "I'm the second attachment's first field value",
											"short": false
										}
				],
				"mrkdwn_in": ["m21", "m22"],
				"footer": "I'm the second attachment's footer"
			}
		]
	}`)
	req, _ := http.NewRequest("POST", "http://example.com/api/rest/v2/notifications/slack", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)

	apiSlack := s.rm.(*slackNotificationPostHandler).APISlack
	s.Equal(restModel.ToStringPtr("This is the Slack's target"), apiSlack.Target)
	s.Equal(restModel.ToStringPtr("This the Slack's message"), apiSlack.Msg)

	s.Equal(restModel.ToStringPtr("I'm the first attachment's color"), apiSlack.Attachments[0].Color)
	s.Equal(restModel.ToStringPtr("I'm the first attachment's fallback"), apiSlack.Attachments[0].Fallback)
	s.Equal(restModel.ToStringPtr("I'm the first attachment's author's name"), apiSlack.Attachments[0].AuthorName)
	s.Equal(restModel.ToStringPtr("I'm the first attachment's author's icon"), apiSlack.Attachments[0].AuthorIcon)
	s.Equal(restModel.ToStringPtr("I'm the first attachment's title"), apiSlack.Attachments[0].Title)
	s.Equal(restModel.ToStringPtr("I'm the first attachment's text"), apiSlack.Attachments[0].Text)
	s.Equal(restModel.ToStringPtr("I'm the first attachment's first field title"), apiSlack.Attachments[0].Fields[0].Title)
	s.Equal(restModel.ToStringPtr("I'm the first attachment's first field value"), apiSlack.Attachments[0].Fields[0].Value)
	s.Equal(true, apiSlack.Attachments[0].Fields[0].Short)
	s.Equal([]string{"m11", "m12"}, apiSlack.Attachments[0].MarkdownIn)
	s.Equal(restModel.ToStringPtr("I'm the first attachment's footer"), apiSlack.Attachments[0].Footer)

	s.Equal(restModel.ToStringPtr("I'm the second attachment's color"), apiSlack.Attachments[1].Color)
	s.Equal(restModel.ToStringPtr("I'm the second attachment's fallback"), apiSlack.Attachments[1].Fallback)
	s.Equal(restModel.ToStringPtr("I'm the second attachment's author's name"), apiSlack.Attachments[1].AuthorName)
	s.Equal(restModel.ToStringPtr("I'm the second attachment's author's icon"), apiSlack.Attachments[1].AuthorIcon)
	s.Equal(restModel.ToStringPtr("I'm the second attachment's title"), apiSlack.Attachments[1].Title)
	s.Equal(restModel.ToStringPtr("I'm the second attachment's text"), apiSlack.Attachments[1].Text)
	s.Equal(restModel.ToStringPtr("I'm the second attachment's first field title"), apiSlack.Attachments[1].Fields[0].Title)
	s.Equal(restModel.ToStringPtr("I'm the second attachment's first field value"), apiSlack.Attachments[1].Fields[0].Value)
	s.Equal(false, apiSlack.Attachments[1].Fields[0].Short)
	s.Equal([]string{"m21", "m22"}, apiSlack.Attachments[1].MarkdownIn)
	s.Equal(restModel.ToStringPtr("I'm the second attachment's footer"), apiSlack.Attachments[1].Footer)
}

///////////////////////////////////////////////////////////////////////
//
// Tests for POST /rest/v2/notifications/email

type EmailNotificationSuite struct {
	rm          gimlet.RouteHandler
	environment evergreen.Environment

	suite.Suite
}

func TestEmailNotificationSuite(t *testing.T) {

	suite.Run(t, new(EmailNotificationSuite))
}

func (s *EmailNotificationSuite) SetupSuite() {
	s.rm = makeEmailNotification(evergreen.GetEnvironment())
}

func (s *EmailNotificationSuite) TestParseValidJSON() {
	ctx := context.Background()
	json := []byte(`{
		"from": "me",
  	"recipients": ["Tom", "Dick", "Harry"],
  	"subject": "This is the email's subject",
  	"body": "This is the email's body",
  	"is_plain_text": true,
		"headers": {"h1": ["v11", "v12"], "h2": ["v21", "v22"]}
	}`)
	req, _ := http.NewRequest("POST", "http://example.com/api/rest/v2/notifications/email", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)

	apiEmail := s.rm.(*emailNotificationPostHandler).APIEmail
	s.Equal(restModel.ToStringPtr("me"), apiEmail.From)
	s.Equal([]string{"Tom", "Dick", "Harry"}, apiEmail.Recipients)
	s.Equal(restModel.ToStringPtr("This is the email's subject"), apiEmail.Subject)
	s.Equal(restModel.ToStringPtr("This is the email's body"), apiEmail.Body)
	s.Equal(true, apiEmail.PlainTextContents)
	s.Equal(map[string][]string{"h1": []string{"v11", "v12"}, "h2": []string{"v21", "v22"}}, apiEmail.Headers)
}
