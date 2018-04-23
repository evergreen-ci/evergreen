package model

import (
	"testing"

	"github.com/evergreen-ci/evergreen/model/event"
	"github.com/stretchr/testify/assert"
)

func TestSubscriberModels(t *testing.T) {
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
	webhookSubscriber := event.Subscriber{
		Type: event.EvergreenWebhookSubscriberType,
		Target: event.WebhookSubscriber{
			URL:    "foo",
			Secret: []byte("bar"),
		},
	}
	slackSubscriber := event.Subscriber{
		Type:   event.SlackSubscriberType,
		Target: "slack message",
	}

	apiPrSubscriber := APISubscriber{}
	err := apiPrSubscriber.BuildFromService(prSubscriber)
	assert.NoError(err)
	apiWebhookSubscriber := APISubscriber{}
	err = apiWebhookSubscriber.BuildFromService(webhookSubscriber)
	assert.NoError(err)
	apiSlackSubscriber := APISubscriber{}
	err = apiSlackSubscriber.BuildFromService(slackSubscriber)
	assert.NoError(err)

	origPrSubscriber, err := apiPrSubscriber.ToService()
	assert.NoError(err)
	assert.EqualValues(prSubscriber, origPrSubscriber)
	origWebhookSubscriber, err := apiWebhookSubscriber.ToService()
	assert.NoError(err)
	assert.EqualValues(webhookSubscriber, origWebhookSubscriber)
	origSlackSubscriber, err := apiSlackSubscriber.ToService()
	assert.NoError(err)
	assert.EqualValues(slackSubscriber, origSlackSubscriber)
}
