package patch

import (
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/google/go-github/v52/github"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.mongodb.org/mongo-driver/bson"
)

func TestGithubMergeIntent(t *testing.T) {
	defer func() {
		require.NoError(t, db.Clear(IntentCollection))
	}()
	for tName, tCase := range map[string]func(t *testing.T, mg *github.MergeGroup){
		"EmptyMessageDeliveryIDErrors": func(t *testing.T, mg *github.MergeGroup) {
			intent, err := NewGithubMergeIntent("", "auto", mg)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"EmptyCallerErrors": func(t *testing.T, mg *github.MergeGroup) {
			intent, err := NewGithubMergeIntent("abc123", "", mg)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"MissingHeadRefErrors": func(t *testing.T, mg *github.MergeGroup) {
			mg.HeadRef = nil
			intent, err := NewGithubMergeIntent("abc123", "auto", mg)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"MissingHeadSHAErrors": func(t *testing.T, mg *github.MergeGroup) {
			mg.HeadSHA = nil
			intent, err := NewGithubMergeIntent("abc123", "auto", mg)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"CorrectArgsSucceed": func(t *testing.T, mg *github.MergeGroup) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mg)
			assert.NotNil(t, intent)
			assert.NoError(t, err)
		},
		"RoundTrip": func(t *testing.T, mg *github.MergeGroup) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mg)
			assert.NotNil(t, intent)
			assert.NoError(t, err)
			assert.NoError(t, intent.Insert())
			intents := []githubMergeIntent{}
			err = db.FindAllQ(IntentCollection, db.Query(bson.M{}), &intents)
			assert.NoError(t, err)
			assert.Len(t, intents, 1)
			assert.Equal(t, intent, &intents[0])
		},
		"SetProcessed": func(t *testing.T, mg *github.MergeGroup) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mg)
			assert.False(t, intent.IsProcessed())
			assert.NotNil(t, intent)
			assert.NoError(t, err)
			assert.NoError(t, intent.Insert())
			assert.NoError(t, intent.SetProcessed())
			intents := []githubMergeIntent{}
			err = db.FindAllQ(IntentCollection, db.Query(bson.M{}), &intents)
			assert.NoError(t, err)
			assert.Len(t, intents, 1)
			assert.True(t, intents[0].IsProcessed())
		},
		"Accessors": func(t *testing.T, mg *github.MergeGroup) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mg)
			assert.NotNil(t, intent)
			assert.NoError(t, err)
			assert.Equal(t, GithubMergeIntentType, intent.GetType())
			assert.Equal(t, "abc123", intent.ID())
			assert.True(t, intent.ShouldFinalizePatch())
			s, b := intent.RepeatPreviousPatchDefinition()
			assert.Empty(t, s)
			assert.False(t, b)
			s, b = intent.RepeatFailedTasksAndVariants()
			assert.Empty(t, s)
			assert.False(t, b)
			assert.Equal(t, evergreen.GithubMergeRequester, intent.RequesterIdentity())
			assert.Equal(t, "auto", intent.GetCalledBy())
			assert.Equal(t, evergreen.CommitQueueAlias, intent.GetAlias())
		},
		"NewPatch": func(t *testing.T, mg *github.MergeGroup) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mg)
			assert.NotNil(t, intent)
			assert.NoError(t, err)
			assert.NoError(t, intent.Insert())
			p := intent.NewPatch()
			assert.Equal(t, evergreen.CommitQueueAlias, p.Alias)
			assert.Equal(t, *mg.HeadSHA, p.GithubMergeData.HeadSHA)
			assert.Equal(t, *mg.HeadRef, p.GithubMergeData.HeadRef)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(IntentCollection))
			HeadSHA := "a"
			HeadRef := "b"
			mg := github.MergeGroup{
				HeadSHA: &HeadSHA,
				HeadRef: &HeadRef,
			}
			tCase(t, &mg)
		})
	}
}
