package patch

import (
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	mgobson "github.com/evergreen-ci/evergreen/db/mgo/bson"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/evergreen/thirdparty"
	"github.com/evergreen-ci/utility"
	"github.com/stretchr/testify/suite"
	"go.mongodb.org/mongo-driver/bson"
)

type GithubSuite struct {
	suite.Suite
	pr       int
	hash     string
	baseHash string
	url      string
	baseRepo string
	headRepo string
	user     string
	title    string
}

func TestGithubSuite(t *testing.T) {
	suite.Run(t, new(GithubSuite))
}

func (s *GithubSuite) SetupSuite() {
	s.pr = 5
	s.hash = "67da19930b1b18d346477e99a8e18094a672f48a"
	s.baseHash = "57da19930b1b18d346477e99a8e18094a672f48a"
	s.url = "https://www.example.com/1.diff"
	s.user = "octocat"
	s.baseRepo = "evergreen-ci/evergreen"
	s.headRepo = "octocat/evergreen"
	s.title = "Art of Pull Requests"
}

func (s *GithubSuite) SetupTest() {
	s.Require().NoError(db.Clear(IntentCollection))
	s.Require().NoError(db.Clear(Collection))
}

func (s *GithubSuite) TestNewGithubIntent() {
	intent, err := NewGithubIntent("1", "", "", testutil.NewGithubPR(0, s.baseRepo, s.baseHash, s.headRepo, s.hash, s.user, s.title))
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", "", "", testutil.NewGithubPR(s.pr, "", s.baseHash, s.headRepo, s.hash, s.user, s.title))
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, "", s.hash, s.user, s.title))
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, "", s.headRepo, s.hash, s.user, s.title))
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, "", s.user, s.title))
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, "", s.title))
	s.Nil(intent)
	s.Error(err)

	// Creates new intent with callers
	intent, err = NewGithubIntent("2", "", AutomatedCaller, testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, "", s.title))
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", "", ManualCaller, testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, "", s.title))
	s.Nil(intent)
	s.Error(err)

	// PRs can't have an empty title
	intent, err = NewGithubIntent("2", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, s.user, ""))
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("4", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, s.user, s.title))
	s.NoError(err)
	s.NotNil(intent)
	s.Implements((*Intent)(nil), intent)
	ghIntent, ok := intent.(*githubIntent)
	s.True(ok)
	s.Equal("4", ghIntent.MsgID)
	s.Equal(s.baseRepo, ghIntent.BaseRepoName)
	s.Equal(s.headRepo, ghIntent.HeadRepoName)
	s.Equal(s.pr, ghIntent.PRNumber)
	s.Equal(s.user, ghIntent.User)
	s.Equal(s.hash, ghIntent.HeadHash)
	s.Equal(1234, ghIntent.UID)
	s.Zero(ghIntent.ProcessedAt)
	s.False(intent.IsProcessed())
	s.Equal(GithubIntentType, intent.GetType())
	s.Equal(evergreen.GithubPRRequester, intent.RequesterIdentity())

	patchDoc := intent.NewPatch()
	s.Require().NotNil(patchDoc)
	baseRepo := strings.Split(s.baseRepo, "/")
	headRepo := strings.Split(s.headRepo, "/")
	s.Equal(fmt.Sprintf("'%s' pull request #%d by %s: %s (https://github.com/evergreen-ci/evergreen/pull/5)", s.baseRepo, s.pr, s.user, s.title), patchDoc.Description)
	s.Equal(evergreen.GithubPatchUser, patchDoc.Author)
	s.Equal(evergreen.VersionCreated, patchDoc.Status)

	s.Equal(s.pr, patchDoc.GithubPatchData.PRNumber)
	s.Equal(baseRepo[0], patchDoc.GithubPatchData.BaseOwner)
	s.Equal(baseRepo[1], patchDoc.GithubPatchData.BaseRepo)
	s.Equal(headRepo[0], patchDoc.GithubPatchData.HeadOwner)
	s.Equal(headRepo[1], patchDoc.GithubPatchData.HeadRepo)
	s.Equal(s.hash, patchDoc.GithubPatchData.HeadHash)
	s.Equal(s.user, patchDoc.GithubPatchData.Author)
	patchId := "5aeb4514f27e4f9984646d97"
	patch := Patch{
		Id:      mgobson.ObjectIdHex(patchId),
		Project: "mci",
		GithubPatchData: thirdparty.GithubPatch{
			PRNumber:               5,
			RepeatPatchIdNextPatch: patchId,
		},
	}
	s.NoError(patch.Insert())
	intent, err = NewGithubIntent("4", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, s.user, s.title))
	s.NoError(err)
	s.NotNil(intent)
	s.Implements((*Intent)(nil), intent)
	ghIntent, ok = intent.(*githubIntent)
	s.True(ok)
	s.Equal(patchId, ghIntent.RepeatPatchId)
}

