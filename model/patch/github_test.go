package patch

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/suite"
	"gopkg.in/mgo.v2/bson"
)

type GithubSuite struct {
	suite.Suite
	pr   int
	hash string
	url  string
	repo string
	user string
}

func TestGithubSuite(t *testing.T) {
	suite.Run(t, new(GithubSuite))
}

func (s *GithubSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.pr = 5
	s.hash = "67da19930b1b18d346477e99a8e18094a672f48a"
	s.url = "http://www.example.com"
	s.user = "octocat"
	s.repo = "evergreen-ci/evergreen"
}

func (s *GithubSuite) SetupTest() {
	s.Require().NoError(db.Clear(IntentCollection))
}

func (s *GithubSuite) TestNewGithubIntent() {
	intent, err := NewGithubIntent("1", s.repo, 0, s.user, s.hash, s.url)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", s.repo, s.pr, s.user, "", s.url)
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("3", s.repo, s.pr, s.user, s.hash, "foo")
	s.Nil(intent)
	s.Error(err)

	intent, err = NewGithubIntent("4", s.repo, s.pr, s.user, s.hash, s.url)
	s.NoError(err)
	s.NotNil(intent)
	s.Implements((*Intent)(nil), intent)
	githubIntent, ok := intent.(*githubIntent)
	s.True(ok)
	s.Equal("4", githubIntent.MsgID)
	s.Equal(s.repo, githubIntent.RepoName)
	s.Equal(s.pr, githubIntent.PRNumber)
	s.Equal(s.user, githubIntent.User)
	s.Equal(s.hash, githubIntent.BaseHash)
	s.Equal(s.url, githubIntent.URL)
	s.Zero(githubIntent.ProcessedAt)
	s.False(intent.IsProcessed())
	s.Equal(GithubIntentType, intent.GetType())
}

func (s *GithubSuite) TestInsert() {
	intent, err := NewGithubIntent("1", s.repo, s.pr, s.user, s.hash, s.url)
	s.NoError(err)
	s.NotNil(intent)
	s.NoError(intent.Insert())

	intents, err := FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(intents, 1)

	found := intents[0]
	s.Equal(s.pr, found.PRNumber)
	s.Equal(s.hash, found.BaseHash)
	s.Equal(s.url, found.URL)
	s.False(found.IsProcessed())
	s.Equal(GithubIntentType, found.GetType())
}

func (s *GithubSuite) TestSetProcessed() {
	intent, err := NewGithubIntent("1", s.repo, s.pr, s.user, s.hash, s.url)
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
	s.Equal(s.hash, intents[0].BaseHash)
	s.Equal(s.url, intents[0].URL)
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
