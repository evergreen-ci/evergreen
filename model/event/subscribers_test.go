package event

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
)

func TestSubscribers(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.ClearCollections(SubscriptionsCollection))
	email := "hi@example.com"
	targetProject := "BF"
	targetTicket := "BF-1234"
	subs := []Subscriber{
		{
			Type: "github_pull_request",
			Target: &GithubPullRequestSubscriber{
				Owner:    "evergreen-ci",
				Repo:     "evergreen",
				PRNumber: 9001,
				Ref:      "sadasdkjsad",
			},
		},
		{
			Type: "evergreen-webhook",
			Target: &WebhookSubscriber{
				URL:    "https://example.com",
				Secret: []byte("hi"),
			},
		},
		{
			Type:   "email",
			Target: &email,
		},
		{
			Type:   "jira-issue",
			Target: &targetProject,
		},
		{
			Type:   "jira-comment",
			Target: &targetTicket,
		},
	}

	for i := range subs {
		assert.NoError(db.Insert(SubscriptionsCollection, subs[i]))
	}

	fetchedSubs := []Subscriber{}
	assert.NoError(db.FindAllQ(SubscriptionsCollection, db.Q{}, &fetchedSubs))

	assert.Len(fetchedSubs, 5)

	for i := range subs {
		assert.Contains(fetchedSubs, subs[i])
	}
}
