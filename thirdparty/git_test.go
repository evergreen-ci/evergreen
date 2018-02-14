package thirdparty

import (
	"testing"

	"github.com/evergreen-ci/evergreen/db"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
)

var testConfig = testutil.TestConfig()

const (
	commitsURL = "https://api.github.com/repos/deafgoat/mci-test/commits"

	patchText = `
diff --git a/test.txt b/test.txt
index 4897035..09740ad 100644
--- a/test.txt
+++ b/test.txt
@@ -1,2 +1,5 @@
-Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua. Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat. Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur. Excepteur sint occaecat cupidatat non proident, sunt in culpa qui officia deserunt mollit anim id est laborum.
+Lorem ipsum dolor sit amet, consectetur adipiscing elit, sed do eiusmod tempor incididunt ut labore et dolore magna aliqua.
 
+Ut enim ad minim veniam, quis nostrud exercitation ullamco laboris nisi ut aliquip ex ea commodo consequat.
+
+Duis aute irure dolor in reprehenderit in voluptate velit esse cillum dolore eu fugiat nulla pariatur.
diff --git a/test2.txt b/test2.txt
deleted file mode 100644
index d9f48e5..0000000
--- a/test2.txt
+++ /dev/null
@@ -1 +0,0 @@
-more text!
diff --git a/test3.txt b/test3.txt
new file mode 100644
index 0000000..e69de29
`
)

func init() {
	db.SetGlobalSessionProvider(testConfig.SessionFactory())
}

func TestGetGithubCommits(t *testing.T) {
	testutil.ConfigureIntegrationTest(t, testConfig, "TestGetGithubCommits")
	assert := assert.New(t)

	githubCommits, _, err := GetGithubCommits("", commitsURL)
	assert.NoError(err)
	assert.Len(githubCommits, 3)
}

func TestGetPatchSummaries(t *testing.T) {
	assert := assert.New(t)

	summaries, err := GetPatchSummaries(patchText)
	assert.NoError(err)
	assert.Len(summaries, 3)

	assert.Equal("test.txt", summaries[0].Name)
	assert.Equal(4, summaries[0].Additions)
	assert.Equal(1, summaries[0].Deletions)

	assert.Equal("test2.txt", summaries[1].Name)
	assert.Equal(0, summaries[1].Additions)
	assert.Equal(1, summaries[1].Deletions)

	assert.Equal("test3.txt", summaries[2].Name)
	assert.Equal(0, summaries[2].Additions)
	assert.Equal(0, summaries[2].Deletions)
}
