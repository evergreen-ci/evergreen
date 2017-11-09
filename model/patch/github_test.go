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
	intent, err := NewGithubIntent(0, s.sha, s.url)
	s.Equal(&GithubIntent{}, intent)
	s.Error(err)

	intent, err = NewGithubIntent(s.pr, "12345", s.url)
	s.Equal(&GithubIntent{}, intent)
	s.Error(err)

	intent, err = NewGithubIntent(s.pr, s.sha, "foo")
	s.Equal(&GithubIntent{}, intent)
	s.Error(err)

	intent, err = NewGithubIntent(s.pr, s.sha, s.url)
	s.Implements((*Intent)(nil), intent)
	s.Equal(s.pr, intent.PRNumber)
	s.Equal(s.sha, intent.HeadSHA)
	s.Equal(s.url, intent.URL)
	s.False(intent.IsProcessed())
	s.Equal(GithubIntentType, intent.GetType())
	s.NoError(err)
}

func (s *GithubSuite) TestInsert() {
	intent, err := NewGithubIntent(s.pr, s.sha, s.url)
	s.NoError(err)
	s.NoError(intent.Insert())

	intents, err := FindUnprocessedGithubIntents()
	s.NoError(err)
	s.Len(intents, 1)

	found := intents[0]
	s.Equal(s.pr, found.PRNumber)
	s.Equal(s.sha, found.HeadSHA)
	s.Equal(s.url, found.URL)
	s.False(found.IsProcessed())
	s.Equal(GithubIntentType, found.GetType())
}

func (s *GithubSuite) TestSetProcessed() {
	intent, err := NewGithubIntent(s.pr, s.sha, s.url)
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
	s.Equal(s.sha, intents[0].HeadSHA)
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
