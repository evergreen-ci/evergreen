package event

import (
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSubscribers(t *testing.T) {
	assert := assert.New(t)

	assert.NoError(db.ClearCollections(SubscriptionsCollection))
	email := "hi@example.com"
	targetProject := "BF"
	targetTicket := "BF-1234"
	subs := []Subscriber{
		{
			Type: GithubPullRequestSubscriberType,
			Target: &GithubPullRequestSubscriber{
				Owner:    "evergreen-ci",
				Repo:     "evergreen",
				PRNumber: 9001,
				Ref:      "sadasdkjsad",
			},
		},
		{
			Type: EvergreenWebhookSubscriberType,
			Target: &WebhookSubscriber{
				URL:    "https://example.com",
				Secret: []byte("hi"),
			},
		},
		{
			Type:   EmailSubscriberType,
			Target: &email,
		},
		{
			Type: JIRAIssueSubscriberType,
			Target: &JIRAIssueSubscriber{
				Project:   targetProject,
				IssueType: "Fail",
			},
		},
		{
			Type:   JIRACommentSubscriberType,
			Target: &targetTicket,
		},
	}
	expected := []string{"github_pull_request-evergreen-ci-evergreen-9001-sadasdkjsad-",
		"evergreen-webhook-https://example.com", "email-hi@example.com",
		"jira-issue-BF-Fail", "jira-comment-BF-1234"}
	for i := range subs {
		assert.NoError(db.Insert(SubscriptionsCollection, subs[i]))
		assert.Equal(expected[i], subs[i].String())
	}

	fetchedSubs := []Subscriber{}
	assert.NoError(db.FindAllQ(SubscriptionsCollection, db.Q{}, &fetchedSubs))

	assert.Len(fetchedSubs, 5)

	for i := range subs {
		assert.Contains(fetchedSubs, subs[i])
	}

	// test we reject unknown subscribers
	assert.NoError(db.ClearCollections(SubscriptionsCollection))
	assert.NoError(db.Insert(SubscriptionsCollection, Subscriber{
		Type:   "something completely different",
		Target: "*boom*",
	}))
	fetchedSubs = []Subscriber{}
	err := db.FindAllQ(SubscriptionsCollection, db.Q{}, &fetchedSubs)

	require.Error(t, err)
	assert.Contains(err.Error(), "unknown subscriber type: 'something completely different'")

	if len(fetchedSubs) == 1 {
		assert.Zero(fetchedSubs[0])
	} else {
		assert.Empty(fetchedSubs)
	}
}

func TestSubscribersStringerWithMissingAttributes(t *testing.T) {
	assert := assert.New(t)

	subs := []Subscriber{
		{
			Type: GithubPullRequestSubscriberType,
		},
		{
			Type: EvergreenWebhookSubscriberType,
		},
		{
			Type: EmailSubscriberType,
		},
		{
			Type: JIRAIssueSubscriberType,
		},
		{
			Type: RunChildPatchSubscriberType,
		},
		{
			Type: JIRACommentSubscriberType,
		},
	}

	for i := range subs {
		assert.True(strings.HasSuffix(subs[i].String(), "NIL_SUBSCRIBER"))
	}

	webhookSub := WebhookSubscriber{}

	assert.True(strings.HasSuffix(webhookSub.String(), "NIL_URL"))
}
