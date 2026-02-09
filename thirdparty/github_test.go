package thirdparty

import (
	"context"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/evergreen-ci/evergreen"
	"github.com/evergreen-ci/evergreen/mock"
	"github.com/evergreen-ci/evergreen/model/githubapp"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
	"github.com/pkg/errors"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/stretchr/testify/suite"
)

func TestGithubSuite(t *testing.T) {
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config)

	suite.Run(t, &githubSuite{config: config})
}

type githubSuite struct {
	suite.Suite
	config *evergreen.Settings

	ctx    context.Context
	cancel func()
}

func (s *githubSuite) SetupTest() {
	var err error
	s.NoError(err)

	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Second)
}

func (s *githubSuite) TearDownTest() {
	s.NoError(s.ctx.Err())
	s.cancel()
}

func (s *githubSuite) TestGithubShouldRetry() {
	req := &http.Request{
		URL: &url.URL{
			Scheme: "https",
			Host:   "www.example.com",
		},
	}

	s.Run("RetryTrue", func() {
		resp := &http.Response{
			StatusCode: 200,
			Header: http.Header{
				"X-Ratelimit-Limit":     []string{"10"},
				"X-Ratelimit-Remaining": []string{"10"},
			},
		}

		retryFn := githubShouldRetry("", retryConfig{retry: true})
		s.False(retryFn(0, req, resp, nil))
		s.True(retryFn(0, req, resp, &net.DNSError{IsTimeout: true}))
		s.False(retryFn(0, req, resp, net.InvalidAddrError("wrong address")))

		resp.StatusCode = http.StatusBadGateway
		s.True(retryFn(0, req, resp, nil))

		resp.StatusCode = http.StatusNotFound
		s.False(retryFn(0, req, resp, nil))

		resp.Header = http.Header{
			"X-Ratelimit-Limit":     []string{"10"},
			"X-Ratelimit-Remaining": []string{"0"},
		}
		s.False(retryFn(0, req, resp, nil))
	})

	s.Run("Retry404", func() {
		resp := &http.Response{
			StatusCode: 200,
			Header: http.Header{
				"X-Ratelimit-Limit":     []string{"10"},
				"X-Ratelimit-Remaining": []string{"10"},
			},
		}

		retryFn := githubShouldRetry("", retryConfig{retry404: true})
		s.False(retryFn(0, req, resp, nil))
		s.True(retryFn(0, req, resp, &net.DNSError{IsTimeout: true}))
		s.False(retryFn(0, req, resp, net.InvalidAddrError("wrong address")))

		resp.StatusCode = http.StatusBadGateway
		s.True(retryFn(0, req, resp, nil))

		resp.StatusCode = http.StatusNotFound
		s.True(retryFn(0, req, resp, nil))

		resp.Header = http.Header{
			"X-Ratelimit-Limit":     []string{"10"},
			"X-Ratelimit-Remaining": []string{"0"},
		}
		s.False(retryFn(0, req, resp, nil))
	})

	s.Run("RetryFalse", func() {
		resp := &http.Response{
			StatusCode: 200,
			Header: http.Header{
				"X-Ratelimit-Limit":     []string{"10"},
				"X-Ratelimit-Remaining": []string{"10"},
			},
		}

		retryFn := githubShouldRetry("", retryConfig{})
		s.False(retryFn(0, req, resp, nil))
		s.False(retryFn(0, req, resp, &net.DNSError{IsTimeout: true}))
		s.False(retryFn(0, req, resp, net.InvalidAddrError("wrong address")))

		resp.StatusCode = http.StatusBadGateway
		s.False(retryFn(0, req, resp, nil))

		resp.StatusCode = http.StatusNotFound
		s.False(retryFn(0, req, resp, nil))

		resp.Header = http.Header{
			"X-Ratelimit-Limit":     []string{"10"},
			"X-Ratelimit-Remaining": []string{"0"},
		}
		s.False(retryFn(0, req, resp, nil))
	})

	s.Run("IgnoreCodes", func() {
		code := http.StatusBadGateway
		resp := &http.Response{
			StatusCode: code,
			Header: http.Header{
				"X-Ratelimit-Limit":     []string{"10"},
				"X-Ratelimit-Remaining": []string{"10"},
			},
		}

		retryFn := githubShouldRetry("", retryConfig{retry: true})
		s.True(retryFn(0, req, resp, nil))

		retryFn = githubShouldRetry("", retryConfig{retry: true, ignoreCodes: []int{code}})
		s.False(retryFn(0, req, resp, nil))
	})
}

func (s *githubSuite) TestCheckGithubAPILimit() {
	rem, err := CheckGithubResource(s.ctx)
	s.NoError(err)
	s.NotNil(rem)
}

