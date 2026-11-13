package thirdparty

import (
	"encoding/base64"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/evergreen-ci/utility"
	"github.com/google/go-github/v70/github"
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
)

func TestGetPatchSummaries(t *testing.T) {
	summaries, err := GetPatchSummaries(t.Context(), patchText)
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

	httpsUrl = "https://api.github.com/repos/evergreen-ci/sample/commits/somecommit"
	owner, repo, err = ParseGitUrl(httpsUrl)
	assert.NoError(err)
	assert.Equal("evergreen-ci", owner)
	assert.Equal("sample", repo)
}

func TestParseGitVersionString(t *testing.T) {
	versionStrings := map[string]struct {
		expectedVersion  string
		isAppleOrWindows bool
	}{
		"git version 2.19.1":                   {"2.19.1", false},
		"git version 2.24.3 (Apple Git-128)":   {"2.24.3", true},
		"git version 2.21.1 (Apple Git-122.3)": {"2.21.1", true},
		"git version 2.16.2.windows.1":         {"2.16.2.windows.1", true},
		"git version 2.47.1.windows.2":         {"2.47.1.windows.2", true},
	}

	for versionString, expected := range versionStrings {
		parsedVersion, err := ParseGitVersion(versionString)
		require.NoError(t, err)
		assert.Equal(t, expected.expectedVersion, parsedVersion)
	}
}

func TestGetGitHubFileFromGit(t *testing.T) {
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config)

	const (
		owner = "evergreen-ci"
		repo  = "sample"
		rev   = "7e05633b9bc529e19eba18b1fc88f78d346855b2"
		file  = "README.md"
	)
	t.Run("RestoresSameFileAsGitHubAPI", func(t *testing.T) {
		gitFileContent, err := GetGitHubFileFromGit(t.Context(), owner, repo, rev, file, "")
		require.NoError(t, err)

		comparisonFile, err := GetGithubFile(t.Context(), owner, repo, file, rev, nil)
		require.NoError(t, err)
		comparisonFileContent, err := base64.StdEncoding.DecodeString(utility.FromStringPtr(comparisonFile.Content))
		require.NoError(t, err)

		assert.Equal(t, comparisonFileContent, gitFileContent, "git file content should exactly match the data retrieved directly from the GitHub API")
	})
	t.Run("ReturnsFileForBranchName", func(t *testing.T) {
		const branch = "main"
		gitFileContent, err := GetGitHubFileFromGit(t.Context(), owner, repo, branch, file, "")
		require.NoError(t, err)

		comparisonFile, err := GetGithubFile(t.Context(), owner, repo, file, branch, nil)
		require.NoError(t, err)
		comparisonFileContent, err := base64.StdEncoding.DecodeString(utility.FromStringPtr(comparisonFile.Content))
		require.NoError(t, err)

		assert.Equal(t, comparisonFileContent, gitFileContent, "git file content should exactly match the data retrieved directly from the GitHub API")
	})
	t.Run("ReturnsFileNotFoundForNonexistentFile", func(t *testing.T) {
		_, err := GetGitHubFileFromGit(t.Context(), owner, repo, rev, "nonexistent-file", "")
		assert.Error(t, err)
		assert.True(t, IsFileNotFound(err))
	})
}

func TestGitLsRemote(t *testing.T) {
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config)

	t.Run("ResolvesBranchToSameCommitAsGitHubAPI", func(t *testing.T) {
		sha, err := GitLsRemote(t.Context(), "evergreen-ci", "sample", "main")
		require.NoError(t, err)

		commits, _, err := GetGithubCommits(t.Context(), "evergreen-ci", "sample", &github.CommitsListOptions{SHA: "main", ListOptions: github.ListOptions{PerPage: 1}})
		require.NoError(t, err)
		require.Len(t, commits, 1)
		assert.Equal(t, commits[0].GetSHA(), sha)
	})
	t.Run("ResolvesAnnotatedTagToCommit", func(t *testing.T) {
		sha, err := GitLsRemote(t.Context(), "evergreen-ci", "evergreen", "v1")
		require.NoError(t, err)
		assert.Equal(t, "da7cb3cbbce495e7bb66602d2c4cd79ae64ce668", sha)
	})
	t.Run("NonexistentRefShouldError", func(t *testing.T) {
		_, err := GitLsRemote(t.Context(), "evergreen-ci", "sample", "nonexistent-branch")
		assert.Error(t, err)
	})
}

