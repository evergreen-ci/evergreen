package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/assert"
)

func TestSubscriberModelsGithubStatusAPI(t *testing.T) {
	assert := assert.New(t)

	target := event.GithubPullRequestSubscriber{
		Owner:    "me",
		Repo:     "mine",
		PRNumber: 5,
		Ref:      "abc",
	}
	prSubscriber := event.Subscriber{
		Type:   event.GithubPullRequestSubscriberType,
		Target: &target,
	}
	apiPrSubscriber := APISubscriber{}
	err := apiPrSubscriber.BuildFromService(prSubscriber)
	assert.NoError(err)

	origPrSubscriber, err := apiPrSubscriber.ToService()
	assert.NoError(err)

	assert.EqualValues(prSubscriber.Type, origPrSubscriber.Type)
	assert.EqualValues(target, origPrSubscriber.Target)

	// incoming subscribers have target serialized as a map
	incoming := APISubscriber{
		Type: utility.ToStringPtr(event.GithubPullRequestSubscriberType),
		Target: map[string]interface{}{
			"owner":     "me",
			"repo":      "mine",
			"pr_number": 5,
			"ref":       "abc",
		},
	}

	serviceModel, err := incoming.ToService()
	assert.NoError(err)
	assert.EqualValues(origPrSubscriber, serviceModel)
}

func TestSubscriberModelsWebhook(t *testing.T) {
	assert := assert.New(t)

	target := event.WebhookSubscriber{
		URL:        "foo",
		Secret:     []byte("bar"),
		Retries:    3,
		MinDelayMS: 500,
		TimeoutMS:  10000,
		Headers:    []event.WebhookHeader{},
	}
	webhookSubscriber := event.Subscriber{
		Type:   event.EvergreenWebhookSubscriberType,
		Target: &target,
	}
	apiWebhookSubscriber := APISubscriber{}
	err := apiWebhookSubscriber.BuildFromService(webhookSubscriber)
	assert.NoError(err)

	origWebhookSubscriber, err := apiWebhookSubscriber.ToService()
	assert.NoError(err)

	target.Secret = []byte(evergreen.RedactedValue)

	assert.EqualValues(webhookSubscriber.Type, origWebhookSubscriber.Type)
	assert.EqualValues(target, origWebhookSubscriber.Target)

	// incoming subscribers have target serialized as a map
	incoming := APISubscriber{
		Type: utility.ToStringPtr(event.EvergreenWebhookSubscriberType),
		Target: map[string]interface{}{
			"url":          "foo",
			"secret":       evergreen.RedactedValue,
			"retries":      3,
			"min_delay_ms": 500,
			"timeout_ms":   10000,
		},
	}

	serviceModel, err := incoming.ToService()
	assert.NoError(err)
	assert.EqualValues(origWebhookSubscriber, serviceModel)
}

func TestSubscriberModelsJIRAIssue(t *testing.T) {
	assert := assert.New(t)

	target := event.JIRAIssueSubscriber{
		Project:   "ABC",
		IssueType: "123",
	}
	jiraIssueSubscriber := event.Subscriber{
		Type:   event.JIRAIssueSubscriberType,
		Target: &target,
	}
	apiJIRAIssueSubscriber := APISubscriber{}
	err := apiJIRAIssueSubscriber.BuildFromService(jiraIssueSubscriber)
	assert.NoError(err)

	origJIRAIssueSubscriber, err := apiJIRAIssueSubscriber.ToService()
	assert.NoError(err)

	assert.EqualValues(jiraIssueSubscriber.Type, origJIRAIssueSubscriber.Type)
	assert.EqualValues(target, origJIRAIssueSubscriber.Target)

	// incoming subscribers have target serialized as a map
	incoming := APISubscriber{
		Type: utility.ToStringPtr(event.JIRAIssueSubscriberType),
		Target: map[string]interface{}{
			"project":    "ABC",
			"issue_type": "123",
		},
	}

	serviceModel, err := incoming.ToService()
	assert.NoError(err)
	assert.EqualValues(origJIRAIssueSubscriber, serviceModel)
}

func TestSubscriberModelsSlack(t *testing.T) {
	assert := assert.New(t)

	slackSubscriber := event.Subscriber{
		Type:   event.SlackSubscriberType,
		Target: "slack message",
	}

	apiSlackSubscriber := APISubscriber{}
	err := apiSlackSubscriber.BuildFromService(slackSubscriber)
	assert.NoError(err)

	origSlackSubscriber, err := apiSlackSubscriber.ToService()
	assert.NoError(err)
	assert.EqualValues(slackSubscriber, origSlackSubscriber)
}
