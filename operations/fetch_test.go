package operations

import (
	"io/ioutil"
	"os"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

func TestClone(t *testing.T) {
	settings := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, settings, "TestClone")
	token, err := settings.GetGithubOauthToken()
	assert.NoError(t, err)

	type testCase struct {
		opts      cloneOptions
		isPassing bool
	}

	testCases := map[string]testCase{
		"SimpleHTTPS": {isPassing: true, opts: cloneOptions{
			owner:      "evergreen-ci",
			repository: "sample",
			revision:   "cf46076567e4949f9fc68e0634139d4ac495c89b",
			branch:     "master",
			token:      token,
		}},
		"InvalidRepo": {isPassing: false, opts: cloneOptions{
			owner:      "evergreen-ci",
			repository: "foo",
			revision:   "cf46076567e4949f9fc68e0634139d4ac495c89b",
			branch:     "master",
			token:      token,
		}},
		"InvalidRevision": {isPassing: false, opts: cloneOptions{
			owner:      "evergreen-ci",
			repository: "sample",
			revision:   "9999999999999999999999999999999999999999",
			branch:     "master",
			token:      token,
		}},
		"InvalidToken": {isPassing: false, opts: cloneOptions{
			owner:      "10gen",
			repository: "kernel-tools",
			revision:   "cabca3defc4b251c8a0be268969606717e01f906",
			branch:     "master",
			token:      "foo",
		}},
	}

	for name, test := range testCases {
		t.Run(name, func(t *testing.T) {
			runCloneTest(t, test.opts, test.isPassing)
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
}