func TestParseLsRemote(t *testing.T) {
	const output = "1111111111111111111111111111111111111111\trefs/heads/both\n" +
		"2222222222222222222222222222222222222222\trefs/tags/both\n" +
		"3333333333333333333333333333333333333333\trefs/tags/both^{}\n" +
		"4444444444444444444444444444444444444444\trefs/tags/lightweight\n" +
		"5555555555555555555555555555555555555555\trefs/heads/feature/main\n"

	for name, tc := range map[string]struct {
		ref      string
		expected string
	}{
		"PeeledTagShouldTakePrecedenceOverBranch": {ref: "both", expected: "3333333333333333333333333333333333333333"},
		"LightweightTagShouldResolveToTag":        {ref: "lightweight", expected: "4444444444444444444444444444444444444444"},
		"BranchShouldResolveToBranch":             {ref: "feature/main", expected: "5555555555555555555555555555555555555555"},
		"PartialNameShouldNotMatch":               {ref: "main"},
	} {
		t.Run(name, func(t *testing.T) {
			sha, ok := parseLsRemote(output, tc.ref)
			assert.Equal(t, tc.expected != "", ok)
			assert.Equal(t, tc.expected, sha)
		})
	}
}

func TestIsFullCommitSHA(t *testing.T) {
	assert.True(t, IsFullCommitSHA("7e05633b9bc529e19eba18b1fc88f78d346855b2"))
	assert.False(t, IsFullCommitSHA("7e05633"))
	assert.False(t, IsFullCommitSHA("main"))
	assert.False(t, IsFullCommitSHA(""))
}

func TestGitCreateWorktree(t *testing.T) {
	config := testutil.TestConfig()
	testutil.ConfigureIntegrationTest(t, config)

	const (
		owner = "evergreen-ci"
		repo  = "sample"
		rev   = "7e05633b9bc529e19eba18b1fc88f78d346855b2"
	)

	t.Run("Succeeds", func(t *testing.T) {
		dir, err := GitCloneMinimal(t.Context(), owner, repo, rev)
		require.NoError(t, err)
		defer func() {
			assert.NoError(t, os.RemoveAll(dir))
		}()

		worktreeDir := filepath.Join(dir, "worktree")
		require.NoError(t, GitCreateWorktree(t.Context(), dir, worktreeDir))
		require.NoError(t, err)
		assert.True(t, utility.FileExists(worktreeDir))

		dotGitDir := filepath.Join(worktreeDir, ".git")
		assert.True(t, utility.FileExists(dotGitDir), ".git directory should exist in worktree")

		var stdout, stderr strings.Builder
		cmd := exec.CommandContext(t.Context(), "git", "rev-parse", "HEAD")
		cmd.Dir = worktreeDir
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		require.NoError(t, cmd.Run(), "git rev-parse failed: '%s'", stderr.String())
		assert.Equal(t, rev, strings.TrimSpace(stdout.String()), "worktree should be at the same revision as the git clone directory")
	})
	t.Run("FailsWithoutGitDirectory", func(t *testing.T) {
		tmpDir := t.TempDir()
		assert.Error(t, GitCreateWorktree(t.Context(), tmpDir, "worktree"))
	})
}

func TestValidateFileIsWithinDirectory(t *testing.T) {
	const parentDir = "/tmp/dir"

	t.Run("RelativePathAllowed", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, "src/main.go")
		assert.NoError(t, err)
	})

	t.Run("RelativePathAllowedWithRedundantSeparators", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, "src//main.go")
		assert.NoError(t, err)
	})

	t.Run("RelativePathWithDoubleDotInFileNameAllowed", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, "src/config..backup.yml")
		assert.NoError(t, err)
	})

	t.Run("FilePathWithTraversalDisallowed", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, "../etc/passwd")
		assert.Error(t, err)
	})

	t.Run("AbsolutePathDisallowed", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, "/absolute/path/to/file")
		assert.Error(t, err)
	})

	t.Run("FilePathWithMixedTraversalDisallowed", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, "src/../etc/../../passwd")
		assert.Error(t, err)
	})

	t.Run("LeadingDashDisallowed", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, "--pathspec-from-file=/etc/hostname")
		assert.Error(t, err)
	})

	t.Run("ShortOptionDisallowed", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, "-p")
		assert.Error(t, err)
	})

	t.Run("LeadingColonDisallowed", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, ":/")
		assert.Error(t, err)
	})

	t.Run("MagicPathspecDisallowed", func(t *testing.T) {
		err := validateFileIsWithinDirectory(parentDir, ":(exclude)foo")
		assert.Error(t, err)
	})
}

func TestValidateFileIsNotSymlink(t *testing.T) {
	t.Run("DoesNotErrorForRegularFile", func(t *testing.T) {
		const fileName = "file.txt"
		tmpDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, fileName), []byte("hello world"), 0644))
		assert.True(t, utility.FileExists(filepath.Join(tmpDir, fileName)))
		assert.NoError(t, validateFileIsNotSymlink(tmpDir, fileName))
	})
	t.Run("ErrorsForSymlink", func(t *testing.T) {
		const fileName = "symlink"
		tmpDir := t.TempDir()
		require.NoError(t, os.Symlink("/etc/passwd", filepath.Join(tmpDir, fileName)))
		assert.True(t, utility.FileExists(filepath.Join(tmpDir, fileName)))
		assert.Error(t, validateFileIsNotSymlink(tmpDir, fileName))
	})
}
