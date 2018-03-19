package thirdparty

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/suite"
)

func TestGithubSuite(t *testing.T) {
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config, "TestGithubSuite")

	suite.Run(t, &githubSuite{config: config})
}

type githubSuite struct {
	suite.Suite
	config *evergreen.Settings

	token string
}

func (s *githubSuite) SetupSuite() {
	db.SetGlobalSessionProvider(s.config.SessionFactory())
}

func (s *githubSuite) SetupTest() {
	var err error
	s.token, err = s.config.GetGithubOauthToken()
	s.NoError(err)
}

func (s *githubSuite) TestGetGithubAPIStatus() {
	status, err := GetGithubAPIStatus()
	s.NoError(err)
	s.Contains([]string{GithubAPIStatusGood, GithubAPIStatusMinor, GithubAPIStatusMajor}, status)
}

func (s *githubSuite) TestCheckGithubAPILimit() {
	rem, err := CheckGithubAPILimit(s.token)
	s.NoError(err)
	s.NotNil(rem)
}

func (s *githubSuite) TestGetGithubCommits() {
	githubCommits, _, err := GetGithubCommits(s.token, "deafgoat", "mci-test", "", 0)
	s.NoError(err)
	s.Len(githubCommits, 3)
}

func (s *githubSuite) TestGetBranchEvent() {
	branch, err := GetBranchEvent(s.token, "evergreen-ci", "evergreen", "master")
	s.NoError(err)
	s.NotPanics(func() {
		s.Equal("master", *branch.Name)
		s.NotNil(*branch.Commit)
	})
}

func (s *githubSuite) TestGithubMergeBaseRevision() {
	rev, err := GetGithubMergeBaseRevision(s.token, "evergreen-ci", "evergreen", "105bbb4b34e7da59c42cb93d92954710b1f101ee", "49bb297759edd1284ef6adee665180e7b7bac299")
	s.NoError(err)
	s.Equal("2c282735952d7b15e5af7075f483a896783dc2a4", rev)
}

func (s *githubSuite) TestGetGithubFile() {
	file, err := GetGithubFile(s.token, "evergreen-ci", "evergreen", "self-tests.yml", "105bbb4b34e7da59c42cb93d92954710b1f101ee")
	s.NoError(err)
	s.NotPanics(func() {
		s.Equal("0fa81053f0f5158c36ca92e347c463e8890f66a1", *file.SHA)
		s.Equal(24075, *file.Size)
		s.Equal("base64", *file.Encoding)
		s.Equal("self-tests.yml", *file.Name)
		s.Equal("self-tests.yml", *file.Path)
		s.Equal("file", *file.Type)
		s.Len(*file.Content, 32635)

		s.NotEmpty(file.URL)
		s.NotEmpty(file.GitURL)
		s.NotEmpty(file.HTMLURL)
		s.NotEmpty(file.DownloadURL)
	})
}

func (s *githubSuite) TestGetCommitEvent() {
	commit, err := GetCommitEvent(s.token, "evergreen-ci", "evergreen", "nope")
	s.Error(err)
	s.Nil(commit)

	commit, err = GetCommitEvent(s.token, "evergreen-ci", "evergreen", "ddf48e044c307e3f8734279be95f2d9d7134410f")
	s.NoError(err)

	s.NotPanics(func() {
		s.Equal("richardsamuels", *commit.Author.Login)
		s.Equal("ddf48e044c307e3f8734279be95f2d9d7134410f", *commit.SHA)
		s.Len(commit.Files, 16)
	})
}
func TestVerifyGithubAPILimitHeader(t *testing.T) {
	assert := assert.New(t)
	header := http.Header{}

	// header should have both X-Ratelimit-{Remaining,Limit}
	// return an error if both are not present
	header["X-Ratelimit-Remaining"] = []string{"5000"}
	rem, err := verifyGithubAPILimitHeader(header)
	assert.Error(err)
	assert.Equal(0, rem)

	header["X-Ratelimit-Limit"] = []string{"5000"}
	delete(header, "X-Ratelimit-Remaining")
	rem, err = verifyGithubAPILimitHeader(header)
	assert.Error(err)
	assert.Equal(0, rem)

	// Valid rate limit headers should work fine
	header["X-Ratelimit-Limit"] = []string{"5000"}
	header["X-Ratelimit-Remaining"] = []string{"4000"}
	rem, err = verifyGithubAPILimitHeader(header)
	assert.NoError(err)
	assert.Equal(400, rem)
}

// verifyGithubAPILimitHeader parses a Github API header to find the number of requests remaining
func verifyGithubAPILimitHeader(header http.Header) (int64, error) {
	h := (map[string][]string)(header)
	limStr, okLim := h["X-Ratelimit-Limit"]
	remStr, okRem := h["X-Ratelimit-Remaining"]

	if !okLim || !okRem || len(limStr) == 0 || len(remStr) == 0 {
		return 0, errors.New("Could not get rate limit data")
	}

	rem, err := strconv.ParseInt(remStr[0], 10, 0)
	if err != nil {
		return 0, errors.Errorf("Could not parse rate limit data: limit=%q, rate=%t", limStr, okLim)
	}

	return rem, nil
}
