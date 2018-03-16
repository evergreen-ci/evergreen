package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

type subscribersSuite struct {
	suite.Suite
	subs []Subscriber
}

func TestSubscribers(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.ClearCollections(SubscriptionsCollection))
	subs := []Subscriber{
		{
			Type: "github_pull_request",
			Target: GithubPullRequestSubscriber{
				Owner:    "evergreen-ci",
				Repo:     "evergreen",
				PRNumber: 9001,
				Ref:      "sadasdkjsad",
			},
		},
		{
			Type: "evergreen-webhook",
			Target: WebhookSubscriber{
				URL:    "https://example.com",
				Secret: []byte("hi"),
			},
		},
		{
			Type:   "email",
			Target: "hi@example.com",
		},
		{
			Type:   "jira-issue",
			Target: "BF",
		},
		{
			Type:   "jira-commenht",
			Target: "BF-1234",
		},
	}

	for i := range subs {
		assert.NoError(db.Insert(SubscriptionsCollection, subs[i]))
	}

	fetchedSubs := []Subscriber{}
	assert.NoError(db.FindAllQ(SubscriptionsCollection, db.Q{}, &fetchedSubs))

	assert.Len(fetchedSubs, 5)
}
