package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/assert"
)

func TestSubscriberModelsGithubStatusAPI(t *testing.T) {
	assert := assert.New(t)

	prSubscriber := event.Subscriber{
		Type: event.GithubPullRequestSubscriberType,
		Target: event.GithubPullRequestSubscriber{
			Owner:    "me",
			Repo:     "mine",
			PRNumber: 5,
			Ref:      "abc",
		},
	}
	apiPrSubscriber := APISubscriber{}
	err := apiPrSubscriber.BuildFromService(prSubscriber)
	assert.NoError(err)

	origPrSubscriber, err := apiPrSubscriber.ToService()
	assert.NoError(err)
	assert.EqualValues(prSubscriber, origPrSubscriber)

	// incoming subscribers have target serialized as a map
	incoming := APISubscriber{
		Type: ToAPIString(event.GithubPullRequestSubscriberType),
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

func TestSubscriberModelsGithubMerge(t *testing.T) {
	assert := assert.New(t)

	subscriber := event.Subscriber{
		Type: event.GithubMergeSubscriberType,
		Target: event.GithubMergeSubscriber{
			Owner:         "me",
			Repo:          "mine",
			PRNumber:      5,
			CommitMessage: "abcd",
			MergeMethod:   "squash",
			CommitTitle:   "merged by evergreen",
			SHA:           "deadbeef",
		},
	}
	apiSubscriber := APISubscriber{}
	err := apiSubscriber.BuildFromService(subscriber)
	assert.NoError(err)

	origSubscriber, err := apiSubscriber.ToService()
	assert.NoError(err)
	assert.EqualValues(subscriber, origSubscriber)

	// incoming subscribers have target serialized as a map
	incoming := APISubscriber{
		Type: ToAPIString(event.GithubMergeSubscriberType),
		Target: map[string]interface{}{
			"owner":          "me",
			"repo":           "mine",
			"pr_number":      5,
			"commit_message": "abcd",
			"merge_method":   "squash",
			"commit_title":   "merged by evergreen",
			"sha":            "deadbeef",
		},
	}

	serviceModel, err := incoming.ToService()
	assert.NoError(err)
	assert.EqualValues(origSubscriber, serviceModel)
}

func TestSubscriberModelsWebhook(t *testing.T) {
	assert := assert.New(t)

	webhookSubscriber := event.Subscriber{
		Type: event.EvergreenWebhookSubscriberType,
		Target: event.WebhookSubscriber{
			URL:    "foo",
			Secret: []byte("bar"),
		},
	}
	apiWebhookSubscriber := APISubscriber{}
	err := apiWebhookSubscriber.BuildFromService(webhookSubscriber)
	assert.NoError(err)

	origWebhookSubscriber, err := apiWebhookSubscriber.ToService()
	assert.NoError(err)
	assert.EqualValues(webhookSubscriber, origWebhookSubscriber)

	// incoming subscribers have target serialized as a map
	incoming := APISubscriber{
		Type: ToAPIString(event.EvergreenWebhookSubscriberType),
		Target: map[string]interface{}{
			"url":    "foo",
			"secret": "bar",
		},
	}

	serviceModel, err := incoming.ToService()
	assert.NoError(err)
	assert.EqualValues(origWebhookSubscriber, serviceModel)
}

func TestSubscriberModelsJIRAIssue(t *testing.T) {
	assert := assert.New(t)

	jiraIssueSubscriber := event.Subscriber{
		Type: event.JIRAIssueSubscriberType,
		Target: event.JIRAIssueSubscriber{
			Project:   "ABC",
			IssueType: "123",
		},
	}
	apiJIRAIssueSubscriber := APISubscriber{}
	err := apiJIRAIssueSubscriber.BuildFromService(jiraIssueSubscriber)
	assert.NoError(err)

	origJIRAIssueSubscriber, err := apiJIRAIssueSubscriber.ToService()
	assert.NoError(err)
	assert.EqualValues(jiraIssueSubscriber, origJIRAIssueSubscriber)

	// incoming subscribers have target serialized as a map
	incoming := APISubscriber{
		Type: ToAPIString(event.JIRAIssueSubscriberType),
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