func (s *githubSuite) TestGetGithubCommits() {
	githubCommits, _, err := GetGithubCommits(s.ctx, "evergreen-ci", "sample", "", time.Time{}, 0)
	s.NoError(err)
	s.Len(githubCommits, 18)
}

func (s *githubSuite) TestGetGithubCommitsUntil() {
	until := time.Date(2015, time.January, 1, 0, 0, 0, 0, time.UTC)
	githubCommits, _, err := GetGithubCommits(s.ctx, "evergreen-ci", "sample", "", until, 0)
	s.NoError(err)
	s.Len(githubCommits, 4)
}

func (s *githubSuite) TestGetInstallationTokenCached() {
	token, err := getInstallationToken(s.ctx, "evergreen-ci", "sample", nil)
	s.NoError(err)
	s.NotZero(token)

	for i := 0; i < 10; i++ {
		cachedToken, err := getInstallationToken(s.ctx, "evergreen-ci", "sample", nil)
		s.NoError(err)
		s.Equal(token, cachedToken, "should return same exact cached token since it is still valid")
	}
}

// getInstallationTokenNoCache is the same as getInstallationToken but it always
// returns a fresh token rather than a cached token.
func getInstallationTokenNoCache(ctx context.Context, owner, repo string, opts *github.InstallationTokenOptions) (string, error) {
	settings, err := evergreen.GetConfig(ctx)
	if err != nil {
		return "", errors.Wrap(err, "getting config")
	}

	token, _, err := githubapp.CreateGitHubAppAuth(settings).CreateInstallationToken(ctx, owner, repo, opts)
	if err != nil {
		return "", errors.Wrap(err, "creating installation token")
	}

	return token, nil
}

func (s *githubSuite) TestRevokeInstallationToken() {
	// Most GitHub API operations cache tokens for reuse to reduce the rate at
	// which Evergreen requests installation tokens. However, this test is
	// intentionally testing revokation, and revoking the cached token will
	// cause other tests to fail when they attempt to use the cached token.
	token, err := getInstallationTokenNoCache(s.ctx, "evergreen-ci", "sample", nil)
	s.NoError(err)
	s.NotEmpty(token)

	// Revoking the first time should succeed.
	err = RevokeInstallationToken(s.ctx, token)
	s.NoError(err)

	// Revoking the second time should fail.
	err = RevokeInstallationToken(s.ctx, token)
	s.Error(err)
}

func (s *githubSuite) TestGetTaggedCommitFromGithub() {
	s.Run("AnnotatedTag", func() {
		commit, err := GetTaggedCommitFromGithub(s.ctx, "evergreen-ci", "spruce", "refs/tags/v3.0.97")
		s.NoError(err)
		s.Equal("89549e0939ecf5fc59ddd288d860b8a150e6346e", commit)
	})

	s.Run("LightweightTag", func() {
		commit, err := GetTaggedCommitFromGithub(s.ctx, "evergreen-ci", "evergreen", "refs/tags/v3")
		s.NoError(err)
		s.Equal("f0dd0c11df68b975c7dab3d7d3ee675d27119736", commit)
	})
}

func (s *githubSuite) TestGetBranchEvent() {
	branch, err := GetBranchEvent(s.ctx, "evergreen-ci", "evergreen", "main")
	s.NoError(err)
	s.NotPanics(func() {
		s.Equal("main", *branch.Name)
		s.NotNil(*branch.Commit)
	})
}

func (s *githubSuite) TestGithubMergeBaseRevision() {
	rev, err := GetGithubMergeBaseRevision(s.ctx, "evergreen-ci", "evergreen",
		"105bbb4b34e7da59c42cb93d92954710b1f101ee", "49bb297759edd1284ef6adee665180e7b7bac299")
	s.NoError(err)
	s.Equal("2c282735952d7b15e5af7075f483a896783dc2a4", rev)
}

