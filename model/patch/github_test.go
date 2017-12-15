package patch

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type GithubSuite struct {
	suite.Suite
	pr       int
	hash     string
	url      string
	baseRepo string
	headRepo string
	user     string
}

func TestGithubSuite(t *testing.T) {
	suite.Run(t, new(GithubSuite))
}

func (s *GithubSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.pr = 5
	s.hash = "67da19930b1b18d346477e99a8e18094a672f48a"
	s.url = "https://www.example.com/1.diff"
	s.user = "octocat"
	s.baseRepo = "evergreen-ci/evergreen"
	s.headRepo = "octocat/evergreen"
}

func (s *GithubSuite) SetupTest() {
	s.Require().NoError(db.Clear(IntentCollection))
}

func (s *GithubSuite) TestNewGithubIntent() {
	intent, err := NewGithubIntent("1", 0, s.baseRepo, s.headRepo, s.hash, s.user, s.url)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", s.pr, "", s.headRepo, s.hash, s.user, s.url)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", s.pr, s.baseRepo, "", s.hash, s.user, s.url)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", s.pr, s.baseRepo, s.headRepo, "", s.user, s.url)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", s.pr, s.baseRepo, s.headRepo, s.hash, "", s.url)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", s.pr, s.baseRepo, s.headRepo, s.hash, s.user, "")
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("3", s.pr, s.baseRepo, s.headRepo, s.hash, s.user, "foo")
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("3", s.pr, s.baseRepo, s.headRepo, s.hash, s.user, "https://example.com/1.patch")
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("3", s.pr, s.baseRepo, s.headRepo, s.hash, s.user, "http://example.com/1.diff")
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("4", s.pr, s.baseRepo, s.headRepo, s.hash, s.user, s.url)
	s.NoError(err)
	s.NotNil(intent)
	s.Implements((*Intent)(nil), intent)
	githubIntent, ok := intent.(*githubIntent)
	s.True(ok)
	s.Equal("4", githubIntent.MsgID)
	s.Equal(s.baseRepo, githubIntent.BaseRepoName)
	s.Equal(s.headRepo, githubIntent.HeadRepoName)
	s.Equal(s.pr, githubIntent.PRNumber)
	s.Equal(s.user, githubIntent.User)
	s.Equal(s.hash, githubIntent.HeadHash)
	s.Equal(s.url, githubIntent.DiffURL)
	s.Zero(githubIntent.ProcessedAt)
	s.False(intent.IsProcessed())
	s.Equal(GithubIntentType, intent.GetType())
	s.Equal(evergreen.GithubPRRequester, intent.RequesterIdentity())

	patchDoc := intent.NewPatch()
	s.Require().NotNil(patchDoc)
	baseRepo := strings.Split(s.baseRepo, "/")
	headRepo := strings.Split(s.headRepo, "/")
	s.Equal(fmt.Sprintf("%s pull request #%d", s.baseRepo, s.pr), patchDoc.Description)
	s.Equal(evergreen.GithubPatchUser, patchDoc.Author)
	s.Equal(evergreen.PatchCreated, patchDoc.Status)
	s.NotZero(patchDoc.Id)

	s.Equal(s.pr, patchDoc.GithubPatchData.PRNumber)
	s.Equal(baseRepo[0], patchDoc.GithubPatchData.BaseOwner)
	s.Equal(baseRepo[1], patchDoc.GithubPatchData.BaseRepo)
	s.Equal(headRepo[0], patchDoc.GithubPatchData.HeadOwner)
	s.Equal(headRepo[1], patchDoc.GithubPatchData.HeadRepo)
	s.Equal(s.hash, patchDoc.GithubPatchData.HeadHash)
	s.Equal(s.user, patchDoc.GithubPatchData.Author)
	s.Equal(s.url, patchDoc.GithubPatchData.DiffURL)
}

func (s *GithubSuite) TestInsert() {
	intent, err := NewGithubIntent("1", s.pr, s.baseRepo, s.headRepo, s.hash, s.user, s.url)
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	intents, err := FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(intents, 1)

	found := intents[0]
	s.Equal(s.baseRepo, found.BaseRepoName)
	s.Equal(s.headRepo, found.HeadRepoName)
	s.Equal(s.pr, found.PRNumber)
	s.Equal(s.user, found.User)
	s.Equal(s.hash, found.HeadHash)
	s.Equal(s.url, found.DiffURL)
	s.False(found.IsProcessed())
	s.Equal(GithubIntentType, found.GetType())
}

func (s *GithubSuite) TestSetProcessed() {
	intent, err := NewGithubIntent("1", s.pr, s.baseRepo, s.headRepo, s.hash, s.user, s.url)
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())
	s.NoError(intent.SetProcessed())

	found, err := FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(found, 0)

	var intents []githubIntent
	s.NoError(db.FindAllQ(IntentCollection, db.Query(bson.M{processedKey: true}), &intents))
	s.Len(intents, 1)
	s.Equal(s.pr, intents[0].PRNumber)
	s.Equal(s.hash, intents[0].HeadHash)
	s.Equal(s.url, intents[0].DiffURL)
	s.Equal(s.baseRepo, intents[0].BaseRepoName)
	s.Equal(s.headRepo, intents[0].HeadRepoName)
	s.Equal(s.pr, intents[0].PRNumber)
	s.Equal(s.user, intents[0].User)
	s.Equal(s.hash, intents[0].HeadHash)
	s.Equal(s.url, intents[0].DiffURL)
	s.True(intents[0].IsProcessed())
	s.Equal(GithubIntentType, intents[0].GetType())
}

func (s *GithubSuite) FindUnprocessedGithubIntents() {
	intents := []githubIntent{
		githubIntent{
			Processed: true,
		},
		githubIntent{
			Processed: true,
		},
		githubIntent{
			Processed: true,
		},
		githubIntent{
			Processed: true,
		},
		githubIntent{},
		githubIntent{},
		githubIntent{},
	}

	for _, intent := range intents {
		s.NoError(intent.Insert())
	}

	found, err := FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(found, 3)
}
