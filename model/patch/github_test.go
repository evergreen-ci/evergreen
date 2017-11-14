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
	pr  int
	sha string
	url string
}

func TestGithubSuite(t *testing.T) {
	suite.Run(t, new(GithubSuite))
}

func (s *GithubSuite) SetupSuite() {
	db.SetGlobalSessionProvider(testutil.TestConfig().SessionFactory())
	s.pr = 5
	s.sha = "67da19930b1b18d346477e99a8e18094a672f48a"
	s.url = "http://www.example.com"
}

func (s *GithubSuite) SetupTest() {
	s.Require().NoError(db.Clear(IntentCollection))
}

func (s *GithubSuite) TestNewGithubIntent() {
	intent, err := NewGithubIntent("1", "evergreen-ci/evergreen", 0, "octocat", s.sha, s.url)
	s.Equal(nil, intent)
	s.Error(err)

	intent, err = NewGithubIntent("2", "evergreen-ci/evergreen", s.pr, "octocat", "", s.url)
	s.Equal(nil, intent)
	s.Error(err)

	intent, err = NewGithubIntent("3", "evergreen-ci/evergreen", s.pr, "octocat", s.sha, "foo")
	s.Equal(nil, intent)
	s.Error(err)

	intent, err = NewGithubIntent("4", "evergreen-ci/evergreen", s.pr, "octocat", s.sha, s.url)
	s.Implements((*Intent)(nil), intent)
	githubIntent, ok := intent.(*GithubIntent)
	s.True(ok)
	s.Equal(s.pr, githubIntent.PRNumber)
	s.Equal(s.sha, githubIntent.BaseHash)
	s.Equal(s.url, githubIntent.URL)
	s.False(intent.IsProcessed())
	s.Equal(GithubIntentType, intent.GetType())
	s.NoError(err)
}

func (s *GithubSuite) TestInsert() {
	intent, err := NewGithubIntent("1", "evergreen-ci/evergreen", s.pr, "octocat", s.sha, s.url)
	s.NoError(err)
	s.NoError(intent.Insert())

	intents, err := FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(intents, 1)

	found := intents[0]
	s.Equal(s.pr, found.PRNumber)
	s.Equal(s.sha, found.BaseHash)
	s.Equal(s.url, found.URL)
	s.False(found.IsProcessed())
	s.Equal(GithubIntentType, found.GetType())
}

func (s *GithubSuite) TestSetProcessed() {
	intent, err := NewGithubIntent("1", "evergreen-ci/evergreen", s.pr, "octocat", s.sha, s.url)
	s.NoError(err)
	s.NoError(intent.Insert())
	s.NoError(intent.SetProcessed())

	found, err := FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(found, 0)

	var intents []GithubIntent
	s.NoError(db.FindAllQ(IntentCollection, db.Query(bson.M{processedKey: true}), &intents))
	s.Len(intents, 1)
	s.Equal(s.pr, intents[0].PRNumber)
	s.Equal(s.sha, intents[0].BaseHash)
	s.Equal(s.url, intents[0].URL)
	s.True(intents[0].IsProcessed())
	s.Equal(GithubIntentType, intents[0].GetType())
}

func (s *GithubSuite) FindUnprocessedGithubIntents() {
	intents := []GithubIntent{
		GithubIntent{
			Processed: true,
		},
		GithubIntent{
			Processed: true,
		},
		GithubIntent{
			Processed: true,
		},
		GithubIntent{
			Processed: true,
		},
		GithubIntent{},
		GithubIntent{},
		GithubIntent{},
	}

	for _, intent := range intents {
		s.NoError(intent.Insert())
	}

	found, err := FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(found, 3)
}
