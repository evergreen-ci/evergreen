package jasper

import (
	"bytes"
	"os"
	"runtime"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStringMembership(t *testing.T) {
	cases := []struct {
		id      string
		group   []string
		name    string
		outcome bool
	}{
		{
			id:      "EmptySet",
			group:   []string{},
			name:    "anything",
			outcome: false,
		},
		{
			id:      "ZeroArguments",
			outcome: false,
		},
		{
			id:      "OneExists",
			group:   []string{"a"},
			name:    "a",
			outcome: true,
		},
		{
			id:      "OneOfMany",
			group:   []string{"a", "a", "a"},
			name:    "a",
			outcome: true,
		},
		{
			id:      "OneOfManyDifferentSet",
			group:   []string{"a", "b", "c"},
			name:    "c",
			outcome: true,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.id, func(t *testing.T) {
			assert.Equal(t, testCase.outcome, sliceContains(testCase.group, testCase.name))
		})
	}
}

func TestMakeEnclosingDirectories(t *testing.T) {
	path := "foo"
	_, err := os.Stat(path)
	require.True(t, os.IsNotExist(err))
	assert.NoError(t, makeEnclosingDirectories(path))
	defer os.RemoveAll(path)

	path = "util_test.go"
	info, err := os.Stat(path)
	require.False(t, os.IsNotExist(err))
	require.False(t, info.IsDir())
	assert.Error(t, makeEnclosingDirectories(path))
}

func TestWriteFile(t *testing.T) {
	for testName, testCase := range map[string]struct {
		content    string
		path       string
		shouldPass bool
	}{
		"FailsForInsufficientMkdirPermissions": {
			content:    "foo",
			path:       "/bar",
			shouldPass: false,
		},
		"FailsForInsufficientFileWritePermissions": {
			content:    "foo",
			path:       "/etc/hosts",
			shouldPass: false,
		},
		"FailsForInsufficientFileOpenPermissions": {
			content:    "foo",
			path:       "/etc/whatever",
			shouldPass: false,
		},
		"WriteToFileSucceeds": {
			content:    "foo",
			path:       "/dev/null",
			shouldPass: true,
		},
	} {
		t.Run(testName, func(t *testing.T) {
			if os.Geteuid() == 0 {
				t.Skip("cannot test download permissions as root")
			} else if runtime.GOOS == "windows" {
				t.Skip("cannot run file write tests on windows")
			}
			err := writeFile(bytes.NewBufferString(testCase.content), testCase.path)
			if testCase.shouldPass {
				assert.NoError(t, err)
			} else {
				assert.Error(t, err)
			}
		})
	}
}
