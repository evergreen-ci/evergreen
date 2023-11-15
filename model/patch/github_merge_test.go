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
	for tName, tCase := range map[string]func(t *testing.T, mge *github.MergeGroupEvent){
		"EmptyMessageDeliveryIDErrors": func(t *testing.T, mge *github.MergeGroupEvent) {
			intent, err := NewGithubMergeIntent("", "auto", mge)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"EmptyCallerErrors": func(t *testing.T, mge *github.MergeGroupEvent) {
			intent, err := NewGithubMergeIntent("abc123", "", mge)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"MissingHeadRefErrors": func(t *testing.T, mge *github.MergeGroupEvent) {
			mge.MergeGroup.HeadRef = nil
			intent, err := NewGithubMergeIntent("abc123", "auto", mge)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"MissingHeadSHAErrors": func(t *testing.T, mge *github.MergeGroupEvent) {
			mge.MergeGroup.HeadSHA = nil
			intent, err := NewGithubMergeIntent("abc123", "auto", mge)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"MissingOrgErrors": func(t *testing.T, mge *github.MergeGroupEvent) {
			mge.Org.Login = nil
			intent, err := NewGithubMergeIntent("abc123", "auto", mge)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"MissingRepoErrors": func(t *testing.T, mge *github.MergeGroupEvent) {
			mge.Repo.Name = nil
			intent, err := NewGithubMergeIntent("abc123", "auto", mge)
			assert.Nil(t, intent)
			assert.Error(t, err)
		},
		"CorrectArgsSucceed": func(t *testing.T, mge *github.MergeGroupEvent) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mge)
			assert.NotNil(t, intent)
			assert.NoError(t, err)
		},
		"RoundTrip": func(t *testing.T, mge *github.MergeGroupEvent) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mge)
			assert.NotNil(t, intent)
			assert.NoError(t, err)
			assert.NoError(t, intent.Insert())
			intents := []githubMergeIntent{}
			err = db.FindAllQ(IntentCollection, db.Query(bson.M{}), &intents)
			assert.NoError(t, err)
			assert.Len(t, intents, 1)
			assert.Equal(t, intent, &intents[0])
		},
		"SetProcessed": func(t *testing.T, mge *github.MergeGroupEvent) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mge)
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
		"Accessors": func(t *testing.T, mge *github.MergeGroupEvent) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mge)
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
		"NewPatch": func(t *testing.T, mge *github.MergeGroupEvent) {
			intent, err := NewGithubMergeIntent("abc123", "auto", mge)
			assert.NotNil(t, intent)
			assert.NoError(t, err)
			assert.NoError(t, intent.Insert())
			p := intent.NewPatch()
			assert.Equal(t, evergreen.CommitQueueAlias, p.Alias)
			assert.Equal(t, *mge.MergeGroup.BaseSHA, p.Githash)
			assert.Equal(t, *mge.MergeGroup.HeadSHA, p.GithubMergeData.HeadSHA)
			assert.Equal(t, mge.MergeGroup.GetHeadCommit().GetMessage(), p.GithubMergeData.HeadCommit)
			assert.Equal(t, *mge.Org.Login, p.GithubMergeData.Org)
			assert.Equal(t, *mge.Repo.Name, p.GithubMergeData.Repo)
			assert.Equal(t, "main", p.GithubMergeData.BaseBranch)
			assert.Equal(t, "gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056", p.GithubMergeData.HeadBranch)
		},
	} {
		t.Run(tName, func(t *testing.T) {
			require.NoError(t, db.Clear(IntentCollection))
			HeadSHA := "a"
			HeadCommit := "commit message"
			BaseSHA := "b"
			HeadRef := "refs/heads/gh-readonly-queue/main/pr-515-9cd8a2532bcddf58369aa82eb66ba88e2323c056"
			OrgName := "my_org"
			RepoName := "my_repo"
			org := github.Organization{
				Login: &OrgName,
			}
			repo := github.Repository{
				Name: &RepoName,
			}
			mg := github.MergeGroup{
				HeadSHA: &HeadSHA,
				HeadCommit: &github.Commit{
					Message: &HeadCommit,
				},
				HeadRef: &HeadRef,
				BaseSHA: &BaseSHA,
			}
			mge := github.MergeGroupEvent{
				MergeGroup: &mg,
				Org:        &org,
				Repo:       &repo,
			}
			tCase(t, &mge)
		})
	}
}
