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
