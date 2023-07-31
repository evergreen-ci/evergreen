package route

import (
	"bytes"
	"context"
	"net/http"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/gimlet"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
)

///////////////////////////////////////////////////////////////////////
//
// Tests for POST /rest/v2/notifications/slack

type SlackNotificationSuite struct {
	rm  gimlet.RouteHandler
	env evergreen.Environment

	suite.Suite
}

func TestSlackNotificationSuite(t *testing.T) {
	s := new(SlackNotificationSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.env = testutil.NewEnvironment(ctx, t)
	suite.Run(t, s)
}

func (s *SlackNotificationSuite) SetupSuite() {
	s.rm = makeSlackNotification(s.env)
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
	req, _ := http.NewRequest(http.MethodPost, "http://example.com/api/rest/v2/notifications/slack", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)

	apiSlack := s.rm.(*slackNotificationPostHandler).APISlack
	s.Equal(utility.ToStringPtr("This is the Slack's target"), apiSlack.Target)
	s.Equal(utility.ToStringPtr("This the Slack's message"), apiSlack.Msg)

	s.Equal(utility.ToStringPtr("I'm the first attachment's color"), apiSlack.Attachments[0].Color)
	s.Equal(utility.ToStringPtr("I'm the first attachment's fallback"), apiSlack.Attachments[0].Fallback)
	s.Equal(utility.ToStringPtr("I'm the first attachment's author's name"), apiSlack.Attachments[0].AuthorName)
	s.Equal(utility.ToStringPtr("I'm the first attachment's author's icon"), apiSlack.Attachments[0].AuthorIcon)
	s.Equal(utility.ToStringPtr("I'm the first attachment's title"), apiSlack.Attachments[0].Title)
	s.Equal(utility.ToStringPtr("I'm the first attachment's text"), apiSlack.Attachments[0].Text)
	s.Equal(utility.ToStringPtr("I'm the first attachment's first field title"), apiSlack.Attachments[0].Fields[0].Title)
	s.Equal(utility.ToStringPtr("I'm the first attachment's first field value"), apiSlack.Attachments[0].Fields[0].Value)
	s.Equal(true, apiSlack.Attachments[0].Fields[0].Short)
	s.Equal([]string{"m11", "m12"}, apiSlack.Attachments[0].MarkdownIn)
	s.Equal(utility.ToStringPtr("I'm the first attachment's footer"), apiSlack.Attachments[0].Footer)

	s.Equal(utility.ToStringPtr("I'm the second attachment's color"), apiSlack.Attachments[1].Color)
	s.Equal(utility.ToStringPtr("I'm the second attachment's fallback"), apiSlack.Attachments[1].Fallback)
	s.Equal(utility.ToStringPtr("I'm the second attachment's author's name"), apiSlack.Attachments[1].AuthorName)
	s.Equal(utility.ToStringPtr("I'm the second attachment's author's icon"), apiSlack.Attachments[1].AuthorIcon)
	s.Equal(utility.ToStringPtr("I'm the second attachment's title"), apiSlack.Attachments[1].Title)
	s.Equal(utility.ToStringPtr("I'm the second attachment's text"), apiSlack.Attachments[1].Text)
	s.Equal(utility.ToStringPtr("I'm the second attachment's first field title"), apiSlack.Attachments[1].Fields[0].Title)
	s.Equal(utility.ToStringPtr("I'm the second attachment's first field value"), apiSlack.Attachments[1].Fields[0].Value)
	s.Equal(false, apiSlack.Attachments[1].Fields[0].Short)
	s.Equal([]string{"m21", "m22"}, apiSlack.Attachments[1].MarkdownIn)
	s.Equal(utility.ToStringPtr("I'm the second attachment's footer"), apiSlack.Attachments[1].Footer)
}

///////////////////////////////////////////////////////////////////////
//
// Tests for POST /rest/v2/notifications/email

type EmailNotificationSuite struct {
	rm  gimlet.RouteHandler
	env evergreen.Environment

	suite.Suite
}

func TestEmailNotificationSuite(t *testing.T) {
	s := new(EmailNotificationSuite)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	s.env = testutil.NewEnvironment(ctx, t)
	suite.Run(t, s)
}

func (s *EmailNotificationSuite) SetupSuite() {
	s.rm = makeEmailNotification(s.env)
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
	req, _ := http.NewRequest(http.MethodPost, "http://example.com/api/rest/v2/notifications/email", bytes.NewBuffer(json))
	err := s.rm.Parse(ctx, req)
	s.NoError(err)

	apiEmail := s.rm.(*emailNotificationPostHandler).APIEmail
	s.Equal(utility.ToStringPtr("me"), apiEmail.From)
	s.Equal([]string{"Tom", "Dick", "Harry"}, apiEmail.Recipients)
	s.Equal(utility.ToStringPtr("This is the email's subject"), apiEmail.Subject)
	s.Equal(utility.ToStringPtr("This is the email's body"), apiEmail.Body)
	s.Equal(true, apiEmail.PlainTextContents)
	s.Equal(map[string][]string{"h1": []string{"v11", "v12"}, "h2": []string{"v21", "v22"}}, apiEmail.Headers)
}
