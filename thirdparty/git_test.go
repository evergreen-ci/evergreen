package thirdparty

import (
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

	httpsUrl = "https://api.github.com/repos/evergreen-ci/sample/commits/somecommit"
	owner, repo, err = ParseGitUrl(httpsUrl)
	assert.NoError(err)
	assert.Equal("evergreen-ci", owner)
	assert.Equal("sample", repo)
}

func TestVersionMeetsMinimum(t *testing.T) {
	// VersionMeetsMinimum should return true when version is greater than or equal to the minVersion
	tests := []struct {
		version    string
		minVersion string
		expected   bool
	}{
		{version: "2.41.0", minVersion: "2.42.1", expected: false},
		{version: "2.42.1", minVersion: "2.42.1", expected: true},
		{version: "2.38.0", minVersion: "2.38.1", expected: false},
		{version: "2.38.1", minVersion: "2.38.0", expected: true},
		{version: "2.38", minVersion: "2.37.9", expected: true},
	}

	for _, test := range tests {
		t.Run(test.version+" < "+test.minVersion, func(t *testing.T) {
			result := VersionMeetsMinimum(test.version, test.minVersion)
			assert.Equal(t, test.expected, result)
		})
	}
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
		parsedVersion, isAppleOrWindows, err := ParseGitVersion(versionString)
		require.NoError(t, err)
		assert.Equal(t, expected.expectedVersion, parsedVersion)
		assert.Equal(t, expected.isAppleOrWindows, isAppleOrWindows)
	}
}
