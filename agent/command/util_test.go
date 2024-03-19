package command

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/evergreen-ci/evergreen/agent/internal"
	"github.com/evergreen-ci/evergreen/testutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateEnclosingDirectory(t *testing.T) {
	assert := assert.New(t)

	// create a temp directory and ensure that its cleaned up.
	dirname := t.TempDir()

	// write data to a temp file and then ensure that the directory existing predicate is valid
	fileName := filepath.Join(dirname, "foo")
	assert.False(dirExists(fileName))
	assert.NoError(os.WriteFile(fileName, []byte("hello world"), 0744))
	assert.False(dirExists(fileName))
	_, err := os.Stat(fileName)
	assert.True(!os.IsNotExist(err))
	assert.NoError(os.Remove(fileName))
	_, err = os.Stat(fileName)
	assert.True(os.IsNotExist(err))

	// ensure that we create an enclosing directory if needed
	assert.False(dirExists(fileName))
	fileName = filepath.Join(fileName, "bar")
	assert.NoError(createEnclosingDirectoryIfNeeded(fileName))
	assert.True(dirExists(filepath.Join(dirname, "foo")))
}

func TestGetJoinedWithWorkDir(t *testing.T) {
	relativeDir := "bar"
	absoluteDir, err := filepath.Abs("/bar")
	require.NoError(t, err)
	conf := &internal.TaskConfig{
		WorkDir: "/foo",
	}
	expected, err := filepath.Abs("/foo/bar")
	require.NoError(t, err)
	expected = filepath.ToSlash(expected)
	actual, err := filepath.Abs(GetWorkingDirectory(conf, relativeDir))
	require.NoError(t, err)
	actual = filepath.ToSlash(actual)
	assert.Equal(t, expected, actual)

	expected, err = filepath.Abs("/bar")
	require.NoError(t, err)
	expected = filepath.ToSlash(expected)
	assert.Equal(t, expected, filepath.ToSlash(GetWorkingDirectory(conf, absoluteDir)))
}

func TestGetWorkingDirectoryLegacy(t *testing.T) {
	curdir := testutil.GetDirectoryOfFile()

	conf := &internal.TaskConfig{
		WorkDir: curdir,
	}

	// make sure that we fall back to the configured working directory
	out, err := getWorkingDirectoryLegacy(conf, "")
	assert.NoError(t, err)
	assert.Equal(t, conf.WorkDir, out)

	// check for a directory that we know exists
	out, err = getWorkingDirectoryLegacy(conf, "testdata")
	require.NoError(t, err)
	assert.Equal(t, out, filepath.Join(curdir, "testdata"))

	// check for a file not a directory
	out, err = getWorkingDirectoryLegacy(conf, "exec.go")
	assert.Error(t, err)
	assert.Equal(t, "", out)

	// presumably for a directory that doesn't exist
	out, err = getWorkingDirectoryLegacy(conf, "does-not-exist")
	assert.Error(t, err)
	assert.Equal(t, "", out)
}
