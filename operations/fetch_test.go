package operations

import (
	"io/ioutil"
	"os"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	git "gopkg.in/src-d/go-git.v4"
)

func TestClone(t *testing.T) {
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, "TestClone")
	tokenParts := strings.Split(settings.Credentials["github"], " ")
	var token string
	if len(tokenParts) > 0 {
		token = tokenParts[1]
	}
	nossh := os.Getenv("NO_SSH") != ""

	passingCases := map[string]cloneOptions{
		"SimpleHTTPS": cloneOptions{
			owner:      "evergreen-ci",
			repository: "sample",
			revision:   "cf46076567e4949f9fc68e0634139d4ac495c89b",
			branch:     "master",
			token:      token,
		},
		"PrivateHTTPS": cloneOptions{
			owner:      "10gen",
			repository: "kernel-tools",
			revision:   "cabca3defc4b251c8a0be268969606717e01f906",
			branch:     "master",
			token:      token,
		},
	}
	failingCases := map[string]cloneOptions{
		"InvalidRepo": cloneOptions{
			owner:      "evergreen-ci",
			repository: "foo",
			revision:   "cf46076567e4949f9fc68e0634139d4ac495c89b",
			branch:     "master",
			token:      token,
		},
		"InvalidRevision": cloneOptions{
			owner:      "evergreen-ci",
			repository: "sample",
			revision:   "9999999999999999999999999999999999999999",
			branch:     "master",
			token:      token,
		},
		"InvalidToken": cloneOptions{
			owner:      "10gen",
			repository: "kernel-tools",
			revision:   "cabca3defc4b251c8a0be268969606717e01f906",
			branch:     "master",
			token:      "foo",
		},
	}

	if !nossh {
		passingCases["SimpleSSH"] = cloneOptions{
			owner:      "evergreen-ci",
			repository: "sample",
			revision:   "cf46076567e4949f9fc68e0634139d4ac495c89b",
			branch:     "master",
		}
	}

	for name, opts := range passingCases {
		t.Run(name, func(t *testing.T) {
			runCloneTest(t, opts, true)
		})
	}
	for name, opts := range failingCases {
		t.Run(name, func(t *testing.T) {
			runCloneTest(t, opts, false)
		})
	}
}

func runCloneTest(t *testing.T, opts cloneOptions, pass bool) {
	tempDirName, err := ioutil.TempDir("", "testclone-")
	defer func() {
		assert.NoError(t, os.RemoveAll(tempDirName))
	}()
	assert.NoError(t, err)
	opts.rootDir = tempDirName
	if !pass {
		assert.Error(t, clone(opts))
		return
	}
	assert.NoError(t, clone(opts))

	repo, err := git.PlainOpen(tempDirName)
	assert.NoError(t, err)
	branch, err := repo.Head()
	assert.NoError(t, err)
	require.NotNil(t, branch)
	assert.Equal(t, opts.revision, branch.Hash().String())
}
