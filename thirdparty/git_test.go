package thirdparty

import (
	"fmt"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var testConfig = testutil.TestConfig()

const (
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

	mboxPatch = `From 8af7f21625315b8c24975016aa2107cf5a8a12b1 Mon Sep 17 00:00:00 2001
From: ablack12 <annie.black@10gen.com>
Date: Thu, 2 Jan 2020 10:41:34 -0500
Subject: EVG-6799 remove one commit validation

---
operations/commit_queue.go | 16 +++++++++-------
2 files changed, 9 insertions(+), 7 deletions(-)

diff --git a/operations/commit_queue.go b/units/commit_queue.go
index 3fd24ea7e..800e17d2f 100644
--- a/operations/commit_queue.go
+++ b/operations/commit_queue.go
@@ -122,6 +122,7 @@ func mergeCommand() cli.Command {
                                Usage: "force item to front of queue",
                        },
                )),
+               Before: setPlainLogger,
                Action: func(c *cli.Context) error {
                        ctx, cancel := context.WithCancel(context.Background())
                        defer cancel()

From 8c030c565ebca71380f3ca5c88d895fa9f25bebd Mon Sep 17 00:00:00 2001
From: ablack12 <annie.black@10gen.com>
Date: Thu, 2 Jan 2020 13:35:10 -0500
Subject: Commit message
with an extended description
---
units/commit_queue.go | 5 +++--
1 file changed, 3 insertions(+), 2 deletions(-)

diff --git a/units/commit_queue.go b/units/commit_queue.go
index ce0542e91..718dd8099 100644
--- a/units/commit_queue.go
+++ b/units/commit_queue.go
@@ -512,6 +512,7 @@ func ValidateBranch(branch *github.Branch) error {
 }

 func addMergeTaskAndVariant(patchDoc *patch.Patch, project *model.Project) error {
+       grip.Log("From (hoping this doesn't mess anything up)")'"
        settings, err := evergreen.GetConfig()
        if err != nil {
                return errors.Wrap(err, "error retrieving Evergreen config")
`
)

func TestGetPatchSummaries(t *testing.T) {
	summaries, err := GetPatchSummaries(patchText)
	require.NoError(t, err)
	require.Len(t, summaries, 3)

	assert.Equal(t, "test.txt", summaries[0].Name)
	assert.Equal(t, 4, summaries[0].Additions)
	assert.Equal(t, 1, summaries[0].Deletions)

	assert.Equal(t, "test2.txt", summaries[1].Name)
	assert.Equal(t, 0, summaries[1].Additions)
	assert.Equal(t, 1, summaries[1].Deletions)

	assert.Equal(t, "test3.txt", summaries[2].Name)
	assert.Equal(t, 0, summaries[2].Additions)
	assert.Equal(t, 0, summaries[2].Deletions)
}

func TestParseGitUrl(t *testing.T) {
	assert := assert.New(t)

	httpsUrl := "https://github.com/evergreen-ci/sample.git"
	owner, repo, err := ParseGitUrl(httpsUrl)
	assert.NoError(err)
	assert.Equal("evergreen-ci", owner)
	assert.Equal("sample", repo)

	httpsUrl = "https://github.com/sample.git"
	owner, repo, err = ParseGitUrl(httpsUrl)
	assert.Error(err)
	assert.Equal("", owner)
	assert.Equal("", repo)

	sshUrl := "git@github.com:evergreen-ci/sample.git"
	owner, repo, err = ParseGitUrl(sshUrl)
	assert.NoError(err)
	assert.Equal("evergreen-ci", owner)
	assert.Equal("sample", repo)

	sshUrl = "git@github.com:evergreen-ci/sample"
	owner, repo, err = ParseGitUrl(sshUrl)
	assert.Error(err)
	assert.Equal("evergreen-ci", owner)
	assert.Equal("", repo)
}

func TestGetPatchSummariesByCommit(t *testing.T) {
	summaries, commitMessages, err := GetPatchSummariesFromMboxPatch(mboxPatch)
	assert.NoError(t, err)
	require.Len(t, summaries, 2)
	assert.Equal(t, "EVG-6799 remove one commit validation", summaries[0].Description)
	assert.Equal(t, "Commit message...", summaries[1].Description)

	assert.Equal(t, "operations/commit_queue.go", summaries[0].Name)
	assert.Equal(t, "units/commit_queue.go", summaries[1].Name)

	require.Len(t, commitMessages, 2)
	assert.Equal(t, "EVG-6799 remove one commit validation", commitMessages[0])
	assert.Equal(t, "Commit message...", commitMessages[1])
}

func TestGetPatchSummariesByCommitLong(t *testing.T) {
	str := strings.Repeat("this is a long string", 1000)
	msg := fmt.Sprintf(`From 8af7f21625315b8c24975016aa2107cf5a8a12b1 Mon Sep 17 00:00:00 2001
From: ablack12 <annie.black@10gen.com>
Date: Thu, 2 Jan 2020 10:41:34 -0500
Subject: EVG-6799 remove one commit validation

---
diff --git a/thirdparty/git.go b/thirdparty/git.go
index 03362f816..a9ae2024e 100644
--- a/thirdparty/git.go
+++ b/thirdparty/git.go
@@ -28,6 +28,7 @@ type Summary struct {
 // GitApplyNumstat attempts to apply a given patch; it returns the patch's bytes
 // if it is successful
 func GitApplyNumstat(patch string) (*bytes.Buffer, error) {
+       // %s
        handle, err := ioutil.TempFile("", utility.RandomString())
        if err != nil {
                return nil, errors.New("Unable to create local patch file")
`, str)
	assert.True(t, len([]byte(str)) > 1000)
	summaries, commitMessages, err := GetPatchSummariesFromMboxPatch(msg)
	assert.NoError(t, err)
	assert.NotNil(t, summaries)
	assert.NotNil(t, commitMessages)
}