func (s *GithubSuite) TestInsert() {
	intent, err := NewGithubIntent("1", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, s.user, s.title))
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
	s.False(found.IsProcessed())
	s.Equal(GithubIntentType, found.GetType())
}

func (s *GithubSuite) TestFindIntentSpecifically() {
	intent, err := NewGithubIntent("300", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, s.user, s.title))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	found, err := FindIntent(intent.ID(), intent.GetType())
	s.NoError(err)
	s.NotNil(found)

	found.(*githubIntent).ProcessedAt = time.Time{}
	intent.(*githubIntent).ProcessedAt = time.Time{}

	s.Equal(intent, found)
	s.Equal(intent.NewPatch().Description, found.NewPatch().Description)
	s.Equal(intent.NewPatch().CreateTime, found.NewPatch().CreateTime)
	s.Equal(intent.NewPatch().GithubPatchData, found.NewPatch().GithubPatchData)
}

func (s *GithubSuite) TestSetProcessed() {
	intent, err := NewGithubIntent("1", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, s.user, s.title))
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
	s.Equal(s.baseRepo, intents[0].BaseRepoName)
	s.Equal(s.headRepo, intents[0].HeadRepoName)
	s.Equal(s.pr, intents[0].PRNumber)
	s.Equal(s.user, intents[0].User)
	s.Equal(s.hash, intents[0].HeadHash)
	s.True(intents[0].IsProcessed())
	s.Equal(GithubIntentType, intents[0].GetType())
}

func (s *GithubSuite) TestFindUnprocessedGithubIntents() {
	intents := []githubIntent{
		{
			DocumentID: utility.RandomString(),
			IntentType: GithubIntentType,
			Processed:  true,
		},
		{
			DocumentID: utility.RandomString(),
			IntentType: GithubIntentType,
			Processed:  true,
		},
		{
			DocumentID: utility.RandomString(),
			IntentType: GithubIntentType,
			Processed:  true,
		},
		{
			DocumentID: utility.RandomString(),
			IntentType: GithubIntentType,
			Processed:  true,
		},
		{
			DocumentID: utility.RandomString(),
			IntentType: GithubIntentType,
		},
		{
			DocumentID: utility.RandomString(),
			IntentType: GithubIntentType,
		},
		{
			DocumentID: utility.RandomString(),
			IntentType: GithubIntentType,
		},
	}

	for _, intent := range intents {
		s.NoError(intent.Insert())
	}

	found, err := FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(found, 3)
}

func (s *GithubSuite) TestNewPatch() {
	s.NoError(db.Clear(IntentCollection))
	intent, err := NewGithubIntent("4", "", "", testutil.NewGithubPR(s.pr, s.baseRepo, s.baseHash, s.headRepo, s.hash, s.user, s.title))
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	patchDoc := intent.NewPatch()
	s.NotNil(patchDoc)
	s.Equal("'evergreen-ci/evergreen' pull request #5 by octocat: Art of Pull Requests (https://github.com/evergreen-ci/evergreen/pull/5)", patchDoc.Description)
	s.Empty(patchDoc.Project)
	s.Zero(patchDoc.PatchNumber)
	s.Empty(patchDoc.Version)
	s.Equal(evergreen.VersionCreated, patchDoc.Status)
	s.NotZero(patchDoc.CreateTime)
	s.Zero(patchDoc.StartTime)
	s.Zero(patchDoc.FinishTime)
	s.Empty(patchDoc.BuildVariants)
	s.Empty(patchDoc.Tasks)
	s.Empty(patchDoc.VariantsTasks)
	s.Empty(patchDoc.Patches)
	s.False(patchDoc.Activated)
	s.Equal(evergreen.GithubPRAlias, patchDoc.Alias)
	s.Equal(5, patchDoc.GithubPatchData.PRNumber)
	s.Equal("evergreen-ci", patchDoc.GithubPatchData.BaseOwner)
	s.Equal("evergreen", patchDoc.GithubPatchData.BaseRepo)
	s.Equal("octocat", patchDoc.GithubPatchData.HeadOwner)
	s.Equal("evergreen", patchDoc.GithubPatchData.HeadRepo)
	s.Equal("67da19930b1b18d346477e99a8e18094a672f48a", patchDoc.GithubPatchData.HeadHash)
	s.Equal(s.baseHash, patchDoc.Githash)
	s.Equal("octocat", patchDoc.GithubPatchData.Author)
	s.Equal(1234, patchDoc.GithubPatchData.AuthorUID)
}