func (s *githubSuite) TestGetGithubFile() {
	_, err := GetGithubFile(s.ctx, "evergreen-ci", "evergreen",
		"doesntexist.txt", "105bbb4b34e7da59c42cb93d92954710b1f101ee", nil)
	s.Error(err)
	s.IsType(FileNotFoundError{}, err)
	s.EqualError(err, "Requested file at doesntexist.txt not found")

	file, err := GetGithubFile(s.ctx, "evergreen-ci", "evergreen",
		"self-tests.yml", "105bbb4b34e7da59c42cb93d92954710b1f101ee", nil)
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
	commit, err := GetCommitEvent(s.ctx, "evergreen-ci", "evergreen", "nope")
	s.Error(err)
	s.Nil(commit)

	commit, err = GetCommitEvent(s.ctx, "evergreen-ci", "evergreen", "ddf48e044c307e3f8734279be95f2d9d7134410f")
	s.NoError(err)

	s.NotPanics(func() {
		s.Equal("richardsamuels", *commit.Author.Login)
		s.Equal("ddf48e044c307e3f8734279be95f2d9d7134410f", *commit.SHA)
		s.Len(commit.Files, 16)
	})

	ghCommitKey := fmt.Sprintf("%s/%s/%s", "evergreen-ci", "evergreen", "ddf48e044c307e3f8734279be95f2d9d7134410f")
	s.Run("StoresItInCache", func() {
		commit, found := ghCommitCache.Get(s.ctx, ghCommitKey, 0)
		s.True(found)
		s.NotNil(commit)
	})
	s.Run("ReleasedFromCache", func() {
		runtime.GC()
		commit, found := ghCommitCache.Get(s.ctx, ghCommitKey, 0)
		s.False(found)
		s.Nil(commit)
	})
}

func (s *githubSuite) TestGetGithubUser() {
	user, err := GetGithubUser(s.ctx, "octocat")
	s.NoError(err)
	s.Require().NotNil(user)
	s.NotPanics(func() {
		s.Equal("octocat", *user.Login)
		s.Equal(583231, int(*user.ID))
	})
}

func (s *githubSuite) TestGetPullRequestMergeBase() {
	evergreen666PR := &github.PullRequest{
		Base: &github.PullRequestBranch{
			Repo: &github.Repository{
				Owner: &github.User{
					Name: utility.ToStringPtr("evergreen-ci"),
				},
				Name: utility.ToStringPtr("evergreen"),
			},
		},
		Number: utility.ToIntPtr(666),
	}
	hash, err := GetPullRequestMergeBase(s.ctx, evergreen666PR)
	s.NoError(err)
	s.Equal("61d770097ca0515e46d29add8f9b69e9d9272b94", hash)

	s.Run("TestCommitWithMergeCommitMain", func() {
		// This test uses the commits found in https://github.com/evergreen-ci/commit-queue-sandbox/pull/802.
		// The merge base of the branch commit in reference to the main branch commit
		// should be 4139a07 despite the branch being cut from 4aa48dc.
		// This is because the branch commit has a merge commit from main as its parent that
		// contains 4139a07.
		mainHash := "4139a07"
		branchHash := "1c413b1"
		expectedMergeBase := "4139a07989ec3a5dfd9c3055161f753c61ef90f8"

		commitQueueSandboxPR := &github.PullRequest{
			Base: &github.PullRequestBranch{
				Repo: &github.Repository{
					Owner: &github.User{
						Name: utility.ToStringPtr("evergreen-ci"),
					},
					Name: utility.ToStringPtr("commit-queue-sandbox"),
				},
				SHA: &mainHash,
			},
			Head: &github.PullRequestBranch{
				SHA: &branchHash,
			},
			Number: utility.ToIntPtr(802),
		}

		hash, err = GetPullRequestMergeBase(s.ctx, commitQueueSandboxPR)
		s.NoError(err)
		s.Equal(expectedMergeBase, hash)
	})

	// This test should fail, but it triggers the retry logic which in turn
	// causes the context to expire, so we reset the context with a longer
	// deadline here
	coniferPR := &github.PullRequest{
		Base: &github.PullRequestBranch{
			Repo: &github.Repository{
				Owner: &github.User{
					Name: utility.ToStringPtr("evergreen-ci"),
				},
				Name: utility.ToStringPtr("conifer"),
			},
		},
		Number: utility.ToIntPtr(666),
	}
	s.cancel()
	s.ctx, s.cancel = context.WithTimeout(context.Background(), 30*time.Second)
	s.Require().NotNil(s.ctx)
	s.Require().NotNil(s.cancel)
	hash, err = GetPullRequestMergeBase(s.ctx, coniferPR)
	s.Error(err)
	s.Empty(hash)
}

func (s *githubSuite) TestGithubUserInOrganization() {
	isMember, err := GithubUserInOrganization(s.ctx, "evergreen-ci", "octocat")
	s.NoError(err)
	s.False(isMember)
}

func (s *githubSuite) TestGitHubUserPermissionLevel() {
	hasPermission, err := GitHubUserHasWritePermission(s.ctx, "evergreen-ci", "evergreen", "octocat")
	s.NoError(err)
	s.False(hasPermission)
}

func (s *githubSuite) TestGetGithubPullRequestDiff() {
	p := GithubPatch{
		PRNumber:   448,
		BaseOwner:  "evergreen-ci",
		BaseRepo:   "evergreen",
		BaseBranch: "main",
	}

	diff, summaries, err := GetGithubPullRequestDiff(s.ctx, p)
	s.NoError(err)
	s.Len(summaries, 2)
	s.Contains(diff, "diff --git a/cli/host.go b/cli/host.go")
}

