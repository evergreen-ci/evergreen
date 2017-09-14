package command

import (
	"io/ioutil"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCreateEncluosingDirectory(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	// create a temp directory and ensure that its cleaned up.
	dirname, err := ioutil.TempDir("", "command-test")
	require.NoError(err)
	assert.True(dirExists(dirname))
	defer os.RemoveAll(dirname)

	// write data to a temp file and then ensure that the directory existing predicate is valid
	fileName := filepath.Join(dirname, "foo")
	assert.False(dirExists(fileName))
	assert.NoError(ioutil.WriteFile(fileName, []byte("hello world"), 0744))
	assert.False(dirExists(fileName))
	_, err = os.Stat(fileName)
	assert.True(!os.IsNotExist(err))
	assert.NoError(os.Remove(fileName))
	_, err = os.Stat(fileName)
	assert.True(os.IsNotExist(err))

	// ensure that we create an enclosing directory if needed
	assert.False(dirExists(fileName))
	fileName = filepath.Join(fileName, "bar")
	assert.NoError(createEnclosingDirectoryIfNeeded(fileName))
	assert.True(dirExists(filepath.Join(dirname, "foo")))

	// ensure that directory existence check is correct
	assert.True(dirExists(dirname))
	assert.NoError(os.RemoveAll(dirname))
	assert.False(dirExists(dirname))
}