func (s *githubSuite) TestGetBranchProtectionRules() {
	_, err := GetEvergreenBranchProtectionRules(s.ctx, "evergreen-ci", "evergreen", "main")
	s.NoError(err)
}

func TestVerifyGithubAPILimitHeader(t *testing.T) {
	assert := assert.New(t)
	header := http.Header{}

	// header should have both X-Ratelimit-{Remaining,Limit}
	// return an error if both are not present
	header["X-Ratelimit-Remaining"] = []string{"5000"}
	rem, err := verifyGithubAPILimitHeader(header)
	assert.Error(err)
	assert.Equal(int64(0), rem)

	header = http.Header{}
	header["X-Ratelimit-Limit"] = []string{"5000"}
	rem, err = verifyGithubAPILimitHeader(header)
	assert.Error(err)
	assert.Equal(int64(0), rem)

	// Valid rate limit headers should work fine
	header = http.Header{}
	header["X-Ratelimit-Limit"] = []string{"5000"}
	header["X-Ratelimit-Remaining"] = []string{"4000"}
	rem, err = verifyGithubAPILimitHeader(header)
	assert.NoError(err)
	assert.Equal(int64(4000), rem)
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

func TestValidatePR(t *testing.T) {
	assert := assert.New(t)

	prBody, err := os.ReadFile(filepath.Join(testutil.GetDirectoryOfFile(), "..", "units", "testdata", "pull_request.json"))
	assert.NoError(err)
	assert.Len(prBody, 24706)
	webhookInterface, err := github.ParseWebHook("pull_request", prBody)
	assert.NoError(err)
	prEvent, ok := webhookInterface.(*github.PullRequestEvent)
	assert.True(ok)
	pr := prEvent.GetPullRequest()

	assert.NoError(ValidatePR(pr))

	pr.Base = nil
	assert.Error(ValidatePR(pr))
}

func TestParseGithubErrorResponse(t *testing.T) {
	message := "my message"
	url := "www.github.com"
	resp := &github.Response{
		Response: &http.Response{
			StatusCode: http.StatusNotFound,
			Body:       io.NopCloser(strings.NewReader(fmt.Sprintf(`{"message": "%s", "documentation_url": "%s"}`, message, url))),
		},
	}

	err := parseGithubErrorResponse(resp)
	apiRequestErr, ok := err.(APIRequestError)
	assert.True(t, ok)
	assert.Equal(t, message, apiRequestErr.Message)
	assert.Equal(t, http.StatusNotFound, apiRequestErr.StatusCode)
	assert.Equal(t, url, apiRequestErr.DocumentationUrl)
}

func TestGetRulesWithEvergreenPrefix(t *testing.T) {
	rules := getRulesWithEvergreenPrefix([]string{"evergreen", "evergreen/foo", "bar/baz"})
	assert.Len(t, rules, 2)
	assert.Contains(t, rules, "evergreen")
	assert.Contains(t, rules, "evergreen/foo")
}

func TestGetGitHubSender(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	env := &mock.Environment{}
	require.NoError(t, env.Configure(ctx))
	testutil.ConfigureIntegrationTest(t, env.Settings())

	sender, err := env.GetGitHubSender("evergreen-ci", "evergreen", func(context.Context, string, string) (string, error) {
		return "", nil
	})
	require.NoError(t, err)
	assert.NotZero(t, sender.ErrorHandler, "fallback error handler should be set")
}

func TestValidateCheckRunOutput(t *testing.T) {
	f, err := os.CreateTemp(os.TempDir(), "")
	require.NoError(t, err)
	defer os.Remove(f.Name())

	// an invalid output
	invalidOutputString := `
{
        "title": "This is my report",
        "text": "It looks like there are some errors on lines 2 and 4.",
        "annotations": [
            {
                "path": "README.md",
                "title": "Error Detector",
                "message": "a message",
                "raw_details": "Do you mean this other thing?",
                "start_line": 2,
                "end_line": 4
            }
        ]
}
`
	_, err = f.WriteString(invalidOutputString)
	require.NoError(t, err)
	assert.NoError(t, f.Close())

	checkRunOutput := &github.CheckRunOutput{}

	err = utility.ReadJSONFile(f.Name(), &checkRunOutput)
	require.NoError(t, err)
	err = ValidateCheckRunOutput(checkRunOutput)

	expectedError := "the checkRun output 'This is my report' has no summary\n" +
		"checkRun output 'This is my report' specifies an annotation 'Error Detector' with no annotation level"
	require.NotNil(t, err)
	assert.Equal(t, expectedError, err.Error())
}
